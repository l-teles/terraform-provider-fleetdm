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
var _ datasource.DataSource = &SoftwareVersionsDataSource{}

// NewSoftwareVersionsDataSource creates a new software versions data source.
func NewSoftwareVersionsDataSource() datasource.DataSource {
	return &SoftwareVersionsDataSource{}
}

// SoftwareVersionsDataSource defines the data source implementation.
type SoftwareVersionsDataSource struct {
	client *fleetdm.Client
}

// SoftwareVersionsDataSourceModel describes the data source data model.
type SoftwareVersionsDataSourceModel struct {
	TeamID           types.Int64                `tfsdk:"team_id"`
	Query            types.String               `tfsdk:"query"`
	VulnerableOnly   types.Bool                 `tfsdk:"vulnerable_only"`
	TotalCount       types.Int64                `tfsdk:"total_count"`
	SoftwareVersions []SoftwareVersionListModel `tfsdk:"software_versions"`
}

// SoftwareVersionListModel represents a software version in the list.
type SoftwareVersionListModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Version    types.String `tfsdk:"version"`
	Source     types.String `tfsdk:"source"`
	Vendor     types.String `tfsdk:"vendor"`
	HostsCount types.Int64  `tfsdk:"hosts_count"`
	TitleID    types.Int64  `tfsdk:"title_id"`
}

func (d *SoftwareVersionsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_versions"
}

func (d *SoftwareVersionsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about all FleetDM software versions.",

		Attributes: map[string]schema.Attribute{
			"team_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Filter by team ID.",
			},
			"query": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Search query to filter software by name.",
			},
			"vulnerable_only": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Only return software with known vulnerabilities.",
			},
			"total_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Total count of software versions.",
			},
			"software_versions": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of software versions.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The unique identifier of the software version.",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The name of the software.",
						},
						"version": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The version string.",
						},
						"source": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The source of the software.",
						},
						"vendor": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The software vendor.",
						},
						"hosts_count": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The number of hosts with this version.",
						},
						"title_id": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The ID of the parent software title.",
						},
					},
				},
			},
		},
	}
}

func (d *SoftwareVersionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *SoftwareVersionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SoftwareVersionsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	opts := fleetdm.SoftwareVersionListOptions{}

	opts.TeamID = optionalIntPtr(data.TeamID)
	if !data.Query.IsNull() && !data.Query.IsUnknown() {
		opts.Query = data.Query.ValueString()
	}
	if !data.VulnerableOnly.IsNull() && !data.VulnerableOnly.IsUnknown() {
		opts.VulnerableOnly = data.VulnerableOnly.ValueBool()
	}

	versions, count, err := d.client.ListSoftwareVersions(ctx, opts)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list software versions: %s", err))
		return
	}

	data.TotalCount = types.Int64Value(int64(count))
	data.SoftwareVersions = make([]SoftwareVersionListModel, len(versions))
	for i, v := range versions {
		data.SoftwareVersions[i] = SoftwareVersionListModel{
			ID:         types.StringValue(strconv.Itoa(v.ID)),
			Name:       types.StringValue(v.Name),
			Version:    types.StringValue(v.Version),
			Source:     types.StringValue(v.Source),
			Vendor:     types.StringValue(v.Vendor),
			HostsCount: types.Int64Value(int64(v.HostsCount)),
			TitleID:    types.Int64Value(int64(v.TitleID)),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
