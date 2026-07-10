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

// StartPipeline launches the render pipeline in a background goroutine.
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

// runPipeline is the synchronous render logic running inside a goroutine.
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
	// so the user's workspace is not left in a rolled-back state.
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
	// value immediately, not just after the first frame is captured.
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
	setViewMatrix(cfg.CameraMode, &viewCfg)
	if viewCfg.ViewMatrix == "" {
		viewCfg.ViewMatrix = "isometric"
	}

	// Get the bounding box of the completed model to compute the pixel size.
	// In "once" mode this value is cached for every frame; in "each" mode it's
	// only used as a fallback if the per-frame call fails.
	cachedPixelSize, bboxErr := computePixelSize(ctx, deps, job, viewCfg)
	if bboxErr != nil {
		log.Warn("failed to get bounding box from completed model", "err", bboxErr)
	}
	if cachedPixelSize > 0 {
		log.Info("computed pixel size from completed model bounding box",
			"pixelSize", cachedPixelSize, "mode", cfg.BBoxMode)
		viewCfg.PixelSize = cachedPixelSize
	}

	// Take a test shot BEFORE any rollback to verify GetShadedView works.
	log.Info("capturing test frame at original rollback state")
	testPng, testErr := deps.Onshape.GetShadedView(ctx, job.DocumentID, "w", job.WorkspaceID, job.ElementID, viewCfg)
	if testErr != nil {
		log.Warn("test frame failed", "err", testErr)
	} else {
		log.Info("test frame received", "bytes", len(testPng))
		if err := writeFrame(testPng); err != nil {
			return err
		}
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
		switch cfg.BBoxMode {
		case "each":
			bbox, err := deps.Onshape.GetBoundingBoxes(ctx, job.DocumentID, "w", job.WorkspaceID, job.ElementID, false, false)
			if err != nil || bbox == nil {
				log.Warn("per-frame bounding box failed, using fallback zoom", "err", err)
				viewCfg.PixelSize = cachedPixelSize
			} else {
				viewCfg.PixelSize = isometricPixelSize(bbox, viewCfg.OutputWidth, viewCfg.OutputHeight, 0.75)
			}
		default: // "once" or unset. Reuse the cached pixelSize from the completed model.
			viewCfg.PixelSize = cachedPixelSize
		}

		// Capture the shaded view.
		pngBytes, err := deps.Onshape.GetShadedView(ctx, job.DocumentID, "w", job.WorkspaceID, job.ElementID, viewCfg)
		if err != nil {
			return fmt.Errorf("get shaded view at step %d: %w", stepIdx, err)
		}

		// Onshape always returns transparent PNGs. Composite onto a solid
		// background unless the user explicitly requested transparency.
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
		if err := createZIP(paths); err != nil {
			return fmt.Errorf("create ZIP: %w", err)
		}
		size := fileSize(paths.OutputZIP)
		outputs = append(outputs, storage.OutputFile{Format: "zip", Path: paths.OutputZIP, Size: size})
	}

	if slices.Contains(cfg.Formats, "png") {
		// PNG sequence: frames are already on disk, just record them.
		outputs = append(outputs, storage.OutputFile{Format: "png", Path: paths.FramesDir, Size: 0})
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

// applyRollback sets the workspace rollback bar. It tries the requested index
// first; if Onshape rejects it (409 for an invalid position, e.g. inside a
// folder group), it falls back to -1 (the "show all" sentinel) and then
// scans backward from the desired index to find a valid boundary. Unlike
// earlier versions, this does NOT call GetFeatureList to verify. We wait a
// short fixed delay after each call so Onshape can begin regenerating
// geometry before the shaded view is captured.
func applyRollback(ctx context.Context, deps Dependencies, job *Job, index int, log *slog.Logger) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := trySetRollback(ctx, deps, job, index); err == nil {
		goto wait
	}

	// The -1 sentinel is an alias for "end of the feature list" (show all).
	// It always succeeds when the exact index is the max valid value.
	if err := trySetRollback(ctx, deps, job, -1); err == nil {
		log.Warn("rollback set to -1 (end of feature list)", "desired", index)
		goto wait
	}

	// Onshape rejects rollback positions inside a folder group.
	// Scan backward to find the nearest valid boundary.
	for offset := 1; offset <= 5; offset++ {
		if candidate := index - offset; candidate >= 0 {
			if err := trySetRollback(ctx, deps, job, candidate); err == nil {
				log.Warn("rollback set to index-offset", "desired", index, "actual", candidate, "offset", offset)
				goto wait
			}
		}
	}

	return fmt.Errorf("set rollback to %d: no valid position found", index)

wait:
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(200 * time.Millisecond):
	}
	return nil
}

// trySetRollback calls SetRollback and ignores errors only for invalid indices.
func trySetRollback(ctx context.Context, deps Dependencies, job *Job, index int) error {
	if index < 0 {
		return fmt.Errorf("invalid index %d", index)
	}
	return deps.Onshape.SetRollback(ctx, job.DocumentID, "w", job.WorkspaceID, job.ElementID, index)
}

// toStorageFeatures converts onshape features into the storage-local manifest type.
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

// shouldSkip returns true if a feature should be excluded from the render.
func shouldSkip(f onshape.Feature, cfg storage.ExportConfig) bool {
	// Folder features organize other features and produce no geometry, so
	// they never get their own capture frame.
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

// isGeometryFeature returns true for feature types known to produce 3D geometry.
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

// setViewMatrix applies a standard camera orientation based on cameraMode.
// Onshape's shadedViews API accepts named views ("isometric", "front", "top")
// or a 12-value column-major transformation matrix.
func setViewMatrix(cameraMode string, cfg *onshape.ShadedViewConfig) {
	switch strings.ToLower(cameraMode) {
	case "isometric":
		cfg.ViewMatrix = "isometric"
	case "front":
		cfg.ViewMatrix = "front"
	case "top":
		cfg.ViewMatrix = "top"
	default:
		// "current" camera: leave ViewMatrix empty to use whatever is active.
		// In a temp workspace there is no stored camera, so the
		// pipeline falls back to isometric after calling this function.
		cfg.ViewMatrix = ""
	}
}

// computePixelSize fetches the bounding box of the completed model (at the
// original rollback state) and returns the pixel size that frames it at 75%
// fill. Returns 0 if the API call fails, which signals callers to use a
// fallback (e.g. Onshape's default zoom).
func computePixelSize(ctx context.Context, deps Dependencies, job *Job, viewCfg onshape.ShadedViewConfig) (float64, error) {
	bbox, err := deps.Onshape.GetBoundingBoxes(ctx, job.DocumentID, "w", job.WorkspaceID, job.ElementID, false, false)
	if err != nil {
		return 0, err
	}
	return isometricPixelSize(bbox, viewCfg.OutputWidth, viewCfg.OutputHeight, 0.75), nil
}

// isometricPixelSize computes the pixelSize needed to frame the bounding box
// within the viewport under Onshape's isometric view projection. The fill
// parameter controls what fraction of the viewport the model should occupy
// (e.g. 0.75 = 75% fill).
func isometricPixelSize(bbox *onshape.BoundingBox, width, height int, fill float64) float64 {
	cos30 := math.Cos(math.Pi / 6)
	sin30 := math.Sin(math.Pi / 6)

	// Project the 8 corners of the bbox through an isometric projection:
	//   x_screen = (x - z) * cos(30°)
	//   y_screen = (x + z) * sin(30°) + y
	corners := [8][2]float64{
		{(bbox.LowX - bbox.LowZ) * cos30, (bbox.LowX+bbox.LowZ)*sin30 + bbox.LowY},
		{(bbox.HighX - bbox.LowZ) * cos30, (bbox.HighX+bbox.LowZ)*sin30 + bbox.LowY},
		{(bbox.LowX - bbox.HighZ) * cos30, (bbox.LowX+bbox.HighZ)*sin30 + bbox.LowY},
		{(bbox.HighX - bbox.HighZ) * cos30, (bbox.HighX+bbox.HighZ)*sin30 + bbox.LowY},
		{(bbox.LowX - bbox.LowZ) * cos30, (bbox.LowX+bbox.LowZ)*sin30 + bbox.HighY},
		{(bbox.HighX - bbox.LowZ) * cos30, (bbox.HighX+bbox.LowZ)*sin30 + bbox.HighY},
		{(bbox.LowX - bbox.HighZ) * cos30, (bbox.LowX+bbox.HighZ)*sin30 + bbox.HighY},
		{(bbox.HighX - bbox.HighZ) * cos30, (bbox.HighX+bbox.HighZ)*sin30 + bbox.HighY},
	}

	minX, maxX := corners[0][0], corners[0][0]
	minY, maxY := corners[0][1], corners[0][1]
	for _, c := range corners[1:] {
		if c[0] < minX {
			minX = c[0]
		}
		if c[0] > maxX {
			maxX = c[0]
		}
		if c[1] < minY {
			minY = c[1]
		}
		if c[1] > maxY {
			maxY = c[1]
		}
	}

	screenW := maxX - minX
	screenH := maxY - minY

	// If the bbox has no extent on screen, fall back to auto-fit.
	if screenW <= 0 || screenH <= 0 {
		return 0
	}

	px := screenW / (float64(width) * fill)
	py := screenH / (float64(height) * fill)
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

// createZIP packages all frames into a ZIP archive.
func createZIP(paths storage.Paths) error {
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
		src, err := os.Open(paths.FramesDir + "/" + e.Name())
		if err != nil {
			return err
		}
		dst, err := w.Create(e.Name())
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
