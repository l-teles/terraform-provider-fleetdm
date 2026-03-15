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
	_ resource.Resource                = &setupExperienceResource{}
	_ resource.ResourceWithConfigure   = &setupExperienceResource{}
	_ resource.ResourceWithImportState = &setupExperienceResource{}
)

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
	ID                    types.Int64 `tfsdk:"id"`
	TeamID                types.Int64 `tfsdk:"team_id"`
	EnableEndUserAuth     types.Bool  `tfsdk:"enable_end_user_authentication"`
	EnableReleaseManually types.Bool  `tfsdk:"enable_release_device_manually"`
}

// Metadata returns the resource type name.
func (r *setupExperienceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_setup_experience"
}

// Schema defines the schema for the resource.
func (r *setupExperienceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages FleetDM setup experience settings for a team. This is a Premium feature. Setup experience controls the enrollment flow for macOS devices enrolled via DEP.",
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
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *setupExperienceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
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
	enableEndUserAuth := plan.EnableEndUserAuth.ValueBool()
	enableReleaseManually := plan.EnableReleaseManually.ValueBool()

	// Update the setup experience
	updateReq := &fleetdm.UpdateSetupExperienceRequest{
		TeamID:                teamID,
		EnableEndUserAuth:     &enableEndUserAuth,
		EnableReleaseManually: &enableReleaseManually,
	}

	err := r.client.UpdateSetupExperience(ctx, updateReq)
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

	// Update state with read values
	state.EnableEndUserAuth = types.BoolValue(experience.EnableEndUserAuth)
	state.EnableReleaseManually = types.BoolValue(experience.EnableReleaseManually)

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
	enableEndUserAuth := plan.EnableEndUserAuth.ValueBool()
	enableReleaseManually := plan.EnableReleaseManually.ValueBool()

	// Update the setup experience
	updateReq := &fleetdm.UpdateSetupExperienceRequest{
		TeamID:                teamID,
		EnableEndUserAuth:     &enableEndUserAuth,
		EnableReleaseManually: &enableReleaseManually,
	}

	err := r.client.UpdateSetupExperience(ctx, updateReq)
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

	// Reset setup experience to defaults
	enableEndUserAuth := false
	enableReleaseManually := false
	updateReq := &fleetdm.UpdateSetupExperienceRequest{
		TeamID:                teamID,
		EnableEndUserAuth:     &enableEndUserAuth,
		EnableReleaseManually: &enableReleaseManually,
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
