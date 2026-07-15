package onshape

import (
	"context"
	"fmt"
	"net/url"
)

// BoundingBox represents the axis-aligned bounding box of parts in a Part Studio.
type BoundingBox struct {
	LowX  float64 `json:"lowX"`
	LowY  float64 `json:"lowY"`
	LowZ  float64 `json:"lowZ"`
	HighX float64 `json:"highX"`
	HighY float64 `json:"highY"`
	HighZ float64 `json:"highZ"`
}

// Returns the axis-aligned bounding box of all visible parts
// in the Part Studio. The returned values are in meters and are approximate
// (meant for graphics/visualization, not precise measurement).
func (c *Client) GetBoundingBoxes(ctx context.Context, documentID, wvmType, wvmID, elementID string, includeHidden, includeWireBodies bool) (*BoundingBox, error) {
	path := fmt.Sprintf("/partstudios/d/%s/%s/%s/e/%s/boundingboxes",
		documentID, wvmType, wvmID, elementID)

	q := url.Values{}
	q.Set("includeHidden", fmt.Sprintf("%t", includeHidden))
	q.Set("includeWireBodies", fmt.Sprintf("%t", includeWireBodies))

	var bbox BoundingBox
	if err := c.get(ctx, path, q, &bbox); err != nil {
		return nil, fmt.Errorf("GetBoundingBoxes: %w", err)
	}
	return &bbox, nil
}
