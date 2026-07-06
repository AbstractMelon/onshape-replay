package onshape

import (
	"context"
	"fmt"
)

// rollbackBody is the JSON body sent to updateRollback.
type rollbackBody struct {
	Rollback rollbackEntry `json:"rollback"`
}

type rollbackEntry struct {
	// Index is the 0-based index of the rollback bar position in the feature list.
	// Setting it to N means features 0..N-1 are active.
	Index int `json:"index"`
}

// SetRollback advances the rollback bar to the given 0-based feature index.
// All features at positions >= index will be suppressed (rolled back).
// To show all N features, pass index = N.
func (c *Client) SetRollback(ctx context.Context, documentID, wvmType, wvmID, elementID string, index int) error {
	path := fmt.Sprintf("/partstudios/d/%s/%s/%s/e/%s/features/rollback",
		documentID, wvmType, wvmID, elementID)

	body := rollbackBody{
		Rollback: rollbackEntry{Index: index},
	}

	if err := c.postJSON(ctx, path, body, nil); err != nil {
		return fmt.Errorf("SetRollback(index=%d): %w", index, err)
	}
	return nil
}
