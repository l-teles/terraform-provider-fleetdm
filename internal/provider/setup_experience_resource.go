package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                   = &setupExperienceResource{}
	_ resource.ResourceWithConfigure      = &setupExperienceResource{}
	_ resource.ResourceWithImportState    = &setupExperienceResource{}
	_ resource.ResourceWithValidateConfig = &setupExperienceResource{}
)

// setupExperienceOptInNote marks the Fleet 4.90 settings whose opt-in
// semantics are described in the resource description.
const setupExperienceOptInNote = " Opt-in: omitting the attribute leaves Fleet's own value alone."

// NewSetupExperienceResource is a helper function to simplify the provider implementation.
func NewSetupExperienceResource() resource.Resource {
	return &setupExperienceResource{}
}

// setupExperienceResource is the resource implementation.
type setupExperienceResource struct {
	client *fleetdm.Client
}

// setupExperienceResourceModel maps the resource schema data.
type setupExperienceResourceModel struct {
	ID                        types.Int64 `tfsdk:"id"`
	TeamID                    types.Int64 `tfsdk:"team_id"`
	EnableEndUserAuth         types.Bool  `tfsdk:"enable_end_user_authentication"`
	EnableReleaseManually     types.Bool  `tfsdk:"enable_release_device_manually"`
	LockEndUserInfo           types.Bool  `tfsdk:"lock_end_user_info"`
	RequireAllSoftwareMacOS   types.Bool  `tfsdk:"require_all_software_macos"`
	RequireAllSoftwareWindows types.Bool  `tfsdk:"require_all_software_windows"`
	ManualAgentInstall        types.Bool  `tfsdk:"manual_agent_install"`
}

// Metadata returns the resource type name.
func (r *setupExperienceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_setup_experience"
}

// Schema defines the schema for the resource.
func (r *setupExperienceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages FleetDM setup experience settings for a team. This is a Premium feature. " +
			"Setup experience controls the enrollment flow for macOS devices enrolled via DEP.\n\n" +
			"The attributes marked *opt-in* (the settings Fleet added in 4.90) are only sent to Fleet when they " +
			"are set in HCL, and are only tracked in state once set. Omitting one leaves whatever value Fleet " +
			"holds — including a value set in Fleet's UI — untouched, which also keeps this resource usable " +
			"against Fleet versions that predate the setting. Destroying the resource resets only the settings " +
			"Terraform managed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "The unique identifier (same as team_id).",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"team_id": schema.Int64Attribute{
				Description: "The ID of the team to configure setup experience for. Required.",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"enable_end_user_authentication": schema.BoolAttribute{
				Description: "Whether to require end user authentication during device setup. Defaults to false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"enable_release_device_manually": schema.BoolAttribute{
				Description: "Whether to require an admin to manually release the device after setup. Defaults to false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"lock_end_user_info": schema.BoolAttribute{
				Description: "Whether to prevent end users from editing the name and email Fleet collected during " +
					"IdP authentication. Requires `enable_end_user_authentication = true` — Fleet rejects the " +
					"combination otherwise. Requires Fleet 4.90 or later." + setupExperienceOptInNote,
				Optional: true,
			},
			"require_all_software_macos": schema.BoolAttribute{
				Description: "Whether macOS hosts must finish installing every setup-experience software title " +
					"before the device is released. Requires Fleet 4.90 or later." + setupExperienceOptInNote,
				Optional: true,
			},
			"require_all_software_windows": schema.BoolAttribute{
				Description: "Whether Windows hosts must finish installing every setup-experience software title " +
					"before the device is released. Requires Fleet 4.90 or later, and Windows MDM turned on when " +
					"set to `true`." + setupExperienceOptInNote,
				Optional: true,
			},
			"manual_agent_install": schema.BoolAttribute{
				Description: "Whether fleetd is installed by the team's bootstrap package instead of by Fleet " +
					"during Setup Assistant. Fleet rejects `true` unless the team has a bootstrap package and has " +
					"no setup-experience software or script configured. Requires Fleet 4.90 or later." +
					setupExperienceOptInNote,
				Optional: true,
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *setupExperienceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// ValidateConfig rejects lock_end_user_info without end user authentication at
// plan time. Fleet 4.90 returns a 422 for the same combination.
func (r *setupExperienceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config setupExperienceResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.LockEndUserInfo.IsNull() || config.LockEndUserInfo.IsUnknown() || !config.LockEndUserInfo.ValueBool() {
		return
	}
	if config.EnableEndUserAuth.IsUnknown() {
		return
	}
	if !config.EnableEndUserAuth.ValueBool() {
		resp.Diagnostics.AddAttributeError(
			path.Root("lock_end_user_info"),
			"Invalid setup experience configuration",
			"lock_end_user_info can only be enabled when enable_end_user_authentication is set to true.",
		)
	}
}

// setupExperienceUpdateRequest builds the update payload for a plan. The
// Fleet 4.90 settings are only sent when the plan holds a value: Fleet gates
// some of them on presence alone, so sending an unmanaged field would break
// the request on a Fleet without MDM turned on.
func setupExperienceUpdateRequest(teamID int, plan setupExperienceResourceModel) *fleetdm.UpdateSetupExperienceRequest {
	enableEndUserAuth := plan.EnableEndUserAuth.ValueBool()
	enableReleaseManually := plan.EnableReleaseManually.ValueBool()

	return &fleetdm.UpdateSetupExperienceRequest{
		TeamID:                    teamID,
		EnableEndUserAuth:         &enableEndUserAuth,
		EnableReleaseManually:     &enableReleaseManually,
		LockEndUserInfo:           optionalBoolPtr(plan.LockEndUserInfo),
		RequireAllSoftwareMacOS:   optionalBoolPtr(plan.RequireAllSoftwareMacOS),
		RequireAllSoftwareWindows: optionalBoolPtr(plan.RequireAllSoftwareWindows),
		ManualAgentInstall:        optionalBoolPtr(plan.ManualAgentInstall),
	}
}

// readOptionalBool adopts a value Fleet returned only for a setting Terraform
// already manages, keeping omitted attributes null in state.
func readOptionalBool(current types.Bool, remote *bool) types.Bool {
	if current.IsNull() || remote == nil {
		return current
	}
	return types.BoolValue(*remote)
}

// Create creates the resource and sets the initial Terraform state.
func (r *setupExperienceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan setupExperienceResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := int(plan.TeamID.ValueInt64())

	// Update the setup experience
	err := r.client.UpdateSetupExperience(ctx, setupExperienceUpdateRequest(teamID, plan))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating setup experience",
			"Could not update setup experience: "+err.Error(),
		)
		return
	}

	// Update state with computed values
	plan.ID = types.Int64Value(int64(teamID))

	// Set the state
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data.
func (r *setupExperienceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state setupExperienceResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := int(state.TeamID.ValueInt64())

	// Get the setup experience
	experience, err := r.client.GetSetupExperience(ctx, teamID)
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading setup experience",
			"Could not read setup experience: "+err.Error(),
		)
		return
	}

	// Update state with read values. The Fleet 4.90 settings are only adopted
	// when state already holds a value for them, so an omitted attribute keeps
	// tracking Fleet's own value instead of drifting into the plan.
	state.EnableEndUserAuth = types.BoolValue(experience.EnableEndUserAuth)
	state.EnableReleaseManually = types.BoolValue(experience.EnableReleaseManually)
	state.LockEndUserInfo = readOptionalBool(state.LockEndUserInfo, experience.LockEndUserInfo)
	state.RequireAllSoftwareMacOS = readOptionalBool(state.RequireAllSoftwareMacOS, experience.RequireAllSoftwareMacOS)
	state.RequireAllSoftwareWindows = readOptionalBool(state.RequireAllSoftwareWindows, experience.RequireAllSoftwareWindows)
	state.ManualAgentInstall = readOptionalBool(state.ManualAgentInstall, experience.ManualAgentInstall)

	// Set the state
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state.
func (r *setupExperienceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan setupExperienceResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := int(plan.TeamID.ValueInt64())

	// Update the setup experience
	err := r.client.UpdateSetupExperience(ctx, setupExperienceUpdateRequest(teamID, plan))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating setup experience",
			"Could not update setup experience: "+err.Error(),
		)
		return
	}

	// Set the state
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the resource and removes the Terraform state.
func (r *setupExperienceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state setupExperienceResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := int(state.TeamID.ValueInt64())

	// Reset setup experience to defaults, clearing only the Fleet 4.90 settings
	// this resource was managing.
	enableEndUserAuth := false
	enableReleaseManually := false
	cleared := false
	updateReq := &fleetdm.UpdateSetupExperienceRequest{
		TeamID:                teamID,
		EnableEndUserAuth:     &enableEndUserAuth,
		EnableReleaseManually: &enableReleaseManually,
	}
	if !state.LockEndUserInfo.IsNull() {
		updateReq.LockEndUserInfo = &cleared
	}
	if !state.RequireAllSoftwareMacOS.IsNull() {
		updateReq.RequireAllSoftwareMacOS = &cleared
	}
	if !state.RequireAllSoftwareWindows.IsNull() {
		updateReq.RequireAllSoftwareWindows = &cleared
	}
	if !state.ManualAgentInstall.IsNull() {
		updateReq.ManualAgentInstall = &cleared
	}

	err := r.client.UpdateSetupExperience(ctx, updateReq)
	if err != nil {
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error resetting setup experience",
			"Could not reset setup experience: "+err.Error(),
		)
		return
	}
}

// ImportState imports an existing resource by ID.
func (r *setupExperienceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: team_id
	teamID, ok := parseIDFromString(req.ID, "Setup Experience", &resp.Diagnostics)
	if !ok {
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), teamID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team_id"), teamID)...)
}
