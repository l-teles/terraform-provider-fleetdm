package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/boolvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// siblingAttribute builds a path to another attribute in the same nested block,
// for the mutual-requirement validators below.
func siblingAttribute(name string) path.Expression {
	return path.MatchRelative().AtParent().AtName(name)
}

var (
	// webhookURLRegex accepts an absolute http or https URL, or the empty
	// string, which is how a webhook destination is cleared. Fleet applies the
	// same rule (url.ParseRequestURI plus an http/https scheme check), so
	// catching it here turns an apply-time 422 into a plan-time error.
	// Plain http is not rejected because lab and on-premise deployments
	// legitimately use it.
	webhookURLRegex = regexp.MustCompile(`^(https?://\S+)?$`)

	// osUpdateDeadlineRegex accepts a YYYY-MM-DD date, or the empty string
	// used to clear the setting. Fleet parses it with the same layout.
	osUpdateDeadlineRegex = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})?$`)
)

// webhookURLValidators returns the validators shared by every webhook
// destination attribute in this resource.
func webhookURLValidators() []validator.String {
	return []validator.String{
		stringvalidator.RegexMatches(
			webhookURLRegex,
			`must be an absolute "http://" or "https://" URL, or "" to clear it`,
		),
	}
}

// webhookURLSecurityNote is appended to every webhook destination description.
// Webhook payloads carry host identifiers, and a webhook URL is often a
// capability URL whose path is the only thing authenticating the caller, so
// both the payload and the URL itself want transport encryption.
const webhookURLSecurityNote = " Use https: the payloads carry host identifiers, and webhook URLs frequently embed a secret token in the path, both of which travel in the clear over http."

// This file holds the nested settings blocks of fleetdm_fleet: webhook_settings,
// mdm, integrations and features. They all follow the same opt-in convention:
//
//   - Every attribute is Optional and never Computed, and has no default.
//   - A block is only sent to Fleet when the practitioner declared it.
//   - A block is only refreshed from Fleet when it was already in state.
//
// The convention exists because these settings are frequently managed outside
// Terraform (the Fleet UI, GitOps) and because Fleet returns a concrete value
// for every field whether or not it was ever set. Marking them Computed with
// defaults would make the provider claim ownership of settings the practitioner
// never mentioned, and would send zero values that Fleet applies verbatim.
//
// The corresponding request structs in internal/fleetdm use pointers for the
// same reason; see the commentary on TeamMDMSettings.

// ---------------------------------------------------------------------------
// Models
// ---------------------------------------------------------------------------

type fleetWebhookSettingsModel struct {
	FailingPoliciesWebhook *fleetFailingPoliciesWebhookModel `tfsdk:"failing_policies_webhook"`
	HostStatusWebhook      *fleetHostStatusWebhookModel      `tfsdk:"host_status_webhook"`
}

type fleetFailingPoliciesWebhookModel struct {
	Enable         types.Bool   `tfsdk:"enable_failing_policies_webhook"`
	DestinationURL types.String `tfsdk:"destination_url"`
	PolicyIDs      types.Set    `tfsdk:"policy_ids"`
	HostBatchSize  types.Int64  `tfsdk:"host_batch_size"`
}

type fleetHostStatusWebhookModel struct {
	Enable         types.Bool    `tfsdk:"enable_host_status_webhook"`
	DestinationURL types.String  `tfsdk:"destination_url"`
	HostPercentage types.Float64 `tfsdk:"host_percentage"`
	DaysCount      types.Int64   `tfsdk:"days_count"`
}

type fleetMDMModel struct {
	EnableRecoveryLockPassword types.Bool   `tfsdk:"enable_recovery_lock_password"`
	WindowsRequireBitlockerPIN types.Bool   `tfsdk:"windows_require_bitlocker_pin"`
	NameTemplate               types.String `tfsdk:"name_template"`

	MacOSUpdates   *fleetAppleOSUpdatesModel `tfsdk:"macos_updates"`
	IOSUpdates     *fleetAppleOSUpdatesModel `tfsdk:"ios_updates"`
	IPadOSUpdates  *fleetAppleOSUpdatesModel `tfsdk:"ipados_updates"`
	WindowsUpdates *fleetWindowsUpdatesModel `tfsdk:"windows_updates"`
}

type fleetAppleOSUpdatesModel struct {
	MinimumVersion types.String `tfsdk:"minimum_version"`
	Deadline       types.String `tfsdk:"deadline"`
	UpdateNewHosts types.Bool   `tfsdk:"update_new_hosts"`
}

type fleetWindowsUpdatesModel struct {
	DeadlineDays    types.Int64 `tfsdk:"deadline_days"`
	GracePeriodDays types.Int64 `tfsdk:"grace_period_days"`
}

type fleetIntegrationsModel struct {
	GoogleCalendar           *fleetGoogleCalendarModel `tfsdk:"google_calendar"`
	ConditionalAccessEnabled types.Bool                `tfsdk:"conditional_access_enabled"`
}

type fleetGoogleCalendarModel struct {
	EnableCalendarEvents types.Bool   `tfsdk:"enable_calendar_events"`
	WebhookURL           types.String `tfsdk:"webhook_url"`
}

type fleetFeaturesModel struct {
	HistoricalData *fleetHistoricalDataModel `tfsdk:"historical_data"`
}

type fleetHistoricalDataModel struct {
	Uptime          types.Bool `tfsdk:"uptime"`
	Vulnerabilities types.Bool `tfsdk:"vulnerabilities"`
}

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

func fleetWebhookSettingsAttribute() schema.Attribute {
	const desc = "Webhook automations for this fleet. " +
		"Fleet replaces this whole block on every write, so a sub-block you omit is cleared server-side rather than left untouched: declare both sub-blocks if you want both to persist. " +
		"The same applies one level down -- a sub-block you do declare is sent in full, with any attribute you left out sent as its zero value -- so declare each sub-block completely. " +
		"Omitting `webhook_settings` entirely leaves the fleet's webhook configuration alone."

	return schema.SingleNestedAttribute{
		Description:         desc,
		MarkdownDescription: desc,
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"failing_policies_webhook": schema.SingleNestedAttribute{
				Description:         "Sends a webhook when a host starts failing a policy. Sent to Fleet in full: attributes you omit are written as their zero values.",
				MarkdownDescription: "Sends a webhook when a host starts failing a policy. Sent to Fleet in full: attributes you omit are written as their zero values.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"enable_failing_policies_webhook": schema.BoolAttribute{
						Description:         "Whether the failing policies webhook is enabled.",
						MarkdownDescription: "Whether the failing policies webhook is enabled.",
						Optional:            true,
					},
					"destination_url": schema.StringAttribute{
						Description:         "URL the webhook payload is sent to." + webhookURLSecurityNote,
						MarkdownDescription: "URL the webhook payload is sent to." + webhookURLSecurityNote,
						Optional:            true,
						Validators:          webhookURLValidators(),
					},
					"policy_ids": schema.SetAttribute{
						Description:         "Policy IDs the webhook fires for. Omit to fire for all of the fleet's policies.",
						MarkdownDescription: "Policy IDs the webhook fires for. Omit to fire for all of the fleet's policies.",
						Optional:            true,
						ElementType:         types.Int64Type,
					},
					"host_batch_size": schema.Int64Attribute{
						Description:         "Maximum number of hosts per webhook request. 0 sends them all in one request.",
						MarkdownDescription: "Maximum number of hosts per webhook request. `0` sends them all in one request.",
						Optional:            true,
					},
				},
			},
			"host_status_webhook": schema.SingleNestedAttribute{
				Description:         "Sends a webhook when too many of the fleet's hosts stop reporting in. Sent to Fleet in full: attributes you omit are written as their zero values.",
				MarkdownDescription: "Sends a webhook when too many of the fleet's hosts stop reporting in. Sent to Fleet in full: attributes you omit are written as their zero values.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"enable_host_status_webhook": schema.BoolAttribute{
						Description:         "Whether the host status webhook is enabled.",
						MarkdownDescription: "Whether the host status webhook is enabled.",
						Optional:            true,
					},
					"destination_url": schema.StringAttribute{
						Description:         "URL the webhook payload is sent to." + webhookURLSecurityNote,
						MarkdownDescription: "URL the webhook payload is sent to." + webhookURLSecurityNote,
						Optional:            true,
						Validators:          webhookURLValidators(),
					},
					"host_percentage": schema.Float64Attribute{
						Description:         "Percentage of offline hosts that triggers the webhook.",
						MarkdownDescription: "Percentage of offline hosts that triggers the webhook.",
						Optional:            true,
					},
					"days_count": schema.Int64Attribute{
						Description:         "Number of days a host must be offline before it counts towards host_percentage.",
						MarkdownDescription: "Number of days a host must be offline before it counts towards `host_percentage`.",
						Optional:            true,
					},
				},
			},
		},
	}
}

func fleetMDMAttribute() schema.Attribute {
	const desc = "MDM settings for this fleet. Requires Fleet 4.87.0 or later. " +
		"Configuration profiles are deliberately not exposed here; use the fleetdm_configuration_profile resource. " +
		"Each attribute is sent only when you declare it, so settings you leave out keep whatever value they already have in Fleet."

	appleUpdates := func(platform string) schema.Attribute {
		desc := "Minimum " + platform + " version enforced on this fleet's hosts. " +
			"Fleet validates minimum_version against Apple's Software Lookup Service, so it must be a version Apple still publishes and must be given exactly (for example \"26.6.1\", not \"26.6\"). " +
			"Set both minimum_version and deadline to \"\" to clear the requirement."
		return schema.SingleNestedAttribute{
			Description:         desc,
			MarkdownDescription: desc,
			Optional:            true,
			Attributes: map[string]schema.Attribute{
				"minimum_version": schema.StringAttribute{
					Description:         "Required minimum OS version, for example \"26.6.1\". Must be set together with deadline.",
					MarkdownDescription: "Required minimum OS version, for example `26.6.1`. Must be set together with `deadline`.",
					Optional:            true,
					Validators: []validator.String{
						stringvalidator.AlsoRequires(siblingAttribute("deadline")),
					},
				},
				"deadline": schema.StringAttribute{
					Description:         "Date by which the update must be installed, as YYYY-MM-DD. Must be set together with minimum_version.",
					MarkdownDescription: "Date by which the update must be installed, as `YYYY-MM-DD`. Must be set together with `minimum_version`.",
					Optional:            true,
					Validators: []validator.String{
						stringvalidator.AlsoRequires(siblingAttribute("minimum_version")),
						stringvalidator.RegexMatches(
							osUpdateDeadlineRegex,
							`must be a date in YYYY-MM-DD form, or "" to clear it`,
						),
					},
				},
				"update_new_hosts": schema.BoolAttribute{
					Description:         "Enforce the latest version only on hosts that enroll from now on.",
					MarkdownDescription: "Enforce the latest version only on hosts that enroll from now on.",
					Optional:            true,
				},
			},
		}
	}

	return schema.SingleNestedAttribute{
		Description:         desc,
		MarkdownDescription: desc,
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"enable_recovery_lock_password": schema.BoolAttribute{
				Description:         "Whether Fleet escrows a recovery lock password for the fleet's Apple silicon hosts. Requires MDM to be turned on in Fleet.",
				MarkdownDescription: "Whether Fleet escrows a recovery lock password for the fleet's Apple silicon hosts. Requires MDM to be turned on in Fleet.",
				Optional:            true,
			},
			"windows_require_bitlocker_pin": schema.BoolAttribute{
				Description:         "Whether a BitLocker PIN is required before Fleet considers a Windows host compliant.",
				MarkdownDescription: "Whether a BitLocker PIN is required before Fleet considers a Windows host compliant.",
				Optional:            true,
			},
			"name_template": schema.StringAttribute{
				Description:         "Template Fleet uses to name the fleet's MDM-enrolled hosts, for example \"$FLEET_VAR_HOST_HARDWARE_SERIAL\". Set to \"\" to clear it.",
				MarkdownDescription: "Template Fleet uses to name the fleet's MDM-enrolled hosts, for example `$FLEET_VAR_HOST_HARDWARE_SERIAL`. Set to `\"\"` to clear it.",
				Optional:            true,
			},
			"macos_updates":  appleUpdates("macOS"),
			"ios_updates":    appleUpdates("iOS"),
			"ipados_updates": appleUpdates("iPadOS"),
			"windows_updates": schema.SingleNestedAttribute{
				Description:         "Windows update enforcement for this fleet. Set both attributes to 0 to clear it.",
				MarkdownDescription: "Windows update enforcement for this fleet. Set both attributes to `0` to clear it.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"deadline_days": schema.Int64Attribute{
						Description:         "Days a host has to install an available update before it is forced, between 0 and 30. Must be set together with grace_period_days.",
						MarkdownDescription: "Days a host has to install an available update before it is forced, between `0` and `30`. Must be set together with `grace_period_days`.",
						Optional:            true,
						Validators: []validator.Int64{
							int64validator.Between(0, 30),
							int64validator.AlsoRequires(siblingAttribute("grace_period_days")),
						},
					},
					"grace_period_days": schema.Int64Attribute{
						Description:         "Days a host has to restart once the deadline has passed, between 0 and 7. Must be set together with deadline_days.",
						MarkdownDescription: "Days a host has to restart once the deadline has passed, between `0` and `7`. Must be set together with `deadline_days`.",
						Optional:            true,
						Validators: []validator.Int64{
							int64validator.Between(0, 7),
							int64validator.AlsoRequires(siblingAttribute("deadline_days")),
						},
					},
				},
			},
		},
	}
}

func fleetIntegrationsAttribute() schema.Attribute {
	const desc = "Third-party integrations for this fleet. " +
		"Jira and Zendesk are not exposed because Fleet only accepts a fleet-level ticketing integration that exactly matches one already present in the global configuration, which Terraform cannot express as a standalone fleet attribute."

	return schema.SingleNestedAttribute{
		Description:         desc,
		MarkdownDescription: desc,
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"google_calendar": schema.SingleNestedAttribute{
				Description: "Google Calendar event automation. Enabling it requires a Google Calendar integration in the global configuration. " +
					"Like the webhook_settings sub-blocks, this object is sent to Fleet in full, so both attributes are written on every apply and must be declared together.",
				MarkdownDescription: "Google Calendar event automation. Enabling it requires a Google Calendar integration in the global configuration. " +
					"Like the `webhook_settings` sub-blocks, this object is sent to Fleet in full, so both attributes are written on every apply and must be declared together.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"enable_calendar_events": schema.BoolAttribute{
						Description:         "Whether calendar events are created for this fleet's failing policies.",
						MarkdownDescription: "Whether calendar events are created for this fleet's failing policies.",
						Optional:            true,
						Validators: []validator.Bool{
							boolvalidator.AlsoRequires(siblingAttribute("webhook_url")),
						},
					},
					"webhook_url": schema.StringAttribute{
						Description:         "URL called when a calendar event is due." + webhookURLSecurityNote,
						MarkdownDescription: "URL called when a calendar event is due." + webhookURLSecurityNote,
						Optional:            true,
						Validators:          webhookURLValidators(),
					},
				},
			},
			"conditional_access_enabled": schema.BoolAttribute{
				Description:         "Whether conditional access is enforced for this fleet. Enabling it requires a conditional access integration in the global configuration.",
				MarkdownDescription: "Whether conditional access is enforced for this fleet. Enabling it requires a conditional access integration in the global configuration.",
				Optional:            true,
			},
		},
	}
}

func fleetFeaturesAttribute() schema.Attribute {
	const desc = "Feature settings for this fleet. " +
		"Only historical_data is writable through the fleet API; enable_host_users, enable_software_inventory and additional_queries can only be set per-fleet through Fleet's GitOps fleet spec, so they are not exposed here."

	return schema.SingleNestedAttribute{
		Description:         desc,
		MarkdownDescription: desc,
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"historical_data": schema.SingleNestedAttribute{
				Description:         "Which historical datasets Fleet collects for this fleet. Each sub-attribute is applied independently.",
				MarkdownDescription: "Which historical datasets Fleet collects for this fleet. Each sub-attribute is applied independently.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"uptime": schema.BoolAttribute{
						Description:         "Whether historical host uptime is collected.",
						MarkdownDescription: "Whether historical host uptime is collected.",
						Optional:            true,
					},
					"vulnerabilities": schema.BoolAttribute{
						Description:         "Whether historical vulnerability data is collected.",
						MarkdownDescription: "Whether historical vulnerability data is collected.",
						Optional:            true,
					},
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Model -> request
// ---------------------------------------------------------------------------

// buildWebhookSettings converts the webhook_settings block into a request
// payload, or nil when the block was not declared.
func buildWebhookSettings(ctx context.Context, m *fleetWebhookSettingsModel, diags *diag.Diagnostics) *fleetdm.TeamWebhookSettings {
	if m == nil {
		return nil
	}

	out := &fleetdm.TeamWebhookSettings{}

	if fp := m.FailingPoliciesWebhook; fp != nil {
		var policyIDs []int64
		if !fp.PolicyIDs.IsNull() && !fp.PolicyIDs.IsUnknown() {
			diags.Append(fp.PolicyIDs.ElementsAs(ctx, &policyIDs, false)...)
		}
		out.FailingPoliciesWebhook = &fleetdm.FailingPoliciesWebhookSettings{
			Enable:         fp.Enable.ValueBool(),
			DestinationURL: fp.DestinationURL.ValueString(),
			PolicyIDs:      policyIDs,
			HostBatchSize:  int(fp.HostBatchSize.ValueInt64()),
		}
	}

	if hs := m.HostStatusWebhook; hs != nil {
		out.HostStatusWebhook = &fleetdm.HostStatusWebhookSettings{
			Enable:         hs.Enable.ValueBool(),
			DestinationURL: hs.DestinationURL.ValueString(),
			HostPercentage: hs.HostPercentage.ValueFloat64(),
			DaysCount:      int(hs.DaysCount.ValueInt64()),
		}
	}

	return out
}

// buildMDMSettings merges the legacy top-level enable_disk_encryption attribute
// with the mdm block. Fleet only accepts one `mdm` object per request, so both
// sources have to end up in the same payload. Returns nil when neither is set.
func buildMDMSettings(enableDiskEncryption types.Bool, m *fleetMDMModel) *fleetdm.TeamMDMSettings {
	diskEncryption := optionalBoolPtr(enableDiskEncryption)
	if diskEncryption == nil && m == nil {
		return nil
	}

	out := &fleetdm.TeamMDMSettings{EnableDiskEncryption: diskEncryption}
	if m == nil {
		return out
	}

	out.EnableRecoveryLockPassword = optionalBoolPtr(m.EnableRecoveryLockPassword)
	out.WindowsRequireBitlockerPIN = optionalBoolPtr(m.WindowsRequireBitlockerPIN)
	out.NameTemplate = optionalStringPtr(m.NameTemplate)
	out.MacOSUpdates = buildAppleOSUpdates(m.MacOSUpdates)
	out.IOSUpdates = buildAppleOSUpdates(m.IOSUpdates)
	out.IPadOSUpdates = buildAppleOSUpdates(m.IPadOSUpdates)

	if w := m.WindowsUpdates; w != nil {
		out.WindowsUpdates = &fleetdm.WindowsUpdates{
			DeadlineDays:    optionalIntPtr(w.DeadlineDays),
			GracePeriodDays: optionalIntPtr(w.GracePeriodDays),
		}
	}

	return out
}

func buildAppleOSUpdates(m *fleetAppleOSUpdatesModel) *fleetdm.AppleOSUpdates {
	if m == nil {
		return nil
	}
	return &fleetdm.AppleOSUpdates{
		MinimumVersion: optionalStringPtr(m.MinimumVersion),
		Deadline:       optionalStringPtr(m.Deadline),
		UpdateNewHosts: optionalBoolPtr(m.UpdateNewHosts),
	}
}

// buildIntegrations converts the integrations block into a request payload, or
// nil when the block was not declared.
func buildIntegrations(m *fleetIntegrationsModel) *fleetdm.TeamIntegrations {
	if m == nil {
		return nil
	}

	out := &fleetdm.TeamIntegrations{
		ConditionalAccessEnabled: optionalBoolPtr(m.ConditionalAccessEnabled),
	}
	if gc := m.GoogleCalendar; gc != nil {
		out.GoogleCalendar = &fleetdm.TeamGoogleCalendarIntegration{
			EnableCalendarEvents: gc.EnableCalendarEvents.ValueBool(),
			WebhookURL:           gc.WebhookURL.ValueString(),
		}
	}
	return out
}

// buildFeatures converts the features block into a request payload, or nil when
// the block was not declared.
func buildFeatures(m *fleetFeaturesModel) *fleetdm.TeamFeatures {
	if m == nil {
		return nil
	}

	out := &fleetdm.TeamFeatures{}
	if hd := m.HistoricalData; hd != nil {
		out.HistoricalData = &fleetdm.HistoricalDataSettings{
			Uptime:          optionalBoolPtr(hd.Uptime),
			Vulnerabilities: optionalBoolPtr(hd.Vulnerabilities),
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Response -> model
// ---------------------------------------------------------------------------

// refreshWebhookSettings refreshes the webhook_settings block in place. A block
// or sub-block that was never declared stays nil.
//
// Two kinds of nil are distinguished throughout. A nil *parent* block means the
// response carried no information about it, so state is left untouched rather
// than reporting a removal Fleet never made. A nil host_status_webhook is
// different: Fleet reports that one as an explicit null when it is not
// configured, so nil there is real information and the block is dropped from
// state, letting the next plan show the drift.
func refreshWebhookSettings(ctx context.Context, m *fleetWebhookSettingsModel, api *fleetdm.TeamWebhookSettings, diags *diag.Diagnostics) {
	if m == nil || api == nil {
		return
	}

	if fp := m.FailingPoliciesWebhook; fp != nil {
		if a := api.FailingPoliciesWebhook; a == nil {
			m.FailingPoliciesWebhook = nil
		} else {
			fp.Enable = refreshOptionalBool(fp.Enable, &a.Enable)
			fp.DestinationURL = refreshOptionalString(fp.DestinationURL, &a.DestinationURL)
			fp.HostBatchSize = refreshOptionalInt64(fp.HostBatchSize, &a.HostBatchSize)
			if !fp.PolicyIDs.IsNull() {
				set, d := types.SetValueFrom(ctx, types.Int64Type, a.PolicyIDs)
				diags.Append(d...)
				if !d.HasError() {
					fp.PolicyIDs = set
				}
			}
		}
	}

	if hs := m.HostStatusWebhook; hs != nil {
		if a := api.HostStatusWebhook; a == nil {
			m.HostStatusWebhook = nil
		} else {
			hs.Enable = refreshOptionalBool(hs.Enable, &a.Enable)
			hs.DestinationURL = refreshOptionalString(hs.DestinationURL, &a.DestinationURL)
			hs.HostPercentage = refreshOptionalFloat64(hs.HostPercentage, &a.HostPercentage)
			hs.DaysCount = refreshOptionalInt64(hs.DaysCount, &a.DaysCount)
		}
	}
}

// refreshMDM refreshes the mdm block in place. Fleet always reports the OS
// update sub-blocks as objects, so a nil one carries no information and leaves
// state alone.
func refreshMDM(m *fleetMDMModel, api *fleetdm.TeamMDMSettings) {
	if m == nil || api == nil {
		return
	}

	m.EnableRecoveryLockPassword = refreshOptionalBool(m.EnableRecoveryLockPassword, api.EnableRecoveryLockPassword)
	m.WindowsRequireBitlockerPIN = refreshOptionalBool(m.WindowsRequireBitlockerPIN, api.WindowsRequireBitlockerPIN)
	m.NameTemplate = refreshOptionalString(m.NameTemplate, api.NameTemplate)

	refreshAppleOSUpdates(m.MacOSUpdates, api.MacOSUpdates)
	refreshAppleOSUpdates(m.IOSUpdates, api.IOSUpdates)
	refreshAppleOSUpdates(m.IPadOSUpdates, api.IPadOSUpdates)

	if w, a := m.WindowsUpdates, api.WindowsUpdates; w != nil && a != nil {
		w.DeadlineDays = refreshOptionalInt64(w.DeadlineDays, a.DeadlineDays)
		w.GracePeriodDays = refreshOptionalInt64(w.GracePeriodDays, a.GracePeriodDays)
	}
}

func refreshAppleOSUpdates(m *fleetAppleOSUpdatesModel, api *fleetdm.AppleOSUpdates) {
	if m == nil || api == nil {
		return
	}
	m.MinimumVersion = refreshOptionalString(m.MinimumVersion, api.MinimumVersion)
	m.Deadline = refreshOptionalString(m.Deadline, api.Deadline)
	m.UpdateNewHosts = refreshOptionalBool(m.UpdateNewHosts, api.UpdateNewHosts)
}

// refreshIntegrations refreshes the integrations block in place. Fleet reports
// an unconfigured google_calendar as an explicit null, so nil there means the
// integration is gone.
func refreshIntegrations(m *fleetIntegrationsModel, api *fleetdm.TeamIntegrations) {
	if m == nil || api == nil {
		return
	}

	m.ConditionalAccessEnabled = refreshOptionalBool(m.ConditionalAccessEnabled, api.ConditionalAccessEnabled)

	if gc := m.GoogleCalendar; gc != nil {
		if a := api.GoogleCalendar; a == nil {
			m.GoogleCalendar = nil
		} else {
			gc.EnableCalendarEvents = refreshOptionalBool(gc.EnableCalendarEvents, &a.EnableCalendarEvents)
			gc.WebhookURL = refreshOptionalString(gc.WebhookURL, &a.WebhookURL)
		}
	}
}

// refreshFeatures refreshes the features block in place.
func refreshFeatures(m *fleetFeaturesModel, api *fleetdm.TeamFeatures) {
	if m == nil || api == nil {
		return
	}

	if hd, a := m.HistoricalData, api.HistoricalData; hd != nil && a != nil {
		hd.Uptime = refreshOptionalBool(hd.Uptime, a.Uptime)
		hd.Vulnerabilities = refreshOptionalBool(hd.Vulnerabilities, a.Vulnerabilities)
	}
}
