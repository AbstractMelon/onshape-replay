package render

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/abstractmelon/onshape-replay/internal/ffmpeg"
	"github.com/abstractmelon/onshape-replay/internal/onshape"
	"github.com/abstractmelon/onshape-replay/internal/storage"
)

// Dependencies groups the external services the pipeline needs.
type Dependencies struct {
	Onshape     *onshape.Client
	Encoder     *ffmpeg.Encoder
	Queue       *Queue
	Log         *slog.Logger
	StorageRoot string
}

// Launches the render pipeline in a background goroutine.
// It returns immediately; progress is observable via Queue.Subscribe.
func StartPipeline(job *Job, deps Dependencies) {
	ctx, cancel := context.WithCancel(context.Background())
	deps.Queue.mu.Lock()
	job.cancel = cancel
	job.progress = newProgressBroadcaster()
	deps.Queue.mu.Unlock()

	go func() {
		defer cancel()
		err := runPipeline(ctx, job, deps)
		if err != nil && ctx.Err() == nil {
			// Non-cancellation error.
			deps.Log.Error("pipeline failed", "jobId", job.ID, "err", err)
			deps.Queue.SetStatus(job.ID, StatusFailed, err.Error())
			// Write a failure manifest so the job is recorded on disk.
			paths := storage.Layout(deps.StorageRoot, job.DocumentID, job.ElementID, job.ID)
			failManifest := &storage.Manifest{
				JobID:        job.ID,
				DocumentID:   job.DocumentID,
				WorkspaceID:  job.WorkspaceID,
				ElementID:    job.ElementID,
				ExportConfig: job.Config,
				Features:     toStorageFeatures(job.Features),
				Status:       storage.StatusFailed,
				ErrorMsg:     err.Error(),
				CreatedAt:    job.CreatedAt,
				StartedAt:    job.StartedAt,
				CompletedAt:  job.CompletedAt,
			}
			if mErr := storage.WriteManifest(paths.Manifest, failManifest); mErr != nil {
				deps.Log.Error("failed to write failure manifest", "err", mErr)
			}
		}
	}()
}

// The synchronous render logic running inside a goroutine.
func runPipeline(ctx context.Context, job *Job, deps Dependencies) error {
	log := deps.Log.With("jobId", job.ID)
	paths := storage.Layout(deps.StorageRoot, job.DocumentID, job.ElementID, job.ID)

	// Ensure directories exist before any writes.
	if err := storage.EnsureJobDirs(paths); err != nil {
		return err
	}

	deps.Queue.SetStatus(job.ID, StatusRunning, "")
	startedAt := time.Now()

	// Get the feature list and current rollback position from the user's workspace.
	log.Info("fetching feature list")
	features, origRollback, maxRollbackIdx, err := deps.Onshape.GetFeatureList(ctx, job.DocumentID, "w", job.WorkspaceID, job.ElementID)
	if err != nil {
		return fmt.Errorf("get feature list: %w", err)
	}
	log.Info("feature list retrieved", "count", len(features), "rollbackIndex", origRollback, "maxRollbackIndex", maxRollbackIdx)

	deps.Queue.mu.Lock()
	job.Features = features
	deps.Queue.mu.Unlock()

	// Restore the original rollback position after the job completes or fails,
	// So the user's workspace is not left in a rolled-back state.
	defer func() {
		cleanCtx := context.Background()
		log.Info("restoring rollback position", "index", origRollback)
		if rErr := deps.Onshape.SetRollback(cleanCtx, job.DocumentID, "w", job.WorkspaceID, job.ElementID, origRollback); rErr != nil {
			log.Warn("failed to restore rollback; the workspace may need manual reset",
				"err", rErr)
		}
	}()

	// Check for cancellation before starting expensive work.
	if err := ctx.Err(); err != nil {
		deps.Queue.SetStatus(job.ID, StatusCancelled, "cancelled before feature capture")
		return nil
	}

	cfg := job.Config
	res := ffmpeg.DefaultResolution
	if r, ok := ffmpeg.Resolutions[cfg.Resolution]; ok {
		res = r
	}

	// Build the list of features to capture after applying filters.
	type captureStep struct {
		featureIndex int // 1-based rollback position
		feature      onshape.Feature
	}

	var steps []captureStep
	for i, f := range features {
		if shouldSkip(f, cfg) {
			continue
		}
		fi := i + 1
		if fi > maxRollbackIdx {
			fi = maxRollbackIdx
		}
		steps = append(steps, captureStep{featureIndex: fi, feature: f})
	}

	// Set total feature count on the job so API consumers see a non-zero
	// Value immediately, not just after the first frame is captured.
	total := len(steps)
	if total == 0 {
		return fmt.Errorf("no features to render after applying filters")
	}
	deps.Queue.mu.Lock()
	job.TotalFeatures = total
	deps.Queue.mu.Unlock()

	frameIndex := 1 // 1-based frame counter for file naming.

	writeFrame := func(pngBytes []byte) error {
		framePath := storage.FramePath(paths, frameIndex)
		if err := os.WriteFile(framePath, pngBytes, 0o644); err != nil {
			return fmt.Errorf("write frame %d: %w", frameIndex, err)
		}
		frameIndex++
		return nil
	}

	viewCfg := onshape.ShadedViewConfig{
		OutputWidth:     res.Width,
		OutputHeight:    res.Height,
		ShowAllParts:    true,
		UseAntiAliasing: true,
		Transparent:     cfg.Transparent,
	}

	// Orient the view using the requested camera mode (or the supplied named
	// view matrix) and re-center it on the model's bounding-box center so the
	// part is framed instead of floating above the world origin.
	R := orientationFor(cfg.CameraMode, cfg.ViewMatrix)
	if cfg.ViewMatrix != "" {
		viewCfg.ViewMatrix = convertViewMatrix(cfg.ViewMatrix)
	}

	bbox, bboxErr := deps.Onshape.GetBoundingBoxes(ctx, job.DocumentID, "w", job.WorkspaceID, job.ElementID, false, false)
	if bboxErr != nil || bbox == nil {
		log.Warn("failed to get bounding box; view may be off-center", "err", bboxErr)
	}

	var cachedPixelSize float64
	if cfg.CameraViewport != "" {
		cachedPixelSize = pixelSizeFromCameraViewport(cfg.CameraViewport, viewCfg.OutputWidth, viewCfg.OutputHeight)
		if cachedPixelSize > 0 {
			log.Info("computed pixel size from named view cameraViewport",
				"pixelSize", cachedPixelSize, "viewport", cfg.CameraViewport)
		}
	}
	if bbox != nil {
		center := [3]float64{
			(bbox.LowX + bbox.HighX) / 2,
			(bbox.LowY + bbox.HighY) / 2,
			(bbox.LowZ + bbox.HighZ) / 2,
		}
		viewCfg.ViewMatrix = centeredViewMatrix(R, center)
		if cachedPixelSize <= 0 {
			fill := cfg.Zoom
			if fill <= 0 {
				fill = 0.75
			}
			cachedPixelSize = computePixelSizeFromBBox(bbox, viewCfg, fill, R)
			if cachedPixelSize > 0 {
				log.Info("computed pixel size from bounding box",
					"pixelSize", cachedPixelSize)
			}
		}
	}
	if cachedPixelSize > 0 {
		viewCfg.PixelSize = cachedPixelSize
	}
	// Capture loop.
	for stepIdx, step := range steps {
		if err := ctx.Err(); err != nil {
			deps.Queue.SetStatus(job.ID, StatusCancelled, "cancelled during capture")
			return nil
		}

		// Set the rollback bar to include up to and including this feature,
		// and wait until Onshape has actually applied the change. Onshape
		// applies the rollback (and regenerates geometry) asynchronously, so
		// capturing immediately yields the previous state for every frame --
		// resulting in identical, empty screenshots.
		if err := applyRollback(ctx, deps, job, step.featureIndex, log); err != nil {
			return err
		}

		// Determine the pixel size for this frame.
		// Named views have a fixed cameraViewport that doesn't change per frame.
		if cfg.CameraViewport != "" {
			viewCfg.PixelSize = cachedPixelSize
		} else {
			switch cfg.BBoxMode {
			case "each":
				bbox, err := deps.Onshape.GetBoundingBoxes(ctx, job.DocumentID, "w", job.WorkspaceID, job.ElementID, false, false)
				if err != nil || bbox == nil {
					log.Warn("per-frame bounding box failed, using fallback zoom", "err", err)
					viewCfg.PixelSize = cachedPixelSize
				} else {
					fill := cfg.Zoom
					if fill <= 0 {
						fill = 1
					}
					viewCfg.PixelSize = computePixelSizeFromBBox(bbox, viewCfg, fill, R)
				fcenter := [3]float64{
					(bbox.LowX + bbox.HighX) / 2,
					(bbox.LowY + bbox.HighY) / 2,
					(bbox.LowZ + bbox.HighZ) / 2,
				}
				viewCfg.ViewMatrix = centeredViewMatrix(R, fcenter)
				}
			default: // "once" or unset. Reuse the cached pixelSize from the completed model.
				viewCfg.PixelSize = cachedPixelSize
			}
		}

		// Capture the shaded view.
		pngBytes, err := deps.Onshape.GetShadedView(ctx, job.DocumentID, "w", job.WorkspaceID, job.ElementID, viewCfg)
		if err != nil {
			return fmt.Errorf("get shaded view at step %d: %w", stepIdx, err)
		}

		// Onshape always returns transparent PNGs. Composite onto a solid
		// Background unless the user explicitly requested transparency.
		if !cfg.Transparent && !viewCfg.Transparent {
			bgBytes, bgErr := addBackground(pngBytes, cfg.BgColor)
			if bgErr != nil {
				log.Warn("failed to add background", "err", bgErr)
			} else {
				pngBytes = bgBytes
			}
		}

		// Overlay feature name text if configured.
		if cfg.FeatureLabel {
			labeled, err := overlayFeatureText(pngBytes, step.feature.Name)
			if err != nil {
				log.Warn("failed to overlay feature text", "feature", step.feature.Name, "err", err)
			} else {
				pngBytes = labeled
			}
		}

		// Write hold frames for the first feature.
		if stepIdx == 0 && cfg.HoldFirst > 0 {
			for h := 0; h < cfg.HoldFirst; h++ {
				if err := writeFrame(pngBytes); err != nil {
					return err
				}
			}
		}

		if err := writeFrame(pngBytes); err != nil {
			return err
		}

		// Write hold frames for the last feature.
		if stepIdx == total-1 && cfg.HoldLast > 0 {
			for h := 0; h < cfg.HoldLast; h++ {
				if err := writeFrame(pngBytes); err != nil {
					return err
				}
			}
		}

		deps.Queue.UpdateProgress(job.ID, stepIdx+1, step.feature.Name, total, startedAt)
	}

	totalFrames := frameIndex - 1
	log.Info("frame capture complete", "frames", totalFrames)

	deps.Queue.UpdateProgress(job.ID, total, "Encoding video...", total, startedAt)

	if err := ctx.Err(); err != nil {
		deps.Queue.SetStatus(job.ID, StatusCancelled, "cancelled after capture")
		return nil
	}

	// FFmpeg encoding.
	fps := cfg.FrameRate
	if fps <= 0 {
		fps = 24
	}

	var outputs []storage.OutputFile

	frameGlob := storage.FrameGlob(paths)

	if slices.Contains(cfg.Formats, "mp4") {
		log.Info("encoding MP4")
		if err := deps.Encoder.EncodeMP4(ctx, frameGlob, paths.OutputMP4, fps, res); err != nil {
			return fmt.Errorf("encode MP4: %w", err)
		}
		size := fileSize(paths.OutputMP4)
		outputs = append(outputs, storage.OutputFile{Format: "mp4", Path: paths.OutputMP4, Size: size})
	}

	if slices.Contains(cfg.Formats, "gif") {
		log.Info("encoding GIF")
		if err := deps.Encoder.EncodeGIF(ctx, frameGlob, paths.OutputGIF, fps, res); err != nil {
			return fmt.Errorf("encode GIF: %w", err)
		}
		size := fileSize(paths.OutputGIF)
		outputs = append(outputs, storage.OutputFile{Format: "gif", Path: paths.OutputGIF, Size: size})
	}

	if slices.Contains(cfg.Formats, "zip") {
		log.Info("creating ZIP")
		if err := createZIP(paths, cfg.FileNaming); err != nil {
			return fmt.Errorf("create ZIP: %w", err)
		}
		size := fileSize(paths.OutputZIP)
		outputs = append(outputs, storage.OutputFile{Format: "zip", Path: paths.OutputZIP, Size: size})
	}

	deps.Queue.SetOutputs(job.ID, outputs)
	deps.Queue.SetStatus(job.ID, StatusCompleted, "")

	// Convert features to storage-local type for the manifest.
	storeFeatures := toStorageFeatures(job.Features)

	// Write manifest to disk.
	manifest := &storage.Manifest{
		JobID:        job.ID,
		DocumentID:   job.DocumentID,
		WorkspaceID:  job.WorkspaceID,
		ElementID:    job.ElementID,
		ExportConfig: cfg,
		Features:     storeFeatures,
		Status:       storage.StatusCompleted,
		CreatedAt:    job.CreatedAt,
		StartedAt:    job.StartedAt,
		CompletedAt:  job.CompletedAt,
		Outputs:      outputs,
	}
	if err := storage.WriteManifest(paths.Manifest, manifest); err != nil {
		log.Error("failed to write manifest", "err", err)
	}

	log.Info("job completed", "outputs", len(outputs))
	return nil
}

// Sets the workspace rollback bar. It tries the requested index
// First; if Onshape rejects it (409 for an invalid position, e.g. inside a
// folder group), it falls back to -1 (the "show all" sentinel) and then
// scans backward from the desired index to find a valid boundary. Unlike
// earlier versions, this does NOT call GetFeatureList to verify. We wait a
// short fixed delay after each call so Onshape can begin regenerating
// geometry before the shaded view is captured.
func applyRollback(ctx context.Context, deps Dependencies, job *Job, index int, log *slog.Logger) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	candidates := []int{index, -1}
	for offset := 1; offset <= 5; offset++ {
		if candidate := index - offset; candidate >= 0 {
			candidates = append(candidates, candidate)
		}
	}

	for _, candidate := range candidates {
		if err := trySetRollback(ctx, deps, job, candidate); err != nil {
			continue
		}
		switch {
		case candidate == -1:
			log.Warn("rollback set to -1 (end of feature list)", "desired", index)
		case candidate != index:
			log.Warn("rollback set to index-offset", "desired", index, "actual", candidate, "offset", index-candidate)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		return nil
	}

	return fmt.Errorf("set rollback to %d: no valid position found", index)
}

// Calls SetRollback and ignores errors only for invalid indices.
func trySetRollback(ctx context.Context, deps Dependencies, job *Job, index int) error {
	if index < -1 {
		return fmt.Errorf("invalid index %d", index)
	}
	return deps.Onshape.SetRollback(ctx, job.DocumentID, "w", job.WorkspaceID, job.ElementID, index)
}

// Converts onshape features into the storage-local manifest type.
func toStorageFeatures(in []onshape.Feature) []storage.Feature {
	out := make([]storage.Feature, len(in))
	for i, f := range in {
		out[i] = storage.Feature{
			ID:         f.ID,
			Name:       f.Name,
			Type:       f.Type,
			Suppressed: f.Suppressed,
		}
	}
	return out
}

// Returns true if a feature should be excluded from the render.
func shouldSkip(f onshape.Feature, cfg storage.ExportConfig) bool {
	// Folder features organize other features and produce no geometry, so
	// They never get their own capture frame.
	if f.Type == onshape.FolderType {
		return true
	}
	if cfg.SkipSuppressed && f.Suppressed {
		return true
	}
	ft := strings.ToLower(f.Type)
	if cfg.SkipSketches && strings.Contains(ft, "sketch") {
		return true
	}
	if cfg.SkipConstruction && strings.Contains(ft, "construction") {
		return true
	}
	if cfg.GeometryOnly {
		return !isGeometryFeature(ft)
	}
	return false
}

// Returns true for feature types known to produce 3D geometry.
func isGeometryFeature(featureType string) bool {
	geometryTypes := []string{
		"extrude", "revolve", "loft", "sweep", "shell", "fillet", "chamfer",
		"hole", "boolean", "pattern", "mirror", "split", "thicken", "offset",
		"mate", "import",
	}
	for _, t := range geometryTypes {
		if strings.Contains(featureType, t) {
			return true
		}
	}
	return false
}

// Computes the pixel size from a named view's
// cameraViewport values. The viewport is [left, right, bottom, top] in
// model-space meters. This produces exact framing that matches the named
// view, unlike bounding-box approximations.
func pixelSizeFromCameraViewport(cameraViewport string, outputWidth, outputHeight int) float64 {
	parts := strings.Split(cameraViewport, ",")
	if len(parts) != 4 {
		return 0
	}
	left, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	right, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	bottom, err3 := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	top, err4 := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return 0
	}

	viewportWidth := math.Abs(right - left)
	viewportHeight := math.Abs(top - bottom)
	if viewportWidth <= 0 || viewportHeight <= 0 {
		return 0
	}

	px := viewportWidth / float64(outputWidth)
	py := viewportHeight / float64(outputHeight)
	if px > py {
		return px
	}
	return py
}

// mat3 is a row-major 3x3 rotation (model -> view space).
type mat3 [3][3]float64

func rotX(a float64) mat3 {
	c, s := math.Cos(a), math.Sin(a)
	return mat3{
		{1, 0, 0},
		{0, c, -s},
		{0, s, c},
	}
}

// Standard Onshape model->view orientations. Onshape's shadedviews viewMatrix
// is a 12-number, row-major 3x4 matrix applied as a MODEL transform:
// view = R*model + t. The first three columns are the (orthonormal, positive
// determinant) rotation R; the 4th column t translates the model origin in
// meters. The image center is view coordinate (0,0), so to center the model we
// set t = -R*center.
//
//   - viewTop:   look straight down -Z (default on-screen orientation, X right,
//                Y up).
//   - viewFront: look along -Y (model X right, model Z up).
//   - viewIso:   standard engineering isometric. Onshape projects the model
//                axes to screen as +X -> down-right, +Y -> up-right, +Z -> up,
//                which is what this matrix reproduces.
var (
	viewTop = mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	viewFront = rotX(-math.Pi / 2)
	viewIso = mat3{
		{1 / math.Sqrt2, 1 / math.Sqrt2, 0},
		{-1 / math.Sqrt(6), 1 / math.Sqrt(6), math.Sqrt(2.0 / 3.0)},
		{1 / math.Sqrt(3), -1 / math.Sqrt(3), 1 / math.Sqrt(3)},
	}
)

// orientationFor returns the model->view rotation for the camera config. A
// supplied viewMatrix is parsed as the row-major matrix the frontend sends and
// its 3x3 rotation is extracted (the translation is recomputed when centering).
func orientationFor(cameraMode, viewMatrix string) mat3 {
	if viewMatrix != "" {
		return rotationFromMatrixString(viewMatrix)
	}
	switch strings.ToLower(cameraMode) {
	case "front":
		return viewFront
	case "top":
		return viewTop
	default:
		return viewIso
	}
}

// rotationFromMatrixString parses a comma-separated number list as a row-major
// 4x4 (or 3x4) matrix and returns its upper-left 3x3 rotation.
func rotationFromMatrixString(s string) mat3 {
	nums := parseMatrixFloats(s)
	var m mat3
	get := func(r, c int) float64 {
		idx := r*4 + c
		if idx < len(nums) {
			return nums[idx]
		}
		return 0
	}
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			m[r][c] = get(r, c)
		}
	}
	return m
}

// convertViewMatrix normalizes the supplied row-major matrix (as sent by the
// frontend) into the 12-number, row-major 3x4 form Onshape expects, preserving
// its original translation.
func convertViewMatrix(s string) string {
	nums := parseMatrixFloats(s)
	get := func(r, c int) float64 {
		idx := r*4 + c
		if idx < len(nums) {
			return nums[idx]
		}
		return 0
	}
	var R mat3
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			R[r][c] = get(r, c)
		}
	}
	t := [3]float64{get(0, 3), get(1, 3), get(2, 3)}
	return rowMajor12(R, t)
}

// centeredViewMatrix builds the 12-number, row-major 3x4 view matrix that
// orients the model by R and projects the model's bounding-box center onto the
// image center. Onshape's shadedviews centers the rendered image on the
// projected MODEL ORIGIN, not on the model -- so without this re-centering the
// model floats above/below the origin (almost always "too high", since parts
// are typically modelled above the part-studio origin).
func centeredViewMatrix(R mat3, center [3]float64) string {
	t := [3]float64{
		-(R[0][0]*center[0] + R[0][1]*center[1] + R[0][2]*center[2]),
		-(R[1][0]*center[0] + R[1][1]*center[1] + R[1][2]*center[2]),
		-(R[2][0]*center[0] + R[2][1]*center[1] + R[2][2]*center[2]),
	}
	return rowMajor12(R, t)
}

// rowMajor12 lays a rotation R and translation t out as the 12-number,
// row-major 3x4 string Onshape's shadedviews viewMatrix expects.
func rowMajor12(R mat3, t [3]float64) string {
	v := [12]float64{
		R[0][0], R[0][1], R[0][2], t[0],
		R[1][0], R[1][1], R[1][2], t[1],
		R[2][0], R[2][1], R[2][2], t[2],
	}
	parts := make([]string, 12)
	for i, x := range v {
		parts[i] = strconv.FormatFloat(x, 'g', -1, 64)
	}
	return strings.Join(parts, ",")
}

func parseMatrixFloats(s string) []float64 {
	parts := strings.Split(s, ",")
	nums := make([]float64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if v, err := strconv.ParseFloat(p, 64); err == nil {
			nums = append(nums, v)
		}
	}
	return nums
}

// projectionExtent projects the 8 corners of the bbox through R and returns the
// on-screen width/height of the model in model-space units.
func projectionExtent(R mat3, bbox *onshape.BoundingBox) (float64, float64) {
	xs := [2]float64{bbox.LowX, bbox.HighX}
	ys := [2]float64{bbox.LowY, bbox.HighY}
	zs := [2]float64{bbox.LowZ, bbox.HighZ}
	minX, maxX := math.MaxFloat64, -math.MaxFloat64
	minY, maxY := math.MaxFloat64, -math.MaxFloat64
	for _, x := range xs {
		for _, y := range ys {
			for _, z := range zs {
				sx := R[0][0]*x + R[0][1]*y + R[0][2]*z
				sy := R[1][0]*x + R[1][1]*y + R[1][2]*z
				if sx < minX {
					minX = sx
				}
				if sx > maxX {
					maxX = sx
				}
				if sy < minY {
					minY = sy
				}
				if sy > maxY {
					maxY = sy
				}
			}
		}
	}
	return maxX - minX, maxY - minY
}

// Computes the pixel size that frames the given bounding box within the
// viewport at the specified fill fraction, using the chosen view orientation.
// The pixelSize only controls zoom; centering is handled separately by
// centeredViewMatrix.
func computePixelSizeFromBBox(bbox *onshape.BoundingBox, viewCfg onshape.ShadedViewConfig, fill float64, R mat3) float64 {
	screenW, screenH := projectionExtent(R, bbox)
	if screenW <= 0 || screenH <= 0 {
		return 0
	}
	px := screenW / (float64(viewCfg.OutputWidth) * fill)
	py := screenH / (float64(viewCfg.OutputHeight) * fill)
	if px > py {
		return px
	}
	return py
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// Packages all frames into a ZIP archive.
// If namePattern is non-empty, each frame entry in the archive is renamed
// according to the pattern with {index} replaced by the frame number.
// Otherwise the original filename (frame_0001.png) is used.
func createZIP(paths storage.Paths, namePattern string) error {
	zf, err := os.Create(paths.OutputZIP)
	if err != nil {
		return err
	}
	defer zf.Close()

	w := zip.NewWriter(zf)
	defer w.Close()

	entries, err := os.ReadDir(paths.FramesDir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".png") {
			continue
		}

		entryName := e.Name()
		if namePattern != "" {
			frameNum := 0
			if _, err := fmt.Sscanf(e.Name(), "frame_%d.png", &frameNum); err == nil {
				entryName = strings.ReplaceAll(namePattern, "{index}", fmt.Sprintf("%d", frameNum))
			} else {
				return fmt.Errorf("unexpected frame filename format: %s", e.Name())
			}
		}

		src, err := os.Open(paths.FramesDir + "/" + e.Name())
		if err != nil {
			return err
		}
		dst, err := w.Create(entryName)
		if err != nil {
			src.Close()
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			src.Close()
			return err
		}
		src.Close()
	}
	return nil
}
