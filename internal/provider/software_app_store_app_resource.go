package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                   = &softwareAppStoreAppResource{}
	_ resource.ResourceWithConfigure      = &softwareAppStoreAppResource{}
	_ resource.ResourceWithImportState    = &softwareAppStoreAppResource{}
	_ resource.ResourceWithValidateConfig = &softwareAppStoreAppResource{}
)

// autoUpdateWindowTimeRegex matches a 24-hour HH:MM time of day, the format
// Fleet documents for auto_update_window_start / auto_update_window_end.
// Anchored and strict about the hour range so "24:00" and "9:5" are rejected
// at plan time rather than by Fleet mid-apply.
var autoUpdateWindowTimeRegex = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// NewSoftwareAppStoreAppResource is the constructor registered with the
// provider.
func NewSoftwareAppStoreAppResource() resource.Resource {
	return &softwareAppStoreAppResource{}
}

// softwareAppStoreAppResource manages a VPP (Apple Volume Purchase Program)
// App Store app bound to a Fleet team. Fleet uses a different set of API
// endpoints for these than for user-uploaded packages — there's no
// installer binary to manage, just an Adam ID linking to Apple's catalog.
//
// This is one of three type-specific resources that replace the legacy
// fleetdm_software_package resource.
type softwareAppStoreAppResource struct {
	client *fleetdm.Client
}

// softwareAppStoreAppResourceModel maps the resource schema data. VPP has
// no install_script / uninstall_script / pre_install_query / post_install_script
// (Apple manages the install flow), no package_path / package_s3 / filename
// (there's no installer to upload), and no SHA256.
type softwareAppStoreAppResourceModel struct {
	ID                       types.Int64  `tfsdk:"id"`
	TitleID                  types.Int64  `tfsdk:"title_id"`
	TeamID                   types.Int64  `tfsdk:"team_id"`
	AppStoreID               types.String `tfsdk:"app_store_id"`
	Name                     types.String `tfsdk:"name"`
	Version                  types.String `tfsdk:"version"`
	Platform                 types.String `tfsdk:"platform"`
	DisplayName              types.String `tfsdk:"display_name"`
	SelfService              types.Bool   `tfsdk:"self_service"`
	InstallDuringSetup       types.Bool   `tfsdk:"install_during_setup"`
	AutoUpdateEnabled        types.Bool   `tfsdk:"auto_update_enabled"`
	AutoUpdateWindowStart    types.String `tfsdk:"auto_update_window_start"`
	AutoUpdateWindowEnd      types.String `tfsdk:"auto_update_window_end"`
	Configuration            types.String `tfsdk:"configuration"`
	LabelsIncludeAny         types.List   `tfsdk:"labels_include_any"`
	LabelsExcludeAny         types.List   `tfsdk:"labels_exclude_any"`
	LabelsIncludeAll         types.List   `tfsdk:"labels_include_all"`
	AutomaticInstallPolicies types.List   `tfsdk:"automatic_install_policies"`
}

// Metadata returns the resource type name.
func (r *softwareAppStoreAppResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_app_store_app"
}

// Schema defines the schema for the resource. It's the union of the shared
// software attributes and `app_store_id`. The VPP API ignores install
// scripts and queries, so those attributes are intentionally absent here.
func (r *softwareAppStoreAppResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := softwareCommonSchemaAttributes()
	attrs["app_store_id"] = schema.StringAttribute{
		Description: "The App Store ID (Adam ID) for the VPP app. Required. Changing this forces a new resource.",
		Required:    true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	// platform accepts `android` on this resource in addition to the Apple
	// values, so override the shared description. Fleet's own enum error on
	// POST /software/app_store_apps reads "platform must be one of 'ios',
	// 'ipados', 'darwin', or 'android'".
	attrs["platform"] = schema.StringAttribute{
		Description: "The platform the app targets: `darwin` (macOS, the Fleet default), `ios`, `ipados`, or `android` " +
			"(Google Play, Fleet 4.90 or later — requires Android MDM to be enabled on your Fleet server). Computed when omitted.",
		Optional: true,
		Computed: true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
	attrs["auto_update_enabled"] = schema.BoolAttribute{
		Description: "Whether Fleet automatically updates this App Store app on hosts. Currently applies to iOS and " +
			"iPadOS apps. Fleet Premium, Fleet 4.90 or later. " +
			"\n\n" +
			"When set to `true`, both `auto_update_window_start` and `auto_update_window_end` are required — Fleet " +
			"rejects the request with \"Start and end time must both be set\" otherwise. " +
			"\n\n" +
			"Managing this is **opt-in**: omitting the attribute leaves Fleet's current setting untouched and keeps " +
			"it out of state, so a value set in the Fleet UI is not fought over. Set it explicitly to manage it " +
			"from Terraform.",
		Optional: true,
	}
	attrs["auto_update_window_start"] = schema.StringAttribute{
		Description: "Start of the daily maintenance window during which automatic updates may run, in the host's " +
			"local time, formatted `HH:MM` on a 24-hour clock (e.g. `\"01:30\"`). Requires `auto_update_enabled` " +
			"and `auto_update_window_end`.",
		Optional: true,
		Validators: []validator.String{
			stringvalidator.RegexMatches(
				autoUpdateWindowTimeRegex,
				"must be a 24-hour time of day formatted HH:MM, e.g. \"01:30\" or \"23:00\"",
			),
			stringvalidator.AlsoRequires(path.Expressions{
				path.MatchRoot("auto_update_enabled"),
				path.MatchRoot("auto_update_window_end"),
			}...),
		},
	}
	attrs["auto_update_window_end"] = schema.StringAttribute{
		Description: "End of the daily maintenance window during which automatic updates may run, in the host's " +
			"local time, formatted `HH:MM` on a 24-hour clock (e.g. `\"04:00\"`). An end time earlier than the " +
			"start time wraps to the next day. Requires `auto_update_enabled` and `auto_update_window_start`.",
		Optional: true,
		Validators: []validator.String{
			stringvalidator.RegexMatches(
				autoUpdateWindowTimeRegex,
				"must be a 24-hour time of day formatted HH:MM, e.g. \"01:30\" or \"23:00\"",
			),
			stringvalidator.AlsoRequires(path.Expressions{
				path.MatchRoot("auto_update_enabled"),
				path.MatchRoot("auto_update_window_start"),
			}...),
		},
	}
	attrs["configuration"] = schema.StringAttribute{
		Description: "The app's managed app configuration, as a raw string. Supported for `ios`, `ipados`, and " +
			"`android` apps only (Fleet ignores it for `darwin`). Fleet Premium, Fleet 4.90 or later. " +
			"\n\n" +
			"The expected format depends on the platform: **XML** for iOS and iPadOS (the managed-configuration " +
			"dictionary), and **JSON** for Android Play Store apps. Supply the payload in that natural form — the " +
			"provider performs whatever encoding Fleet's API requires. Do **not** wrap it in `jsonencode()`. " +
			"Configuration keys vary per app, so consult the app vendor's documentation; the provider does not " +
			"validate the contents beyond requiring the value to be non-empty and, for a value that looks like " +
			"JSON, that it parses. For Android, Fleet accepts only the `managedConfiguration` " +
			"and `workProfileWidgets` keys from Google's " +
			"[application policy](https://developers.google.com/android/management/reference/rest/v1/enterprises.policies#ApplicationPolicy). " +
			"Use `file()` to keep the payload in its own file. Whitespace and JSON key ordering are not significant: " +
			"the provider compares your value against Fleet's semantically, so a differently-formatted response " +
			"does not produce a diff. " +
			"\n\n" +
			"**Do not embed secrets** (license keys, API tokens, pre-shared values) in the payload: the value is " +
			"stored in plaintext in Terraform state and displayed in plan output. Keep secrets in the app vendor's " +
			"server-side configuration where possible. " +
			"\n\n" +
			"Managing this is **opt-in** in the same way as `auto_update_enabled`: omit the attribute to leave " +
			"Fleet's stored configuration alone.",
		Optional: true,
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
		},
	}
	resp.Schema = schema.Schema{
		Description: "Manages a VPP (Apple Volume Purchase Program / App Store) app bound to a Fleet team. " +
			"Use `data.fleetdm_vpp_token` to verify your VPP integration before creating one of these. Fleet Premium only.",
		Attributes: attrs,
	}
}

// Configure injects the API client.
func (r *softwareAppStoreAppResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// ValidateConfig enforces the one rule the schema validators can't express:
// Fleet requires both window bounds when automatic updates are *enabled*, but
// tolerates `auto_update_enabled = false` on its own. AlsoRequires on the two
// window attributes covers the reverse direction (a window needs the flag), and
// is value-blind, so it can't be used to make the windows conditional on the
// flag being true. Catching it here turns Fleet's mid-apply
// "Start and end time must both be set" 4xx into a plan-time error.
func (r *softwareAppStoreAppResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data softwareAppStoreAppResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Unknown values (e.g. driven by another resource's attribute) can't be
	// checked at plan time; Fleet's own validation is the backstop there.
	if data.AutoUpdateEnabled.IsNull() || data.AutoUpdateEnabled.IsUnknown() || !data.AutoUpdateEnabled.ValueBool() {
		return
	}
	for _, attr := range []struct {
		name  string
		value types.String
	}{
		{"auto_update_window_start", data.AutoUpdateWindowStart},
		{"auto_update_window_end", data.AutoUpdateWindowEnd},
	} {
		if attr.value.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root(attr.name),
				"Missing automatic-update window",
				fmt.Sprintf("%s is required when auto_update_enabled is true. Fleet rejects enabling automatic updates without both a window start and a window end.", attr.name),
			)
		}
	}
}

// Create adds the VPP app to the specified team.
func (r *softwareAppStoreAppResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan softwareAppStoreAppResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := 0
	if !plan.TeamID.IsNull() && !plan.TeamID.IsUnknown() {
		teamID = int(plan.TeamID.ValueInt64())
	}

	addReq := &fleetdm.AddAppStoreAppRequest{
		AppStoreID:  plan.AppStoreID.ValueString(),
		TeamID:      teamID,
		Platform:    plan.Platform.ValueString(),
		SelfService: plan.SelfService.ValueBool(),
		DisplayName: plan.DisplayName.ValueString(),
	}

	// Fleet's Add endpoint accepts `configuration`, so the managed app config
	// lands in the same request as the app itself — no follow-up needed. The
	// auto_update_* fields only exist on the Update endpoint and are applied
	// below.
	if !plan.Configuration.IsNull() && !plan.Configuration.IsUnknown() {
		cfg, err := fleetdm.EncodeAppConfiguration(plan.Configuration.ValueString())
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("configuration"), "Invalid app configuration", err.Error())
			return
		}
		addReq.Configuration = cfg
	}

	title, err := r.client.AddAppStoreApp(ctx, addReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error adding VPP app",
			"Could not add App Store app: "+err.Error(),
		)
		return
	}

	plan.ID = types.Int64Value(int64(title.ID))
	plan.TitleID = types.Int64Value(int64(title.ID))
	plan.Name = types.StringValue(title.Name)
	plan.DisplayName = types.StringValue(title.DisplayName)
	plan.Version = types.StringValue("")
	if title.AppStoreApp != nil && title.AppStoreApp.LatestVersion != "" {
		plan.Version = types.StringValue(title.AppStoreApp.LatestVersion)
	} else if len(title.Versions) > 0 {
		plan.Version = types.StringValue(title.Versions[0].Version)
	}
	if title.AppStoreApp != nil && title.AppStoreApp.Platform != "" {
		plan.Platform = types.StringValue(title.AppStoreApp.Platform)
	} else if plan.Platform.IsNull() || plan.Platform.IsUnknown() {
		plan.Platform = types.StringValue("")
	}
	plan.AutomaticInstallPolicies = automaticInstallPoliciesFromTitle(title)

	// Fleet's AddAppStoreApp endpoint accepts neither labels nor the
	// auto_update_* settings. If the user set any of them in HCL, follow up
	// with a single UpdateAppStoreApp call to apply them — otherwise the state
	// would permanently diverge from Fleet (Fleet returns no labels, Read's
	// non-null-state guard keeps the HCL value in state forever).
	needsFollowup := !plan.LabelsIncludeAny.IsNull() || !plan.LabelsExcludeAny.IsNull() || !plan.LabelsIncludeAll.IsNull() ||
		!plan.AutoUpdateEnabled.IsNull() || !plan.AutoUpdateWindowStart.IsNull() || !plan.AutoUpdateWindowEnd.IsNull()
	if needsFollowup {
		tid := 0
		if !plan.TeamID.IsNull() && !plan.TeamID.IsUnknown() {
			tid = int(plan.TeamID.ValueInt64())
		}
		labelReq := &fleetdm.UpdateAppStoreAppRequest{
			TeamID:      tid,
			SelfService: plan.SelfService.ValueBool(),
			DisplayName: plan.DisplayName.ValueString(),
		}
		applyAutoUpdateFields(&plan, labelReq)
		var d diag.Diagnostics
		d = extractLabels(ctx, plan.LabelsIncludeAny, &labelReq.LabelsIncludeAny)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		d = extractLabels(ctx, plan.LabelsExcludeAny, &labelReq.LabelsExcludeAny)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		d = extractLabels(ctx, plan.LabelsIncludeAll, &labelReq.LabelsIncludeAll)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := r.client.UpdateAppStoreApp(ctx, title.ID, labelReq); err != nil {
			resp.Diagnostics.AddError(
				"Error applying labels or automatic-update settings on VPP create",
				"The VPP app was added successfully, but the follow-up call to apply labels / automatic-update settings failed: "+err.Error()+
					". The resource is tracked in state; re-running `terraform apply` will retry.",
			)
			_ = resp.State.Set(ctx, plan)
			return
		}
	}

	// Normalize Unknown → false (Fleet's default for a freshly-added title).
	// See the analogous block in software_custom_package_resource.go.
	if plan.InstallDuringSetup.IsNull() || plan.InstallDuringSetup.IsUnknown() {
		plan.InstallDuringSetup = types.BoolValue(false)
	}
	preFlipPlan := plan
	preDiags := resp.State.Set(ctx, preFlipPlan)
	resp.Diagnostics.Append(preDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Post-create: route install_during_setup via PUT /setup_experience/software.
	if plan.InstallDuringSetup.ValueBool() {
		if err := r.client.SetSetupExperienceSoftwareInclude(ctx, optionalIntPtr(plan.TeamID), plan.Platform.ValueString(), title.ID); err != nil {
			resp.Diagnostics.AddError(
				"Error setting install_during_setup",
				"The VPP app was added successfully but enabling install_during_setup failed: "+err.Error()+
					". The resource is tracked in state; re-running `terraform apply` will retry the flip.",
			)
			return
		}
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// applyAutoUpdateFields copies the three auto_update_* plan values onto an
// UpdateAppStoreAppRequest, leaving the request's pointers nil for attributes
// the user didn't set. nil means "omitted from the JSON body", which Fleet
// reads as "leave the current setting alone" — that's what makes managing
// these attributes opt-in and keeps the request wire-compatible with Fleet
// versions before 4.90. Shared by Create's follow-up PATCH and Update.
func applyAutoUpdateFields(plan *softwareAppStoreAppResourceModel, req *fleetdm.UpdateAppStoreAppRequest) {
	if !plan.AutoUpdateEnabled.IsNull() && !plan.AutoUpdateEnabled.IsUnknown() {
		req.AutoUpdateEnabled = plan.AutoUpdateEnabled.ValueBoolPointer()
	}
	if !plan.AutoUpdateWindowStart.IsNull() && !plan.AutoUpdateWindowStart.IsUnknown() {
		req.AutoUpdateWindowStart = plan.AutoUpdateWindowStart.ValueStringPointer()
	}
	if !plan.AutoUpdateWindowEnd.IsNull() && !plan.AutoUpdateWindowEnd.IsUnknown() {
		req.AutoUpdateWindowEnd = plan.AutoUpdateWindowEnd.ValueStringPointer()
	}
}

// Read refreshes state from Fleet. Verifies the title is actually a VPP app
// before mapping fields — a user who imports a custom-package or FMA title
// into this resource gets a clear error instead of silent state corruption.
func (r *softwareAppStoreAppResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state softwareAppStoreAppResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	titleID := int(state.TitleID.ValueInt64())
	teamID := optionalIntPtr(state.TeamID)

	title, err := r.client.GetSoftwareTitle(ctx, titleID, teamID)
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading VPP app",
			"Could not read software title: "+err.Error(),
		)
		return
	}
	if title == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	if title.AppStoreApp == nil {
		// Two scenarios:
		//   1. Fresh import: a custom-package / FMA title got imported into
		//      this resource by mistake. Prior state's Name is null
		//      (ImportState only sets id/title_id/team_id; Create always
		//      sets Name). Fail loudly so the user can correct the
		//      resource type.
		//   2. Previously-managed resource: this VPP title was destroyed
		//      out of band and Fleet reused the ID for a non-VPP title.
		//      RemoveResource so the next apply can recreate.
		if state.Name.IsNull() {
			resp.Diagnostics.AddError(
				"Wrong software type",
				fmt.Sprintf("title %d is not a VPP/App Store app; use fleetdm_software_custom_package or fleetdm_software_fleet_maintained_app instead", titleID),
			)
			return
		}
		resp.State.RemoveResource(ctx)
		return
	}

	app := title.AppStoreApp
	state.Name = types.StringValue(title.Name)
	state.DisplayName = types.StringValue(title.DisplayName)
	if app.LatestVersion != "" {
		state.Version = types.StringValue(app.LatestVersion)
	} else if len(title.Versions) > 0 {
		state.Version = types.StringValue(title.Versions[0].Version)
	}
	if app.Platform != "" {
		state.Platform = types.StringValue(app.Platform)
	}
	state.AppStoreID = types.StringValue(app.AdamID)
	state.SelfService = types.BoolValue(app.SelfService)
	if app.InstallDuringSetup != nil {
		state.InstallDuringSetup = types.BoolValue(*app.InstallDuringSetup)
	}
	state.AutomaticInstallPolicies = automaticInstallPoliciesFromTitle(title)
	if app.LabelsIncludeAny != nil && !state.LabelsIncludeAny.IsNull() {
		state.LabelsIncludeAny = labelsToStringListValue(app.LabelsIncludeAny)
	}
	if app.LabelsExcludeAny != nil && !state.LabelsExcludeAny.IsNull() {
		state.LabelsExcludeAny = labelsToStringListValue(app.LabelsExcludeAny)
	}
	if app.LabelsIncludeAll != nil && !state.LabelsIncludeAll.IsNull() {
		state.LabelsIncludeAll = labelsToStringListValue(app.LabelsIncludeAll)
	}
	// The 4.90 additions all follow the opt-in convention used by
	// install_during_setup (software_common_schema.go): refresh only when
	// Terraform is already managing the attribute, so values set in the Fleet
	// UI on a resource whose HCL omits them never materialize into state.
	//
	// Fleet returns the automatic-update settings at the *title* level rather
	// than inside app_store_app.
	//
	// auto_update_enabled maps an absent value to false, mirroring the
	// pinned_version absent → "" convention. Fleet's response struct tags the
	// field `omitempty`, so a title whose automatic updates were switched off in
	// the Fleet UI can come back with the key missing rather than set to false.
	// Preserving state on nil would make that specific transition — the one
	// users most need to see — permanently invisible to drift detection.
	if !state.AutoUpdateEnabled.IsNull() {
		if title.AutoUpdateEnabled != nil {
			state.AutoUpdateEnabled = types.BoolValue(*title.AutoUpdateEnabled)
		} else {
			state.AutoUpdateEnabled = types.BoolValue(false)
		}
	}
	// The window bounds deliberately do NOT map absent → "". Fleet documents
	// them as "only applicable when viewing a title in the context of a team",
	// so absence has meanings other than "cleared", and inventing a "" here
	// would manufacture drift on a config that is actually in sync. Whether
	// Fleet keeps echoing the bounds once automatic updates are disabled is
	// unverified — VPP needs a real Apple token — so this stays conservative:
	// a present value is adopted, an absent one leaves state alone.
	if !state.AutoUpdateWindowStart.IsNull() && title.AutoUpdateWindowStart != nil {
		state.AutoUpdateWindowStart = types.StringValue(*title.AutoUpdateWindowStart)
	}
	if !state.AutoUpdateWindowEnd.IsNull() && title.AutoUpdateWindowEnd != nil {
		state.AutoUpdateWindowEnd = types.StringValue(*title.AutoUpdateWindowEnd)
	}
	// Adopt Fleet's echoed configuration only when it actually differs in
	// meaning. A byte comparison would be wrong in both directions: the stored
	// value may be written in a different but equivalent form (pre-encoded XML,
	// or JSON with different key order/whitespace than Fleet returns), and
	// overwriting it with the echo would leave a diff that no apply can settle.
	// See SameAppConfiguration.
	if !state.Configuration.IsNull() && len(app.Configuration) > 0 {
		echoed := fleetdm.DecodeAppConfiguration(app.Configuration)
		if !fleetdm.SameAppConfiguration(state.Configuration.ValueString(), echoed) {
			state.Configuration = types.StringValue(echoed)
		}
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update sends a PATCH to Fleet's app_store_apps endpoint. self_service,
// display_name, and labels are the only updatable fields.
func (r *softwareAppStoreAppResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan softwareAppStoreAppResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	titleID := int(plan.TitleID.ValueInt64())
	tid := 0
	if !plan.TeamID.IsNull() && !plan.TeamID.IsUnknown() {
		tid = int(plan.TeamID.ValueInt64())
	}

	var state softwareAppStoreAppResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := &fleetdm.UpdateAppStoreAppRequest{
		TeamID:      tid,
		SelfService: plan.SelfService.ValueBool(),
		DisplayName: plan.DisplayName.ValueString(),
	}
	applyAutoUpdateFields(&plan, updateReq)
	if !plan.Configuration.IsNull() && !plan.Configuration.IsUnknown() {
		cfg, err := fleetdm.EncodeAppConfiguration(plan.Configuration.ValueString())
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("configuration"), "Invalid app configuration", err.Error())
			return
		}
		updateReq.Configuration = cfg
	}

	// UpdateAppStoreAppRequest is JSON-encoded with no `omitempty` on the
	// label fields, so a nil slice serializes as `null` (Fleet treats as
	// "no change") and an empty slice as `[]` (Fleet treats as "clear").
	// See the convention documented on UpdatePolicyRequest in policies.go.
	var d diag.Diagnostics
	d = extractLabels(ctx, plan.LabelsIncludeAny, &updateReq.LabelsIncludeAny)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	d = extractLabels(ctx, plan.LabelsExcludeAny, &updateReq.LabelsExcludeAny)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	d = extractLabels(ctx, plan.LabelsIncludeAll, &updateReq.LabelsIncludeAll)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateAppStoreApp(ctx, titleID, updateReq); err != nil {
		resp.Diagnostics.AddError(
			"Error updating VPP app",
			"Could not update App Store app: "+err.Error(),
		)
		return
	}

	// Carry over Computed attributes that the PATCH path doesn't refresh.
	if plan.AutomaticInstallPolicies.IsUnknown() {
		plan.AutomaticInstallPolicies = state.AutomaticInstallPolicies
	}
	if plan.DisplayName.IsUnknown() {
		plan.DisplayName = state.DisplayName
	}

	// install_during_setup diff routes through the separate
	// PUT /setup_experience/software endpoint.
	if !plan.InstallDuringSetup.Equal(state.InstallDuringSetup) {
		teamPtr := optionalIntPtr(plan.TeamID)
		if plan.InstallDuringSetup.ValueBool() {
			if err := r.client.SetSetupExperienceSoftwareInclude(ctx, teamPtr, plan.Platform.ValueString(), titleID); err != nil {
				resp.Diagnostics.AddError("Error enabling install_during_setup", err.Error())
				return
			}
		} else {
			if err := r.client.SetSetupExperienceSoftwareExclude(ctx, teamPtr, plan.Platform.ValueString(), titleID); err != nil {
				resp.Diagnostics.AddError("Error disabling install_during_setup", err.Error())
				return
			}
		}
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete removes the VPP app from the team.
func (r *softwareAppStoreAppResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state softwareAppStoreAppResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	titleID := int(state.TitleID.ValueInt64())
	teamID := optionalIntPtr(state.TeamID)

	// VPP titles can be the target of install_software policy automation
	// (Fleet's policies API accepts any software_title_id, VPP included).
	// Patch policies don't apply to VPP, but the shared helper handles the
	// patch list as a no-op when the title has no patch references.
	if diags := detachPoliciesBeforeTitleDelete(ctx, r.client, titleID, teamID); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	err := r.client.DeleteSoftwarePackage(ctx, titleID, teamID)
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError(
			"Error deleting VPP app",
			"Could not delete VPP app: "+err.Error(),
		)
	}
}

// ImportState imports an existing VPP app by ID. Format: `title_id` or
// `title_id:team_id`. The next Read after import populates app_store_id
// from the response and refuses non-VPP titles via the wrong-type guard.
func (r *softwareAppStoreAppResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) < 1 || len(parts) > 2 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be in format: title_id or title_id:team_id",
		)
		return
	}

	titleID, err := strconv.Atoi(parts[0])
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid title ID",
			fmt.Sprintf("Could not parse title ID %q: %s", parts[0], err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), int64(titleID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("title_id"), int64(titleID))...)

	if len(parts) == 2 {
		tid, err := strconv.Atoi(parts[1])
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid team ID",
				fmt.Sprintf("Could not parse team ID %q: %s", parts[1], err.Error()),
			)
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team_id"), int64(tid))...)
	}
}
