package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &FleetMaintainedAppsDataSource{}

// NewFleetMaintainedAppsDataSource creates a new Fleet Maintained Apps data source.
func NewFleetMaintainedAppsDataSource() datasource.DataSource {
	return &FleetMaintainedAppsDataSource{}
}

// FleetMaintainedAppsDataSource defines the data source implementation.
type FleetMaintainedAppsDataSource struct {
	client *fleetdm.Client
}

// FleetMaintainedAppsDataSourceModel describes the data source data model.
type FleetMaintainedAppsDataSourceModel struct {
	TeamID              types.Int64                  `tfsdk:"team_id"`
	FleetMaintainedApps []FleetMaintainedAppModel `tfsdk:"fleet_maintained_apps"`
}

// FleetMaintainedAppModel describes a single Fleet Maintained App in the list.
type FleetMaintainedAppModel struct {
	ID              types.Int64  `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Slug            types.String `tfsdk:"slug"`
	Platform        types.String `tfsdk:"platform"`
	Version         types.String `tfsdk:"version"`
	SoftwareTitleID types.Int64  `tfsdk:"software_title_id"`
}

func (d *FleetMaintainedAppsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_fleet_maintained_apps"
}

func (d *FleetMaintainedAppsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about all Fleet Maintained Apps.",

		Attributes: map[string]schema.Attribute{
			"team_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Filter/annotate by team. If specified, software_title_id will be populated for apps already added to the team.",
			},
			"fleet_maintained_apps": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of all Fleet Maintained Apps.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The Fleet Maintained App ID.",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The app name.",
						},
						"slug": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The app slug.",
						},
						"platform": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The platform (darwin, windows, linux).",
						},
						"version": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The latest version.",
						},
						"software_title_id": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Set if the app is already added to the team.",
						},
					},
				},
			},
		},
	}
}

func (d *FleetMaintainedAppsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *FleetMaintainedAppsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data FleetMaintainedAppsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := optionalIntPtr(data.TeamID)

	apps, err := d.client.ListFleetMaintainedApps(ctx, teamID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list Fleet Maintained Apps: %s", err))
		return
	}

	// Map response to model
	data.FleetMaintainedApps = make([]FleetMaintainedAppModel, len(apps))
	for i, app := range apps {
		model := FleetMaintainedAppModel{
			ID:       types.Int64Value(int64(app.ID)),
			Name:     types.StringValue(app.Name),
			Slug:     types.StringValue(app.Slug),
			Platform: types.StringValue(app.Platform),
			Version:  types.StringValue(app.Version),
		}
		if app.SoftwareTitleID != nil {
			model.SoftwareTitleID = types.Int64Value(int64(*app.SoftwareTitleID))
		} else {
			model.SoftwareTitleID = types.Int64Null()
		}
		data.FleetMaintainedApps[i] = model
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
