package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// mdmScope words the MDM settings sub-blocks that fleetdm_fleet and
// fleetdm_configuration share. Fleet uses the same wire shape for a fleet's
// mdm object and the global one, which applies to hosts not in any fleet.
type mdmScope struct {
	// forScope completes "... MDM settings ", e.g. "for this fleet".
	forScope string
	// hostsOf qualifies hosts, e.g. "this fleet's" in "this fleet's macOS hosts".
	hostsOf string
	// enabledFor completes "disk encryption is enabled ", e.g. "for the fleet".
	enabledFor string
	// gateNote is appended to the blocks that carry disk encryption keys.
	gateNote string
	// fleetOnly adds the notes about fleetdm_fleet's deprecated attributes.
	fleetOnly bool
}

var (
	fleetMDMScope = mdmScope{
		forScope:   "for this fleet",
		hostsOf:    "this fleet's",
		enabledFor: "for the fleet",
		gateNote:   diskEncryptionGateNote,
		fleetOnly:  true,
	}
	globalMDMScope = mdmScope{
		forScope:   "for hosts that are not assigned to a fleet",
		hostsOf:    "unassigned",
		enabledFor: "for unassigned hosts",
		gateNote:   " Fleet only accepts a disk encryption change while Apple or Windows MDM is turned on, and enabling one needs the Fleet server private key to be configured.",
	}
)

func appleOSUpdatesAttribute(platform string, s mdmScope) schema.Attribute {
	desc := "Minimum " + platform + " version enforced on " + s.hostsOf + " hosts. " +
		"Give an exact version Apple still publishes (for example \"26.6.1\", not \"26.6\") together with deadline, which Fleet validates against Apple's Software Lookup Service. " +
		"Alternatively set minimum_version to \"latest\" together with deadline_days to track whatever version Apple currently publishes for each host's hardware, with the deadline computed per version from its release date. " +
		"Set both minimum_version and deadline to \"\" to clear the requirement."
	mdDesc := "Minimum " + platform + " version enforced on " + s.hostsOf + " hosts.\n\n" +
		"Give an exact version Apple still publishes (for example `26.6.1`, not `26.6`) together with `deadline`, which Fleet validates against Apple's Software Lookup Service. " +
		"Alternatively set `minimum_version` to `\"latest\"` together with `deadline_days` to track whatever version Apple currently publishes for each host's hardware, with the deadline computed per version from its release date (requires Fleet 4.91.0 or later). " +
		"Set both `minimum_version` and `deadline` to `\"\"` to clear the requirement."
	return schema.SingleNestedAttribute{
		Description:         desc,
		MarkdownDescription: mdDesc,
		Optional:            true,
		Validators: []validator.Object{
			appleOSUpdatesConsistentValidator{},
		},
		Attributes: map[string]schema.Attribute{
			"minimum_version": schema.StringAttribute{
				Description:         "Required minimum OS version, for example \"26.6.1\", or \"latest\" to track Apple's newest release. Set it with deadline, or with deadline_days when it is \"latest\".",
				MarkdownDescription: "Required minimum OS version, for example `26.6.1`, or `\"latest\"` to track Apple's newest release. Set it with `deadline`, or with `deadline_days` when it is `\"latest\"`.",
				Optional:            true,
			},
			"deadline": schema.StringAttribute{
				Description:         "Date by which the update must be installed, as YYYY-MM-DD. Set it together with an exact minimum_version; Fleet rejects it when minimum_version is \"latest\".",
				MarkdownDescription: "Date by which the update must be installed, as `YYYY-MM-DD`. Set it together with an exact `minimum_version`; Fleet rejects it when `minimum_version` is `\"latest\"`.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.AlsoRequires(siblingAttribute("minimum_version")),
					stringvalidator.RegexMatches(
						osUpdateDeadlineRegex,
						`must be a date in YYYY-MM-DD form, or "" to clear it`,
					),
				},
			},
			"deadline_days": schema.Int64Attribute{
				Description:         "Days after a version's release date before the update is enforced. Only valid when minimum_version is \"latest\", where it replaces deadline. Requires Fleet 4.91.0 or later.",
				MarkdownDescription: "Days after a version's release date before the update is enforced. Only valid when `minimum_version` is `\"latest\"`, where it replaces `deadline`. Requires Fleet 4.91.0 or later.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"update_new_hosts": updateNewHostsAttribute(platform),
		},
	}
}

func updateNewHostsAttribute(platform string) schema.BoolAttribute {
	a := schema.BoolAttribute{
		Description:         "Enforce the latest version only on hosts that enroll from now on.",
		MarkdownDescription: "Enforce the latest version only on hosts that enroll from now on.",
		Optional:            true,
	}
	if platform != "macOS" {
		a.Validators = []validator.Bool{macOSOnlyUpdateNewHostsValidator{}}
	}
	return a
}

// macOSOnlyUpdateNewHostsValidator warns that Fleet drops update_new_hosts for
// iOS and iPadOS on both the fleet and the global endpoint, so the value only
// ever exists in Terraform state.
type macOSOnlyUpdateNewHostsValidator struct{}

func (macOSOnlyUpdateNewHostsValidator) Description(context.Context) string {
	return "Fleet only applies update_new_hosts to macOS"
}

func (v macOSOnlyUpdateNewHostsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (macOSOnlyUpdateNewHostsValidator) ValidateBool(_ context.Context, req validator.BoolRequest, resp *validator.BoolResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	resp.Diagnostics.AddAttributeWarning(req.Path, "Setting ignored by Fleet",
		"Fleet only applies update_new_hosts to macOS and discards it for iOS and iPadOS, so this value has no effect. Remove it.")
}

func macOSSettingsAttribute(s mdmScope) schema.Attribute {
	return schema.SingleNestedAttribute{
		Description: "macOS-specific MDM settings " + s.forScope + ". Configuration profiles are deliberately not exposed here; use the fleetdm_configuration_profile resource. " +
			"Requires Fleet 4.92.0 or later." + s.gateNote,
		MarkdownDescription: "macOS-specific MDM settings " + s.forScope + ". Configuration profiles are deliberately not exposed here; use the `fleetdm_configuration_profile` resource. " +
			"Requires Fleet 4.92.0 or later." + s.gateNote,
		Optional: true,
		Attributes: map[string]schema.Attribute{
			"enable_disk_encryption": schema.BoolAttribute{
				Description:         "Whether Fleet enforces FileVault on " + s.hostsOf + " macOS hosts.",
				MarkdownDescription: "Whether Fleet enforces FileVault on " + s.hostsOf + " macOS hosts.",
				Optional:            true,
			},
			"enable_escrow_disk_encryption_key": schema.BoolAttribute{
				Description:         "Whether Fleet escrows the FileVault recovery key of " + s.hostsOf + " macOS hosts, independently of whether Fleet enforces FileVault.",
				MarkdownDescription: "Whether Fleet escrows the FileVault recovery key of " + s.hostsOf + " macOS hosts, independently of whether Fleet enforces FileVault.",
				Optional:            true,
			},
		},
	}
}

func windowsSettingsAttribute(s mdmScope) schema.Attribute {
	pinDesc := "Whether a BitLocker PIN is required before Fleet considers a Windows host compliant. Fleet only accepts true while Windows disk encryption is enabled " + s.enabledFor + "."
	pinMD := "Whether a BitLocker PIN is required before Fleet considers a Windows host compliant. Fleet only accepts `true` while Windows disk encryption is enabled " + s.enabledFor + "."
	if s.fleetOnly {
		pinDesc += " Replaces the deprecated mdm.windows_require_bitlocker_pin."
		pinMD += " Replaces the deprecated `mdm.windows_require_bitlocker_pin`."
	}
	return schema.SingleNestedAttribute{
		Description: "Windows-specific MDM settings " + s.forScope + ". Configuration profiles are deliberately not exposed here; use the fleetdm_configuration_profile resource. " +
			"Requires Fleet 4.91.0 or later; enable_disk_encryption and require_bitlocker_pin require Fleet 4.92.0 or later." + s.gateNote,
		MarkdownDescription: "Windows-specific MDM settings " + s.forScope + ". Configuration profiles are deliberately not exposed here; use the `fleetdm_configuration_profile` resource. " +
			"Requires Fleet 4.91.0 or later; `enable_disk_encryption` and `require_bitlocker_pin` require Fleet 4.92.0 or later." + s.gateNote,
		Optional: true,
		Attributes: map[string]schema.Attribute{
			"enable_managed_local_account": schema.BoolAttribute{
				Description:         "Whether fleetd creates a managed local admin account on " + s.hostsOf + " Windows hosts during enrollment. Requires fleetd 1.60.0 or later on the host.",
				MarkdownDescription: "Whether `fleetd` creates a managed local admin account on " + s.hostsOf + " Windows hosts during enrollment. Requires `fleetd` 1.60.0 or later on the host.",
				Optional:            true,
			},
			"enable_disk_encryption": schema.BoolAttribute{
				Description:         "Whether Fleet enforces BitLocker on " + s.hostsOf + " Windows hosts.",
				MarkdownDescription: "Whether Fleet enforces BitLocker on " + s.hostsOf + " Windows hosts.",
				Optional:            true,
			},
			"require_bitlocker_pin": schema.BoolAttribute{
				Description:         pinDesc,
				MarkdownDescription: pinMD,
				Optional:            true,
			},
		},
	}
}

func linuxSettingsAttribute(s mdmScope) schema.Attribute {
	return schema.SingleNestedAttribute{
		Description:         "Linux-specific MDM settings " + s.forScope + ". Requires Fleet 4.92.0 or later." + s.gateNote,
		MarkdownDescription: "Linux-specific MDM settings " + s.forScope + ". Requires Fleet 4.92.0 or later." + s.gateNote,
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"enable_escrow_disk_encryption_key": schema.BoolAttribute{
				Description:         "Whether Fleet escrows the LUKS passphrase of " + s.hostsOf + " Linux hosts.",
				MarkdownDescription: "Whether Fleet escrows the LUKS passphrase of " + s.hostsOf + " Linux hosts.",
				Optional:            true,
			},
		},
	}
}

func windowsUpdatesAttribute(s mdmScope) schema.Attribute {
	return schema.SingleNestedAttribute{
		Description:         "Windows update enforcement " + s.forScope + ". Set both attributes to 0 to clear it.",
		MarkdownDescription: "Windows update enforcement " + s.forScope + ". Set both attributes to `0` to clear it.",
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
	}
}
