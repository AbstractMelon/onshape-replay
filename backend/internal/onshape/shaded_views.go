package onshape

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
)

// ShadedViewConfig configures how the shaded view is captured.
type ShadedViewConfig struct {
	// OutputHeight in pixels (e.g. 720, 1080, 2160).
	OutputHeight int
	// OutputWidth in pixels.
	OutputWidth int
	// PixelSize controls the rendering scale. Leave 0 for API default.
	PixelSize float64
	// ShowAllParts renders all parts in the Part Studio.
	ShowAllParts bool
	// UseAntiAliasing enables SSAA.
	UseAntiAliasing bool
	// ViewMatrix is a 4x4 column-major transformation matrix as a
	// comma-separated string. Leave empty to use the current camera.
	ViewMatrix string
	// BgColor is the background color for the rendered view.
	// Empty means Onshape default; other values are passed to outputColorMethod.
	BgColor string
	// Transparent requests a transparent background when the output supports it.
	Transparent bool
}

// shadedViewsResponse is the raw API response from getPartStudioShadedViews.
type shadedViewsResponse struct {
	Images []string `json:"images"`
}

// GetShadedView captures a shaded PNG of the Part Studio and returns the raw
// image bytes. The Onshape v6 API returns base64-encoded PNG strings inside a
// JSON envelope; this method decodes them and returns raw PNG bytes.
func (c *Client) GetShadedView(ctx context.Context, documentID, wvmType, wvmID, elementID string, cfg ShadedViewConfig) ([]byte, error) {
	path := fmt.Sprintf("/partstudios/d/%s/%s/%s/e/%s/shadedviews",
		documentID, wvmType, wvmID, elementID)

	q := url.Values{}
	if cfg.OutputHeight > 0 {
		q.Set("outputHeight", fmt.Sprintf("%d", cfg.OutputHeight))
	}
	if cfg.OutputWidth > 0 {
		q.Set("outputWidth", fmt.Sprintf("%d", cfg.OutputWidth))
	}
	if cfg.PixelSize > 0 {
		q.Set("pixelSize", fmt.Sprintf("%f", cfg.PixelSize))
	}
	q.Set("showAllParts", boolStr(cfg.ShowAllParts))
	q.Set("useAntiAliasing", boolStr(cfg.UseAntiAliasing))

	if cfg.ViewMatrix != "" {
		q.Set("viewMatrix", cfg.ViewMatrix)
	}
	if cfg.BgColor != "" {
		q.Set("outputColorMethod", cfg.BgColor)
	}
	if cfg.Transparent {
		q.Set("outputColorMethod", "transparent")
	}

	var resp shadedViewsResponse
	if err := c.get(ctx, path, q, &resp); err != nil {
		return nil, fmt.Errorf("GetShadedView: %w", err)
	}

	if len(resp.Images) == 0 {
		return nil, fmt.Errorf("GetShadedView: no images returned")
	}

	raw, err := base64.StdEncoding.DecodeString(resp.Images[0])
	if err != nil {
		return nil, fmt.Errorf("GetShadedView: base64 decode: %w", err)
	}
	return raw, nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
