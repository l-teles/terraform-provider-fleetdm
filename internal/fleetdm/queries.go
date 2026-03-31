package fleetdm

import (
	"context"
	"fmt"
	"strconv"
)

// Query represents a FleetDM report (query).
type Query struct {
	ID                 int    `json:"id,omitempty"`
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	Query              string `json:"query"`
	Platform           string `json:"platform,omitempty"`
	MinOsqueryVersion  string `json:"min_osquery_version,omitempty"`
	Interval           int    `json:"interval,omitempty"`
	ObserverCanRun     bool   `json:"observer_can_run,omitempty"`
	AutomationsEnabled bool   `json:"automations_enabled,omitempty"`
	Logging            string `json:"logging,omitempty"`
	DiscardData        bool   `json:"discard_data,omitempty"`
	TeamID             *int   `json:"fleet_id,omitempty"`
	AuthorID           int    `json:"author_id,omitempty"`
	AuthorName         string `json:"author_name,omitempty"`
	AuthorEmail        string `json:"author_email,omitempty"`
	Saved              bool   `json:"saved,omitempty"`
	Packs              []Pack `json:"packs,omitempty"`
	Stats              *Stats `json:"stats,omitempty"`
	CreatedAt          string `json:"created_at,omitempty"`
	UpdatedAt          string `json:"updated_at,omitempty"`
}

// Pack represents a query pack.
type Pack struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Stats represents query statistics.
type Stats struct {
	UserTimeP50     float64 `json:"user_time_p50,omitempty"`
	UserTimeP95     float64 `json:"user_time_p95,omitempty"`
	SystemTimeP50   float64 `json:"system_time_p50,omitempty"`
	SystemTimeP95   float64 `json:"system_time_p95,omitempty"`
	TotalExecutions int     `json:"total_executions,omitempty"`
}

// ListQueriesResponse represents the response from the list reports endpoint.
type ListQueriesResponse struct {
	Queries []Query `json:"reports"`
}

// GetQueryResponse represents the response from the get report endpoint.
type GetQueryResponse struct {
	Query Query `json:"report"`
}

// CreateQueryRequest represents the request to create a report.
type CreateQueryRequest struct {
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	Query              string `json:"query"`
	Platform           string `json:"platform,omitempty"`
	MinOsqueryVersion  string `json:"min_osquery_version,omitempty"`
	Interval           int    `json:"interval,omitempty"`
	ObserverCanRun     bool   `json:"observer_can_run,omitempty"`
	AutomationsEnabled bool   `json:"automations_enabled,omitempty"`
	Logging            string `json:"logging,omitempty"`
	DiscardData        bool   `json:"discard_data,omitempty"`
	TeamID             *int   `json:"fleet_id,omitempty"`
}

// CreateQueryResponse represents the response from the create report endpoint.
type CreateQueryResponse struct {
	Query Query `json:"report"`
}

// UpdateQueryRequest represents the request to update a report.
type UpdateQueryRequest struct {
	Name               string `json:"name,omitempty"`
	Description        string `json:"description,omitempty"`
	Query              string `json:"query,omitempty"`
	Platform           string `json:"platform,omitempty"`
	MinOsqueryVersion  string `json:"min_osquery_version,omitempty"`
	Interval           int    `json:"interval,omitempty"`
	ObserverCanRun     bool   `json:"observer_can_run,omitempty"`
	AutomationsEnabled bool   `json:"automations_enabled,omitempty"`
	Logging            string `json:"logging,omitempty"`
	DiscardData        bool   `json:"discard_data,omitempty"`
}

// UpdateQueryResponse represents the response from the update report endpoint.
type UpdateQueryResponse struct {
	Query Query `json:"report"`
}

// ListQueries retrieves all reports.
func (c *Client) ListQueries(ctx context.Context) ([]Query, error) {
	var resp ListQueriesResponse
	err := c.Get(ctx, "/reports", nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to list reports: %w", err)
	}
	return resp.Queries, nil
}

// ListQueriesByTeam retrieves reports for a specific fleet.
func (c *Client) ListQueriesByTeam(ctx context.Context, teamID int) ([]Query, error) {
	var resp ListQueriesResponse
	params := map[string]string{
		"fleet_id": strconv.Itoa(teamID),
	}
	err := c.Get(ctx, "/reports", params, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to list reports for fleet %d: %w", teamID, err)
	}
	return resp.Queries, nil
}

// GetQuery retrieves a report by ID.
func (c *Client) GetQuery(ctx context.Context, id int) (*Query, error) {
	var resp GetQueryResponse
	endpoint := fmt.Sprintf("/reports/%d", id)
	err := c.Get(ctx, endpoint, nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to get report %d: %w", id, err)
	}
	return &resp.Query, nil
}

// CreateQuery creates a new report.
func (c *Client) CreateQuery(ctx context.Context, req CreateQueryRequest) (*Query, error) {
	var resp CreateQueryResponse
	err := c.Post(ctx, "/reports", req, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to create report: %w", err)
	}
	return &resp.Query, nil
}

// UpdateQuery updates an existing report.
func (c *Client) UpdateQuery(ctx context.Context, id int, req UpdateQueryRequest) (*Query, error) {
	var resp UpdateQueryResponse
	endpoint := fmt.Sprintf("/reports/%d", id)
	err := c.Patch(ctx, endpoint, req, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to update report %d: %w", id, err)
	}
	return &resp.Query, nil
}

// DeleteQuery deletes a report by ID.
func (c *Client) DeleteQuery(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/reports/id/%d", id)
	err := c.Delete(ctx, endpoint, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete report %d: %w", id, err)
	}
	return nil
}
