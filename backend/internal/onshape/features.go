package onshape

import (
	"context"
	"fmt"
	"strings"
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
// The v6 API returns feature objects directly in the features array
// (NOT wrapped in a "feature" key). Individual features can be compact
// (BTMFeature-134) or expanded (BTMSketch-151 etc.) depending on whether
// the API includes their entity/geometry detail inline.
type featureListResponse struct {
	Features      []featureFlatEntry `json:"features"`
	RollbackIndex int                `json:"rollbackIndex"`
}

// featureFlatEntry parses both compact (BTMFeature-134) and expanded
// (BTMSketch-151 etc.) feature representations. Compact entries carry
// featureId/featureType/name/suppressed directly; expanded sketches carry
// entityId/suppressionState and use subFeatures for folder children.
type featureFlatEntry struct {
	BtType           string             `json:"btType"`
	FeatureID        string             `json:"featureId"`
	EntityID         string             `json:"entityId"`
	Name             string             `json:"name"`
	FeatureType      string             `json:"featureType"`
	Suppressed       bool               `json:"suppressed"`
	SuppressionState interface{}        `json:"suppressionState"`
	SubFeatures      []featureFlatEntry `json:"subFeatures"`
}

// deriveFeatureType infers a feature type string from the btType discriminator
// when the feature's own featureType field is absent (e.g. expanded sketches).
func deriveFeatureType(btType string) string {
	if strings.Contains(btType, "Sketch") {
		return "newSketch"
	}
	return ""
}

// flatten expands the (possibly nested) feature list into a single slice in
// document order. Folder entries are kept in place (they occupy a rollback
// position in Onshape's ordering) followed by their children, recursively.
// This matches the order Onshape uses for its rollbackIndex.
func flatten(entries []featureFlatEntry, out *[]Feature) {
	for _, f := range entries {
		id := f.FeatureID
		if id == "" {
			id = f.EntityID
		}

		ftype := f.FeatureType
		if ftype == "" {
			ftype = deriveFeatureType(f.BtType)
		}

		suppressed := f.Suppressed
		if !suppressed && f.SuppressionState != nil {
			suppressed = true
		}

		*out = append(*out, Feature{
			ID:         id,
			Name:       f.Name,
			Type:       ftype,
			Suppressed: suppressed,
		})
		if len(f.SubFeatures) > 0 {
			flatten(f.SubFeatures, out)
		}
	}
}

// FolderType is the featureType Onshape uses for folder features.
const FolderType = "folder"

// GetFeatureList returns the ordered list of features in the Part Studio,
// the current rollback bar index, and the maximum valid rollback index.
// Folders are flattened in document order so the returned indices line up
// with Onshape's rollbackIndex. The maxRollbackIndex is the count of
// top-level (non-flattened) features. SetRollback only accepts indices
// in [0, maxRollbackIndex]. wvmType is "w" for workspace, "v" for version,
// "m" for microversion.
func (c *Client) GetFeatureList(ctx context.Context, documentID, wvmType, wvmID, elementID string) (features []Feature, origRollback int, maxRollbackIndex int, err error) {
	path := fmt.Sprintf("/partstudios/d/%s/%s/%s/e/%s/features",
		documentID, wvmType, wvmID, elementID)

	var raw featureListResponse
	if err := c.get(ctx, path, nil, &raw); err != nil {
		return nil, 0, 0, fmt.Errorf("GetFeatureList: %w", err)
	}

	features = make([]Feature, 0, len(raw.Features))
	flatten(raw.Features, &features)
	return features, raw.RollbackIndex, len(raw.Features), nil
}
