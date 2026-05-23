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
		DisplayName: plan.DisplayName.ValueString(),
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

	// Fleet's AddAppStoreApp endpoint doesn't accept labels. If the user
	// set any of the three label attributes in HCL, follow up with an
	// UpdateAppStoreApp call to apply them — otherwise the state would
	// permanently diverge from Fleet (Fleet returns no labels, Read's
	// non-null-state guard keeps the HCL value in state forever).
	if !plan.LabelsIncludeAny.IsNull() || !plan.LabelsExcludeAny.IsNull() || !plan.LabelsIncludeAll.IsNull() {
		tid := 0
		if !plan.TeamID.IsNull() && !plan.TeamID.IsUnknown() {
			tid = int(plan.TeamID.ValueInt64())
		}
		labelReq := &fleetdm.UpdateAppStoreAppRequest{
			TeamID:      tid,
			SelfService: plan.SelfService.ValueBool(),
			DisplayName: plan.DisplayName.ValueString(),
		}
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
				"Error applying labels on VPP create",
				"The VPP app was added successfully, but the follow-up call to apply labels failed: "+err.Error()+
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
