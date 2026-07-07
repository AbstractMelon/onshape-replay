package onshape

import (
	"context"
	"fmt"
)

// rollbackBody is the JSON body sent to updateRollback.
// Per the Onshape API, the body is a single top-level "rollbackIndex" integer.
// Features with a 0-based entry index >= rollbackIndex are rolled back
// (suppressed). A value of -1 is an alias for "end of the feature list"
// (show all features). The value must be in the range 0..len(features).
type rollbackBody struct {
	RollbackIndex int `json:"rollbackIndex"`
}

// SetRollback moves the rollback bar so that features at 0-based positions
// >= index are rolled back and features 0..index-1 remain active.
// To show only feature 0, pass index = 1; to show all N features, pass
// index = N (or -1).
func (c *Client) SetRollback(ctx context.Context, documentID, wvmType, wvmID, elementID string, index int) error {
	path := fmt.Sprintf("/partstudios/d/%s/%s/%s/e/%s/features/rollback",
		documentID, wvmType, wvmID, elementID)

	body := rollbackBody{RollbackIndex: index}

	if err := c.postJSON(ctx, path, body, nil); err != nil {
		return fmt.Errorf("SetRollback(index=%d): %w", index, err)
	}
	return nil
}
