package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &ConfigurationProfilesDataSource{}
	_ datasource.DataSourceWithConfigure = &ConfigurationProfilesDataSource{}
)

// NewConfigurationProfilesDataSource is a helper function to simplify the provider implementation.
func NewConfigurationProfilesDataSource() datasource.DataSource {
	return &ConfigurationProfilesDataSource{}
}

// ConfigurationProfilesDataSource is the data source implementation.
type ConfigurationProfilesDataSource struct {
	client *fleetdm.Client
}

// ConfigurationProfilesDataSourceModel describes the data source data model.
type ConfigurationProfilesDataSourceModel struct {
	ID       types.String                `tfsdk:"id"`
	TeamID   types.Int64                 `tfsdk:"team_id"`
	Profiles []ConfigurationProfileModel `tfsdk:"profiles"`
}

// ConfigurationProfileModel describes a single configuration profile.
type ConfigurationProfileModel struct {
	ProfileUUID      types.String        `tfsdk:"profile_uuid"`
	TeamID           types.Int64         `tfsdk:"team_id"`
	Name             types.String        `tfsdk:"name"`
	Platform         types.String        `tfsdk:"platform"`
	Identifier       types.String        `tfsdk:"identifier"`
	Checksum         types.String        `tfsdk:"checksum"`
	CreatedAt        types.String        `tfsdk:"created_at"`
	UploadedAt       types.String        `tfsdk:"uploaded_at"`
	LabelsIncludeAll []ProfileLabelModel `tfsdk:"labels_include_all"`
	LabelsIncludeAny []ProfileLabelModel `tfsdk:"labels_include_any"`
	LabelsExcludeAny []ProfileLabelModel `tfsdk:"labels_exclude_any"`
}

// ProfileLabelModel describes a label associated with a profile.
type ProfileLabelModel struct {
	Name   types.String `tfsdk:"name"`
	ID     types.Int64  `tfsdk:"id"`
	Broken types.Bool   `tfsdk:"broken"`
}

// Metadata returns the data source type name.
func (d *ConfigurationProfilesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_configuration_profiles"
}

// Schema defines the schema for the data source.
func (d *ConfigurationProfilesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a list of MDM configuration profiles from FleetDM.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Placeholder identifier for the data source.",
				Computed:    true,
			},
			"team_id": schema.Int64Attribute{
				Description: "Filter profiles by team ID. Use 0 or omit for global profiles.",
				Optional:    true,
			},
			"profiles": schema.ListNestedAttribute{
				Description: "List of configuration profiles.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"profile_uuid": schema.StringAttribute{
							Description: "The unique identifier of the profile.",
							Computed:    true,
						},
						"team_id": schema.Int64Attribute{
							Description: "The ID of the team the profile belongs to.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The name of the profile.",
							Computed:    true,
						},
						"platform": schema.StringAttribute{
							Description: "The platform (darwin, windows, ios, ipados).",
							Computed:    true,
						},
						"identifier": schema.StringAttribute{
							Description: "The profile identifier (macOS only).",
							Computed:    true,
						},
						"checksum": schema.StringAttribute{
							Description: "The checksum of the profile content.",
							Computed:    true,
						},
						"created_at": schema.StringAttribute{
							Description: "When the profile was created.",
							Computed:    true,
						},
						"uploaded_at": schema.StringAttribute{
							Description: "When the profile was last uploaded.",
							Computed:    true,
						},
						"labels_include_all": schema.ListNestedAttribute{
							Description: "Labels that must all match for the profile to apply.",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: profileLabelSchema(),
							},
						},
						"labels_include_any": schema.ListNestedAttribute{
							Description: "Labels where any match will cause the profile to apply.",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: profileLabelSchema(),
							},
						},
						"labels_exclude_any": schema.ListNestedAttribute{
							Description: "Labels that will exclude the profile if any match.",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: profileLabelSchema(),
							},
						},
					},
				},
			},
		},
	}
}

// profileLabelSchema returns the schema for profile labels.
func profileLabelSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name": schema.StringAttribute{
			Description: "The name of the label.",
			Computed:    true,
		},
		"id": schema.Int64Attribute{
			Description: "The ID of the label.",
			Computed:    true,
		},
		"broken": schema.BoolAttribute{
			Description: "Whether the label is broken (no longer exists).",
			Computed:    true,
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *ConfigurationProfilesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (d *ConfigurationProfilesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state ConfigurationProfilesDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build options
	opts := &fleetdm.ListMDMConfigProfilesOptions{}
	opts.TeamID = optionalIntPtr(state.TeamID)

	// Get profiles from FleetDM
	profiles, err := d.client.ListMDMConfigProfiles(ctx, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FleetDM Configuration Profiles",
			err.Error(),
		)
		return
	}

	// Map response to model
	state.Profiles = make([]ConfigurationProfileModel, len(profiles))
	for i, profile := range profiles {
		state.Profiles[i] = ConfigurationProfileModel{
			ProfileUUID: types.StringValue(profile.ProfileUUID),
			Name:        types.StringValue(profile.Name),
			Platform:    types.StringValue(profile.Platform),
			Identifier:  types.StringValue(profile.Identifier),
			Checksum:    types.StringValue(profile.Checksum),
			CreatedAt:   types.StringValue(profile.CreatedAt),
			UploadedAt:  types.StringValue(profile.UploadedAt),
		}

		state.Profiles[i].TeamID = intPtrToInt64(profile.TeamID)

		// Map labels
		state.Profiles[i].LabelsIncludeAll = mapProfileLabels(profile.LabelsIncludeAll)
		state.Profiles[i].LabelsIncludeAny = mapProfileLabels(profile.LabelsIncludeAny)
		state.Profiles[i].LabelsExcludeAny = mapProfileLabels(profile.LabelsExcludeAny)
	}

	// Set ID
	if !state.TeamID.IsNull() {
		state.ID = types.StringValue(fmt.Sprintf("team-%d", state.TeamID.ValueInt64()))
	} else {
		state.ID = types.StringValue("global")
	}

	// Set state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// mapProfileLabels converts API labels to model labels.
func mapProfileLabels(labels []fleetdm.ConfigurationProfileLabel) []ProfileLabelModel {
	if len(labels) == 0 {
		return nil
	}
	result := make([]ProfileLabelModel, len(labels))
	for i, label := range labels {
		result[i] = ProfileLabelModel{
			Name:   types.StringValue(label.LabelName),
			Broken: types.BoolValue(label.Broken),
		}
		result[i].ID = intPtrToInt64(label.LabelID)
	}
	return result
}
