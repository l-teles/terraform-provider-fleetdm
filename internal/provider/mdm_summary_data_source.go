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
	_ datasource.DataSource              = &MDMSummaryDataSource{}
	_ datasource.DataSourceWithConfigure = &MDMSummaryDataSource{}
)

// NewMDMSummaryDataSource is a helper function to simplify the provider implementation.
func NewMDMSummaryDataSource() datasource.DataSource {
	return &MDMSummaryDataSource{}
}

// MDMSummaryDataSource is the data source implementation.
type MDMSummaryDataSource struct {
	client *fleetdm.Client
}

// MDMSummaryDataSourceModel describes the data source data model.
type MDMSummaryDataSourceModel struct {
	ID                          types.String       `tfsdk:"id"`
	Platform                    types.String       `tfsdk:"platform"`
	TeamID                      types.Int64        `tfsdk:"team_id"`
	CountsUpdatedAt             types.String       `tfsdk:"counts_updated_at"`
	EnrolledManualHostsCount    types.Int64        `tfsdk:"enrolled_manual_hosts_count"`
	EnrolledAutomatedHostsCount types.Int64        `tfsdk:"enrolled_automated_hosts_count"`
	EnrolledPersonalHostsCount  types.Int64        `tfsdk:"enrolled_personal_hosts_count"`
	UnenrolledHostsCount        types.Int64        `tfsdk:"unenrolled_hosts_count"`
	PendingHostsCount           types.Int64        `tfsdk:"pending_hosts_count"`
	HostsCount                  types.Int64        `tfsdk:"hosts_count"`
	MDMSolutions                []MDMSolutionModel `tfsdk:"mdm_solutions"`
}

// MDMSolutionModel describes an MDM solution.
type MDMSolutionModel struct {
	ID         types.Int64  `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	ServerURL  types.String `tfsdk:"server_url"`
	HostsCount types.Int64  `tfsdk:"hosts_count"`
}

// Metadata returns the data source type name.
func (d *MDMSummaryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mdm_summary"
}

// Schema defines the schema for the data source.
func (d *MDMSummaryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches the MDM enrollment summary from FleetDM.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Placeholder identifier for the data source.",
				Computed:    true,
			},
			"platform": schema.StringAttribute{
				Description: "Filter by platform (darwin, windows, ios, ipados, android). Optional.",
				Optional:    true,
			},
			"team_id": schema.Int64Attribute{
				Description: "Filter by team ID. Optional.",
				Optional:    true,
			},
			"counts_updated_at": schema.StringAttribute{
				Description: "When the counts were last updated.",
				Computed:    true,
			},
			"enrolled_manual_hosts_count": schema.Int64Attribute{
				Description: "Number of hosts enrolled manually.",
				Computed:    true,
			},
			"enrolled_automated_hosts_count": schema.Int64Attribute{
				Description: "Number of hosts enrolled automatically (DEP/ABM).",
				Computed:    true,
			},
			"enrolled_personal_hosts_count": schema.Int64Attribute{
				Description: "Number of hosts enrolled as personal devices.",
				Computed:    true,
			},
			"unenrolled_hosts_count": schema.Int64Attribute{
				Description: "Number of unenrolled hosts.",
				Computed:    true,
			},
			"pending_hosts_count": schema.Int64Attribute{
				Description: "Number of hosts pending enrollment.",
				Computed:    true,
			},
			"hosts_count": schema.Int64Attribute{
				Description: "Total number of hosts.",
				Computed:    true,
			},
			"mdm_solutions": schema.ListNestedAttribute{
				Description: "List of MDM solutions and host counts.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Description: "The MDM solution ID.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The MDM solution name.",
							Computed:    true,
						},
						"server_url": schema.StringAttribute{
							Description: "The MDM server URL.",
							Computed:    true,
						},
						"hosts_count": schema.Int64Attribute{
							Description: "Number of hosts using this MDM solution.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *MDMSummaryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (d *MDMSummaryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state MDMSummaryDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build options
	platform := ""
	if !state.Platform.IsNull() {
		platform = state.Platform.ValueString()
	}

	teamID := optionalIntPtr(state.TeamID)

	// Get MDM summary from FleetDM
	summary, err := d.client.GetMDMSummary(ctx, platform, teamID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FleetDM MDM Summary",
			err.Error(),
		)
		return
	}

	// Map response to model
	state.CountsUpdatedAt = types.StringValue(summary.CountsUpdatedAt)
	state.EnrolledManualHostsCount = types.Int64Value(int64(summary.EnrollmentStatus.EnrolledManualHostsCount))
	state.EnrolledAutomatedHostsCount = types.Int64Value(int64(summary.EnrollmentStatus.EnrolledAutomatedHostsCount))
	state.EnrolledPersonalHostsCount = types.Int64Value(int64(summary.EnrollmentStatus.EnrolledPersonalHostsCount))
	state.UnenrolledHostsCount = types.Int64Value(int64(summary.EnrollmentStatus.UnenrolledHostsCount))
	state.PendingHostsCount = types.Int64Value(int64(summary.EnrollmentStatus.PendingHostsCount))
	state.HostsCount = types.Int64Value(int64(summary.EnrollmentStatus.HostsCount))

	// Map MDM solutions
	if len(summary.MDMSolutions) > 0 {
		state.MDMSolutions = make([]MDMSolutionModel, len(summary.MDMSolutions))
		for i, solution := range summary.MDMSolutions {
			state.MDMSolutions[i] = MDMSolutionModel{
				ID:         types.Int64Value(int64(solution.ID)),
				Name:       types.StringValue(solution.Name),
				ServerURL:  types.StringValue(solution.ServerURL),
				HostsCount: types.Int64Value(int64(solution.HostsCount)),
			}
		}
	} else {
		state.MDMSolutions = nil
	}

	// Set ID
	id := "mdm-summary"
	if platform != "" {
		id += "-" + platform
	}
	if teamID != nil {
		id += fmt.Sprintf("-team-%d", *teamID)
	}
	state.ID = types.StringValue(id)

	// Set state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
