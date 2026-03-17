package fleetdm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// GetScriptContent retrieves the content of a script by ID using the alt=media query parameter.
func (c *Client) GetScriptContent(ctx context.Context, id int64) (string, error) {
	endpoint := fmt.Sprintf("/scripts/%d?alt=media", id)
	reqURL := c.BaseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("failed to get script content %d: HTTP %d: %s", id, resp.StatusCode, string(body))
	}

	return string(body), nil
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

