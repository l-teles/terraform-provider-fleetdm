package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &scriptResource{}
	_ resource.ResourceWithConfigure   = &scriptResource{}
	_ resource.ResourceWithImportState = &scriptResource{}
)

// NewScriptResource is a helper function to simplify the provider implementation.
func NewScriptResource() resource.Resource {
	return &scriptResource{}
}

// scriptResource is the resource implementation.
type scriptResource struct {
	client *fleetdm.Client
}

// scriptResourceModel maps the resource schema data.
type scriptResourceModel struct {
	ID        types.Int64  `tfsdk:"id"`
	TeamID    types.Int64  `tfsdk:"team_id"`
	Name      types.String `tfsdk:"name"`
	Content   types.String `tfsdk:"content"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

// Metadata returns the resource type name.
func (r *scriptResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_script"
}

// Schema defines the schema for the resource.
func (r *scriptResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a FleetDM script.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "The unique identifier of the script.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"team_id": schema.Int64Attribute{
				Description: "The ID of the team this script belongs to. If not specified, the script is available for hosts with no team.",
				Optional:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the script file (e.g., 'install-app.sh' or 'configure.ps1').",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"content": schema.StringAttribute{
				Description: "The content of the script.",
				Required:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "When the script was created.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Description: "When the script was last updated.",
				Computed:    true,
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *scriptResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// Create creates the resource and sets the initial Terraform state.
func (r *scriptResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan scriptResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Prepare team ID
	teamID := optionalIntPtr(plan.TeamID)

	// Create script via API
	script, err := r.client.CreateScript(
		ctx,
		teamID,
		plan.Name.ValueString(),
		[]byte(plan.Content.ValueString()),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating FleetDM Script",
			"Could not create script, unexpected error: "+err.Error(),
		)
		return
	}

	// Map response body to schema and populate Computed attribute values
	plan.ID = types.Int64Value(int64(script.ID))
	plan.Name = types.StringValue(script.Name)
	plan.CreatedAt = types.StringValue(script.CreatedAt)
	plan.UpdatedAt = types.StringValue(script.UpdatedAt)

	plan.TeamID = intPtrToInt64(script.TeamID)

	// Set state to fully populated data
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *scriptResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state scriptResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get refreshed script value from FleetDM
	script, err := r.client.GetScript(ctx, int(state.ID.ValueInt64()))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading FleetDM Script",
			"Could not read script ID "+strconv.FormatInt(state.ID.ValueInt64(), 10)+": "+err.Error(),
		)
		return
	}

	// Update state from API response
	state.ID = types.Int64Value(int64(script.ID))
	state.Name = types.StringValue(script.Name)
	state.CreatedAt = types.StringValue(script.CreatedAt)
	state.UpdatedAt = types.StringValue(script.UpdatedAt)

	state.TeamID = intPtrToInt64(script.TeamID)

	// Fetch script content via alt=media endpoint
	content, err := r.client.GetScriptContent(ctx, int(state.ID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddWarning(
			"Unable to Read Script Content",
			"Could not read script content for ID "+strconv.FormatInt(state.ID.ValueInt64(), 10)+": "+err.Error(),
		)
	} else {
		state.Content = types.StringValue(content)
	}

	// Set refreshed state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *scriptResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan scriptResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Update script via API
	script, err := r.client.UpdateScript(
		ctx,
		int(plan.ID.ValueInt64()),
		plan.Name.ValueString(),
		[]byte(plan.Content.ValueString()),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating FleetDM Script",
			"Could not update script, unexpected error: "+err.Error(),
		)
		return
	}

	// Update resource state with updated values
	plan.ID = types.Int64Value(int64(script.ID))
	plan.Name = types.StringValue(script.Name)
	plan.CreatedAt = types.StringValue(script.CreatedAt)
	plan.UpdatedAt = types.StringValue(script.UpdatedAt)

	plan.TeamID = intPtrToInt64(script.TeamID)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *scriptResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state scriptResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete existing script
	err := r.client.DeleteScript(ctx, int(state.ID.ValueInt64()))
	if err != nil {
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting FleetDM Script",
			"Could not delete script, unexpected error: "+err.Error(),
		)
		return
	}
}

// ImportState imports an existing resource into Terraform.
func (r *scriptResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Parse the ID from the import identifier
	id, ok := parseIDFromString(req.ID, "Script", &resp.Diagnostics)
	if !ok {
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)

	// Fetch script content via alt=media endpoint.
	// ParseInt uses bitSize=64 so the cast to int is safe for any valid script ID.
	content, err := r.client.GetScriptContent(ctx, int(id)) // #nosec G115 -- script IDs are small positive integers
	if err != nil {
		resp.Diagnostics.AddWarning(
			"Unable to Import Script Content",
			"Could not read script content for ID "+strconv.FormatInt(id, 10)+": "+err.Error()+". Content will be empty.",
		)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("content"), "")...)
	} else {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("content"), content)...)
	}
}
