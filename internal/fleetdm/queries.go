package fleetdm

import (
	"context"
	"fmt"
	"strconv"
)

// Query represents a FleetDM report (query).
//
// Fleet's API is transitioning field names: the "report" response wrapper uses
// "report" (SQL) and "fleet_id", while the legacy "query" wrapper uses "query"
// and "team_id". We deserialize both and normalize via normalizeQuery(), which
// prefers the new names and falls back to the legacy ones.
type Query struct {
	ID                 int    `json:"id,omitempty"`
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	Report             string `json:"report,omitempty"`  // new name for the SQL field
	Query              string `json:"query,omitempty"`   // legacy name for the SQL field
	Platform           string `json:"platform,omitempty"`
	MinOsqueryVersion  string `json:"min_osquery_version,omitempty"`
	Interval           int    `json:"interval,omitempty"`
	ObserverCanRun     bool   `json:"observer_can_run,omitempty"`
	AutomationsEnabled bool   `json:"automations_enabled,omitempty"`
	Logging            string `json:"logging,omitempty"`
	DiscardData        bool   `json:"discard_data,omitempty"`
	FleetID            *int   `json:"fleet_id,omitempty"` // new name for the team field
	TeamID             *int   `json:"team_id,omitempty"`  // legacy name for the team field
	AuthorID           int    `json:"author_id,omitempty"`
	AuthorName         string `json:"author_name,omitempty"`
	AuthorEmail        string `json:"author_email,omitempty"`
	Saved              bool   `json:"saved,omitempty"`
	Packs              []Pack `json:"packs,omitempty"`
	Stats              *Stats `json:"stats,omitempty"`
	CreatedAt          string `json:"created_at,omitempty"`
	UpdatedAt          string `json:"updated_at,omitempty"`
}

// normalizeQuery consolidates the dual field names after deserialization.
// It prefers the new names ("report", "fleet_id") and falls back to the
// legacy ones ("query", "team_id") when the new ones are empty.
func normalizeQuery(q *Query) {
	// SQL field: prefer Report, fall back to Query.
	if q.Report != "" {
		q.Query = q.Report
	} else {
		q.Report = q.Query
	}

	// Team field: prefer FleetID, fall back to TeamID.
	if q.FleetID != nil {
		q.TeamID = q.FleetID
	} else {
		q.FleetID = q.TeamID
	}
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
// Fleet returns both "reports" and "queries" keys; we prefer "reports".
type ListQueriesResponse struct {
	Reports []Query `json:"reports"`
	Queries []Query `json:"queries"`
}

// resolveList returns the report list, preferring the new "reports" key
// and falling back to "queries".
func (r *ListQueriesResponse) resolveList() []Query {
	if len(r.Reports) > 0 {
		return r.Reports
	}
	return r.Queries
}

// GetQueryResponse represents the response from the get report endpoint.
// Fleet returns both "report" and "query" keys; we prefer "report".
type GetQueryResponse struct {
	Report Query `json:"report"`
	Query  Query `json:"query"`
}

// resolve returns the query, preferring the new "report" key
// and falling back to "query".
func (r *GetQueryResponse) resolve() Query {
	if r.Report.ID != 0 {
		return r.Report
	}
	return r.Query
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
	Report Query `json:"report"`
	Query  Query `json:"query"`
}

func (r *CreateQueryResponse) resolve() Query {
	if r.Report.ID != 0 {
		return r.Report
	}
	return r.Query
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
	Report Query `json:"report"`
	Query  Query `json:"query"`
}

func (r *UpdateQueryResponse) resolve() Query {
	if r.Report.ID != 0 {
		return r.Report
	}
	return r.Query
}

// ListQueries retrieves all reports.
func (c *Client) ListQueries(ctx context.Context) ([]Query, error) {
	var resp ListQueriesResponse
	err := c.Get(ctx, "/reports", nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to list reports: %w", err)
	}
	queries := resp.resolveList()
	for i := range queries {
		normalizeQuery(&queries[i])
	}
	return queries, nil
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
	queries := resp.resolveList()
	for i := range queries {
		normalizeQuery(&queries[i])
	}
	return queries, nil
}

// GetQuery retrieves a report by ID.
func (c *Client) GetQuery(ctx context.Context, id int) (*Query, error) {
	var resp GetQueryResponse
	endpoint := fmt.Sprintf("/reports/%d", id)
	err := c.Get(ctx, endpoint, nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to get report %d: %w", id, err)
	}
	q := resp.resolve()
	normalizeQuery(&q)
	return &q, nil
}

// CreateQuery creates a new report.
func (c *Client) CreateQuery(ctx context.Context, req CreateQueryRequest) (*Query, error) {
	var resp CreateQueryResponse
	err := c.Post(ctx, "/reports", req, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to create report: %w", err)
	}
	q := resp.resolve()
	normalizeQuery(&q)
	return &q, nil
}

// UpdateQuery updates an existing report.
func (c *Client) UpdateQuery(ctx context.Context, id int, req UpdateQueryRequest) (*Query, error) {
	var resp UpdateQueryResponse
	endpoint := fmt.Sprintf("/reports/%d", id)
	err := c.Patch(ctx, endpoint, req, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to update report %d: %w", id, err)
	}
	q := resp.resolve()
	normalizeQuery(&q)
	return &q, nil
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
