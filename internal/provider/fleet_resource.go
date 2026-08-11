package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.Resource                = &FleetResource{}
	_ resource.ResourceWithImportState = &FleetResource{}
	_ resource.ResourceWithMoveState   = &FleetResource{}
)

// NewFleetResource creates a new fleet resource.
func NewFleetResource() resource.Resource {
	return &FleetResource{}
}

// NewTeamResource creates a deprecated team resource (alias for FleetResource).
func NewTeamResource() resource.Resource {
	return &FleetResource{deprecated: true}
}

// FleetResource defines the resource implementation.
type FleetResource struct {
	client     *fleetdm.Client
	deprecated bool
}

// FleetResourceModel describes the resource data model.
type FleetResourceModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`

	// Host expiry settings
	HostExpiryEnabled types.Bool  `tfsdk:"host_expiry_enabled"`
	HostExpiryWindow  types.Int64 `tfsdk:"host_expiry_window"`

	// MDM Settings
	EnableDiskEncryption types.Bool `tfsdk:"enable_disk_encryption"`

	// Opt-in nested settings blocks. Nil means the practitioner did not declare
	// the block, in which case it is neither sent to nor read back from Fleet.
	// See fleet_resource_settings.go.
	WebhookSettings *fleetWebhookSettingsModel `tfsdk:"webhook_settings"`
	MDM             *fleetMDMModel             `tfsdk:"mdm"`
	Integrations    *fleetIntegrationsModel    `tfsdk:"integrations"`
	Features        *fleetFeaturesModel        `tfsdk:"features"`

	// Computed fields
	UserCount types.Int64 `tfsdk:"user_count"`
	HostCount types.Int64 `tfsdk:"host_count"`
}

// Metadata returns the resource type name.
func (r *FleetResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	if r.deprecated {
		resp.TypeName = req.ProviderTypeName + "_team"
	} else {
		resp.TypeName = req.ProviderTypeName + "_fleet"
	}
}

// Schema defines the schema for the resource.
func (r *FleetResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	deprecationMsg := ""
	if r.deprecated {
		deprecationMsg = "fleetdm_team is deprecated and will be removed in a future version. Use fleetdm_fleet instead (requires Fleet 4.82.0+)."
	}

	resp.Schema = schema.Schema{
		DeprecationMessage: deprecationMsg,
		Description:        "Manages a FleetDM fleet.",
		MarkdownDescription: "Manages a FleetDM fleet.\n\n" +
			"Fleets are available in Fleet Premium and allow you to group hosts and apply specific configurations, policies, and settings to them.\n\n" +
			"The `webhook_settings`, `mdm`, `integrations` and `features` blocks are opt-in: a block you do not declare is neither written to Fleet nor read into state, so this resource can coexist with settings managed in the Fleet UI or through GitOps. " +
			"Within a block, attributes you omit are left alone too, with one exception: inside a declared `webhook_settings` sub-block or inside `integrations.google_calendar`, Fleet replaces the whole object, so every attribute of that sub-block goes on the wire and the ones you omit are sent as zero values. Declare those sub-blocks in full. " +
			"One consequence of the opt-in design is that `terraform import` brings a fleet in with all four blocks null, since which of them you intend to manage is only expressed in your configuration.",
		Attributes: fleetSchemaAttributes(),
	}
}

// fleetSchemaAttributes returns the schema attributes shared between fleetdm_fleet and the
// deprecated fleetdm_team resource. Extracted to allow reuse in MoveState.
func fleetSchemaAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.Int64Attribute{
			Description:         "The unique identifier of the fleet.",
			MarkdownDescription: "The unique identifier of the fleet.",
			Computed:            true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		},
		"name": schema.StringAttribute{
			Description:         "The name of the fleet.",
			MarkdownDescription: "The name of the fleet.",
			Required:            true,
		},
		"description": schema.StringAttribute{
			Description:         "A description of the fleet.",
			MarkdownDescription: "A description of the fleet.",
			Optional:            true,
			Computed:            true,
			Default:             stringdefault.StaticString(""),
		},
		"host_expiry_enabled": schema.BoolAttribute{
			Description:         "Whether host expiry is enabled for this fleet.",
			MarkdownDescription: "Whether host expiry is enabled for this fleet.",
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
			Description: "Whether disk encryption is enforced for hosts in this fleet. " +
				"Unlike the opt-in mdm block, this attribute defaults to false and is written on every apply, so leaving it out of your configuration actively DISABLES disk encryption -- including encryption an operator turned on in the Fleet UI. " +
				"Set it to true explicitly if this fleet should have disk encryption enforced. " +
				"Making it opt-in like the mdm block would change that behaviour and is deferred to the next major version.",
			MarkdownDescription: "Whether disk encryption is enforced for hosts in this fleet.\n\n" +
				"~> **Warning:** unlike the opt-in `mdm` block, this attribute defaults to `false` and is written on every apply, so leaving it out of your configuration actively **disables** disk encryption -- including encryption an operator turned on in the Fleet UI. " +
				"Set it to `true` explicitly if this fleet should have disk encryption enforced. " +
				"Making it opt-in like the `mdm` block would change that behaviour and is deferred to the next major version.",
			Optional: true,
			Computed: true,
			Default:  booldefault.StaticBool(false),
		},
		"webhook_settings": fleetWebhookSettingsAttribute(),
		"mdm":              fleetMDMAttribute(),
		"integrations":     fleetIntegrationsAttribute(),
		"features":         fleetFeaturesAttribute(),
		"user_count": schema.Int64Attribute{
			Description:         "The number of users in the fleet.",
			MarkdownDescription: "The number of users in the fleet.",
			Computed:            true,
		},
		"host_count": schema.Int64Attribute{
			Description:         "The number of hosts in the fleet.",
			MarkdownDescription: "The number of hosts in the fleet.",
			Computed:            true,
		},
	}
}

// MoveState supports moving state from the deprecated fleetdm_team resource to fleetdm_fleet.
func (r *FleetResource) MoveState(ctx context.Context) []resource.StateMover {
	return []resource.StateMover{
		{
			SourceSchema: &schema.Schema{Attributes: fleetSchemaAttributes()},
			StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
				var data FleetResourceModel
				resp.Diagnostics.Append(req.SourceState.Get(ctx, &data)...)
				if !resp.Diagnostics.HasError() {
					resp.Diagnostics.Append(resp.TargetState.Set(ctx, &data)...)
				}
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *FleetResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// Create creates the resource and sets the initial Terraform state.
func (r *FleetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FleetResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating fleet", map[string]interface{}{
		"name": plan.Name.ValueString(),
	})

	// Create the fleet
	createReq := fleetdm.CreateTeamRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}

	team, err := r.client.CreateTeam(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating FleetDM Fleet",
			"Could not create fleet, unexpected error: "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Fleet created", map[string]interface{}{
		"id":   team.ID,
		"name": team.Name,
	})

	// Save the fleet ID to state immediately. If the subsequent settings update
	// fails, the ID is already in state so Terraform won't create a duplicate
	// fleet on the next apply. Use a separate variable so plan retains the
	// original values for the settings update below.
	initialState := FleetResourceModel{
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

	// POST /fleets only accepts name and description, so everything else needs a
	// follow-up PATCH. That PATCH is unconditional: host_expiry_enabled,
	// host_expiry_window and enable_disk_encryption are Computed with defaults,
	// so the plan always carries values for them and there is always something
	// to write.
	updateReq := buildUpdateTeamRequest(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	team, err = r.client.UpdateTeam(ctx, team.ID, updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating FleetDM Fleet Settings",
			"Fleet was created but settings could not be applied (fleet ID saved to state). Run 'terraform apply' again to retry. Error: "+err.Error(),
		)
		return
	}

	// Map final response to model
	r.mapTeamToModel(ctx, team, &plan, &resp.Diagnostics)

	// Save final data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *FleetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FleetResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading fleet", map[string]interface{}{
		"id": state.ID.ValueInt64(),
	})

	// Get the fleet from the API
	team, err := r.client.GetTeam(ctx, state.ID.ValueInt64())
	if err != nil {
		if isNotFound(err) {
			tflog.Warn(ctx, "Fleet not found, removing from state", map[string]interface{}{
				"id": state.ID.ValueInt64(),
			})
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error Reading FleetDM Fleet",
			"Could not read fleet ID "+strconv.FormatInt(state.ID.ValueInt64(), 10)+": "+err.Error(),
		)
		return
	}

	// Map response to model
	r.mapTeamToModel(ctx, team, &state, &resp.Diagnostics)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *FleetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FleetResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating fleet", map[string]interface{}{
		"id":   plan.ID.ValueInt64(),
		"name": plan.Name.ValueString(),
	})

	// Build update request
	updateReq := buildUpdateTeamRequest(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Update the fleet
	team, err := r.client.UpdateTeam(ctx, plan.ID.ValueInt64(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating FleetDM Fleet",
			"Could not update fleet, unexpected error: "+err.Error(),
		)
		return
	}

	// Map response to model
	r.mapTeamToModel(ctx, team, &plan, &resp.Diagnostics)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *FleetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FleetResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting fleet", map[string]interface{}{
		"id": state.ID.ValueInt64(),
	})

	// Delete the fleet
	err := r.client.DeleteTeam(ctx, state.ID.ValueInt64())
	if err != nil {
		// Check if already deleted
		if isNotFound(err) {
			tflog.Warn(ctx, "Fleet already deleted", map[string]interface{}{
				"id": state.ID.ValueInt64(),
			})
			return
		}

		resp.Diagnostics.AddError(
			"Error Deleting FleetDM Fleet",
			"Could not delete fleet, unexpected error: "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Fleet deleted", map[string]interface{}{
		"id": state.ID.ValueInt64(),
	})
}

// buildUpdateTeamRequest assembles the PATCH body from the plan. Shared by
// Create and Update so the two can never drift apart on which attributes make
// it onto the wire.
func buildUpdateTeamRequest(ctx context.Context, plan *FleetResourceModel, diags *diag.Diagnostics) fleetdm.UpdateTeamRequest {
	req := fleetdm.UpdateTeamRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}

	if !plan.HostExpiryEnabled.IsNull() || !plan.HostExpiryWindow.IsNull() {
		req.HostExpirySettings = &fleetdm.HostExpirySettings{
			HostExpiryEnabled: plan.HostExpiryEnabled.ValueBool(),
			HostExpiryWindow:  int(plan.HostExpiryWindow.ValueInt64()),
		}
	}

	req.MDM = buildMDMSettings(plan.EnableDiskEncryption, plan.MDM)
	req.WebhookSettings = buildWebhookSettings(ctx, plan.WebhookSettings, diags)
	req.Integrations = buildIntegrations(plan.Integrations)
	req.Features = buildFeatures(plan.Features)

	return req
}

// mapTeamToModel maps a FleetDM Team to the Terraform model.
func (r *FleetResource) mapTeamToModel(ctx context.Context, team *fleetdm.Team, model *FleetResourceModel, diags *diag.Diagnostics) {
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

	// enable_disk_encryption is Computed with a default, so unlike the nested
	// blocks it always carries a concrete value.
	if team.MDM != nil && team.MDM.EnableDiskEncryption != nil {
		model.EnableDiskEncryption = types.BoolValue(*team.MDM.EnableDiskEncryption)
	} else {
		model.EnableDiskEncryption = types.BoolValue(false)
	}

	refreshWebhookSettings(ctx, model.WebhookSettings, team.WebhookSettings, diags)
	refreshMDM(model.MDM, team.MDM)
	refreshIntegrations(model.Integrations, team.Integrations)
	refreshFeatures(model.Features, team.Features)
}

// ImportState imports an existing resource by ID.
func (r *FleetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Parse the ID from the import command
	id, ok := parseIDFromString(req.ID, "Fleet", &resp.Diagnostics)
	if !ok {
		return
	}

	tflog.Debug(ctx, "Importing fleet", map[string]interface{}{
		"id": id,
	})

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
