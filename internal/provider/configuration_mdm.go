package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// This file holds the opt-in `mdm` block of fleetdm_configuration, which maps
// the global mdm object of PATCH /config. It follows the same convention as
// fleetdm_fleet's settings blocks (see fleet_resource_settings.go): nothing is
// Computed, an attribute is only sent while configured, and only refreshed
// while it is in state. PATCH /config merges the request onto the stored
// config, so every key left out keeps its value in Fleet.

var (
	// entraGUIDRegex matches Fleet's IsValidEntraGUID.
	entraGUIDRegex = regexp.MustCompile(`^[A-Fa-f0-9]{8}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{12}$`)
	// entraClientIDRegex is the lowercase form Fleet stores client IDs in.
	entraClientIDRegex = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
)

type configurationMDMModel struct {
	NameTemplate               types.String                                  `tfsdk:"name_template"`
	WindowsAutomaticEnrollment *configurationWindowsAutomaticEnrollmentModel `tfsdk:"windows_automatic_enrollment"`
	AppleServerURL             types.String                                  `tfsdk:"apple_server_url"`
	EnableRecoveryLockPassword types.Bool                                    `tfsdk:"enable_recovery_lock_password"`

	MacOSSettings   *fleetMacOSSettingsModel   `tfsdk:"macos_settings"`
	WindowsSettings *fleetWindowsSettingsModel `tfsdk:"windows_settings"`
	LinuxSettings   *fleetLinuxSettingsModel   `tfsdk:"linux_settings"`
	MacOSUpdates    *fleetAppleOSUpdatesModel  `tfsdk:"macos_updates"`
	IOSUpdates      *fleetAppleOSUpdatesModel  `tfsdk:"ios_updates"`
	IPadOSUpdates   *fleetAppleOSUpdatesModel  `tfsdk:"ipados_updates"`
	WindowsUpdates  *fleetWindowsUpdatesModel  `tfsdk:"windows_updates"`

	WindowsMigrationEnabled        types.Bool                        `tfsdk:"windows_migration_enabled"`
	EnableTurnOnWindowsMDMManually types.Bool                        `tfsdk:"enable_turn_on_windows_mdm_manually"`
	WindowsEntraTenantIDs          types.Set                         `tfsdk:"windows_entra_tenant_ids"`
	WindowsEntraClientIDs          types.Set                         `tfsdk:"windows_entra_client_ids"`
	MacOSMigration                 *configurationMacOSMigrationModel `tfsdk:"macos_migration"`
	EndUserAuthentication          *configurationEndUserAuthModel    `tfsdk:"end_user_authentication"`

	AppleRequireHardwareAttestation  types.Bool                                  `tfsdk:"apple_require_hardware_attestation"`
	OnlyAllowAppleBusinessEnrollment types.Bool                                  `tfsdk:"only_allow_apple_business_enrollment"`
	AppleAccountProvisioning         *configurationAppleAccountProvisioningModel `tfsdk:"apple_account_provisioning"`
}

type configurationWindowsAutomaticEnrollmentModel struct {
	DefaultFleet types.String `tfsdk:"default_fleet"`
}

type configurationMacOSMigrationModel struct {
	Enable     types.Bool   `tfsdk:"enable"`
	Mode       types.String `tfsdk:"mode"`
	WebhookURL types.String `tfsdk:"webhook_url"`
}

type configurationEndUserAuthModel struct {
	EntityID    types.String `tfsdk:"entity_id"`
	IssuerURI   types.String `tfsdk:"issuer_uri"`
	Metadata    types.String `tfsdk:"metadata"`
	MetadataURL types.String `tfsdk:"metadata_url"`
	IDPName     types.String `tfsdk:"idp_name"`
}

type configurationAppleAccountProvisioningModel struct {
	OAuthIdPTokenURL            types.String `tfsdk:"oauth_idp_token_url"`
	OAuthIdPClientID            types.String `tfsdk:"oauth_idp_client_id"`
	OAuthIdPClientSecret        types.String `tfsdk:"oauth_idp_client_secret"`
	OAuthIdPClientSecretWO      types.String `tfsdk:"oauth_idp_client_secret_wo"`
	OAuthIdPClientSecretVersion types.Int64  `tfsdk:"oauth_idp_client_secret_wo_version"`
}

func configurationMDMSchema() schema.SingleNestedAttribute {
	str := func(md string, v ...validator.String) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, MarkdownDescription: md, Validators: v}
	}
	boolean := func(md string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, MarkdownDescription: md}
	}

	return schema.SingleNestedAttribute{
		Optional: true,
		MarkdownDescription: "Global MDM settings. Settings that also exist per fleet (OS updates, disk encryption) apply to hosts that " +
			"are not assigned to a fleet; the rest apply to the whole Fleet instance. Only the attributes set here are managed: " +
			"anything left out keeps its current value in Fleet. Configuration profiles, the setup experience, Apple Business " +
			"Manager and VPP are managed by their own resources.",
		Attributes: map[string]schema.Attribute{
			"name_template": str(
				"Template Fleet uses to name MDM-enrolled hosts that are not assigned to a fleet, for example "+
					"`$FLEET_VAR_HOST_HARDWARE_SERIAL`. Set to `\"\"` to clear it. Replaces the deprecated top-level "+
					"`host_name_template` and conflicts with it. Requires Fleet Premium.",
				hostNameTemplateNotPadded{},
				stringvalidator.ConflictsWith(path.MatchRoot("host_name_template")),
			),
			"windows_automatic_enrollment": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Settings for user-driven Windows MDM enrollment (Windows Autopilot, Entra join). Requires Fleet Premium and Fleet 4.91.0 or later.",
				Attributes: map[string]schema.Attribute{
					"default_fleet": str(
						"Name of the fleet new hosts are assigned to, before the Autopilot Enrollment Status Page runs. This is a "+
							"fleet **name** — reference `fleetdm_fleet.example.name`. Set to `\"\"` to leave new hosts unassigned. "+
							"Replaces the deprecated top-level `windows_automatic_enrollment_default_fleet` and conflicts with it.",
						stringvalidator.ConflictsWith(path.MatchRoot("windows_automatic_enrollment_default_fleet")),
					),
				},
			},
			"apple_server_url": str(
				"URL Apple devices use to reach Fleet for MDM, when it differs from the server URL. Set to `\"\"` to use the server URL. "+
					"**Warning:** changing it can leave already-enrolled devices unable to reach Fleet until they re-enroll.",
				noSurroundingWhitespace{},
			),
			"enable_recovery_lock_password": boolean(
				"Whether Fleet escrows a recovery lock password for unassigned Apple silicon hosts. Requires Apple MDM to be turned on."),
			"macos_settings":   macOSSettingsAttribute(globalMDMScope),
			"windows_settings": windowsSettingsAttribute(globalMDMScope),
			"linux_settings":   linuxSettingsAttribute(globalMDMScope),
			"macos_updates":    appleOSUpdatesAttribute("macOS", globalMDMScope),
			"ios_updates":      appleOSUpdatesAttribute("iOS", globalMDMScope),
			"ipados_updates":   appleOSUpdatesAttribute("iPadOS", globalMDMScope),
			"windows_updates":  windowsUpdatesAttribute(globalMDMScope),
			"windows_migration_enabled": boolean(
				"Whether Fleet migrates Windows hosts from another MDM. Requires Windows MDM and Fleet Premium. Cannot be true " +
					"together with `enable_turn_on_windows_mdm_manually`."),
			"enable_turn_on_windows_mdm_manually": boolean(
				"Whether end users turn on Windows MDM themselves from Fleet Desktop instead of Fleet doing it automatically. " +
					"Requires Windows MDM and Fleet Premium. Cannot be true together with `windows_migration_enabled`."),
			"windows_entra_tenant_ids": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				MarkdownDescription: "Microsoft Entra tenant IDs (GUIDs) allowed to enroll Windows hosts automatically. An empty set clears " +
					"the list. Requires Windows MDM and Fleet Premium.",
				Validators: []validator.Set{setvalidator.ValueStringsAre(
					stringvalidator.RegexMatches(entraGUIDRegex, "must be a GUID"))},
			},
			"windows_entra_client_ids": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				MarkdownDescription: "Microsoft Entra application (client) IDs accepted for automatic Windows enrollment, as lowercase GUIDs " +
					"(Fleet stores them in lowercase). An empty set clears the list. Requires Windows MDM and Fleet Premium.",
				Validators: []validator.Set{setvalidator.ValueStringsAre(
					stringvalidator.RegexMatches(entraClientIDRegex, "must be a lowercase GUID, which is how Fleet stores client IDs"))},
			},
			"macos_migration": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Migration of macOS hosts from another MDM to Fleet. Enabling it requires Fleet Premium, a `mode` and a `webhook_url`.",
				Validators:          []validator.Object{macOSMigrationValidator{}},
				Attributes: map[string]schema.Attribute{
					"enable": boolean("Whether end users are prompted to migrate to Fleet."),
					"mode": str("How the end user is prompted: `voluntary` or `forced`.",
						stringvalidator.OneOf("voluntary", "forced", "")),
					"webhook_url": str("URL Fleet calls to have the previous MDM release the host."+webhookURLSecurityNote,
						webhookURLValidators()...),
				},
			},
			"end_user_authentication": schema.SingleNestedAttribute{
				Optional: true,
				MarkdownDescription: "SAML identity provider Fleet uses to authenticate end users during MDM enrollment. Fleet needs " +
					"`entity_id`, `idp_name` and one of `metadata` or `metadata_url` once any of them is set. Set every attribute to `\"\"` to clear it.",
				Attributes: map[string]schema.Attribute{
					"entity_id":    str("Entity ID Fleet presents to the identity provider.", noSurroundingWhitespace{}),
					"issuer_uri":   str("Issuer URI of the identity provider.", noSurroundingWhitespace{}),
					"metadata":     str("Identity provider metadata XML. Alternative to `metadata_url`.", noSurroundingWhitespace{}),
					"metadata_url": str("URL Fleet fetches the identity provider metadata from. Alternative to `metadata`.", noSurroundingWhitespace{}),
					"idp_name":     str("Name of the identity provider, shown to end users.", noSurroundingWhitespace{}),
				},
			},
			"apple_require_hardware_attestation": boolean(
				"Whether iPhones and iPads assigned to Fleet in Apple Business prove their hardware with ACME attestation when they " +
					"enroll. Requires Fleet Premium. **Security:** together with `only_allow_apple_business_enrollment`, Fleet stops issuing " +
					"SCEP certificates altogether, so only attested devices can enroll or renew."),
			"only_allow_apple_business_enrollment": boolean(
				"Whether Apple MDM enrollment is limited to devices assigned to Fleet in Apple Business. Manual, OTA and account-driven " +
					"(BYOD) enrollment are blocked. Requires Fleet Premium and Fleet 4.93.0 or later. **Security:** together with " +
					"`apple_require_hardware_attestation`, Fleet stops issuing SCEP certificates altogether."),
			"apple_account_provisioning": schema.SingleNestedAttribute{
				Optional: true,
				MarkdownDescription: "OAuth identity provider Fleet uses to provision accounts through Platform SSO on macOS. Requires " +
					"Fleet 4.92.0 or later. Fleet never returns the client secret, so a secret changed outside Terraform is not detected. " +
					"Removing the block stops managing it but leaves Fleet's settings in place.",
				Attributes: map[string]schema.Attribute{
					"oauth_idp_token_url": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Token endpoint of the identity provider.",
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1), noSurroundingWhitespace{}},
					},
					"oauth_idp_client_id": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Client ID registered with the identity provider.",
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1), noSurroundingWhitespace{}},
					},
					"oauth_idp_client_secret": schema.StringAttribute{
						Optional:  true,
						Sensitive: true,
						MarkdownDescription: "Client secret registered with the identity provider. Exactly one of `oauth_idp_client_secret` " +
							"and `oauth_idp_client_secret_wo` must be set. This attribute is stored in Terraform state; prefer the " +
							"write-only variant on Terraform 1.11 and later.",
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
							stringvalidator.NoneOf(fleetdm.MaskedCASecret),
							stringvalidator.ExactlyOneOf(
								path.MatchRelative().AtParent().AtName("oauth_idp_client_secret"),
								path.MatchRelative().AtParent().AtName("oauth_idp_client_secret_wo"),
							),
						},
					},
					"oauth_idp_client_secret_wo": schema.StringAttribute{
						Optional:  true,
						Sensitive: true,
						WriteOnly: true,
						MarkdownDescription: "Write-only client secret: Terraform never stores it in the plan or state. Requires Terraform " +
							"1.11 or later. Changing it has no effect on its own; increment `oauth_idp_client_secret_wo_version` in the " +
							"same change to send the new value.",
						Validators: []validator.String{stringvalidator.LengthAtLeast(1), stringvalidator.NoneOf(fleetdm.MaskedCASecret)},
					},
					"oauth_idp_client_secret_wo_version": schema.Int64Attribute{
						Optional: true,
						MarkdownDescription: "Any number; change it to send a new `oauth_idp_client_secret_wo`. Terraform cannot see a " +
							"write-only value, so this is what makes a rotation show up in the plan.",
					},
				},
			},
		},
		Validators: []validator.Object{windowsEnrollmentTogglesValidator{}},
	}
}

// buildConfigurationMDM adds the managed keys of the mdm block to the request.
// The deprecated top-level attributes conflict with their in-block
// equivalents, so at most one source sets each key. clientSecret is resolved
// by the caller because the write-only variant is only readable from config.
func buildConfigurationMDM(ctx context.Context, m *configurationMDMModel, clientSecret *string, req *fleetdm.UpdateAppConfigRequest, diags *diag.Diagnostics) {
	if m == nil {
		return
	}
	if req.MDM == nil {
		req.MDM = &fleetdm.MDMSettingsUpdate{}
	}
	out := req.MDM

	if v := optionalStringPtr(m.NameTemplate); v != nil {
		out.NameTemplate = v
	}
	if w := m.WindowsAutomaticEnrollment; w != nil {
		if v := optionalStringPtr(w.DefaultFleet); v != nil {
			out.WindowsAutomaticEnrollment = &fleetdm.WindowsAutomaticEnrollment{DefaultFleet: *v}
		}
	}
	out.AppleServerURL = optionalStringPtr(m.AppleServerURL)
	out.EnableRecoveryLockPassword = optionalBoolPtr(m.EnableRecoveryLockPassword)

	if ms := m.MacOSSettings; ms != nil {
		out.MacOSSettings = &fleetdm.MacOSMDMSettings{
			EnableDiskEncryption:          optionalBoolPtr(ms.EnableDiskEncryption),
			EnableEscrowDiskEncryptionKey: optionalBoolPtr(ms.EnableEscrowDiskEncryptionKey),
		}
	}
	if ws := m.WindowsSettings; ws != nil {
		out.WindowsSettings = &fleetdm.WindowsMDMSettings{
			EnableManagedLocalAccount: optionalBoolPtr(ws.EnableManagedLocalAccount),
			EnableDiskEncryption:      optionalBoolPtr(ws.EnableDiskEncryption),
			RequireBitlockerPIN:       optionalBoolPtr(ws.RequireBitlockerPIN),
		}
	}
	if ls := m.LinuxSettings; ls != nil {
		out.LinuxSettings = &fleetdm.LinuxMDMSettings{EnableEscrowDiskEncryptionKey: optionalBoolPtr(ls.EnableEscrowDiskEncryptionKey)}
	}
	out.MacOSUpdates = buildAppleOSUpdates(m.MacOSUpdates)
	out.IOSUpdates = buildAppleOSUpdates(m.IOSUpdates)
	out.IPadOSUpdates = buildAppleOSUpdates(m.IPadOSUpdates)
	if w := m.WindowsUpdates; w != nil {
		out.WindowsUpdates = &fleetdm.WindowsUpdates{
			DeadlineDays:    optionalIntPtr(w.DeadlineDays),
			GracePeriodDays: optionalIntPtr(w.GracePeriodDays),
		}
	}

	out.WindowsMigrationEnabled = optionalBoolPtr(m.WindowsMigrationEnabled)
	out.EnableTurnOnWindowsMDMManually = optionalBoolPtr(m.EnableTurnOnWindowsMDMManually)
	out.WindowsEntraTenantIDs = optionalSetPtr(ctx, m.WindowsEntraTenantIDs, diags)
	out.WindowsEntraClientIDs = optionalSetPtr(ctx, m.WindowsEntraClientIDs, diags)
	if mm := m.MacOSMigration; mm != nil {
		out.MacOSMigration = &fleetdm.MacOSMigration{
			Enable:     optionalBoolPtr(mm.Enable),
			Mode:       optionalStringPtr(mm.Mode),
			WebhookURL: optionalStringPtr(mm.WebhookURL),
		}
	}
	if e := m.EndUserAuthentication; e != nil {
		out.EndUserAuthentication = &fleetdm.EndUserAuthentication{
			EntityID:    optionalStringPtr(e.EntityID),
			IssuerURI:   optionalStringPtr(e.IssuerURI),
			Metadata:    optionalStringPtr(e.Metadata),
			MetadataURL: optionalStringPtr(e.MetadataURL),
			IDPName:     optionalStringPtr(e.IDPName),
		}
	}

	out.AppleRequireHardwareAttestation = optionalBoolPtr(m.AppleRequireHardwareAttestation)
	out.OnlyAllowAppleBusinessEnrollment = optionalBoolPtr(m.OnlyAllowAppleBusinessEnrollment)
	if a := m.AppleAccountProvisioning; a != nil {
		out.AppleAccountProvisioning = &fleetdm.AppleAccountProvisioning{
			OAuthIdPTokenURL:     optionalStringPtr(a.OAuthIdPTokenURL),
			OAuthIdPClientID:     optionalStringPtr(a.OAuthIdPClientID),
			OAuthIdPClientSecret: clientSecret,
		}
	}
}

// resolveAppleAccountProvisioningSecret returns the client secret to send,
// from whichever of the pair is configured. Fleet stores the three settings
// together, so the secret is sent on every apply that manages the block.
func resolveAppleAccountProvisioningSecret(ctx context.Context, config tfsdk.Config, m *configurationMDMModel, diags *diag.Diagnostics) *string {
	if m == nil || m.AppleAccountProvisioning == nil {
		return nil
	}
	var wo types.String
	diags.Append(config.GetAttribute(ctx, path.Root("mdm").AtName("apple_account_provisioning").AtName("oauth_idp_client_secret_wo"), &wo)...)
	if !wo.IsNull() && !wo.IsUnknown() {
		return new(wo.ValueString())
	}
	if v := optionalStringPtr(m.AppleAccountProvisioning.OAuthIdPClientSecret); v != nil {
		return v
	}
	diags.AddAttributeError(path.Root("mdm").AtName("apple_account_provisioning"), "Missing client secret",
		"Neither oauth_idp_client_secret nor oauth_idp_client_secret_wo resolved to a value at apply time.")
	return nil
}

// refreshConfigurationMDM copies Fleet's values into the attributes that are
// in state. A key the server did not send (an older Fleet) leaves the
// attribute alone; checkConfigurationMDMApplied reports that on apply.
func refreshConfigurationMDM(m *configurationMDMModel, api *fleetdm.MDMConfig, diags *diag.Diagnostics) {
	if m == nil || api == nil {
		return
	}
	m.NameTemplate = refreshOptionalString(m.NameTemplate, &api.NameTemplate)
	if w, a := m.WindowsAutomaticEnrollment, api.WindowsAutomaticEnrollment; w != nil && a != nil {
		w.DefaultFleet = refreshOptionalString(w.DefaultFleet, &a.DefaultFleet)
	}
	m.AppleServerURL = refreshOptionalString(m.AppleServerURL, api.AppleServerURL)
	m.EnableRecoveryLockPassword = refreshOptionalBool(m.EnableRecoveryLockPassword, api.EnableRecoveryLockPassword)

	if ms, a := m.MacOSSettings, api.MacOSSettings; ms != nil && a != nil {
		ms.EnableDiskEncryption = refreshOptionalBool(ms.EnableDiskEncryption, a.EnableDiskEncryption)
		ms.EnableEscrowDiskEncryptionKey = refreshOptionalBool(ms.EnableEscrowDiskEncryptionKey, a.EnableEscrowDiskEncryptionKey)
	}
	if ws, a := m.WindowsSettings, api.WindowsSettings; ws != nil && a != nil {
		ws.EnableManagedLocalAccount = refreshOptionalBool(ws.EnableManagedLocalAccount, a.EnableManagedLocalAccount)
		ws.EnableDiskEncryption = refreshOptionalBool(ws.EnableDiskEncryption, a.EnableDiskEncryption)
		ws.RequireBitlockerPIN = refreshOptionalBool(ws.RequireBitlockerPIN, a.RequireBitlockerPIN)
	}
	if ls, a := m.LinuxSettings, api.LinuxSettings; ls != nil && a != nil {
		ls.EnableEscrowDiskEncryptionKey = refreshOptionalBool(ls.EnableEscrowDiskEncryptionKey, a.EnableEscrowDiskEncryptionKey)
	}
	refreshAppleOSUpdates(m.MacOSUpdates, api.MacOSUpdates)
	refreshAppleOSUpdates(m.IOSUpdates, api.IOSUpdates)
	refreshAppleOSUpdates(m.IPadOSUpdates, api.IPadOSUpdates)
	if w, a := m.WindowsUpdates, api.WindowsUpdates; w != nil && a != nil {
		w.DeadlineDays = refreshOptionalInt64(w.DeadlineDays, a.DeadlineDays)
		w.GracePeriodDays = refreshOptionalInt64(w.GracePeriodDays, a.GracePeriodDays)
	}

	m.WindowsMigrationEnabled = refreshOptionalBool(m.WindowsMigrationEnabled, api.WindowsMigrationEnabled)
	m.EnableTurnOnWindowsMDMManually = refreshOptionalBool(m.EnableTurnOnWindowsMDMManually, api.EnableTurnOnWindowsMDMManually)
	if !m.WindowsEntraTenantIDs.IsNull() {
		m.WindowsEntraTenantIDs = stringsToSet(api.WindowsEntraTenantIDs, diags)
	}
	if !m.WindowsEntraClientIDs.IsNull() {
		m.WindowsEntraClientIDs = stringsToSet(api.WindowsEntraClientIDs, diags)
	}
	if mm, a := m.MacOSMigration, api.MacOSMigration; mm != nil && a != nil {
		mm.Enable = refreshOptionalBool(mm.Enable, a.Enable)
		mm.Mode = refreshOptionalString(mm.Mode, a.Mode)
		mm.WebhookURL = refreshOptionalString(mm.WebhookURL, a.WebhookURL)
	}
	if e, a := m.EndUserAuthentication, api.EndUserAuthentication; e != nil && a != nil {
		e.EntityID = refreshOptionalString(e.EntityID, a.EntityID)
		e.IssuerURI = refreshOptionalString(e.IssuerURI, a.IssuerURI)
		e.Metadata = refreshOptionalString(e.Metadata, a.Metadata)
		e.MetadataURL = refreshOptionalString(e.MetadataURL, a.MetadataURL)
		e.IDPName = refreshOptionalString(e.IDPName, a.IDPName)
	}

	m.AppleRequireHardwareAttestation = refreshOptionalBool(m.AppleRequireHardwareAttestation, api.AppleRequireHardwareAttestation)
	m.OnlyAllowAppleBusinessEnrollment = refreshOptionalBool(m.OnlyAllowAppleBusinessEnrollment, api.OnlyAllowAppleBusinessEnrollment)
	// The secret reads back masked and stays as it is in state.
	if ap, a := m.AppleAccountProvisioning, api.AppleAccountProvisioning; ap != nil && a != nil {
		ap.OAuthIdPTokenURL = refreshOptionalString(ap.OAuthIdPTokenURL, a.OAuthIdPTokenURL)
		ap.OAuthIdPClientID = refreshOptionalString(ap.OAuthIdPClientID, a.OAuthIdPClientID)
	}
}

// checkConfigurationMDMApplied turns a managed key the server did not report
// into a named error. Fleet before 4.93 drops keys it does not know, so the
// apply would otherwise look successful.
func checkConfigurationMDMApplied(m *configurationMDMModel, api *fleetdm.MDMConfig, diags *diag.Diagnostics) {
	if m == nil {
		return
	}
	var missing []string
	check := func(name string, managed, present bool) {
		if managed && !present {
			missing = append(missing, name)
		}
	}
	a := api
	if a == nil {
		a = &fleetdm.MDMConfig{}
	}
	check("apple_server_url", !m.AppleServerURL.IsNull(), a.AppleServerURL != nil)
	check("windows_migration_enabled", !m.WindowsMigrationEnabled.IsNull(), a.WindowsMigrationEnabled != nil)
	check("enable_turn_on_windows_mdm_manually", !m.EnableTurnOnWindowsMDMManually.IsNull(), a.EnableTurnOnWindowsMDMManually != nil)
	check("macos_migration", m.MacOSMigration != nil, a.MacOSMigration != nil)
	check("end_user_authentication", m.EndUserAuthentication != nil, a.EndUserAuthentication != nil)
	check("apple_require_hardware_attestation", !m.AppleRequireHardwareAttestation.IsNull(), a.AppleRequireHardwareAttestation != nil)
	check("only_allow_apple_business_enrollment", !m.OnlyAllowAppleBusinessEnrollment.IsNull(), a.OnlyAllowAppleBusinessEnrollment != nil)
	check("apple_account_provisioning", m.AppleAccountProvisioning != nil, a.AppleAccountProvisioning != nil)
	check("macos_settings", m.MacOSSettings != nil, a.MacOSSettings != nil)
	check("windows_settings", m.WindowsSettings != nil, a.WindowsSettings != nil)
	check("linux_settings", m.LinuxSettings != nil, a.LinuxSettings != nil)
	if len(missing) > 0 {
		diags.AddAttributeError(path.Root("mdm"), "MDM settings not supported by this Fleet",
			fmt.Sprintf("Fleet accepted the configuration but did not report %s, so this Fleet version does not support them. "+
				"Remove them from the mdm block or upgrade Fleet.", strings.Join(missing, ", ")))
	}
}

// optionalSetPtr returns a pointer to the set's elements, or nil when the set
// is unmanaged. An empty set yields a pointer to an empty slice, sent as [].
func optionalSetPtr(ctx context.Context, set types.Set, diags *diag.Diagnostics) *[]string {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}
	out := stringSetToSlice(ctx, set, diags)
	return &out
}

// noSurroundingWhitespace rejects leading or trailing whitespace, which Fleet
// trims before storing and would then read back as a permanent diff.
type noSurroundingWhitespace struct{}

func (noSurroundingWhitespace) Description(context.Context) string {
	return "must not have leading or trailing whitespace"
}

func (v noSurroundingWhitespace) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (noSurroundingWhitespace) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if v := req.ConfigValue.ValueString(); strings.TrimSpace(v) != v {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid value",
			"The value has leading or trailing whitespace, which Fleet trims and would show as a permanent diff.")
	}
}

// windowsEnrollmentTogglesValidator mirrors Fleet's 422 when Windows MDM
// migration and manual turn-on are both enabled.
type windowsEnrollmentTogglesValidator struct{}

func (windowsEnrollmentTogglesValidator) Description(context.Context) string {
	return "windows_migration_enabled and enable_turn_on_windows_mdm_manually cannot both be true"
}

func (v windowsEnrollmentTogglesValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (windowsEnrollmentTogglesValidator) ValidateObject(_ context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	attrs := req.ConfigValue.Attributes()
	migration, _ := attrs["windows_migration_enabled"].(types.Bool)
	manual, _ := attrs["enable_turn_on_windows_mdm_manually"].(types.Bool)
	if boolKnownTrue(migration) && boolKnownTrue(manual) {
		resp.Diagnostics.AddAttributeError(req.Path.AtName("enable_turn_on_windows_mdm_manually"), "Conflicting Windows enrollment settings",
			"windows_migration_enabled and enable_turn_on_windows_mdm_manually cannot both be true.")
	}
}

// macOSMigrationValidator mirrors Fleet's checks when migration is enabled:
// a valid mode and a webhook URL are required.
type macOSMigrationValidator struct{}

func (macOSMigrationValidator) Description(context.Context) string {
	return "mode and webhook_url are required when enable is true"
}

func (v macOSMigrationValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (macOSMigrationValidator) ValidateObject(_ context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	attrs := req.ConfigValue.Attributes()
	enable, _ := attrs["enable"].(types.Bool)
	if !boolKnownTrue(enable) {
		return
	}
	for _, name := range []string{"mode", "webhook_url"} {
		v, _ := attrs[name].(types.String)
		if v.IsUnknown() {
			continue
		}
		if v.IsNull() || v.ValueString() == "" {
			resp.Diagnostics.AddAttributeError(req.Path.AtName(name), "Missing required value",
				fmt.Sprintf("%s is required when macos_migration.enable is true.", name))
		}
	}
}

// nullifyWriteOnly clears the write-only secret before the model is saved:
// Terraform requires write-only values to be absent from state.
func (m *configurationMDMModel) nullifyWriteOnly() {
	if m == nil || m.AppleAccountProvisioning == nil {
		return
	}
	m.AppleAccountProvisioning.OAuthIdPClientSecretWO = types.StringNull()
}
