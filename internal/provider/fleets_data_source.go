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
var _ datasource.DataSource = &FleetsDataSource{}

// NewFleetsDataSource creates a new fleets data source.
func NewFleetsDataSource() datasource.DataSource {
	return &FleetsDataSource{}
}

// NewTeamsDataSource creates a deprecated teams data source (alias for FleetsDataSource).
func NewTeamsDataSource() datasource.DataSource {
	return &FleetsDataSource{deprecated: true}
}

// FleetsDataSource defines the data source implementation.
type FleetsDataSource struct {
	client     *fleetdm.Client
	deprecated bool
}

// FleetsDataSourceModel describes the data source data model.
type FleetsDataSourceModel struct {
	Fleets []FleetSummaryModel `tfsdk:"fleets"`
}

// TeamsDataSourceModel describes the deprecated teams data source data model.
type TeamsDataSourceModel struct {
	Teams []FleetSummaryModel `tfsdk:"teams"`
}

// FleetSummaryModel describes a fleet in the fleets list.
type FleetSummaryModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	UserCount   types.Int64  `tfsdk:"user_count"`
	HostCount   types.Int64  `tfsdk:"host_count"`
}

// Metadata returns the data source type name.
func (d *FleetsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	if d.deprecated {
		resp.TypeName = req.ProviderTypeName + "_teams"
	} else {
		resp.TypeName = req.ProviderTypeName + "_fleets"
	}
}

// Schema defines the schema for the data source.
func (d *FleetsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	if d.deprecated {
		resp.Schema = schema.Schema{
			DeprecationMessage:  "fleetdm_teams is deprecated and will be removed in a future version. Use fleetdm_fleets instead (requires Fleet 4.82.0+).",
			Description:         "Retrieves a list of all FleetDM fleets.",
			MarkdownDescription: "Retrieves a list of all FleetDM fleets.",
			Attributes: map[string]schema.Attribute{
				"teams": schema.ListNestedAttribute{
					Description:         "The list of fleets.",
					MarkdownDescription: "The list of fleets.",
					Computed:            true,
					NestedObject:        fleetSummaryNestedObject(),
				},
			},
		}
		return
	}

	resp.Schema = schema.Schema{
		Description:         "Retrieves a list of all FleetDM fleets.",
		MarkdownDescription: "Retrieves a list of all FleetDM fleets.",
		Attributes: map[string]schema.Attribute{
			"fleets": schema.ListNestedAttribute{
				Description:         "The list of fleets.",
				MarkdownDescription: "The list of fleets.",
				Computed:            true,
				NestedObject:        fleetSummaryNestedObject(),
			},
		},
	}
}

// fleetSummaryNestedObject returns the nested object schema for a fleet summary.
func fleetSummaryNestedObject() schema.NestedAttributeObject {
	return schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description:         "The unique identifier of the fleet.",
				MarkdownDescription: "The unique identifier of the fleet.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				Description:         "The name of the fleet.",
				MarkdownDescription: "The name of the fleet.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				Description:         "A description of the fleet.",
				MarkdownDescription: "A description of the fleet.",
				Computed:            true,
			},
			"user_count": schema.Int64Attribute{
				Description:         "The number of users in the fleet.",
				MarkdownDescription: "The number of users in the fleet.",
				Computed:            true,
			},
			"host_count": schema.Int64Attribute{
				Description:         "The number of hosts in the fleet.",
				MarkdownDescription: "The number of hosts in the fleet.",
				Computed:            true,
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *FleetsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (d *FleetsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Debug(ctx, "Reading fleets data source")

	// Get all fleets from the API (paginate through all results)
	var allTeams []fleetdm.Team
	page := 0
	perPage := 100

	for {
		teams, err := d.client.ListTeams(ctx, page, perPage)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Reading FleetDM Fleets",
				"Could not read fleets: "+err.Error(),
			)
			return
		}

		allTeams = append(allTeams, teams...)

		if len(teams) < perPage {
			break
		}

		page++
	}

	tflog.Debug(ctx, "Fleets data source read", map[string]interface{}{
		"count": len(allTeams),
	})

	// Map API response to summary models
	summaries := make([]FleetSummaryModel, len(allTeams))
	for i, team := range allTeams {
		summaries[i] = FleetSummaryModel{
			ID:          types.Int64Value(team.ID),
			Name:        types.StringValue(team.Name),
			Description: types.StringValue(team.Description),
			UserCount:   types.Int64Value(int64(team.UserCount)),
			HostCount:   types.Int64Value(int64(team.HostCount)),
		}
	}

	if d.deprecated {
		state := TeamsDataSourceModel{Teams: summaries}
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		state := FleetsDataSourceModel{Fleets: summaries}
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}
}
