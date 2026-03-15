package fleetdm

import (
	"context"
	"fmt"
)

// Activity represents an activity entry from FleetDM.
type Activity struct {
	ID             int            `json:"id"`
	CreatedAt      string         `json:"created_at"`
	ActorFullName  string         `json:"actor_full_name"`
	ActorID        *int           `json:"actor_id,omitempty"`
	ActorGravatar  string         `json:"actor_gravatar"`
	ActorEmail     string         `json:"actor_email"`
	Type           string         `json:"type"`
	FleetInitiated bool           `json:"fleet_initiated"`
	Details        map[string]any `json:"details,omitempty"`
}

// ListActivitiesOptions contains options for listing activities.
type ListActivitiesOptions struct {
	Page           int
	PerPage        int
	OrderKey       string
	OrderDirection string
	Query          string
	ActivityType   string
	StartCreatedAt string
	EndCreatedAt   string
}

// listActivitiesResponse represents the response from the list activities endpoint.
type listActivitiesResponse struct {
	Activities []Activity      `json:"activities"`
	Meta       *PaginationMeta `json:"meta,omitempty"`
}

// ListActivities retrieves activities from FleetDM.
func (c *Client) ListActivities(ctx context.Context, opts *ListActivitiesOptions) ([]Activity, error) {
	params := make(map[string]string)

	if opts != nil {
		if opts.Page > 0 {
			params["page"] = fmt.Sprintf("%d", opts.Page)
		}
		if opts.PerPage > 0 {
			params["per_page"] = fmt.Sprintf("%d", opts.PerPage)
		}
		if opts.OrderKey != "" {
			params["order_key"] = opts.OrderKey
		}
		if opts.OrderDirection != "" {
			params["order_direction"] = opts.OrderDirection
		}
		if opts.Query != "" {
			params["query"] = opts.Query
		}
		if opts.ActivityType != "" {
			params["activity_type"] = opts.ActivityType
		}
		if opts.StartCreatedAt != "" {
			params["start_created_at"] = opts.StartCreatedAt
		}
		if opts.EndCreatedAt != "" {
			params["end_created_at"] = opts.EndCreatedAt
		}
	}

	var response listActivitiesResponse
	if err := c.Get(ctx, "/activities", params, &response); err != nil {
		return nil, fmt.Errorf("failed to list activities: %w", err)
	}

	return response.Activities, nil
}
