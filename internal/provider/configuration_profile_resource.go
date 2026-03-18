package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &ConfigurationProfileResource{}
	_ resource.ResourceWithImportState = &ConfigurationProfileResource{}
)

// NewConfigurationProfileResource creates a new configuration profile resource.
func NewConfigurationProfileResource() resource.Resource {
	return &ConfigurationProfileResource{}
}

// ConfigurationProfileResource defines the resource implementation.
type ConfigurationProfileResource struct {
	client *fleetdm.Client
}

// ConfigurationProfileResourceModel describes the resource data model.
type ConfigurationProfileResourceModel struct {
	ProfileUUID      types.String `tfsdk:"profile_uuid"`
	TeamID           types.Int64  `tfsdk:"team_id"`
	DisplayName      types.String `tfsdk:"display_name"`
	ProfileContent   types.String `tfsdk:"profile_content"`
	Name             types.String `tfsdk:"name"`
	Platform         types.String `tfsdk:"platform"`
	Identifier       types.String `tfsdk:"identifier"`
	Checksum         types.String `tfsdk:"checksum"`
	LabelsIncludeAll types.List   `tfsdk:"labels_include_all"`
	LabelsIncludeAny types.List   `tfsdk:"labels_include_any"`
	LabelsExcludeAny types.List   `tfsdk:"labels_exclude_any"`
	CreatedAt        types.String `tfsdk:"created_at"`
	UploadedAt       types.String `tfsdk:"uploaded_at"`
}

// Metadata returns the resource type name.
func (r *ConfigurationProfileResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_configuration_profile"
}

// Schema defines the schema for the resource.
func (r *ConfigurationProfileResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a FleetDM MDM configuration profile.",
		MarkdownDescription: `Manages a FleetDM MDM configuration profile.

Configuration profiles are used to configure settings on macOS and Windows devices enrolled in FleetDM's MDM.

~> **Note:** Configuration profiles cannot be modified after creation. Any changes to the profile content will force recreation of the resource.

## Example Usage

### macOS Profile

` + "```hcl" + `
resource "fleetdm_configuration_profile" "disable_bluetooth" {
  team_id = fleetdm_team.workstations.id

  profile_content = file("${path.module}/profiles/disable_bluetooth.mobileconfig")
}
` + "```" + `

### Windows Profile with Display Name

` + "```hcl" + `
resource "fleetdm_configuration_profile" "bitlocker" {
  team_id      = fleetdm_team.workstations.id
  display_name = "BitLocker Policy"

  profile_content = file("${path.module}/profiles/bitlocker-policy.xml")
}
` + "```" + `

### Profile with Label Targeting

` + "```hcl" + `
resource "fleetdm_configuration_profile" "vpn_config" {
  team_id = fleetdm_team.workstations.id

  profile_content = file("${path.module}/profiles/vpn.mobileconfig")

  labels_include_all = ["Remote Workers", "VPN Required"]
}
` + "```" + `

## Import

Configuration profiles can be imported using the profile UUID:

` + "```shell" + `
terraform import fleetdm_configuration_profile.vpn_config abc123-def456-ghi789
` + "```",

		Attributes: map[string]schema.Attribute{
			"profile_uuid": schema.StringAttribute{
				Description:         "The unique identifier (UUID) of the configuration profile.",
				MarkdownDescription: "The unique identifier (UUID) of the configuration profile.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"team_id": schema.Int64Attribute{
				Description:         "The ID of the team this profile belongs to. Use 0 or omit for 'No team'.",
				MarkdownDescription: "The ID of the team this profile belongs to. Use `0` or omit for 'No team'.",
				Optional:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				Description:         "The display name for the profile. For Windows profiles, this controls the profile name shown in Fleet (derived from the upload filename). For macOS profiles, the name is extracted from PayloadDisplayName in the XML content and this field is informational only.",
				MarkdownDescription: "The display name for the profile. For Windows profiles, this controls the profile name shown in Fleet (derived from the upload filename). For macOS profiles, the name is extracted from `PayloadDisplayName` in the XML content and this field is informational only.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"profile_content": schema.StringAttribute{
				Description:         "The content of the configuration profile (mobileconfig XML for macOS, or XML for Windows).",
				MarkdownDescription: "The content of the configuration profile (mobileconfig XML for macOS, or XML for Windows).",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description:         "The name of the profile (extracted from the profile content).",
				MarkdownDescription: "The name of the profile (extracted from the profile content).",
				Computed:            true,
			},
			"platform": schema.StringAttribute{
				Description:         "The platform this profile targets (darwin, windows).",
				MarkdownDescription: "The platform this profile targets (`darwin`, `windows`).",
				Computed:            true,
			},
			"identifier": schema.StringAttribute{
				Description:         "The identifier of the profile (extracted from the profile content, macOS only).",
				MarkdownDescription: "The identifier of the profile (extracted from the profile content, macOS only).",
				Computed:            true,
			},
			"checksum": schema.StringAttribute{
				Description:         "The checksum of the profile content.",
				MarkdownDescription: "The checksum of the profile content.",
				Computed:            true,
			},
			"labels_include_all": schema.ListAttribute{
				Description:         "Labels that hosts must have ALL of to receive this profile.",
				MarkdownDescription: "Labels that hosts must have **ALL** of to receive this profile.",
				Optional:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"labels_include_any": schema.ListAttribute{
				Description:         "Labels where hosts must have ANY of to receive this profile.",
				MarkdownDescription: "Labels where hosts must have **ANY** of to receive this profile.",
				Optional:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"labels_exclude_any": schema.ListAttribute{
				Description:         "Labels where hosts with ANY of these will NOT receive this profile.",
				MarkdownDescription: "Labels where hosts with **ANY** of these will **NOT** receive this profile.",
				Optional:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"created_at": schema.StringAttribute{
				Description:         "The timestamp when the profile was created.",
				MarkdownDescription: "The timestamp when the profile was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uploaded_at": schema.StringAttribute{
				Description:         "The timestamp when the profile was last uploaded.",
				MarkdownDescription: "The timestamp when the profile was last uploaded.",
				Computed:            true,
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *ConfigurationProfileResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// Create creates the resource and sets the initial Terraform state.
func (r *ConfigurationProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ConfigurationProfileResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating configuration profile")

	// Build the create request
	profileContent := []byte(plan.ProfileContent.ValueString())
	ext := fleetdm.ProfileExtensionFromContent(profileContent)

	filename := "profile" + ext
	if !plan.DisplayName.IsNull() && !plan.DisplayName.IsUnknown() && plan.DisplayName.ValueString() != "" {
		filename = plan.DisplayName.ValueString() + ext
	}

	createReq := &fleetdm.CreateConfigProfileRequest{
		Profile:  profileContent,
		Filename: filename,
	}

	// Set team ID if provided
	createReq.TeamID = optionalIntPtr(plan.TeamID)

	// Set labels_include_all
	if !plan.LabelsIncludeAll.IsNull() {
		var labels []string
		resp.Diagnostics.Append(plan.LabelsIncludeAll.ElementsAs(ctx, &labels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createReq.LabelsIncludeAll = labels
	}

	// Set labels_include_any
	if !plan.LabelsIncludeAny.IsNull() {
		var labels []string
		resp.Diagnostics.Append(plan.LabelsIncludeAny.ElementsAs(ctx, &labels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createReq.LabelsIncludeAny = labels
	}

	// Set labels_exclude_any
	if !plan.LabelsExcludeAny.IsNull() {
		var labels []string
		resp.Diagnostics.Append(plan.LabelsExcludeAny.ElementsAs(ctx, &labels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createReq.LabelsExcludeAny = labels
	}

	profile, err := r.client.CreateConfigProfile(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating FleetDM Configuration Profile",
			"Could not create configuration profile, unexpected error: "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Configuration profile created", map[string]interface{}{
		"profile_uuid": profile.ProfileUUID,
		"name":         profile.Name,
	})

	// Map response to model
	r.mapProfileToModel(ctx, profile, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *ConfigurationProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ConfigurationProfileResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading configuration profile", map[string]interface{}{
		"profile_uuid": state.ProfileUUID.ValueString(),
	})

	profile, err := r.client.GetMDMConfigProfile(ctx, state.ProfileUUID.ValueString())
	if err != nil {
		if isNotFound(err) {
			tflog.Warn(ctx, "Configuration profile not found, removing from state", map[string]interface{}{
				"profile_uuid": state.ProfileUUID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error Reading FleetDM Configuration Profile",
			"Could not read configuration profile "+state.ProfileUUID.ValueString()+": "+err.Error(),
		)
		return
	}

	r.mapProfileToModel(ctx, profile, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Fetch profile content via alt=media endpoint
	content, err := r.client.GetConfigProfileContent(ctx, state.ProfileUUID.ValueString())
	if err != nil {
		resp.Diagnostics.AddWarning(
			"Unable to Read Profile Content",
			"Could not read profile content for "+state.ProfileUUID.ValueString()+": "+err.Error(),
		)
	} else {
		state.ProfileContent = types.StringValue(content)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates the resource. Configuration profiles cannot be updated, so this
// triggers a replace (handled by RequiresReplace plan modifiers on all attributes).
func (r *ConfigurationProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// This should never be called due to RequiresReplace plan modifiers
	resp.Diagnostics.AddError(
		"Update Not Supported",
		"Configuration profiles cannot be updated. They must be deleted and recreated.",
	)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *ConfigurationProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ConfigurationProfileResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting configuration profile", map[string]interface{}{
		"profile_uuid": state.ProfileUUID.ValueString(),
	})

	err := r.client.DeleteConfigProfile(ctx, state.ProfileUUID.ValueString())
	if err != nil {
		if isNotFound(err) {
			tflog.Warn(ctx, "Configuration profile already deleted", map[string]interface{}{
				"profile_uuid": state.ProfileUUID.ValueString(),
			})
			return
		}

		resp.Diagnostics.AddError(
			"Error Deleting FleetDM Configuration Profile",
			"Could not delete configuration profile, unexpected error: "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Configuration profile deleted", map[string]interface{}{
		"profile_uuid": state.ProfileUUID.ValueString(),
	})
}

// ImportState imports an existing resource by profile UUID.
func (r *ConfigurationProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tflog.Debug(ctx, "Importing configuration profile", map[string]interface{}{
		"profile_uuid": req.ID,
	})

	// Validate that the profile exists
	profile, err := r.client.GetMDMConfigProfile(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing FleetDM Configuration Profile",
			"Could not find configuration profile with UUID "+req.ID+": "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("profile_uuid"), profile.ProfileUUID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("display_name"), profile.Name)...)

	// Fetch profile content via alt=media endpoint
	content, err := r.client.GetConfigProfileContent(ctx, profile.ProfileUUID)
	if err != nil {
		resp.Diagnostics.AddWarning(
			"Unable to Import Profile Content",
			"Could not read profile content for "+profile.ProfileUUID+": "+err.Error()+". Content will be empty.",
		)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("profile_content"), "")...)
	} else {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("profile_content"), content)...)
	}
}

// mapProfileToModel maps an MDMConfigProfile to the Terraform model.
func (r *ConfigurationProfileResource) mapProfileToModel(ctx context.Context, profile *fleetdm.MDMConfigProfile, model *ConfigurationProfileResourceModel, diags *diag.Diagnostics) {
	model.ProfileUUID = types.StringValue(profile.ProfileUUID)
	model.Name = types.StringValue(profile.Name)
	model.Platform = types.StringValue(profile.Platform)

	// Populate display_name from API when not explicitly set by user
	if model.DisplayName.IsNull() || model.DisplayName.IsUnknown() {
		model.DisplayName = types.StringValue(profile.Name)
	}
	model.Identifier = types.StringValue(profile.Identifier)
	model.Checksum = types.StringValue(profile.Checksum)
	model.CreatedAt = types.StringValue(profile.CreatedAt)
	model.UploadedAt = types.StringValue(profile.UploadedAt)

	model.TeamID = intPtrToInt64(profile.TeamID)

	// Map labels
	if len(profile.LabelsIncludeAll) > 0 {
		labelNames := make([]string, len(profile.LabelsIncludeAll))
		for i, l := range profile.LabelsIncludeAll {
			labelNames[i] = l.LabelName
		}
		labelList, d := types.ListValueFrom(ctx, types.StringType, labelNames)
		diags.Append(d...)
		model.LabelsIncludeAll = labelList
	} else {
		model.LabelsIncludeAll = types.ListNull(types.StringType)
	}

	if len(profile.LabelsIncludeAny) > 0 {
		labelNames := make([]string, len(profile.LabelsIncludeAny))
		for i, l := range profile.LabelsIncludeAny {
			labelNames[i] = l.LabelName
		}
		labelList, d := types.ListValueFrom(ctx, types.StringType, labelNames)
		diags.Append(d...)
		model.LabelsIncludeAny = labelList
	} else {
		model.LabelsIncludeAny = types.ListNull(types.StringType)
	}

	if len(profile.LabelsExcludeAny) > 0 {
		labelNames := make([]string, len(profile.LabelsExcludeAny))
		for i, l := range profile.LabelsExcludeAny {
			labelNames[i] = l.LabelName
		}
		labelList, d := types.ListValueFrom(ctx, types.StringType, labelNames)
		diags.Append(d...)
		model.LabelsExcludeAny = labelList
	} else {
		model.LabelsExcludeAny = types.ListNull(types.StringType)
	}
}
