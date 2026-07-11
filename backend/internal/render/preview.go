package render

import (
	"context"
	"fmt"

	"github.com/abstractmelon/onshape-replay/internal/ffmpeg"
	"github.com/abstractmelon/onshape-replay/internal/onshape"
	"github.com/abstractmelon/onshape-replay/internal/storage"
)

// CapturePreview captures a single shaded-view frame of the Part Studio using
// the given export config. It returns raw PNG bytes. Use this for quick
// previews before committing to a full multi-frame render.
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
	setViewMatrix(cfg.CameraMode, cfg.ViewMatrix, &viewCfg)
	if viewCfg.ViewMatrix == "" {
		viewCfg.ViewMatrix = "isometric"
	}

	if cfg.CameraViewport != "" {
		viewCfg.PixelSize = pixelSizeFromCameraViewport(cfg.CameraViewport, viewCfg.OutputWidth, viewCfg.OutputHeight)
	}
	if viewCfg.PixelSize <= 0 {
		bbox, err := client.GetBoundingBoxes(ctx, documentID, wvmType, wvmID, elementID, false, false)
		if err == nil && bbox != nil {
			viewCfg.PixelSize = computePixelSizeFromBBox(bbox, viewCfg, 0.75)
		}
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
