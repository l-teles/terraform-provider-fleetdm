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
var _ datasource.DataSource = &SoftwareVersionDataSource{}

// NewSoftwareVersionDataSource creates a new software version data source.
func NewSoftwareVersionDataSource() datasource.DataSource {
	return &SoftwareVersionDataSource{}
}

// SoftwareVersionDataSource defines the data source implementation.
type SoftwareVersionDataSource struct {
	client *fleetdm.Client
}

// SoftwareVersionDataSourceModel describes the data source data model.
type SoftwareVersionDataSourceModel struct {
	ID               types.String                 `tfsdk:"id"`
	TeamID           types.Int64                  `tfsdk:"team_id"`
	Name             types.String                 `tfsdk:"name"`
	Version          types.String                 `tfsdk:"version"`
	Source           types.String                 `tfsdk:"source"`
	BundleIdentifier types.String                 `tfsdk:"bundle_identifier"`
	Vendor           types.String                 `tfsdk:"vendor"`
	Arch             types.String                 `tfsdk:"arch"`
	GeneratedCPE     types.String                 `tfsdk:"generated_cpe"`
	HostsCount       types.Int64                  `tfsdk:"hosts_count"`
	TitleID          types.Int64                  `tfsdk:"title_id"`
	Vulnerabilities  []SoftwareVulnerabilityModel `tfsdk:"vulnerabilities"`
}

// SoftwareVulnerabilityModel represents a vulnerability.
type SoftwareVulnerabilityModel struct {
	CVE              types.String  `tfsdk:"cve"`
	DetailsLink      types.String  `tfsdk:"details_link"`
	CVSSScore        types.Float64 `tfsdk:"cvss_score"`
	EPSSProbability  types.Float64 `tfsdk:"epss_probability"`
	CISAKnownExploit types.Bool    `tfsdk:"cisa_known_exploit"`
	CVEPublished     types.String  `tfsdk:"cve_published"`
}

func (d *SoftwareVersionDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_version"
}

func (d *SoftwareVersionDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about a FleetDM software version.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The unique identifier of the software version.",
			},
			"team_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Filter by team ID.",
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
			"bundle_identifier": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The bundle identifier (for macOS apps).",
			},
			"vendor": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The software vendor.",
			},
			"arch": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The architecture (e.g., 'x86_64', 'arm64').",
			},
			"generated_cpe": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The generated CPE (Common Platform Enumeration) identifier.",
			},
			"hosts_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The number of hosts with this version installed.",
			},
			"title_id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The ID of the parent software title.",
			},
			"vulnerabilities": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of vulnerabilities for this software version.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"cve": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The CVE identifier.",
						},
						"details_link": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Link to CVE details.",
						},
						"cvss_score": schema.Float64Attribute{
							Computed:            true,
							MarkdownDescription: "The CVSS score.",
						},
						"epss_probability": schema.Float64Attribute{
							Computed:            true,
							MarkdownDescription: "The EPSS probability.",
						},
						"cisa_known_exploit": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether this is a CISA known exploited vulnerability.",
						},
						"cve_published": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "When the CVE was published.",
						},
					},
				},
			},
		},
	}
}

func (d *SoftwareVersionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *SoftwareVersionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SoftwareVersionDataSourceModel

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

	version, err := d.client.GetSoftwareVersion(ctx, id, teamID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to get software version: %s", err))
		return
	}

	// Map response to model
	data.ID = types.StringValue(strconv.Itoa(version.ID))
	data.Name = types.StringValue(version.Name)
	data.Version = types.StringValue(version.Version)
	data.Source = types.StringValue(version.Source)
	data.BundleIdentifier = types.StringValue(version.BundleIdentifier)
	data.Vendor = types.StringValue(version.Vendor)
	data.Arch = types.StringValue(version.Arch)
	data.GeneratedCPE = types.StringValue(version.GeneratedCPE)
	data.HostsCount = types.Int64Value(int64(version.HostsCount))
	data.TitleID = types.Int64Value(int64(version.TitleID))

	// Map vulnerabilities
	if version.Vulnerabilities != nil {
		data.Vulnerabilities = make([]SoftwareVulnerabilityModel, len(version.Vulnerabilities))
		for i, v := range version.Vulnerabilities {
			data.Vulnerabilities[i] = SoftwareVulnerabilityModel{
				CVE:              types.StringValue(v.CVE),
				DetailsLink:      types.StringValue(v.DetailsLink),
				CISAKnownExploit: types.BoolValue(v.CISAKnownExploit),
				CVEPublished:     types.StringValue(v.CVEPublished),
			}
			if v.CVSSScore != nil {
				data.Vulnerabilities[i].CVSSScore = types.Float64Value(*v.CVSSScore)
			} else {
				data.Vulnerabilities[i].CVSSScore = types.Float64Null()
			}
			if v.EPSSProbability != nil {
				data.Vulnerabilities[i].EPSSProbability = types.Float64Value(*v.EPSSProbability)
			} else {
				data.Vulnerabilities[i].EPSSProbability = types.Float64Null()
			}
		}
	} else {
		data.Vulnerabilities = []SoftwareVulnerabilityModel{}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
