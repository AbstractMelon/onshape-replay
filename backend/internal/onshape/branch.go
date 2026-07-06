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
	}

	var resp createWorkspaceResponse
	if err := c.postJSON(ctx, path, body, &resp); err != nil {
		return "", fmt.Errorf("CreateWorkspace: %w", err)
	}
	return resp.ID, nil
}

// DeleteWorkspace deletes a workspace (branch) from a document.
// This is used to clean up the temporary branch created for frame capture.
func (c *Client) DeleteWorkspace(ctx context.Context, documentID, workspaceID string) error {
	path := fmt.Sprintf("/documents/d/%s/workspaces/%s", documentID, workspaceID)

	if err := c.do(ctx, "DELETE", path, nil, nil, nil); err != nil {
		return fmt.Errorf("DeleteWorkspace: %w", err)
	}
	return nil
}
