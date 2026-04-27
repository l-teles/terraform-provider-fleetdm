package provider

import (
	"context"
	"strings"

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
	_ resource.Resource                   = &ConfigurationProfileResource{}
	_ resource.ResourceWithImportState    = &ConfigurationProfileResource{}
	_ resource.ResourceWithValidateConfig = &ConfigurationProfileResource{}
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
				Description:         "The display name for the profile. Required for Windows (.xml) profiles — controls the profile name shown in Fleet. Must not contain path separators (/ or \\) or file extensions. Only applicable to Windows profiles; for macOS and declaration profiles the name is derived from the profile content.",
				MarkdownDescription: "The display name for the profile. **Required for Windows (`.xml`) profiles** — controls the profile name shown in Fleet. Must not contain path separators (`/` or `\\`) or file extensions. Only applicable to Windows profiles; for macOS and declaration profiles the name is derived from the profile content.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					displayNamePlanModifier{},
				},
			},
			"profile_content": schema.StringAttribute{
				Description:         "The content of the configuration profile (mobileconfig XML for macOS, XML for Windows, or JSON for Apple declarations).",
				MarkdownDescription: "The content of the configuration profile (mobileconfig XML for macOS, XML for Windows, or JSON for Apple declarations).",
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

// ValidateConfig validates the resource configuration.
func (r *ConfigurationProfileResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var profileContent types.String
	var displayName types.String

	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("profile_content"), &profileContent)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("display_name"), &displayName)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Skip validation if values are unknown (e.g. computed or from another resource)
	if profileContent.IsUnknown() || profileContent.IsNull() {
		return
	}

	ext := fleetdm.ProfileExtensionFromContent([]byte(profileContent.ValueString()))

	// display_name is only meaningful for Windows profiles; reject it for other types
	// to avoid perpetual diffs (Read always overwrites from API, Create rejects it)
	if ext != ".xml" && !displayName.IsNull() && !displayName.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("display_name"),
			"display_name is not supported for this profile type",
			"display_name only controls the profile name for Windows (.xml) profiles. "+
				"For macOS and Apple declaration profiles, the name is extracted from the profile content. "+
				"Remove display_name from the configuration.",
		)
		return
	}

	if ext == ".xml" {
		// Skip validation when display_name is unknown (e.g. computed from another resource);
		// the value will be resolved at apply time.
		if displayName.IsUnknown() {
			return
		}
		name := displayName.ValueString()
		if displayName.IsNull() || strings.TrimSpace(name) == "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("display_name"),
				"display_name is required for Windows profiles",
				"Windows XML profiles derive their name from the upload filename. "+
					"Set display_name to a non-empty value to control the profile name shown in Fleet.",
			)
			return
		}
		if strings.TrimSpace(name) != name {
			resp.Diagnostics.AddAttributeError(
				path.Root("display_name"),
				"Invalid display_name",
				"display_name must not have leading or trailing whitespace.",
			)
			return
		}
		if strings.ContainsAny(name, "/\\\r\n") {
			resp.Diagnostics.AddAttributeError(
				path.Root("display_name"),
				"Invalid display_name",
				"display_name must not contain path separators (/ or \\) or newline characters.",
			)
		}
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".xml") || strings.HasSuffix(lower, ".mobileconfig") || strings.HasSuffix(lower, ".json") {
			resp.Diagnostics.AddAttributeError(
				path.Root("display_name"),
				"Invalid display_name",
				"display_name must not include a profile file extension (.xml, .mobileconfig, .json). "+
					"Use just the name (e.g. \"BitLocker Policy\" not \"BitLocker Policy.xml\"). "+
					"The correct extension is added automatically based on the profile content.",
			)
		}
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

	// Apply-time guard: reject display_name for non-Windows profiles
	// (ValidateConfig skips this when display_name is unknown at plan time)
	if ext != ".xml" && !plan.DisplayName.IsNull() && !plan.DisplayName.IsUnknown() {
		resp.Diagnostics.AddError(
			"display_name is not supported for this profile type",
			"display_name only controls the profile name for Windows (.xml) profiles. "+
				"For macOS and Apple declaration profiles, the name is extracted from the profile content. "+
				"Remove display_name from the configuration.",
		)
		return
	}

	// For Windows profiles, use display_name as the upload filename stem.
	// Fleet derives the Windows profile name from the filename (minus extension).
	// For macOS/JSON profiles, display_name is informational — name comes from content.
	filename := "profile" + ext
	if ext == ".xml" {
		displayName := plan.DisplayName.ValueString()
		if plan.DisplayName.IsNull() || plan.DisplayName.IsUnknown() || strings.TrimSpace(displayName) == "" {
			// Apply-time guard: ValidateConfig may have been skipped if profile_content was unknown
			resp.Diagnostics.AddError(
				"display_name is required for Windows profiles",
				"Windows XML profiles derive their name from the upload filename. "+
					"Set display_name to a non-empty value to control the profile name shown in Fleet.",
			)
			return
		}
		// Apply-time guard: ValidateConfig skips when display_name is unknown at plan time,
		// so validate the resolved value here too
		if strings.TrimSpace(displayName) != displayName {
			resp.Diagnostics.AddError(
				"Invalid display_name",
				"display_name must not have leading or trailing whitespace.",
			)
			return
		}
		if strings.ContainsAny(displayName, "/\\\r\n") {
			resp.Diagnostics.AddError(
				"Invalid display_name",
				"display_name must not contain path separators (/ or \\) or newline characters.",
			)
			return
		}
		lower := strings.ToLower(displayName)
		if strings.HasSuffix(lower, ".xml") || strings.HasSuffix(lower, ".mobileconfig") || strings.HasSuffix(lower, ".json") {
			resp.Diagnostics.AddError(
				"Invalid display_name",
				"display_name must not include a profile file extension (.xml, .mobileconfig, .json).",
			)
			return
		}
		filename = displayName + ext
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

	// Only set display_name for Windows profiles — for macOS/declaration profiles
	// it is computed from the profile content and must stay null in state to avoid
	// a perpetual diff (users cannot set it in config).
	if profile.Platform == "windows" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("display_name"), profile.Name)...)
	}

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

	// Only set display_name for Windows profiles where it is user-configurable.
	// For macOS/declaration profiles, display_name is derived from the profile
	// content — setting it in state would cause a perpetual diff because users
	// cannot (and are validated against) setting it in config.
	if profile.Platform == "windows" {
		model.DisplayName = types.StringValue(profile.Name)
	} else {
		model.DisplayName = types.StringNull()
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

// displayNamePlanModifier is a custom plan modifier for display_name that only
// forces replacement for Windows profiles (where it is user-configurable).
// For macOS/declaration profiles, display_name is computed from the profile
// content and should not trigger replacement.
type displayNamePlanModifier struct{}

func (m displayNamePlanModifier) Description(_ context.Context) string {
	return "Requires replacement only when display_name is explicitly configured (Windows profiles)."
}

func (m displayNamePlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m displayNamePlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// If the config value is null, the user hasn't set display_name (macOS profile).
	// Use the state value (or null) and do not force replacement.
	if req.ConfigValue.IsNull() {
		resp.PlanValue = req.StateValue
		return
	}

	// If the config value is unknown (e.g. from another resource), force
	// replacement when state already has a value, since Update is not supported.
	if req.ConfigValue.IsUnknown() {
		if !req.StateValue.IsNull() && !req.StateValue.IsUnknown() {
			resp.RequiresReplace = true
		}
		return
	}

	// Config has an explicit value (Windows profile) — if it changed, force replacement.
	if !req.StateValue.IsNull() && !req.StateValue.IsUnknown() &&
		req.ConfigValue.ValueString() != req.StateValue.ValueString() {
		resp.RequiresReplace = true
	}
}
