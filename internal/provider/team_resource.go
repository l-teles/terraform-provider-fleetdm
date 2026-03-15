package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &TeamResource{}
	_ resource.ResourceWithImportState = &TeamResource{}
)

// NewTeamResource creates a new team resource.
func NewTeamResource() resource.Resource {
	return &TeamResource{}
}

// TeamResource defines the resource implementation.
type TeamResource struct {
	client *fleetdm.Client
}

// TeamResourceModel describes the resource data model.
type TeamResourceModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`

	// Host expiry settings
	HostExpiryEnabled types.Bool  `tfsdk:"host_expiry_enabled"`
	HostExpiryWindow  types.Int64 `tfsdk:"host_expiry_window"`

	// MDM Settings
	EnableDiskEncryption types.Bool `tfsdk:"enable_disk_encryption"`

	// Computed fields
	UserCount types.Int64 `tfsdk:"user_count"`
	HostCount types.Int64 `tfsdk:"host_count"`
}

// Metadata returns the resource type name.
func (r *TeamResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team"
}

// Schema defines the schema for the resource.
func (r *TeamResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a FleetDM team.",
		MarkdownDescription: `Manages a FleetDM team.

Teams are available in Fleet Premium and allow you to group hosts and apply specific configurations, policies, and settings to them.

## Example Usage

` + "```hcl" + `
resource "fleetdm_team" "workstations" {
  name        = "Workstations"
  description = "All workstation devices"

  host_expiry_enabled = true
  host_expiry_window  = 30

  enable_disk_encryption = true
}
` + "```" + `

## Import

Teams can be imported using the team ID:

` + "```shell" + `
terraform import fleetdm_team.workstations 123
` + "```",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description:         "The unique identifier of the team.",
				MarkdownDescription: "The unique identifier of the team.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description:         "The name of the team.",
				MarkdownDescription: "The name of the team.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				Description:         "A description of the team.",
				MarkdownDescription: "A description of the team.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
			},
			"host_expiry_enabled": schema.BoolAttribute{
				Description:         "Whether host expiry is enabled for this team.",
				MarkdownDescription: "Whether host expiry is enabled for this team.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"host_expiry_window": schema.Int64Attribute{
				Description:         "The number of days after which hosts are considered expired.",
				MarkdownDescription: "The number of days after which hosts are considered expired.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
			},
			"enable_disk_encryption": schema.BoolAttribute{
				Description:         "Whether disk encryption is enforced for hosts in this team.",
				MarkdownDescription: "Whether disk encryption is enforced for hosts in this team.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"user_count": schema.Int64Attribute{
				Description:         "The number of users in the team.",
				MarkdownDescription: "The number of users in the team.",
				Computed:            true,
			},
			"host_count": schema.Int64Attribute{
				Description:         "The number of hosts in the team.",
				MarkdownDescription: "The number of hosts in the team.",
				Computed:            true,
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *TeamResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// Create creates the resource and sets the initial Terraform state.
func (r *TeamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan TeamResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating team", map[string]interface{}{
		"name": plan.Name.ValueString(),
	})

	// Create the team
	createReq := fleetdm.CreateTeamRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}

	team, err := r.client.CreateTeam(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating FleetDM Team",
			"Could not create team, unexpected error: "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Team created", map[string]interface{}{
		"id":   team.ID,
		"name": team.Name,
	})

	// Save the team ID to state immediately. If the subsequent settings update
	// fails, the ID is already in state so Terraform won't create a duplicate
	// team on the next apply. Use a separate variable so plan retains the
	// original values for the settings update below.
	initialState := TeamResourceModel{
		ID:                   types.Int64Value(team.ID),
		Name:                 types.StringValue(team.Name),
		Description:          types.StringValue(team.Description),
		UserCount:            types.Int64Value(int64(team.UserCount)),
		HostCount:            types.Int64Value(int64(team.HostCount)),
		HostExpiryEnabled:    types.BoolValue(false),
		HostExpiryWindow:     types.Int64Value(0),
		EnableDiskEncryption: types.BoolValue(false),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &initialState)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Update the team with additional settings if needed
	needsUpdate := false
	updateReq := fleetdm.UpdateTeamRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}

	if !plan.HostExpiryEnabled.IsNull() || !plan.HostExpiryWindow.IsNull() {
		needsUpdate = true
		updateReq.HostExpirySettings = &fleetdm.HostExpirySettings{
			HostExpiryEnabled: plan.HostExpiryEnabled.ValueBool(),
			HostExpiryWindow:  int(plan.HostExpiryWindow.ValueInt64()),
		}
	}

	if !plan.EnableDiskEncryption.IsNull() {
		needsUpdate = true
		updateReq.MDM = &fleetdm.TeamMDMSettings{
			EnableDiskEncryption: plan.EnableDiskEncryption.ValueBool(),
		}
	}

	if needsUpdate {
		team, err = r.client.UpdateTeam(ctx, team.ID, updateReq)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Updating FleetDM Team Settings",
				"Team was created but settings could not be applied (team ID saved to state). Run 'terraform apply' again to retry. Error: "+err.Error(),
			)
			return
		}
	}

	// Map final response to model
	r.mapTeamToModel(team, &plan)

	// Save final data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *TeamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TeamResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading team", map[string]interface{}{
		"id": state.ID.ValueInt64(),
	})

	// Get the team from the API
	team, err := r.client.GetTeam(ctx, state.ID.ValueInt64())
	if err != nil {
		if isNotFound(err) {
			tflog.Warn(ctx, "Team not found, removing from state", map[string]interface{}{
				"id": state.ID.ValueInt64(),
			})
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error Reading FleetDM Team",
			"Could not read team ID "+strconv.FormatInt(state.ID.ValueInt64(), 10)+": "+err.Error(),
		)
		return
	}

	// Map response to model
	r.mapTeamToModel(team, &state)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *TeamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan TeamResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating team", map[string]interface{}{
		"id":   plan.ID.ValueInt64(),
		"name": plan.Name.ValueString(),
	})

	// Build update request
	updateReq := fleetdm.UpdateTeamRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}

	if !plan.HostExpiryEnabled.IsNull() {
		updateReq.HostExpirySettings = &fleetdm.HostExpirySettings{
			HostExpiryEnabled: plan.HostExpiryEnabled.ValueBool(),
			HostExpiryWindow:  int(plan.HostExpiryWindow.ValueInt64()),
		}
	}

	if !plan.EnableDiskEncryption.IsNull() {
		updateReq.MDM = &fleetdm.TeamMDMSettings{
			EnableDiskEncryption: plan.EnableDiskEncryption.ValueBool(),
		}
	}

	// Update the team
	team, err := r.client.UpdateTeam(ctx, plan.ID.ValueInt64(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating FleetDM Team",
			"Could not update team, unexpected error: "+err.Error(),
		)
		return
	}

	// Map response to model
	r.mapTeamToModel(team, &plan)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *TeamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TeamResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting team", map[string]interface{}{
		"id": state.ID.ValueInt64(),
	})

	// Delete the team
	err := r.client.DeleteTeam(ctx, state.ID.ValueInt64())
	if err != nil {
		// Check if already deleted
		if isNotFound(err) {
			tflog.Warn(ctx, "Team already deleted", map[string]interface{}{
				"id": state.ID.ValueInt64(),
			})
			return
		}

		resp.Diagnostics.AddError(
			"Error Deleting FleetDM Team",
			"Could not delete team, unexpected error: "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Team deleted", map[string]interface{}{
		"id": state.ID.ValueInt64(),
	})
}

// mapTeamToModel maps a FleetDM Team to the Terraform model.
func (r *TeamResource) mapTeamToModel(team *fleetdm.Team, model *TeamResourceModel) {
	model.ID = types.Int64Value(team.ID)
	model.Name = types.StringValue(team.Name)
	model.Description = types.StringValue(team.Description)
	model.UserCount = types.Int64Value(int64(team.UserCount))
	model.HostCount = types.Int64Value(int64(team.HostCount))

	if team.HostExpirySettings != nil {
		model.HostExpiryEnabled = types.BoolValue(team.HostExpirySettings.HostExpiryEnabled)
		model.HostExpiryWindow = types.Int64Value(int64(team.HostExpirySettings.HostExpiryWindow))
	} else {
		model.HostExpiryEnabled = types.BoolValue(false)
		model.HostExpiryWindow = types.Int64Value(0)
	}

	if team.MDM != nil {
		model.EnableDiskEncryption = types.BoolValue(team.MDM.EnableDiskEncryption)
	} else {
		model.EnableDiskEncryption = types.BoolValue(false)
	}
}

// ImportState imports an existing resource by ID.
func (r *TeamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Parse the ID from the import command
	id, ok := parseIDFromString(req.ID, "Team", &resp.Diagnostics)
	if !ok {
		return
	}

	tflog.Debug(ctx, "Importing team", map[string]interface{}{
		"id": id,
	})

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
