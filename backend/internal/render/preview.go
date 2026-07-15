package render

import (
	"context"
	"fmt"

	"github.com/abstractmelon/onshape-replay/internal/ffmpeg"
	"github.com/abstractmelon/onshape-replay/internal/onshape"
	"github.com/abstractmelon/onshape-replay/internal/storage"
)

// Captures a single shaded-view frame of the Part Studio using the given export config.
// It returns raw PNG bytes.
func CapturePreview(ctx context.Context, client *onshape.Client, documentID, wvmType, wvmID, elementID string, cfg storage.ExportConfig) ([]byte, error) {
	res := ffmpeg.DefaultResolution
	if r, ok := ffmpeg.Resolutions[cfg.Resolution]; ok {
		res = r
	}

	viewCfg := onshape.ShadedViewConfig{
		OutputWidth:     res.Width,
		OutputHeight:    res.Height,
		ShowAllParts:    true,
		UseAntiAliasing: true,
		Transparent:     cfg.Transparent,
	}

	// Orient the view and re-center it on the model's bounding-box center so
	// the part is framed instead of floating above the world origin.
	R := orientationFor(cfg.CameraMode, cfg.ViewMatrix)
	if cfg.ViewMatrix != "" {
		viewCfg.ViewMatrix = convertViewMatrix(cfg.ViewMatrix)
	}

	var cachedPixelSize float64
	if cfg.CameraViewport != "" {
		cachedPixelSize = pixelSizeFromCameraViewport(cfg.CameraViewport, viewCfg.OutputWidth, viewCfg.OutputHeight)
	}
	bbox, err := client.GetBoundingBoxes(ctx, documentID, wvmType, wvmID, elementID, false, false)
	if err == nil && bbox != nil {
		center := [3]float64{
			(bbox.LowX + bbox.HighX) / 2,
			(bbox.LowY + bbox.HighY) / 2,
			(bbox.LowZ + bbox.HighZ) / 2,
		}
		viewCfg.ViewMatrix = centeredViewMatrix(R, center)
		if cachedPixelSize <= 0 {
			fill := cfg.Zoom
			if fill <= 0 {
				fill = 1
			}
			cachedPixelSize = computePixelSizeFromBBox(bbox, viewCfg, fill, R)
		}
	}
	if cachedPixelSize > 0 {
		viewCfg.PixelSize = cachedPixelSize
	}

	pngBytes, err := client.GetShadedView(ctx, documentID, wvmType, wvmID, elementID, viewCfg)
	if err != nil {
		return nil, fmt.Errorf("preview capture: %w", err)
	}

	if !cfg.Transparent {
		bgBytes, bgErr := addBackground(pngBytes, cfg.BgColor)
		if bgErr != nil {
			return nil, fmt.Errorf("preview background: %w", bgErr)
		}
		pngBytes = bgBytes
	}

	return pngBytes, nil
}
