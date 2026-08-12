package fleetdm

import (
	"context"
	"fmt"
)

// Host vitals recognized by a label's Criteria. Fleet 4.90 registers exactly
// these three; anything else is rejected with "unknown vital <x>".
const (
	// HostVitalEndUserIDPGroup matches on the end user's IdP group.
	HostVitalEndUserIDPGroup = "end_user_idp_group"
	// HostVitalEndUserIDPDepartment matches on the end user's IdP department.
	HostVitalEndUserIDPDepartment = "end_user_idp_department"
	// HostVitalCustomHostVital matches on a custom host vital's per-host value.
	// This vital does not self-identify which vital to read, so a criterion
	// using it must also carry CustomHostVitalID.
	HostVitalCustomHostVital = "custom_host_vital"
)

// Comparison operators accepted by HostVitalCriteria.Operator.
const (
	HostVitalOperatorEqual    = "="
	HostVitalOperatorNotEqual = "!="
	HostVitalOperatorGreater  = ">"
	HostVitalOperatorLess     = "<"
	HostVitalOperatorLike     = "LIKE"
)

// HostVitalCriteria defines membership for a host-vitals label: hosts join the
// label when the named vital compares true against Value.
//
// Fleet's wire format also has `and`/`or` arrays for composing criteria, but
// 4.90 rejects them at evaluation time ("And/Or criteria not supported in host
// vitals labels yet"), so only a single condition is modeled here.
type HostVitalCriteria struct {
	Vital    string `json:"vital"`
	Value    string `json:"value"`
	Operator string `json:"operator,omitempty"`
	// CustomHostVitalID selects which custom host vital to match. Required
	// when Vital is HostVitalCustomHostVital, and rejected otherwise.
	CustomHostVitalID *int `json:"custom_host_vital_id,omitempty"`
}

// Label represents a FleetDM label.
type Label struct {
	ID          int    `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Query       string `json:"query"`
	// Criteria is set for host-vitals labels and absent for dynamic
	// (query-based) and manual labels.
	Criteria  *HostVitalCriteria `json:"criteria,omitempty"`
	Platform  string             `json:"platform,omitempty"`
	LabelType string             `json:"label_type,omitempty"`
	// LabelMembershipType is how Fleet resolves membership: "manual",
	// "dynamic" (query-driven) or "host_vitals" (criteria-driven). Every label
	// route reports it, including the list route.
	LabelMembershipType string `json:"label_membership_type,omitempty"`
	// HostCount is the number of hosts in the label. Fleet's wire key is
	// `count`, not `host_count` — POST /labels, GET /labels/{id} and
	// GET /labels all emit `count` and none of them emits `host_count`
	// (verified against Fleet 4.90.0). The tag used to read `host_count`, so
	// this field silently decoded to 0 on every response; the client's own
	// tests missed it because they encode the fixture through this same
	// struct, which makes any tag round-trip symmetrically.
	HostCount   int    `json:"count,omitempty"`
	DisplayText string `json:"display_text,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
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
//
// Fleet accepts at most one of Query, Criteria or hosts/host_ids: a label is
// dynamic, host-vitals, or manual (all three empty). An empty Query string
// alongside Criteria is fine — Fleet only counts a non-empty query.
type CreateLabelRequest struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Query       string             `json:"query"`
	Criteria    *HostVitalCriteria `json:"criteria,omitempty"`
	Platform    string             `json:"platform,omitempty"`
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
//
// Note: Only name and description can be updated. Query, platform and criteria
// are immutable — Fleet's modify-label payload has no field for them, so a
// PATCH carrying `criteria` answers 200 with the *original* criteria still in
// place rather than reporting an error.
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
