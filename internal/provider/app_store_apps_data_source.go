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
var _ datasource.DataSource = &AppStoreAppsDataSource{}

// NewAppStoreAppsDataSource creates a new App Store Apps data source.
func NewAppStoreAppsDataSource() datasource.DataSource {
	return &AppStoreAppsDataSource{}
}

// AppStoreAppsDataSource defines the data source implementation.
type AppStoreAppsDataSource struct {
	client *fleetdm.Client
}

// AppStoreAppsDataSourceModel describes the data source data model.
type AppStoreAppsDataSourceModel struct {
	TeamID       types.Int64        `tfsdk:"team_id"`
	VPPTokenID   types.Int64        `tfsdk:"vpp_token_id"`
	AppStoreApps []AppStoreAppModel `tfsdk:"app_store_apps"`
}

// AppStoreAppModel describes a single App Store app in the list.
type AppStoreAppModel struct {
	AppStoreID    types.String `tfsdk:"app_store_id"`
	Name          types.String `tfsdk:"name"`
	Platform      types.String `tfsdk:"platform"`
	IconURL       types.String `tfsdk:"icon_url"`
	LatestVersion types.String `tfsdk:"latest_version"`
}

func (d *AppStoreAppsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app_store_apps"
}

func (d *AppStoreAppsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve available App Store (VPP) apps for a team.",

		Attributes: map[string]schema.Attribute{
			"team_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The team to list available App Store apps for.",
			},
			"vpp_token_id": schema.Int64Attribute{
				Optional: true,
				MarkdownDescription: "ID of the VPP token to list apps from (see `fleetdm_vpp_tokens`). Fleet 4.95 or later requires it " +
					"when the team has more than one VPP token; omit it to use the team's only token.",
			},
			"app_store_apps": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of available App Store apps.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"app_store_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The App Store ID (Adam ID).",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The app name.",
						},
						"platform": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The platform (darwin, ios, ipados).",
						},
						"icon_url": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "URL to the app icon.",
						},
						"latest_version": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The latest version of the app.",
						},
					},
				},
			},
		},
	}
}

func (d *AppStoreAppsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *AppStoreAppsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data AppStoreAppsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := int(data.TeamID.ValueInt64())

	apps, err := d.client.ListAppStoreApps(ctx, teamID, data.VPPTokenID.ValueInt64Pointer())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list App Store apps: %s", err))
		return
	}

	// Map response to model
	data.AppStoreApps = make([]AppStoreAppModel, len(apps))
	for i, app := range apps {
		data.AppStoreApps[i] = AppStoreAppModel{
			AppStoreID:    types.StringValue(app.AppStoreID),
			Name:          types.StringValue(app.Name),
			Platform:      types.StringValue(app.Platform),
			IconURL:       types.StringValue(app.IconURL),
			LatestVersion: types.StringValue(app.LatestVersion),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
