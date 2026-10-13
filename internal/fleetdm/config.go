package fleetdm

import (
	"context"
	"encoding/json"
	"fmt"
)

// OrgInfo contains organization information.
//
// org_logo_url and org_logo_url_light_background are deprecated aliases that
// Fleet keeps for backwards compatibility; org_logo_url_dark_mode and
// org_logo_url_light_mode are the canonical fields. Fleet mirrors each alias to
// its canonical field, so the pairs always hold the same value in responses.
type OrgInfo struct {
	OrgName                   string `json:"org_name"`
	OrgLogoURL                string `json:"org_logo_url"`
	OrgLogoURLLightBackground string `json:"org_logo_url_light_background"`
	OrgLogoURLDarkMode        string `json:"org_logo_url_dark_mode"`
	OrgLogoURLLightMode       string `json:"org_logo_url_light_mode"`
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

// HostActivitiesWebhookSettings contains the per-fleet host activities webhook
// settings (Fleet 4.91+). Fleet replaces this object wholesale, so both fields
// are plain values: every attribute goes on the wire together.
type HostActivitiesWebhookSettings struct {
	Enable         bool   `json:"enable_host_activities_webhook"`
	DestinationURL string `json:"destination_url"`
}

// FailingPoliciesWebhookSettings contains failing policies webhook settings
type FailingPoliciesWebhookSettings struct {
	Enable         bool   `json:"enable_failing_policies_webhook"`
	DestinationURL string `json:"destination_url"`
	// PolicyIDs are Fleet policy database IDs. Held as int64 to match the rest
	// of this package's ID types, and so the provider needs no numeric
	// conversion when moving them in and out of a Terraform Int64 set.
	PolicyIDs     []int64 `json:"policy_ids"`
	HostBatchSize int     `json:"host_batch_size"`
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

	// AndroidEnabledAndConfigured reports whether an Android enterprise is
	// bound to this Fleet instance. Android-only endpoints such as
	// POST /software/web_apps sit behind Fleet's VerifyAndroidMDM middleware
	// and answer 400 "Android MDM isn't turned on." when this is false.
	AndroidEnabledAndConfigured bool `json:"android_enabled_and_configured"`

	// NameTemplate is the "No team" (global) host name template Fleet applies to
	// MDM-enrolled Apple hosts. It is the global counterpart of the per-team
	// TeamMDMSettings.NameTemplate. An unset template reads back as "".
	NameTemplate string `json:"name_template"`

	// WindowsAutomaticEnrollment configures user-driven Windows MDM enrollment
	// (Fleet 4.91+). Fleet reports it as an object whose default_fleet is ""
	// when no default is set.
	WindowsAutomaticEnrollment *WindowsAutomaticEnrollment `json:"windows_automatic_enrollment,omitempty"`

	// The settings below mirror MDMSettingsUpdate. Pointers distinguish a key
	// the server did not send, which an older Fleet omits, from its zero value.
	AppleServerURL             *string             `json:"apple_server_url"`
	EnableRecoveryLockPassword *bool               `json:"enable_recovery_lock_password"`
	MacOSSettings              *MacOSMDMSettings   `json:"macos_settings"`
	WindowsSettings            *WindowsMDMSettings `json:"windows_settings"`
	LinuxSettings              *LinuxMDMSettings   `json:"linux_settings"`
	MacOSUpdates               *AppleOSUpdates     `json:"macos_updates"`
	IOSUpdates                 *AppleOSUpdates     `json:"ios_updates"`
	IPadOSUpdates              *AppleOSUpdates     `json:"ipados_updates"`
	WindowsUpdates             *WindowsUpdates     `json:"windows_updates"`

	WindowsMigrationEnabled        *bool                     `json:"windows_migration_enabled"`
	EnableTurnOnWindowsMDMManually *bool                     `json:"enable_turn_on_windows_mdm_manually"`
	WindowsEntraTenantIDs          []string                  `json:"windows_entra_tenant_ids"`
	WindowsEntraClientIDs          []string                  `json:"windows_entra_client_ids"`
	MacOSMigration                 *MacOSMigration           `json:"macos_migration"`
	EndUserAuthentication          *EndUserAuthentication    `json:"end_user_authentication"`
	AppleAccountProvisioning       *AppleAccountProvisioning `json:"apple_account_provisioning"`

	AppleRequireHardwareAttestation  *bool `json:"apple_require_hardware_attestation"`
	OnlyAllowAppleBusinessEnrollment *bool `json:"only_allow_apple_business_enrollment"`
}

// MacOSMigration configures migrating macOS hosts from another MDM to Fleet.
// Fields are pointers so a request only carries what is managed.
type MacOSMigration struct {
	Enable     *bool   `json:"enable,omitempty"`
	Mode       *string `json:"mode,omitempty"`
	WebhookURL *string `json:"webhook_url,omitempty"`
}

// EndUserAuthentication is the SAML identity provider Fleet uses to
// authenticate end users during MDM enrollment.
type EndUserAuthentication struct {
	EntityID    *string `json:"entity_id,omitempty"`
	IssuerURI   *string `json:"issuer_uri,omitempty"`
	Metadata    *string `json:"metadata,omitempty"`
	MetadataURL *string `json:"metadata_url,omitempty"`
	IDPName     *string `json:"idp_name,omitempty"`
}

// AppleAccountProvisioning configures Platform SSO account provisioning
// against an OAuth identity provider (Fleet 4.92+). Fleet keeps the client
// secret outside the app config and reports it masked, or null when unset.
type AppleAccountProvisioning struct {
	OAuthIdPTokenURL     *string `json:"oauth_idp_token_url,omitempty"`
	OAuthIdPClientID     *string `json:"oauth_idp_client_id,omitempty"`
	OAuthIdPClientSecret *string `json:"oauth_idp_client_secret,omitempty"`
}

// WindowsAutomaticEnrollment holds the settings applied to new user-driven
// Windows MDM enrollments (Windows Autopilot, Entra join).
//
// DefaultFleet names a fleet rather than referencing its ID, which is Fleet's
// own wire format. Fleet resolves the name on write and rejects an unknown one
// with 422 `fleet %q doesn't exist`; "" means new hosts stay Unassigned.
type WindowsAutomaticEnrollment struct {
	DefaultFleet string `json:"default_fleet"`
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
	Integrations           *AppConfigIntegrations `json:"integrations,omitempty"`
	SandboxEnabled         bool                   `json:"sandbox_enabled"`
}

// AppConfigIntegrations holds the global integration settings the provider
// reads. Jira, Zendesk and Google Calendar are not modelled.
//
// The certificates_* settings (Fleet 4.93+) control who may request a
// certificate through the request_certificate API. Fleet reports a list that
// was cleared with null as null, and with [] as [].
type AppConfigIntegrations struct {
	CertificatesIdPIntrospectionURLs      []string `json:"certificates_idp_introspection_urls"`
	CertificatesIdPClientIDs              []string `json:"certificates_idp_client_ids"`
	CertificatesDisableHostEndUserBinding *bool    `json:"certificates_disable_host_end_user_binding"`

	// HasCertificateControls is true when the response carried the
	// certificates_* keys, which Fleet before 4.93 does not know and drops.
	HasCertificateControls bool `json:"-"`
}

// UnmarshalJSON records whether the certificates_* keys were present, since a
// missing key and a null one decode to the same value.
func (i *AppConfigIntegrations) UnmarshalJSON(data []byte) error {
	type plain AppConfigIntegrations
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}
	if err := json.Unmarshal(data, (*plain)(i)); err != nil {
		return err
	}
	_, i.HasCertificateControls = keys["certificates_idp_introspection_urls"]
	return nil
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

// OrgInfoUpdate is used in PATCH requests. Each canonical org_logo_url_*_mode key
// is always sent together with its deprecated alias set to the SAME value: Fleet
// rejects a PATCH whose incoming value for one differs from the stored value of
// the other ("cannot specify both ... with different values"), even when only one
// is supplied. The logo URLs are pointers so they are omitted when the user did
// not configure a logo, telling Fleet to leave the current value untouched; a
// non-nil pointer to "" clears the logo.
type OrgInfoUpdate struct {
	OrgName                   string  `json:"org_name"`
	OrgLogoURL                *string `json:"org_logo_url,omitempty"`
	OrgLogoURLLightBackground *string `json:"org_logo_url_light_background,omitempty"`
	OrgLogoURLDarkMode        *string `json:"org_logo_url_dark_mode,omitempty"`
	OrgLogoURLLightMode       *string `json:"org_logo_url_light_mode,omitempty"`
	ContactURL                string  `json:"contact_url"`
}

// MDMSettingsUpdate is the partial "mdm" object sent in PATCH /config. Fleet
// unmarshals the request body onto a copy of the stored AppConfig, so any mdm
// key left out keeps its current server-side value; only the fields present here
// are ever touched.
//
// NameTemplate is a pointer so an unmanaged template is omitted entirely
// (leaving Fleet's value alone) while a pointer to "" is still serialized, which
// is how the template is cleared.
type MDMSettingsUpdate struct {
	NameTemplate *string `json:"name_template,omitempty"`
	// WindowsAutomaticEnrollment is omitted unless declared, so a config that
	// does not manage it leaves Fleet's stored default fleet alone.
	WindowsAutomaticEnrollment *WindowsAutomaticEnrollment `json:"windows_automatic_enrollment,omitempty"`

	// Every other key follows the same rule: nil is omitted and keeps Fleet's
	// value; a nested object only carries its managed fields. Lists are
	// pointers so an empty list is still sent, which is how one is cleared.
	AppleServerURL             *string             `json:"apple_server_url,omitempty"`
	EnableRecoveryLockPassword *bool               `json:"enable_recovery_lock_password,omitempty"`
	MacOSSettings              *MacOSMDMSettings   `json:"macos_settings,omitempty"`
	WindowsSettings            *WindowsMDMSettings `json:"windows_settings,omitempty"`
	LinuxSettings              *LinuxMDMSettings   `json:"linux_settings,omitempty"`
	MacOSUpdates               *AppleOSUpdates     `json:"macos_updates,omitempty"`
	IOSUpdates                 *AppleOSUpdates     `json:"ios_updates,omitempty"`
	IPadOSUpdates              *AppleOSUpdates     `json:"ipados_updates,omitempty"`
	WindowsUpdates             *WindowsUpdates     `json:"windows_updates,omitempty"`

	WindowsMigrationEnabled        *bool                     `json:"windows_migration_enabled,omitempty"`
	EnableTurnOnWindowsMDMManually *bool                     `json:"enable_turn_on_windows_mdm_manually,omitempty"`
	WindowsEntraTenantIDs          *[]string                 `json:"windows_entra_tenant_ids,omitempty"`
	WindowsEntraClientIDs          *[]string                 `json:"windows_entra_client_ids,omitempty"`
	MacOSMigration                 *MacOSMigration           `json:"macos_migration,omitempty"`
	EndUserAuthentication          *EndUserAuthentication    `json:"end_user_authentication,omitempty"`
	AppleAccountProvisioning       *AppleAccountProvisioning `json:"apple_account_provisioning,omitempty"`

	AppleRequireHardwareAttestation  *bool `json:"apple_require_hardware_attestation,omitempty"`
	OnlyAllowAppleBusinessEnrollment *bool `json:"only_allow_apple_business_enrollment,omitempty"`
}

// UpdateAppConfigRequest represents a partial app config update
type UpdateAppConfigRequest struct {
	OrgInfo                *OrgInfoUpdate          `json:"org_info,omitempty"`
	ServerSettings         *ServerSettingsUpdate   `json:"server_settings,omitempty"`
	HostExpirySettings     *HostExpirySettings     `json:"host_expiry_settings,omitempty"`
	ActivityExpirySettings *ActivityExpirySettings `json:"activity_expiry_settings,omitempty"`
	Features               *Features               `json:"features,omitempty"`
	FleetDesktop           *FleetDesktopSettings   `json:"fleet_desktop,omitempty"`
	WebhookSettings        *WebhookSettings        `json:"webhook_settings,omitempty"`
	MDM                    *MDMSettingsUpdate      `json:"mdm,omitempty"`
	Integrations           *IntegrationsUpdate     `json:"integrations,omitempty"`
	AgentOptions           *json.RawMessage        `json:"agent_options,omitempty"`
}

// IntegrationsUpdate is the partial "integrations" object sent in PATCH
// /config. Fleet keeps every key left out, including Jira, Zendesk and Google
// Calendar, so only managed settings are set. A pointer to an empty slice is
// sent as [] and clears that list.
type IntegrationsUpdate struct {
	CertificatesIdPIntrospectionURLs      *[]string `json:"certificates_idp_introspection_urls,omitempty"`
	CertificatesIdPClientIDs              *[]string `json:"certificates_idp_client_ids,omitempty"`
	CertificatesDisableHostEndUserBinding *bool     `json:"certificates_disable_host_end_user_binding,omitempty"`
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
