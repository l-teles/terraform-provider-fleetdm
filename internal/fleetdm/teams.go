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

	// Integrations contains third-party integration settings
	Integrations *TeamIntegrations `json:"integrations,omitempty"`

	// Features contains the fleet's feature settings
	Features *TeamFeatures `json:"features,omitempty"`

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
//
// Fleet's PATCH /fleets/{id} handler replaces this block wholesale
// (`team.Config.WebhookSettings = *payload.WebhookSettings`), so a sub-block
// left nil here is *cleared* server-side rather than left alone. The sub-blocks
// reuse the global webhook types, whose fields are deliberately value types:
// once a sub-block is sent at all, every one of its fields is authoritative.
type TeamWebhookSettings struct {
	HostStatusWebhook      *HostStatusWebhookSettings      `json:"host_status_webhook,omitempty"`
	FailingPoliciesWebhook *FailingPoliciesWebhookSettings `json:"failing_policies_webhook,omitempty"`
}

// TeamMDMSettings represents MDM settings for a fleet.
//
// Every writable scalar is a pointer so that nil means "omitted" rather than
// "set to the zero value". This matters because Fleet reads these keys through
// optjson: a key that is present is applied, so serialising an untouched field
// as `false` or `""` silently overwrites whatever an operator configured in the
// UI. Pointers make "leave alone" (nil) distinguishable from "clear"
// (a pointer to the zero value), which is also the only way to clear
// name_template or the OS update settings.
type TeamMDMSettings struct {
	EnableDiskEncryption       *bool   `json:"enable_disk_encryption,omitempty"`
	EnableRecoveryLockPassword *bool   `json:"enable_recovery_lock_password,omitempty"`
	WindowsRequireBitlockerPIN *bool   `json:"windows_require_bitlocker_pin,omitempty"`
	NameTemplate               *string `json:"name_template,omitempty"`

	MacOSUpdates   *AppleOSUpdates `json:"macos_updates,omitempty"`
	IOSUpdates     *AppleOSUpdates `json:"ios_updates,omitempty"`
	IPadOSUpdates  *AppleOSUpdates `json:"ipados_updates,omitempty"`
	WindowsUpdates *WindowsUpdates `json:"windows_updates,omitempty"`

	// The remaining blocks are read-only from this client's perspective:
	// Fleet's team PATCH payload does not carry macos_settings,
	// windows_settings or android_settings (configuration profiles are managed
	// by the fleetdm_configuration_profile resource instead).
	MacOSSettings   *MacOSMDMSettings   `json:"macos_settings,omitempty"`
	WindowsSettings *WindowsMDMSettings `json:"windows_settings,omitempty"`
	MacOSSetup      *MacOSSetup         `json:"macos_setup,omitempty"`
}

// AppleOSUpdates represents the OS update settings for an Apple platform. The
// shape is identical for macOS, iOS and iPadOS.
//
// Note that Fleet validates MinimumVersion against Apple's Software Lookup
// Service (https://gdmf.apple.com/v2/pmv) with an exact ProductVersion match,
// and only when the value changes in the request.
type AppleOSUpdates struct {
	MinimumVersion *string `json:"minimum_version,omitempty"`
	Deadline       *string `json:"deadline,omitempty"`
	UpdateNewHosts *bool   `json:"update_new_hosts,omitempty"`
}

// WindowsUpdates represents Windows update settings.
type WindowsUpdates struct {
	DeadlineDays    *int `json:"deadline_days,omitempty"`
	GracePeriodDays *int `json:"grace_period_days,omitempty"`
}

// TeamIntegrations represents team-level third-party integration settings.
//
// Fleet merges this block per sub-key, so omitting jira/zendesk leaves any
// globally-linked ticketing integrations untouched. They are not modelled here
// because Fleet rejects them unless an exactly matching integration already
// exists in the global config.
type TeamIntegrations struct {
	GoogleCalendar           *TeamGoogleCalendarIntegration `json:"google_calendar,omitempty"`
	ConditionalAccessEnabled *bool                          `json:"conditional_access_enabled,omitempty"`
}

// TeamGoogleCalendarIntegration represents the team's Google Calendar
// integration settings. Enabling it requires a Google Calendar integration in
// the global config.
type TeamGoogleCalendarIntegration struct {
	EnableCalendarEvents bool   `json:"enable_calendar_events"`
	WebhookURL           string `json:"webhook_url"`
}

// TeamFeatures represents a fleet's feature settings.
//
// Only HistoricalData is writable via PATCH /fleets/{id}; EnableHostUsers and
// EnableSoftwareInventory are returned by the API but can only be set per-fleet
// through the GitOps /spec/fleets path, so they carry omitempty and are never
// populated on a request.
type TeamFeatures struct {
	EnableHostUsers         *bool                   `json:"enable_host_users,omitempty"`
	EnableSoftwareInventory *bool                   `json:"enable_software_inventory,omitempty"`
	HistoricalData          *HistoricalDataSettings `json:"historical_data,omitempty"`
}

// HistoricalDataSettings represents which historical datasets are collected.
// Each sub-key is merged independently, so a nil field retains its stored value.
type HistoricalDataSettings struct {
	Uptime          *bool `json:"uptime,omitempty"`
	Vulnerabilities *bool `json:"vulnerabilities,omitempty"`
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
	Integrations       *TeamIntegrations    `json:"integrations,omitempty"`
	Features           *TeamFeatures        `json:"features,omitempty"`
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
