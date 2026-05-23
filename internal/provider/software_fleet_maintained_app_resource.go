package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &softwareFleetMaintainedAppResource{}
	_ resource.ResourceWithConfigure   = &softwareFleetMaintainedAppResource{}
	_ resource.ResourceWithImportState = &softwareFleetMaintainedAppResource{}
)

// NewSoftwareFleetMaintainedAppResource is the constructor registered with
// the provider. It returns a new resource instance.
func NewSoftwareFleetMaintainedAppResource() resource.Resource {
	return &softwareFleetMaintainedAppResource{}
}

// softwareFleetMaintainedAppResource manages Fleet Maintained Apps —
// Fleet-curated installer recipes that get bound to a team and use Fleet's
// software_package management endpoints once added.
//
// This is one of three type-specific resources that replace the legacy
// fleetdm_software_package resource. The split is documented in the
// "Migrating from fleetdm_software_package" guide.
type softwareFleetMaintainedAppResource struct {
	client *fleetdm.Client
}

// softwareFleetMaintainedAppResourceModel maps the resource schema data.
type softwareFleetMaintainedAppResourceModel struct {
	ID                   types.Int64  `tfsdk:"id"`
	TitleID              types.Int64  `tfsdk:"title_id"`
	TeamID               types.Int64  `tfsdk:"team_id"`
	FleetMaintainedAppID types.Int64  `tfsdk:"fleet_maintained_app_id"`
	Name                 types.String `tfsdk:"name"`
	Version              types.String `tfsdk:"version"`
	Platform             types.String `tfsdk:"platform"`
	InstallScript        types.String `tfsdk:"install_script"`
	UninstallScript      types.String `tfsdk:"uninstall_script"`
	PreInstallQuery      types.String `tfsdk:"pre_install_query"`
	PostInstallScript    types.String `tfsdk:"post_install_script"`
	SelfService          types.Bool   `tfsdk:"self_service"`
	AutomaticInstall     types.Bool   `tfsdk:"automatic_install"`
	LabelsIncludeAny     types.List   `tfsdk:"labels_include_any"`
	LabelsExcludeAny     types.List   `tfsdk:"labels_exclude_any"`
}

// Metadata returns the resource type name.
func (r *softwareFleetMaintainedAppResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_fleet_maintained_app"
}

// Schema defines the schema for the resource. It is the union of the shared
// attributes from softwareCommonSchemaAttributes() and the FMA-specific
// attributes (fleet_maintained_app_id and the script fields).
func (r *softwareFleetMaintainedAppResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := softwareCommonSchemaAttributes()
	for k, v := range softwareScriptAttributes() {
		attrs[k] = v
	}
	attrs["fleet_maintained_app_id"] = schema.Int64Attribute{
		Description: "The Fleet Maintained App ID — the catalog identifier returned by `data.fleetdm_fleet_maintained_app`. Required. Changing this forces a new resource.",
		Required:    true,
		PlanModifiers: []planmodifier.Int64{
			int64planmodifier.RequiresReplace(),
		},
	}
	resp.Schema = schema.Schema{
		Description: "Manages a Fleet Maintained App (FMA) — a Fleet-curated installer recipe bound to a team. Use `data.fleetdm_fleet_maintained_app` to look up `fleet_maintained_app_id`. Fleet Premium only.",
		Attributes:  attrs,
	}
}

// Configure injects the API client.
func (r *softwareFleetMaintainedAppResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// Create adds the Fleet Maintained App to the specified team.
func (r *softwareFleetMaintainedAppResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan softwareFleetMaintainedAppResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := 0
	if !plan.TeamID.IsNull() && !plan.TeamID.IsUnknown() {
		teamID = int(plan.TeamID.ValueInt64())
	}

	addReq := &fleetdm.AddFleetMaintainedAppRequest{
		FleetMaintainedAppID: int(plan.FleetMaintainedAppID.ValueInt64()),
		TeamID:               teamID,
		InstallScript:        plan.InstallScript.ValueString(),
		PreInstallQuery:      plan.PreInstallQuery.ValueString(),
		PostInstallScript:    plan.PostInstallScript.ValueString(),
		SelfService:          plan.SelfService.ValueBool(),
		AutomaticInstall:     plan.AutomaticInstall.ValueBool(),
	}

	var d diag.Diagnostics
	d = extractLabels(ctx, plan.LabelsIncludeAny, &addReq.LabelsIncludeAny)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	d = extractLabels(ctx, plan.LabelsExcludeAny, &addReq.LabelsExcludeAny)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	title, err := r.client.AddFleetMaintainedApp(ctx, addReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error adding Fleet Maintained App",
			"Could not add Fleet Maintained App: "+err.Error(),
		)
		return
	}

	plan.ID = types.Int64Value(int64(title.ID))
	plan.TitleID = types.Int64Value(int64(title.ID))
	plan.Name = types.StringValue(title.Name)
	plan.Version = types.StringValue("")
	if len(title.Versions) > 0 {
		plan.Version = types.StringValue(title.Versions[0].Version)
	}
	if title.SoftwarePackage != nil && title.SoftwarePackage.Platform != "" {
		plan.Platform = types.StringValue(title.SoftwarePackage.Platform)
	} else if plan.Platform.IsNull() || plan.Platform.IsUnknown() {
		plan.Platform = types.StringValue("")
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes state from Fleet. Fleet exposes FMA-managed titles via the
// same SoftwarePackage shape as user-uploaded packages, so this Read is
// structurally identical to a custom-package Read (no fma-specific fields
// to populate). The wrong-type guard only catches VPP titles being imported
// into the wrong resource — Fleet's API doesn't distinguish FMA from
// user-uploaded custom packages in the GET response.
func (r *softwareFleetMaintainedAppResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state softwareFleetMaintainedAppResourceModel
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
			"Error reading Fleet Maintained App",
			"Could not read software title: "+err.Error(),
		)
		return
	}
	if title == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	if title.AppStoreApp != nil {
		// Fresh import vs previously-managed resource — see the VPP
		// resource's Read for the rationale.
		if state.Name.IsNull() || state.Name.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Wrong software type",
				fmt.Sprintf("title %d is a VPP/App Store app; use fleetdm_software_app_store_app instead", titleID),
			)
			return
		}
		resp.State.RemoveResource(ctx)
		return
	}
	if title.SoftwarePackage == nil {
		// Neither a VPP app nor a package-shape response — title was likely
		// deleted out of band; drop from state.
		resp.State.RemoveResource(ctx)
		return
	}

	state.Name = types.StringValue(title.Name)
	if len(title.Versions) > 0 {
		state.Version = types.StringValue(title.Versions[0].Version)
	}
	pkg := title.SoftwarePackage
	if pkg.Platform != "" {
		state.Platform = types.StringValue(pkg.Platform)
	}
	if pkg.InstallScript != "" {
		state.InstallScript = types.StringValue(pkg.InstallScript)
	}
	if pkg.UninstallScript != "" {
		state.UninstallScript = types.StringValue(pkg.UninstallScript)
	}
	if pkg.PreInstallQuery != "" {
		state.PreInstallQuery = types.StringValue(pkg.PreInstallQuery)
	}
	if pkg.PostInstallScript != "" {
		state.PostInstallScript = types.StringValue(pkg.PostInstallScript)
	}
	state.SelfService = types.BoolValue(pkg.SelfService)
	if pkg.InstallDuringSetup != nil {
		state.AutomaticInstall = types.BoolValue(*pkg.InstallDuringSetup)
	}
	if pkg.LabelsIncludeAny != nil && !state.LabelsIncludeAny.IsNull() {
		state.LabelsIncludeAny = labelsToStringListValue(pkg.LabelsIncludeAny)
	}
	if pkg.LabelsExcludeAny != nil && !state.LabelsExcludeAny.IsNull() {
		state.LabelsExcludeAny = labelsToStringListValue(pkg.LabelsExcludeAny)
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update sends a PATCH on Fleet's software_package endpoint. FMA shares this
// endpoint with custom packages — Fleet only differs at the Create call.
func (r *softwareFleetMaintainedAppResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan softwareFleetMaintainedAppResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	titleID := int(plan.TitleID.ValueInt64())
	teamID := optionalIntPtr(plan.TeamID)

	patchReq := &fleetdm.PatchSoftwarePackageRequest{
		TeamID:             teamID,
		InstallScript:      plan.InstallScript.ValueString(),
		UninstallScript:    plan.UninstallScript.ValueString(),
		PreInstallQuery:    plan.PreInstallQuery.ValueString(),
		PostInstallScript:  plan.PostInstallScript.ValueString(),
		SelfService:        plan.SelfService.ValueBool(),
		InstallDuringSetup: plan.AutomaticInstall.ValueBool(),
	}

	var d diag.Diagnostics
	d = extractOptionalLabels(ctx, plan.LabelsIncludeAny, &patchReq.LabelsIncludeAny)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	d = extractOptionalLabels(ctx, plan.LabelsExcludeAny, &patchReq.LabelsExcludeAny)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.PatchSoftwarePackage(ctx, titleID, patchReq); err != nil {
		resp.Diagnostics.AddError(
			"Error updating Fleet Maintained App",
			"Could not update FMA metadata: "+err.Error(),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete removes the FMA-managed software title from the team.
func (r *softwareFleetMaintainedAppResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state softwareFleetMaintainedAppResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	titleID := int(state.TitleID.ValueInt64())
	teamID := optionalIntPtr(state.TeamID)

	err := r.client.DeleteSoftwarePackage(ctx, titleID, teamID)
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError(
			"Error deleting Fleet Maintained App",
			"Could not delete FMA: "+err.Error(),
		)
	}
}

// ImportState imports an existing FMA-managed title by ID. Format:
// `title_id` or `title_id:team_id`. The next Read after import verifies the
// title is not a VPP app (catches the most common mismatch).
func (r *softwareFleetMaintainedAppResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
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

	// fleet_maintained_app_id can't be reconstructed from the title GET
	// (the response doesn't echo the upstream FMA catalog ID). Users must
	// set it in HCL after import; the next plan + apply will reconcile.
}
