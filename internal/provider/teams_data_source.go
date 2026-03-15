package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &TeamsDataSource{}

// NewTeamsDataSource creates a new teams data source.
func NewTeamsDataSource() datasource.DataSource {
	return &TeamsDataSource{}
}

// TeamsDataSource defines the data source implementation.
type TeamsDataSource struct {
	client *fleetdm.Client
}

// TeamsDataSourceModel describes the data source data model.
type TeamsDataSourceModel struct {
	Teams []TeamModel `tfsdk:"teams"`
}

// TeamModel describes a team in the teams list.
type TeamModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	UserCount   types.Int64  `tfsdk:"user_count"`
	HostCount   types.Int64  `tfsdk:"host_count"`
}

// Metadata returns the data source type name.
func (d *TeamsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_teams"
}

// Schema defines the schema for the data source.
func (d *TeamsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves a list of all FleetDM teams.",
		MarkdownDescription: `Retrieves a list of all FleetDM teams.

## Example Usage

` + "```hcl" + `
data "fleetdm_teams" "all" {}

output "team_names" {
  value = [for team in data.fleetdm_teams.all.teams : team.name]
}
` + "```",

		Attributes: map[string]schema.Attribute{
			"teams": schema.ListNestedAttribute{
				Description:         "The list of teams.",
				MarkdownDescription: "The list of teams.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Description:         "The unique identifier of the team.",
							MarkdownDescription: "The unique identifier of the team.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							Description:         "The name of the team.",
							MarkdownDescription: "The name of the team.",
							Computed:            true,
						},
						"description": schema.StringAttribute{
							Description:         "A description of the team.",
							MarkdownDescription: "A description of the team.",
							Computed:            true,
						},
						"user_count": schema.Int64Attribute{
							Description:         "The number of users in the team.",
							MarkdownDescription: "The number of users in the team.",
							Computed:            true,
						},
						"host_count": schema.Int64Attribute{
							Description:         "The number of hosts in the team.",
							MarkdownDescription: "The number of hosts in the team.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *TeamsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (d *TeamsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state TeamsDataSourceModel

	tflog.Debug(ctx, "Reading teams data source")

	// Get all teams from the API (paginate through all results)
	var allTeams []fleetdm.Team
	page := 0
	perPage := 100

	for {
		teams, err := d.client.ListTeams(ctx, page, perPage)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Reading FleetDM Teams",
				"Could not read teams: "+err.Error(),
			)
			return
		}

		allTeams = append(allTeams, teams...)

		// Check if we've received all teams
		if len(teams) < perPage {
			break
		}

		page++
	}

	tflog.Debug(ctx, "Teams data source read", map[string]interface{}{
		"count": len(allTeams),
	})

	// Map response to model
	state.Teams = make([]TeamModel, len(allTeams))
	for i, team := range allTeams {
		state.Teams[i] = TeamModel{
			ID:          types.Int64Value(team.ID),
			Name:        types.StringValue(team.Name),
			Description: types.StringValue(team.Description),
			UserCount:   types.Int64Value(int64(team.UserCount)),
			HostCount:   types.Int64Value(int64(team.HostCount)),
		}
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
