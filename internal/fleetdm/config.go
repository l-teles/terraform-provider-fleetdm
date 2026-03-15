package fleetdm

import (
	"context"
	"encoding/json"
	"fmt"
)

// OrgInfo contains organization information
type OrgInfo struct {
	OrgName                   string `json:"org_name"`
	OrgLogoURL                string `json:"org_logo_url"`
	OrgLogoURLLightBackground string `json:"org_logo_url_light_background"`
	ContactURL                string `json:"contact_url"`
}

// ServerSettings contains server configuration
type ServerSettings struct {
	ServerURL            string `json:"server_url"`
	LiveQueryDisabled    bool   `json:"live_query_disabled"`
	EnableAnalytics      bool   `json:"enable_analytics"`
	QueryReportsDisabled bool   `json:"query_reports_disabled"`
	ScriptsDisabled      bool   `json:"scripts_disabled"`
	AIFeaturesDisabled   bool   `json:"ai_features_disabled"`
}

// ActivityExpirySettings contains activity expiry configuration
type ActivityExpirySettings struct {
	ActivityExpiryEnabled bool `json:"activity_expiry_enabled"`
	ActivityExpiryWindow  int  `json:"activity_expiry_window"`
}

// Features contains feature flags
type Features struct {
	EnableHostUsers         bool `json:"enable_host_users"`
	EnableSoftwareInventory bool `json:"enable_software_inventory"`
}

// FleetDesktopSettings contains Fleet Desktop configuration
type FleetDesktopSettings struct {
	TransparencyURL string `json:"transparency_url"`
}

// VulnerabilitySettings contains vulnerability scanning configuration
type VulnerabilitySettings struct {
	DatabasesPath string `json:"databases_path"`
}

// WebhookSettings contains webhook configuration
type WebhookSettings struct {
	Interval               string                          `json:"interval,omitempty"`
	HostStatusWebhook      *HostStatusWebhookSettings      `json:"host_status_webhook,omitempty"`
	FailingPoliciesWebhook *FailingPoliciesWebhookSettings `json:"failing_policies_webhook,omitempty"`
	VulnerabilitiesWebhook *VulnerabilitiesWebhookSettings `json:"vulnerabilities_webhook,omitempty"`
	ActivitiesWebhook      *ActivitiesWebhookSettings      `json:"activities_webhook,omitempty"`
}

// HostStatusWebhookSettings contains host status webhook settings
type HostStatusWebhookSettings struct {
	Enable         bool    `json:"enable_host_status_webhook"`
	DestinationURL string  `json:"destination_url"`
	HostPercentage float64 `json:"host_percentage"`
	DaysCount      int     `json:"days_count"`
}

// FailingPoliciesWebhookSettings contains failing policies webhook settings
type FailingPoliciesWebhookSettings struct {
	Enable         bool   `json:"enable_failing_policies_webhook"`
	DestinationURL string `json:"destination_url"`
	PolicyIDs      []uint `json:"policy_ids"`
	HostBatchSize  int    `json:"host_batch_size"`
}

// VulnerabilitiesWebhookSettings contains vulnerabilities webhook settings
type VulnerabilitiesWebhookSettings struct {
	Enable         bool   `json:"enable_vulnerabilities_webhook"`
	DestinationURL string `json:"destination_url"`
	HostBatchSize  int    `json:"host_batch_size"`
}

// ActivitiesWebhookSettings contains activities webhook settings
type ActivitiesWebhookSettings struct {
	Enable         bool   `json:"enable_activities_webhook"`
	DestinationURL string `json:"destination_url"`
}

// LicenseInfo contains license information
type LicenseInfo struct {
	Tier         string `json:"tier"`
	Organization string `json:"organization,omitempty"`
	DeviceCount  int    `json:"device_count,omitempty"`
	Expiration   string `json:"expiration,omitempty"`
	Note         string `json:"note,omitempty"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Debug  bool          `json:"debug"`
	JSON   bool          `json:"json"`
	Result LoggingPlugin `json:"result"`
	Status LoggingPlugin `json:"status"`
	Audit  LoggingPlugin `json:"audit"`
}

// LoggingPlugin contains logging plugin configuration
type LoggingPlugin struct {
	Plugin string `json:"plugin"`
}

// UpdateIntervalConfig contains update interval configuration
type UpdateIntervalConfig struct {
	OSQueryDetail int64 `json:"osquery_detail"`
	OSQueryPolicy int64 `json:"osquery_policy"`
}

// MDMConfig contains MDM configuration (simplified)
type MDMConfig struct {
	EnabledAndConfigured        bool `json:"enabled_and_configured"`
	AppleBMEnabledAndConfigured bool `json:"apple_bm_enabled_and_configured"`
	AppleBMTermsExpired         bool `json:"apple_bm_terms_expired"`
}

// AppConfig represents the Fleet application configuration
type AppConfig struct {
	OrgInfo                OrgInfo                `json:"org_info"`
	ServerSettings         ServerSettings         `json:"server_settings"`
	HostExpirySettings     HostExpirySettings     `json:"host_expiry_settings"`
	ActivityExpirySettings ActivityExpirySettings `json:"activity_expiry_settings"`
	Features               Features               `json:"features"`
	FleetDesktop           FleetDesktopSettings   `json:"fleet_desktop"`
	VulnerabilitySettings  VulnerabilitySettings  `json:"vulnerability_settings"`
	WebhookSettings        WebhookSettings        `json:"webhook_settings"`
	AgentOptions           *json.RawMessage       `json:"agent_options,omitempty"`
	License                *LicenseInfo           `json:"license,omitempty"`
	Logging                *LoggingConfig         `json:"logging,omitempty"`
	UpdateInterval         *UpdateIntervalConfig  `json:"update_interval,omitempty"`
	MDM                    *MDMConfig             `json:"mdm,omitempty"`
	SandboxEnabled         bool                   `json:"sandbox_enabled"`
}

// EnrollSecretSpec represents the enrollment secrets specification
type EnrollSecretSpec struct {
	Secrets []EnrollSecret `json:"secrets"`
}

// GetAppConfig retrieves the Fleet application configuration
func (c *Client) GetAppConfig(ctx context.Context) (*AppConfig, error) {
	var config AppConfig
	err := c.Get(ctx, "/config", nil, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to get app config: %w", err)
	}

	return &config, nil
}

// GetEnrollSecretSpec retrieves the global enrollment secrets.
// Uses GET /spec/enroll_secret per the Fleet REST API (returns {"spec": {"secrets": [...]}}).
func (c *Client) GetEnrollSecretSpec(ctx context.Context) (*EnrollSecretSpec, error) {
	var response struct {
		Spec EnrollSecretSpec `json:"spec"`
	}
	err := c.Get(ctx, "/spec/enroll_secret", nil, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get enroll secrets: %w", err)
	}
	return &response.Spec, nil
}

// ApplyEnrollSecretSpec replaces the global enrollment secrets.
// Uses POST /spec/enroll_secret per the Fleet REST API (body: {"spec": {"secrets": [...]}}).
func (c *Client) ApplyEnrollSecretSpec(ctx context.Context, spec *EnrollSecretSpec) error {
	body := struct {
		Spec *EnrollSecretSpec `json:"spec"`
	}{Spec: spec}
	err := c.Post(ctx, "/spec/enroll_secret", body, nil)
	if err != nil {
		return fmt.Errorf("failed to apply enroll secrets: %w", err)
	}
	return nil
}

// ServerSettingsUpdate is used in PATCH requests.
// ServerURL is a pointer so it is omitted when not provided, preventing
// an empty string from failing Fleet's URL validation.
type ServerSettingsUpdate struct {
	ServerURL            *string `json:"server_url,omitempty"`
	LiveQueryDisabled    bool    `json:"live_query_disabled"`
	EnableAnalytics      bool    `json:"enable_analytics"`
	QueryReportsDisabled bool    `json:"query_reports_disabled"`
	ScriptsDisabled      bool    `json:"scripts_disabled"`
	AIFeaturesDisabled   bool    `json:"ai_features_disabled"`
}

// UpdateAppConfigRequest represents a partial app config update
type UpdateAppConfigRequest struct {
	OrgInfo                *OrgInfo                `json:"org_info,omitempty"`
	ServerSettings         *ServerSettingsUpdate   `json:"server_settings,omitempty"`
	HostExpirySettings     *HostExpirySettings     `json:"host_expiry_settings,omitempty"`
	ActivityExpirySettings *ActivityExpirySettings `json:"activity_expiry_settings,omitempty"`
	Features               *Features               `json:"features,omitempty"`
	FleetDesktop           *FleetDesktopSettings   `json:"fleet_desktop,omitempty"`
	WebhookSettings        *WebhookSettings        `json:"webhook_settings,omitempty"`
	AgentOptions           *json.RawMessage        `json:"agent_options,omitempty"`
}

// UpdateAppConfig modifies the Fleet application configuration
func (c *Client) UpdateAppConfig(ctx context.Context, config *UpdateAppConfigRequest) (*AppConfig, error) {
	var result AppConfig
	err := c.Patch(ctx, "/config", config, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to update app config: %w", err)
	}

	return &result, nil
}
