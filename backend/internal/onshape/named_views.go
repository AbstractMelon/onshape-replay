package onshape

import (
	"context"
	"fmt"
	"net/url"
)

// NamedViewsResponse is the response from GET .../namedViews.
type NamedViewsResponse struct {
	NamedViews map[string]NamedViewData `json:"namedViews"`
}

// Contains the view data for a single named view.
type NamedViewData struct {
	Perspective    bool      `json:"perspective"`
	CameraViewport []float64 `json:"cameraViewport"`
	Angle          float64   `json:"angle"`
	ViewMatrix     []float64 `json:"viewMatrix"`
}

// Fetches all named views for a Part Studio element.
// The namedViews endpoint uses d/{did}/e/{eid} (no wvm/wvid in the path).
// SkipPerspective and includeSectionCutViews are forwarded as query params.
func (c *Client) GetNamedViews(ctx context.Context, documentID, elementID string, skipPerspective, includeSectionCutViews bool) (*NamedViewsResponse, error) {
	path := fmt.Sprintf("/partstudios/d/%s/e/%s/namedViews",
		documentID, elementID)

	q := url.Values{}
	q.Set("skipPerspective", boolStr(skipPerspective))
	q.Set("includeSectionCutViews", boolStr(includeSectionCutViews))

	var resp NamedViewsResponse
	if err := c.get(ctx, path, q, &resp); err != nil {
		return nil, fmt.Errorf("GetNamedViews: %w", err)
	}
	return &resp, nil
}
