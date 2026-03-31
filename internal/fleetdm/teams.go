package fleetdm

import (
	"context"
	"fmt"
	"strconv"
)

// Team represents a FleetDM fleet (team).
type Team struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	UserCount   int    `json:"user_count,omitempty"`
	HostCount   int    `json:"host_count,omitempty"`

	// AgentOptions contains osquery agent configuration for this fleet
	AgentOptions *AgentOptions `json:"agent_options,omitempty"`

	// Secrets contains enrollment secrets for this fleet
	Secrets []EnrollSecret `json:"secrets,omitempty"`

	// WebhookSettings contains webhook configuration
	WebhookSettings *TeamWebhookSettings `json:"webhook_settings,omitempty"`

	// MDM contains MDM-specific settings
	MDM *TeamMDMSettings `json:"mdm,omitempty"`

	// HostExpirySettings contains host expiry configuration
	HostExpirySettings *HostExpirySettings `json:"host_expiry_settings,omitempty"`
}

// AgentOptions represents osquery agent configuration options.
type AgentOptions struct {
	Config           map[string]interface{} `json:"config,omitempty"`
	Overrides        map[string]interface{} `json:"overrides,omitempty"`
	CommandLineFlags map[string]interface{} `json:"command_line_flags,omitempty"`
}

// EnrollSecret represents an enrollment secret.
type EnrollSecret struct {
	Secret    string `json:"secret"`
	CreatedAt string `json:"created_at,omitempty"`
	TeamID    *int64 `json:"fleet_id,omitempty"`
}

// TeamWebhookSettings represents team-level webhook settings.
type TeamWebhookSettings struct {
	FailingPoliciesWebhook *FailingPoliciesWebhook `json:"failing_policies_webhook,omitempty"`
}

// FailingPoliciesWebhook represents failing policies webhook configuration.
type FailingPoliciesWebhook struct {
	Enable         bool   `json:"enable_failing_policies_webhook"`
	DestinationURL string `json:"destination_url,omitempty"`
	PolicyIDs      []int  `json:"policy_ids,omitempty"`
	HostBatchSize  int    `json:"host_batch_size,omitempty"`
}

// TeamMDMSettings represents MDM settings for a fleet.
type TeamMDMSettings struct {
	EnableDiskEncryption    bool                `json:"enable_disk_encryption"`
	WindowsRequireBitlocker bool                `json:"windows_require_bitlocker_pin"`
	MacOSUpdates            *MacOSUpdates       `json:"macos_updates,omitempty"`
	WindowsUpdates          *WindowsUpdates     `json:"windows_updates,omitempty"`
	MacOSSettings           *MacOSMDMSettings   `json:"macos_settings,omitempty"`
	WindowsSettings         *WindowsMDMSettings `json:"windows_settings,omitempty"`
	MacOSSetup              *MacOSSetup         `json:"macos_setup,omitempty"`
	IOSUpdates              *IOSUpdates         `json:"ios_updates,omitempty"`
	IPadOSUpdates           *IPadOSUpdates      `json:"ipados_updates,omitempty"`
}

// MacOSUpdates represents macOS update settings.
type MacOSUpdates struct {
	MinimumVersion string `json:"minimum_version,omitempty"`
	Deadline       string `json:"deadline,omitempty"`
	UpdateNewHosts bool   `json:"update_new_hosts"`
}

// WindowsUpdates represents Windows update settings.
type WindowsUpdates struct {
	DeadlineDays    int `json:"deadline_days,omitempty"`
	GracePeriodDays int `json:"grace_period_days,omitempty"`
}

// IOSUpdates represents iOS update settings.
type IOSUpdates struct {
	MinimumVersion string `json:"minimum_version,omitempty"`
	Deadline       string `json:"deadline,omitempty"`
}

// IPadOSUpdates represents iPadOS update settings.
type IPadOSUpdates struct {
	MinimumVersion string `json:"minimum_version,omitempty"`
	Deadline       string `json:"deadline,omitempty"`
}

// MacOSMDMSettings represents macOS MDM settings.
type MacOSMDMSettings struct {
	CustomSettings []CustomSetting `json:"custom_settings,omitempty"`
}

// WindowsMDMSettings represents Windows MDM settings.
type WindowsMDMSettings struct {
	CustomSettings []CustomSetting `json:"custom_settings,omitempty"`
}

// CustomSetting represents a custom configuration profile setting.
type CustomSetting struct {
	Path             string   `json:"path,omitempty"`
	Labels           []string `json:"labels,omitempty"`
	LabelsIncludeAll []string `json:"labels_include_all,omitempty"`
	LabelsIncludeAny []string `json:"labels_include_any,omitempty"`
	LabelsExcludeAny []string `json:"labels_exclude_any,omitempty"`
}

// MacOSSetup represents macOS setup experience settings.
type MacOSSetup struct {
	BootstrapPackage            string `json:"bootstrap_package,omitempty"`
	EnableEndUserAuthentication bool   `json:"enable_end_user_authentication"`
	MacOSSetupAssistant         string `json:"macos_setup_assistant,omitempty"`
}

// HostExpirySettings represents host expiry configuration.
type HostExpirySettings struct {
	HostExpiryEnabled bool `json:"host_expiry_enabled"`
	HostExpiryWindow  int  `json:"host_expiry_window"`
}

// ListTeamsResponse represents the response from listing fleets.
type ListTeamsResponse struct {
	Teams []Team `json:"fleets"`
}

// GetTeamResponse represents the response from getting a fleet.
type GetTeamResponse struct {
	Team Team `json:"fleet"`
}

// CreateTeamRequest represents the request to create a fleet.
type CreateTeamRequest struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	Secrets      []EnrollSecret `json:"secrets,omitempty"`
	AgentOptions *AgentOptions  `json:"agent_options,omitempty"`
}

// UpdateTeamRequest represents the request to update a fleet.
type UpdateTeamRequest struct {
	Name               string               `json:"name"`
	Description        string               `json:"description"`
	AgentOptions       *AgentOptions        `json:"agent_options,omitempty"`
	WebhookSettings    *TeamWebhookSettings `json:"webhook_settings,omitempty"`
	MDM                *TeamMDMSettings     `json:"mdm,omitempty"`
	HostExpirySettings *HostExpirySettings  `json:"host_expiry_settings,omitempty"`
}

// ListTeams retrieves all fleets.
func (c *Client) ListTeams(ctx context.Context, page, perPage int) ([]Team, error) {
	params := make(map[string]string)
	if page > 0 {
		params["page"] = strconv.Itoa(page)
	}
	if perPage > 0 {
		params["per_page"] = strconv.Itoa(perPage)
	}

	var response ListTeamsResponse
	err := c.Get(ctx, "/fleets", params, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to list fleets: %w", err)
	}

	return response.Teams, nil
}

// GetTeam retrieves a fleet by ID.
func (c *Client) GetTeam(ctx context.Context, teamID int64) (*Team, error) {
	var response GetTeamResponse
	err := c.Get(ctx, fmt.Sprintf("/fleets/%d", teamID), nil, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get fleet %d: %w", teamID, err)
	}

	return &response.Team, nil
}

// CreateTeam creates a new fleet.
func (c *Client) CreateTeam(ctx context.Context, req CreateTeamRequest) (*Team, error) {
	var response GetTeamResponse
	err := c.Post(ctx, "/fleets", req, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to create fleet: %w", err)
	}

	return &response.Team, nil
}

// UpdateTeam updates an existing fleet.
func (c *Client) UpdateTeam(ctx context.Context, teamID int64, req UpdateTeamRequest) (*Team, error) {
	var response GetTeamResponse
	err := c.Patch(ctx, fmt.Sprintf("/fleets/%d", teamID), req, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to update fleet %d: %w", teamID, err)
	}

	return &response.Team, nil
}

// DeleteTeam deletes a fleet by ID.
func (c *Client) DeleteTeam(ctx context.Context, teamID int64) error {
	err := c.Delete(ctx, fmt.Sprintf("/fleets/%d", teamID), nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete fleet %d: %w", teamID, err)
	}

	return nil
}

// GetTeamEnrollSecrets retrieves the enrollment secrets for a fleet.
func (c *Client) GetTeamEnrollSecrets(ctx context.Context, teamID int64) ([]EnrollSecret, error) {
	var response struct {
		Secrets []EnrollSecret `json:"secrets"`
	}
	err := c.Get(ctx, fmt.Sprintf("/fleets/%d/secrets", teamID), nil, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get fleet %d secrets: %w", teamID, err)
	}

	return response.Secrets, nil
}

// ModifyTeamEnrollSecrets modifies the enrollment secrets for a fleet.
func (c *Client) ModifyTeamEnrollSecrets(ctx context.Context, teamID int64, secrets []EnrollSecret) ([]EnrollSecret, error) {
	req := struct {
		Secrets []EnrollSecret `json:"secrets"`
	}{
		Secrets: secrets,
	}

	var response struct {
		Secrets []EnrollSecret `json:"secrets"`
	}
	err := c.Patch(ctx, fmt.Sprintf("/fleets/%d/secrets", teamID), req, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to modify fleet %d secrets: %w", teamID, err)
	}

	return response.Secrets, nil
}
