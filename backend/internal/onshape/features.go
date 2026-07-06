package onshape

import (
	"context"
	"fmt"
)

// Feature represents a single feature in a Part Studio's feature list.
type Feature struct {
	// ID is the Onshape internal feature ID.
	ID string `json:"featureId"`
	// Name is the user-visible feature name (e.g., "Extrude 1").
	Name string `json:"name"`
	// Type is the feature type string (e.g., "newExtrude", "newSketch").
	Type string `json:"featureType"`
	// Suppressed indicates the feature is currently suppressed.
	Suppressed bool `json:"suppressed"`
}

// featureListResponse is the raw shape returned by getPartStudioFeatures.
// The v6 API returns flat feature objects (NOT wrapped in a "feature" key).
type featureListResponse struct {
	Features []featureFlatEntry `json:"features"`
}

type featureFlatEntry struct {
	FeatureID   string `json:"featureId"`
	Name        string `json:"name"`
	FeatureType string `json:"featureType"`
	Suppressed  bool   `json:"suppressed"`
}

// GetFeatureList returns the ordered list of features in the Part Studio.
// wvmType is "w" for workspace, "v" for version, "m" for microversion.
func (c *Client) GetFeatureList(ctx context.Context, documentID, wvmType, wvmID, elementID string) ([]Feature, error) {
	path := fmt.Sprintf("/partstudios/d/%s/%s/%s/e/%s/features",
		documentID, wvmType, wvmID, elementID)

	var raw featureListResponse
	if err := c.get(ctx, path, nil, &raw); err != nil {
		return nil, fmt.Errorf("GetFeatureList: %w", err)
	}

	features := make([]Feature, 0, len(raw.Features))
	for _, f := range raw.Features {
		features = append(features, Feature{
			ID:         f.FeatureID,
			Name:       f.Name,
			Type:       f.FeatureType,
			Suppressed: f.Suppressed,
		})
	}
	return features, nil
}
