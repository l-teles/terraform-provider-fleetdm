package fleetdm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// Script represents a FleetDM script.
type Script struct {
	ID        int    `json:"id,omitempty"`
	TeamID    *int   `json:"team_id,omitempty"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// ListScriptsResponse represents the response from the list scripts endpoint.
type ListScriptsResponse struct {
	Scripts []*Script       `json:"scripts"`
	Meta    *PaginationMeta `json:"meta,omitempty"`
}

// GetScriptResponse represents the response from the get script endpoint.
type GetScriptResponse struct {
	*Script
}

// CreateScriptResponse represents the response from the create script endpoint.
type CreateScriptResponse struct {
	ScriptID int `json:"script_id"`
}

// UpdateScriptResponse represents the response from the update script endpoint.
type UpdateScriptResponse struct {
	ScriptID int `json:"script_id"`
}

// ListScripts retrieves all scripts, optionally filtered by team.
func (c *Client) ListScripts(ctx context.Context, teamID *int) ([]*Script, error) {
	params := make(map[string]string)
	if teamID != nil {
		params["team_id"] = strconv.Itoa(*teamID)
	}

	var resp ListScriptsResponse
	err := c.Get(ctx, "/scripts", params, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to list scripts: %w", err)
	}
	return resp.Scripts, nil
}

// GetScript retrieves a script by ID.
func (c *Client) GetScript(ctx context.Context, id int) (*Script, error) {
	var resp Script
	endpoint := fmt.Sprintf("/scripts/%d", id)
	err := c.Get(ctx, endpoint, nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to get script %d: %w", id, err)
	}
	return &resp, nil
}

// CreateScript creates a new script by uploading a file.
func (c *Client) CreateScript(ctx context.Context, teamID *int, name string, content []byte) (*Script, error) {
	fields := make(map[string]string)
	if teamID != nil {
		fields["team_id"] = strconv.Itoa(*teamID)
	}

	respBody, err := c.doMultipartRequest(ctx, http.MethodPost, "/scripts", "script", name, content, fields)
	if err != nil {
		return nil, fmt.Errorf("failed to create script: %w", err)
	}

	var createResp CreateScriptResponse
	if err := json.Unmarshal(respBody, &createResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return c.GetScript(ctx, createResp.ScriptID)
}

// UpdateScript updates an existing script's content.
func (c *Client) UpdateScript(ctx context.Context, id int, name string, content []byte) (*Script, error) {
	endpoint := fmt.Sprintf("/scripts/%d", id)

	respBody, err := c.doMultipartRequest(ctx, http.MethodPatch, endpoint, "script", name, content, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to update script %d: %w", id, err)
	}

	var updateResp UpdateScriptResponse
	if err := json.Unmarshal(respBody, &updateResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return c.GetScript(ctx, updateResp.ScriptID)
}

// DeleteScript deletes a script by ID.
func (c *Client) DeleteScript(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/scripts/%d", id)
	err := c.Delete(ctx, endpoint, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete script %d: %w", id, err)
	}
	return nil
}

