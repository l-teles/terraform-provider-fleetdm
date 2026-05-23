package fleetdm

import (
	"context"
	"fmt"
	"strconv"
)

// maxPolicyListPages caps the pagination loop in
// ListPoliciesByInstallSoftwareTitleID as a defense-in-depth measure against a
// Fleet API regression that fails to flip has_next_results to false. 1000
// pages × policyListPerPage per page is well above any realistic team size.
const maxPolicyListPages = 1000

// policyListPerPage is the per_page hint used by ListPoliciesByInstallSoftwareTitleID
// when paginating /global/policies and /fleets/{teamID}/policies. Chosen large
// enough that most fleets fit in a single request, but bounded so a misbehaving
// server can't deliver an unbounded response in one shot.
const policyListPerPage = 100

// PolicyAutomationSoftware echoes the install_software automation attached to a policy.
type PolicyAutomationSoftware struct {
	Name            string `json:"name,omitempty"`
	SoftwareTitleID int    `json:"software_title_id"`
}

// PolicyAutomationPatchSoftware echoes the patch_software target of a patch policy.
type PolicyAutomationPatchSoftware struct {
	Name            string `json:"name,omitempty"`
	DisplayName     string `json:"display_name,omitempty"`
	SoftwareTitleID int    `json:"software_title_id"`
}

// PolicyAutomationScript echoes the run_script automation attached to a policy.
type PolicyAutomationScript struct {
	Name string `json:"name,omitempty"`
	ID   int    `json:"id"`
}

// PolicyLabel is the per-label echo Fleet returns inside labels_include_any
// and labels_exclude_any on policy responses. Note the request side uses
// `[]string` of label names — the API is asymmetric here, so this struct
// is response-only.
type PolicyLabel struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Policy represents a FleetDM policy.
type Policy struct {
	ID                       int                            `json:"id,omitempty"`
	Name                     string                         `json:"name"`
	Description              string                         `json:"description,omitempty"`
	Query                    string                         `json:"query"`
	Critical                 bool                           `json:"critical"`
	Resolution               string                         `json:"resolution,omitempty"`
	Platform                 string                         `json:"platform,omitempty"`
	TeamID                   *int                           `json:"team_id,omitempty"`
	AuthorID                 int                            `json:"author_id,omitempty"`
	AuthorName               string                         `json:"author_name,omitempty"`
	AuthorEmail              string                         `json:"author_email,omitempty"`
	PassingHostCount         int                            `json:"passing_host_count,omitempty"`
	FailingHostCount         int                            `json:"failing_host_count,omitempty"`
	Type                     string                         `json:"type,omitempty"`
	LabelsIncludeAny         []PolicyLabel                  `json:"labels_include_any,omitempty"`
	LabelsExcludeAny         []PolicyLabel                  `json:"labels_exclude_any,omitempty"`
	CalendarEventsEnabled    bool                           `json:"calendar_events_enabled"`
	ConditionalAccessEnabled bool                           `json:"conditional_access_enabled"`
	FleetMaintained          bool                           `json:"fleet_maintained"`
	HostCountUpdatedAt       *string                        `json:"host_count_updated_at"`
	CreatedAt                string                         `json:"created_at,omitempty"`
	UpdatedAt                string                         `json:"updated_at,omitempty"`
	InstallSoftware          *PolicyAutomationSoftware      `json:"install_software,omitempty"`
	RunScript                *PolicyAutomationScript        `json:"run_script,omitempty"`
	PatchSoftware            *PolicyAutomationPatchSoftware `json:"patch_software,omitempty"`
}

// ListPoliciesResponse represents the response from the list policies endpoint.
type ListPoliciesResponse struct {
	Policies []Policy        `json:"policies"`
	Meta     *PaginationMeta `json:"meta,omitempty"`
}

// GetPolicyResponse represents the response from the get policy endpoint.
type GetPolicyResponse struct {
	Policy Policy `json:"policy"`
}

// CreatePolicyRequest represents the request to create a policy.
//
// Query uses omitempty so it can be left unset for patch policies
// (Fleet rejects `query` together with `type=patch`).
type CreatePolicyRequest struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description,omitempty"`
	Query                string   `json:"query,omitempty"`
	Critical             bool     `json:"critical"`
	Resolution           string   `json:"resolution,omitempty"`
	Platform             string   `json:"platform,omitempty"`
	Type                 string   `json:"type,omitempty"`
	PatchSoftwareTitleID *int     `json:"patch_software_title_id,omitempty"`
	SoftwareTitleID      *int     `json:"software_title_id,omitempty"`
	ScriptID             *int     `json:"script_id,omitempty"`
	LabelsIncludeAny     []string `json:"labels_include_any,omitempty"`
	LabelsExcludeAny     []string `json:"labels_exclude_any,omitempty"`
}

// CreatePolicyResponse represents the response from the create policy endpoint.
type CreatePolicyResponse struct {
	Policy Policy `json:"policy"`
}

// UpdatePolicyRequest represents the request to update a policy.
//
// Fields here intentionally drop `omitempty` and use pointers (or, for
// labels, slices) so the wire format can faithfully express the user's
// intent. Two distinct conventions apply per Fleet's API:
//
//   - Pointer fields (script_id, software_title_id, calendar_events_enabled,
//     conditional_access_enabled, conditional_access_bypass_enabled): a Go
//     `nil` serializes as JSON `null`, which Fleet treats as "clear /
//     reset to default". A non-nil pointer is sent as the value.
//
//   - Label slice fields (labels_include_any, labels_exclude_any): a `nil`
//     slice serializes as JSON `null`, which Fleet treats as "no change"
//     (preserve the existing labels). An empty slice (`[]string{}`)
//     serializes as JSON `[]`, which Fleet treats as "clear all labels".
//     Use the empty slice to clear; never use nil if the user has asked
//     for labels to be removed.
//
// `omitempty` would suppress null/empty values entirely, breaking both
// conventions.
type UpdatePolicyRequest struct {
	Name                           string   `json:"name,omitempty"`
	Description                    string   `json:"description,omitempty"`
	Query                          string   `json:"query,omitempty"`
	Critical                       bool     `json:"critical"`
	Resolution                     string   `json:"resolution,omitempty"`
	Platform                       string   `json:"platform,omitempty"`
	SoftwareTitleID                *int     `json:"software_title_id"`
	ScriptID                       *int     `json:"script_id"`
	CalendarEventsEnabled          *bool    `json:"calendar_events_enabled"`
	ConditionalAccessEnabled       *bool    `json:"conditional_access_enabled"`
	ConditionalAccessBypassEnabled *bool    `json:"conditional_access_bypass_enabled"`
	LabelsIncludeAny               []string `json:"labels_include_any"`
	LabelsExcludeAny               []string `json:"labels_exclude_any"`
}

// UpdatePolicyResponse represents the response from the update policy endpoint.
type UpdatePolicyResponse struct {
	Policy Policy `json:"policy"`
}

// ListGlobalPolicies retrieves all global policies.
func (c *Client) ListGlobalPolicies(ctx context.Context) ([]Policy, error) {
	var resp ListPoliciesResponse
	err := c.Get(ctx, "/global/policies", nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to list global policies: %w", err)
	}
	return resp.Policies, nil
}

// ListTeamPolicies retrieves all policies for a specific team.
func (c *Client) ListTeamPolicies(ctx context.Context, teamID int) ([]Policy, error) {
	var resp ListPoliciesResponse
	endpoint := fmt.Sprintf("/fleets/%d/policies", teamID)
	err := c.Get(ctx, endpoint, nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to list fleet %d policies: %w", teamID, err)
	}
	return resp.Policies, nil
}

// GetGlobalPolicy retrieves a global policy by ID.
func (c *Client) GetGlobalPolicy(ctx context.Context, id int) (*Policy, error) {
	var resp GetPolicyResponse
	endpoint := fmt.Sprintf("/global/policies/%d", id)
	err := c.Get(ctx, endpoint, nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to get global policy %d: %w", id, err)
	}
	return &resp.Policy, nil
}

// GetTeamPolicy retrieves a team policy by ID.
func (c *Client) GetTeamPolicy(ctx context.Context, teamID, policyID int) (*Policy, error) {
	var resp GetPolicyResponse
	endpoint := fmt.Sprintf("/fleets/%d/policies/%d", teamID, policyID)
	err := c.Get(ctx, endpoint, nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to get fleet %d policy %d: %w", teamID, policyID, err)
	}
	return &resp.Policy, nil
}

// CreateGlobalPolicy creates a new global policy.
func (c *Client) CreateGlobalPolicy(ctx context.Context, req CreatePolicyRequest) (*Policy, error) {
	var resp CreatePolicyResponse
	err := c.Post(ctx, "/global/policies", req, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to create global policy: %w", err)
	}
	return &resp.Policy, nil
}

// CreateTeamPolicy creates a new policy for a specific team.
func (c *Client) CreateTeamPolicy(ctx context.Context, teamID int, req CreatePolicyRequest) (*Policy, error) {
	var resp CreatePolicyResponse
	endpoint := fmt.Sprintf("/fleets/%d/policies", teamID)
	err := c.Post(ctx, endpoint, req, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to create fleet %d policy: %w", teamID, err)
	}
	return &resp.Policy, nil
}

// UpdateGlobalPolicy updates an existing global policy.
func (c *Client) UpdateGlobalPolicy(ctx context.Context, id int, req UpdatePolicyRequest) (*Policy, error) {
	var resp UpdatePolicyResponse
	endpoint := fmt.Sprintf("/global/policies/%d", id)
	err := c.Patch(ctx, endpoint, req, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to update global policy %d: %w", id, err)
	}
	return &resp.Policy, nil
}

// UpdateTeamPolicy updates an existing team policy.
func (c *Client) UpdateTeamPolicy(ctx context.Context, teamID, policyID int, req UpdatePolicyRequest) (*Policy, error) {
	var resp UpdatePolicyResponse
	endpoint := fmt.Sprintf("/fleets/%d/policies/%d", teamID, policyID)
	err := c.Patch(ctx, endpoint, req, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to update fleet %d policy %d: %w", teamID, policyID, err)
	}
	return &resp.Policy, nil
}

// DeleteGlobalPolicy deletes a global policy by ID.
// FleetDM requires using the POST /global/policies/delete endpoint with IDs in body.
func (c *Client) DeleteGlobalPolicy(ctx context.Context, id int) error {
	_, err := c.DeleteGlobalPolicies(ctx, []int{id})
	if err != nil {
		return fmt.Errorf("failed to delete global policy %d: %w", id, err)
	}
	return nil
}

// DeleteTeamPolicy deletes a team policy by ID.
// FleetDM requires using the POST /fleets/{id}/policies/delete endpoint with IDs in body.
func (c *Client) DeleteTeamPolicy(ctx context.Context, teamID, policyID int) error {
	_, err := c.DeleteTeamPolicies(ctx, teamID, []int{policyID})
	if err != nil {
		return fmt.Errorf("failed to delete fleet %d policy %d: %w", teamID, policyID, err)
	}
	return nil
}

// DeleteGlobalPolicies deletes multiple global policies by ID.
func (c *Client) DeleteGlobalPolicies(ctx context.Context, ids []int) (int, error) {
	var resp struct {
		Deleted []int `json:"deleted"`
	}

	body := struct {
		IDs []int `json:"ids"`
	}{
		IDs: ids,
	}

	err := c.Post(ctx, "/global/policies/delete", body, &resp)
	if err != nil {
		return 0, fmt.Errorf("failed to delete global policies: %w", err)
	}
	return len(resp.Deleted), nil
}

// DeleteTeamPolicies deletes multiple team policies by ID.
func (c *Client) DeleteTeamPolicies(ctx context.Context, teamID int, ids []int) (int, error) {
	var resp struct {
		Deleted []int `json:"deleted"`
	}

	body := struct {
		IDs []int `json:"ids"`
	}{
		IDs: ids,
	}

	endpoint := fmt.Sprintf("/fleets/%d/policies/delete", teamID)
	err := c.Post(ctx, endpoint, body, &resp)
	if err != nil {
		return 0, fmt.Errorf("failed to delete fleet %d policies: %w", teamID, err)
	}
	return len(resp.Deleted), nil
}

// isTeamScoped returns true if teamID is non-nil and positive.
func isTeamScoped(teamID *int) bool {
	return teamID != nil && *teamID > 0
}

// GetPolicy retrieves a policy by ID, determining if it's global or team-scoped.
func (c *Client) GetPolicy(ctx context.Context, id int, teamID *int) (*Policy, error) {
	if isTeamScoped(teamID) {
		return c.GetTeamPolicy(ctx, *teamID, id)
	}
	return c.GetGlobalPolicy(ctx, id)
}

// CreatePolicy creates a policy, either global or team-scoped.
func (c *Client) CreatePolicy(ctx context.Context, teamID *int, req CreatePolicyRequest) (*Policy, error) {
	if isTeamScoped(teamID) {
		return c.CreateTeamPolicy(ctx, *teamID, req)
	}
	return c.CreateGlobalPolicy(ctx, req)
}

// UpdatePolicy updates a policy, either global or team-scoped.
func (c *Client) UpdatePolicy(ctx context.Context, id int, teamID *int, req UpdatePolicyRequest) (*Policy, error) {
	if isTeamScoped(teamID) {
		return c.UpdateTeamPolicy(ctx, *teamID, id, req)
	}
	return c.UpdateGlobalPolicy(ctx, id, req)
}

// DeletePolicy deletes a policy, either global or team-scoped.
func (c *Client) DeletePolicy(ctx context.Context, id int, teamID *int) error {
	if isTeamScoped(teamID) {
		return c.DeleteTeamPolicy(ctx, *teamID, id)
	}
	return c.DeleteGlobalPolicy(ctx, id)
}

// ListPolicies retrieves all policies (global or for a specific team).
func (c *Client) ListPolicies(ctx context.Context, teamID *int) ([]Policy, error) {
	if isTeamScoped(teamID) {
		return c.ListTeamPolicies(ctx, *teamID)
	}
	return c.ListGlobalPolicies(ctx)
}

// ListPoliciesByInstallSoftwareTitleID returns policies in the given scope
// whose install_software automation references the given software title ID.
// Fleet does not expose a server-side filter, so the implementation paginates
// through all policies in the scope and filters client-side.
//
// Scope follows teamID: nil (or zero-pointer) selects global policies;
// non-zero pointer selects the named team. Install_software policies can only
// reference titles in the same scope, so callers should pass the same teamID
// as the title being looked up.
//
// Pagination matters because the underlying /global/policies and
// /fleets/{teamID}/policies endpoints default to per_page=20: without paging,
// any team with more than 20 policies would silently miss matches and the
// caller would then hit Fleet's "Policy automation uses this software" 409.
func (c *Client) ListPoliciesByInstallSoftwareTitleID(ctx context.Context, titleID int, teamID *int) ([]Policy, error) {
	endpoint := "/global/policies"
	if isTeamScoped(teamID) {
		endpoint = fmt.Sprintf("/fleets/%d/policies", *teamID)
	}

	var matches []Policy
	for page := range maxPolicyListPages {
		params := map[string]string{
			"per_page": strconv.Itoa(policyListPerPage),
		}
		if page > 0 {
			params["page"] = strconv.Itoa(page)
		}

		var resp ListPoliciesResponse
		if err := c.Get(ctx, endpoint, params, &resp); err != nil {
			return nil, fmt.Errorf("failed to list policies (page %d): %w", page, err)
		}

		for _, p := range resp.Policies {
			if p.InstallSoftware != nil && p.InstallSoftware.SoftwareTitleID == titleID {
				matches = append(matches, p)
			}
		}

		if resp.Meta == nil || !resp.Meta.HasNextResults {
			return matches, nil
		}
	}
	return nil, fmt.Errorf("policy pagination exceeded %d pages — Fleet API may be returning has_next_results=true indefinitely", maxPolicyListPages)
}

// SetPolicyInstallSoftwareTitleID switches a policy's install_software
// automation to point at the given softwareTitleID. Pass softwareTitleID=nil
// to detach — Fleet treats null as "clear / reset to default" (see the
// UpdatePolicyRequest doc comment).
//
// The implementation does a GET-then-PATCH round-trip so every other field on
// the policy is preserved verbatim. conditional_access_bypass_enabled is not
// echoed in Fleet's GET response and is therefore not preserved across this
// call; that field is being removed from Fleet in favor of `critical`
// (fleetdm/fleet#40521).
func (c *Client) SetPolicyInstallSoftwareTitleID(ctx context.Context, policyID int, teamID *int, softwareTitleID *int) error {
	p, err := c.GetPolicy(ctx, policyID, teamID)
	if err != nil {
		return fmt.Errorf("failed to fetch policy %d before update: %w", policyID, err)
	}

	req := UpdatePolicyRequest{
		Name:                     p.Name,
		Description:              p.Description,
		Query:                    p.Query,
		Critical:                 p.Critical,
		Resolution:               p.Resolution,
		Platform:                 p.Platform,
		SoftwareTitleID:          softwareTitleID,
		CalendarEventsEnabled:    &p.CalendarEventsEnabled,
		ConditionalAccessEnabled: &p.ConditionalAccessEnabled,
		LabelsIncludeAny:         policyLabelsToStrings(p.LabelsIncludeAny),
		LabelsExcludeAny:         policyLabelsToStrings(p.LabelsExcludeAny),
	}
	if p.RunScript != nil {
		id := p.RunScript.ID
		req.ScriptID = &id
	}

	if _, err := c.UpdatePolicy(ctx, policyID, teamID, req); err != nil {
		return fmt.Errorf("failed to update policy %d install_software: %w", policyID, err)
	}
	return nil
}

// policyLabelsToStrings converts the response-side []PolicyLabel into the
// request-side []string of label names. Returns nil for nil/empty input — the
// request layer treats nil as "no change" (see UpdatePolicyRequest doc), which
// is the correct semantics when round-tripping a policy that has no labels.
func policyLabelsToStrings(in []PolicyLabel) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, l := range in {
		out[i] = l.Name
	}
	return out
}
