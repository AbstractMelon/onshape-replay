package render

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
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
	Onshape *onshape.Client
	Encoder *ffmpeg.Encoder
	Queue   *Queue
	Log     *slog.Logger
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
				JobID:       job.ID,
				DocumentID:  job.DocumentID,
				WorkspaceID: job.WorkspaceID,
				ElementID:   job.ElementID,
				ExportConfig: job.Config,
				Status:      storage.StatusFailed,
				ErrorMsg:    err.Error(),
				CreatedAt:   job.CreatedAt,
				StartedAt:   job.StartedAt,
				CompletedAt: job.CompletedAt,
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
	features, origRollback, err := deps.Onshape.GetFeatureList(ctx, job.DocumentID, "w", job.WorkspaceID, job.ElementID)
	if err != nil {
		return fmt.Errorf("get feature list: %w", err)
	}
	log.Info("feature list retrieved", "count", len(features), "rollbackIndex", origRollback)

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
		featureIndex int    // 1-based rollback position
		feature      onshape.Feature
	}

	var steps []captureStep
	for i, f := range features {
		if shouldSkip(f, cfg) {
			continue
		}
		steps = append(steps, captureStep{featureIndex: i + 1, feature: f})
	}

	// Add hold frames at start and end.
	total := len(steps)
	if total == 0 {
		return fmt.Errorf("no features to render after applying filters")
	}

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
	storeFeatures := make([]storage.Feature, len(job.Features))
	for i, f := range job.Features {
		storeFeatures[i] = storage.Feature{
			ID:         f.ID,
			Name:       f.Name,
			Type:       f.Type,
			Suppressed: f.Suppressed,
		}
	}

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

// applyRollback sets the workspace rollback bar and blocks until Onshape has
// actually applied the change. Onshape applies rollbacks (and regenerates
// part geometry) asynchronously on its servers, so a shaded view captured
// immediately after SetRollback would reflect the previous state. We confirm
// the new rollbackIndex via GetFeatureList and retry with a short backoff
// until it matches (or the context is cancelled).
func applyRollback(ctx context.Context, deps Dependencies, job *Job, index int, log *slog.Logger) error {
	const maxAttempts = 20

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := deps.Onshape.SetRollback(ctx, job.DocumentID, "w", job.WorkspaceID, job.ElementID, index); err != nil {
			return fmt.Errorf("set rollback to %d: %w", index, err)
		}

		_, applied, err := deps.Onshape.GetFeatureList(ctx, job.DocumentID, "w", job.WorkspaceID, job.ElementID)
		if err != nil {
			return fmt.Errorf("verify rollback %d: %w", index, err)
		}

		if applied == index {
			return nil
		}

		log.Debug("rollback not yet applied, retrying", "desired", index, "observed", applied, "attempt", attempt+1)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}

	return fmt.Errorf("rollback to %d was never applied by Onshape", index)
}

// shouldSkip returns true if a feature should be excluded from the render.
func shouldSkip(f onshape.Feature, cfg storage.ExportConfig) bool {
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
