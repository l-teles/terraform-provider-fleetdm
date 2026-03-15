package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &SoftwareTitlesDataSource{}

// NewSoftwareTitlesDataSource creates a new software titles data source.
func NewSoftwareTitlesDataSource() datasource.DataSource {
	return &SoftwareTitlesDataSource{}
}

// SoftwareTitlesDataSource defines the data source implementation.
type SoftwareTitlesDataSource struct {
	client *fleetdm.Client
}

// SoftwareTitlesDataSourceModel describes the data source data model.
type SoftwareTitlesDataSourceModel struct {
	TeamID              types.Int64              `tfsdk:"team_id"`
	Query               types.String             `tfsdk:"query"`
	VulnerableOnly      types.Bool               `tfsdk:"vulnerable_only"`
	AvailableForInstall types.Bool               `tfsdk:"available_for_install"`
	TotalCount          types.Int64              `tfsdk:"total_count"`
	SoftwareTitles      []SoftwareTitleListModel `tfsdk:"software_titles"`
}

// SoftwareTitleListModel represents a software title in the list.
type SoftwareTitleListModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	DisplayName      types.String `tfsdk:"display_name"`
	Source           types.String `tfsdk:"source"`
	HostsCount       types.Int64  `tfsdk:"hosts_count"`
	VersionsCount    types.Int64  `tfsdk:"versions_count"`
	BundleIdentifier types.String `tfsdk:"bundle_identifier"`
}

func (d *SoftwareTitlesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_titles"
}

func (d *SoftwareTitlesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about all FleetDM software titles.",

		Attributes: map[string]schema.Attribute{
			"team_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Filter by team ID.",
			},
			"query": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Search query to filter software titles by name.",
			},
			"vulnerable_only": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Only return software with known vulnerabilities.",
			},
			"available_for_install": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Only return software available for install.",
			},
			"total_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Total count of software titles.",
			},
			"software_titles": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of software titles.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The unique identifier of the software title.",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The name of the software.",
						},
						"display_name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The display name of the software.",
						},
						"source": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The source of the software.",
						},
						"hosts_count": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The number of hosts with this software.",
						},
						"versions_count": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The number of versions of this software.",
						},
						"bundle_identifier": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The bundle identifier (for macOS apps).",
						},
					},
				},
			},
		},
	}
}

func (d *SoftwareTitlesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *SoftwareTitlesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SoftwareTitlesDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	opts := fleetdm.SoftwareTitleListOptions{}

	opts.TeamID = optionalIntPtr(data.TeamID)
	if !data.Query.IsNull() && !data.Query.IsUnknown() {
		opts.Query = data.Query.ValueString()
	}
	if !data.VulnerableOnly.IsNull() && !data.VulnerableOnly.IsUnknown() {
		opts.VulnerableOnly = data.VulnerableOnly.ValueBool()
	}
	if !data.AvailableForInstall.IsNull() && !data.AvailableForInstall.IsUnknown() {
		opts.AvailableForInstall = data.AvailableForInstall.ValueBool()
	}

	titles, count, err := d.client.ListSoftwareTitles(ctx, opts)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list software titles: %s", err))
		return
	}

	data.TotalCount = types.Int64Value(int64(count))
	data.SoftwareTitles = make([]SoftwareTitleListModel, len(titles))
	for i, title := range titles {
		data.SoftwareTitles[i] = SoftwareTitleListModel{
			ID:               types.StringValue(strconv.Itoa(title.ID)),
			Name:             types.StringValue(title.Name),
			DisplayName:      types.StringValue(title.DisplayName),
			Source:           types.StringValue(title.Source),
			HostsCount:       types.Int64Value(int64(title.HostsCount)),
			VersionsCount:    types.Int64Value(int64(title.VersionsCount)),
			BundleIdentifier: types.StringValue(title.BundleIdentifier),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
