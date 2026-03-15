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
var _ datasource.DataSource = &SoftwareTitleDataSource{}

// NewSoftwareTitleDataSource creates a new software title data source.
func NewSoftwareTitleDataSource() datasource.DataSource {
	return &SoftwareTitleDataSource{}
}

// SoftwareTitleDataSource defines the data source implementation.
type SoftwareTitleDataSource struct {
	client *fleetdm.Client
}

// SoftwareTitleDataSourceModel describes the data source data model.
type SoftwareTitleDataSourceModel struct {
	ID               types.String                `tfsdk:"id"`
	TeamID           types.Int64                 `tfsdk:"team_id"`
	Name             types.String                `tfsdk:"name"`
	DisplayName      types.String                `tfsdk:"display_name"`
	Source           types.String                `tfsdk:"source"`
	IconURL          types.String                `tfsdk:"icon_url"`
	HostsCount       types.Int64                 `tfsdk:"hosts_count"`
	VersionsCount    types.Int64                 `tfsdk:"versions_count"`
	BundleIdentifier types.String                `tfsdk:"bundle_identifier"`
	Versions         []SoftwareTitleVersionModel `tfsdk:"versions"`
}

// SoftwareTitleVersionModel represents a software version within a title.
type SoftwareTitleVersionModel struct {
	ID              types.String `tfsdk:"id"`
	Version         types.String `tfsdk:"version"`
	HostsCount      types.Int64  `tfsdk:"hosts_count"`
	Vulnerabilities types.List   `tfsdk:"vulnerabilities"`
}

func (d *SoftwareTitleDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_title"
}

func (d *SoftwareTitleDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about a FleetDM software title.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The unique identifier of the software title.",
			},
			"team_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Filter by team ID.",
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
				MarkdownDescription: "The source of the software (e.g., 'programs', 'apps', 'deb_packages').",
			},
			"icon_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "URL to the software icon.",
			},
			"hosts_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The number of hosts with this software installed.",
			},
			"versions_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The number of versions of this software.",
			},
			"bundle_identifier": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The bundle identifier (for macOS apps).",
			},
			"versions": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of versions for this software title.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The version ID.",
						},
						"version": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The version string.",
						},
						"hosts_count": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The number of hosts with this version.",
						},
						"vulnerabilities": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "List of CVE identifiers for vulnerabilities in this version.",
						},
					},
				},
			},
		},
	}
}

func (d *SoftwareTitleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *SoftwareTitleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SoftwareTitleDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := strconv.Atoi(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid ID", fmt.Sprintf("Unable to parse ID: %s", err))
		return
	}

	teamID := optionalIntPtr(data.TeamID)

	title, err := d.client.GetSoftwareTitle(ctx, id, teamID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to get software title: %s", err))
		return
	}

	// Map response to model
	data.ID = types.StringValue(strconv.Itoa(title.ID))
	data.Name = types.StringValue(title.Name)
	data.DisplayName = types.StringValue(title.DisplayName)
	data.Source = types.StringValue(title.Source)
	data.IconURL = types.StringValue(title.IconURL)
	data.HostsCount = types.Int64Value(int64(title.HostsCount))
	data.VersionsCount = types.Int64Value(int64(title.VersionsCount))
	data.BundleIdentifier = types.StringValue(title.BundleIdentifier)

	// Map versions
	if title.Versions != nil {
		data.Versions = make([]SoftwareTitleVersionModel, len(title.Versions))
		for i, v := range title.Versions {
			vulns := make([]types.String, len(v.Vulnerabilities))
			for j, vuln := range v.Vulnerabilities {
				vulns[j] = types.StringValue(vuln)
			}
			vulnList, diags := types.ListValueFrom(ctx, types.StringType, v.Vulnerabilities)
			resp.Diagnostics.Append(diags...)

			data.Versions[i] = SoftwareTitleVersionModel{
				ID:              types.StringValue(strconv.Itoa(v.ID)),
				Version:         types.StringValue(v.Version),
				HostsCount:      types.Int64Value(int64(v.HostsCount)),
				Vulnerabilities: vulnList,
			}
		}
	} else {
		data.Versions = []SoftwareTitleVersionModel{}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
