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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &softwareAppStoreAppResource{}
	_ resource.ResourceWithConfigure   = &softwareAppStoreAppResource{}
	_ resource.ResourceWithImportState = &softwareAppStoreAppResource{}
)

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
	ID               types.Int64  `tfsdk:"id"`
	TitleID          types.Int64  `tfsdk:"title_id"`
	TeamID           types.Int64  `tfsdk:"team_id"`
	AppStoreID       types.String `tfsdk:"app_store_id"`
	Name             types.String `tfsdk:"name"`
	Version          types.String `tfsdk:"version"`
	Platform         types.String `tfsdk:"platform"`
	SelfService      types.Bool   `tfsdk:"self_service"`
	AutomaticInstall types.Bool   `tfsdk:"automatic_install"`
	LabelsIncludeAny types.List   `tfsdk:"labels_include_any"`
	LabelsExcludeAny types.List   `tfsdk:"labels_exclude_any"`
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

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
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
		//      this resource by mistake. Prior state's name is empty
		//      (ImportState only sets id/title_id/team_id). Fail loudly so
		//      the user can correct the resource type.
		//   2. Previously-managed resource: this VPP title was destroyed
		//      out of band and Fleet reused the ID for a non-VPP title.
		//      RemoveResource so the next apply can recreate.
		if state.Name.IsNull() || state.Name.ValueString() == "" {
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
		state.AutomaticInstall = types.BoolValue(*app.InstallDuringSetup)
	}
	if app.LabelsIncludeAny != nil && !state.LabelsIncludeAny.IsNull() {
		state.LabelsIncludeAny = labelsToStringListValue(app.LabelsIncludeAny)
	}
	if app.LabelsExcludeAny != nil && !state.LabelsExcludeAny.IsNull() {
		state.LabelsExcludeAny = labelsToStringListValue(app.LabelsExcludeAny)
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

	updateReq := &fleetdm.UpdateAppStoreAppRequest{
		TeamID:      tid,
		SelfService: plan.SelfService.ValueBool(),
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

	if err := r.client.UpdateAppStoreApp(ctx, titleID, updateReq); err != nil {
		resp.Diagnostics.AddError(
			"Error updating VPP app",
			"Could not update App Store app: "+err.Error(),
		)
		return
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
