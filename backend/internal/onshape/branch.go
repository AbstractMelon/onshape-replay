package onshape

import (
	"context"
	"fmt"
)

// Workspace represents an Onshape workspace (branch).
type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// createWorkspaceBody is the JSON body for creating a new workspace.
type createWorkspaceBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	WorkspaceID string `json:"workspaceId,omitempty"`
}

type createWorkspaceResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateWorkspace creates a new workspace (branch) from the current tip of an
// existing workspace. Returns the new workspace ID.
func (c *Client) CreateWorkspace(ctx context.Context, documentID, sourceWorkspaceID, name string) (string, error) {
	path := fmt.Sprintf("/documents/d/%s/workspaces", documentID)

	body := createWorkspaceBody{
		Name:        name,
		Description: "Onshape Replay temporary branch -- safe to delete",
		WorkspaceID: sourceWorkspaceID,
	}

	var resp createWorkspaceResponse
	if err := c.postJSON(ctx, path, body, &resp); err != nil {
		return "", fmt.Errorf("CreateWorkspace: %w", err)
	}
	return resp.ID, nil
}

// DeleteWorkspace deletes a workspace (branch) from a document.
// This is used to clean up the temporary branch created for frame capture.
// Retries once with a refreshed token if the first attempt gets a 403.
func (c *Client) DeleteWorkspace(ctx context.Context, documentID, workspaceID string) error {
	path := fmt.Sprintf("/documents/d/%s/workspaces/%s", documentID, workspaceID)

	err := c.do(ctx, "DELETE", path, nil, nil, nil)
	if err == nil {
		return nil
	}

	// Onshape sometimes returns 403 "Invalid API key state" when the access
	// token has become stale for this endpoint. Try refreshing the token and
	// retrying once.
	if c.tokenRefresher != nil {
		_, rErr := c.tokenRefresher(ctx)
		if rErr != nil {
			return fmt.Errorf("DeleteWorkspace (token refresh failed): %w", err)
		}
		if retryErr := c.do(ctx, "DELETE", path, nil, nil, nil); retryErr != nil {
			return fmt.Errorf("DeleteWorkspace (retry after refresh): %w", retryErr)
		}
		return nil
	}

	return fmt.Errorf("DeleteWorkspace: %w", err)
}
