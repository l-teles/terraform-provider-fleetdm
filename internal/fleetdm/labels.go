package fleetdm

import (
	"context"
	"fmt"
)

// Label represents a FleetDM label.
type Label struct {
	ID                  int    `json:"id,omitempty"`
	Name                string `json:"name"`
	Description         string `json:"description"`
	Query               string `json:"query"`
	Platform            string `json:"platform,omitempty"`
	LabelType           string `json:"label_type,omitempty"`
	LabelMembershipType string `json:"label_membership_type,omitempty"`
	HostCount           int    `json:"host_count,omitempty"`
	DisplayText         string `json:"display_text,omitempty"`
	CreatedAt           string `json:"created_at,omitempty"`
	UpdatedAt           string `json:"updated_at,omitempty"`
}

// ListLabelsResponse represents the response from the list labels endpoint.
type ListLabelsResponse struct {
	Labels []Label `json:"labels"`
}

// GetLabelResponse represents the response from the get label endpoint.
type GetLabelResponse struct {
	Label Label `json:"label"`
}

// CreateLabelRequest represents the request to create a label.
type CreateLabelRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Query       string `json:"query"`
	Platform    string `json:"platform,omitempty"`
}

// CreateLabelResponse represents the response from the create label endpoint.
type CreateLabelResponse struct {
	Label Label `json:"label"`
}

// UpdateLabelRequest represents the request to update a label.
type UpdateLabelRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description"`
}

// UpdateLabelResponse represents the response from the update label endpoint.
type UpdateLabelResponse struct {
	Label Label `json:"label"`
}

// ListLabels retrieves all labels.
func (c *Client) ListLabels(ctx context.Context) ([]Label, error) {
	var resp ListLabelsResponse
	err := c.Get(ctx, "/labels", nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}
	return resp.Labels, nil
}

// GetLabel retrieves a label by ID.
func (c *Client) GetLabel(ctx context.Context, id int) (*Label, error) {
	var resp GetLabelResponse
	endpoint := fmt.Sprintf("/labels/%d", id)
	err := c.Get(ctx, endpoint, nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to get label %d: %w", id, err)
	}
	return &resp.Label, nil
}

// CreateLabel creates a new dynamic label.
// Dynamic labels are defined by a query and automatically include hosts that match.
func (c *Client) CreateLabel(ctx context.Context, req CreateLabelRequest) (*Label, error) {
	var resp CreateLabelResponse
	err := c.Post(ctx, "/labels", req, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to create label: %w", err)
	}
	return &resp.Label, nil
}

// UpdateLabel updates an existing label.
// Note: Only name and description can be updated. Query and platform are immutable.
func (c *Client) UpdateLabel(ctx context.Context, id int, req UpdateLabelRequest) (*Label, error) {
	var resp UpdateLabelResponse
	endpoint := fmt.Sprintf("/labels/%d", id)
	err := c.Patch(ctx, endpoint, req, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to update label %d: %w", id, err)
	}
	return &resp.Label, nil
}

// DeleteLabel deletes a label by ID.
func (c *Client) DeleteLabel(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/labels/id/%d", id)
	err := c.Delete(ctx, endpoint, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete label %d: %w", id, err)
	}
	return nil
}

