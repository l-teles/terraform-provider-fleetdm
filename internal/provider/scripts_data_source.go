package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &scriptsDataSource{}
	_ datasource.DataSourceWithConfigure = &scriptsDataSource{}
)

// NewScriptsDataSource is a helper function to simplify the provider implementation.
func NewScriptsDataSource() datasource.DataSource {
	return &scriptsDataSource{}
}

// scriptsDataSource is the data source implementation.
type scriptsDataSource struct {
	client *fleetdm.Client
}

// scriptsDataSourceModel maps the data source schema data.
type scriptsDataSourceModel struct {
	TeamID  types.Int64             `tfsdk:"team_id"`
	Scripts []scriptDataSourceModel `tfsdk:"scripts"`
}

// Metadata returns the data source type name.
func (d *scriptsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_scripts"
}

// Schema defines the schema for the data source.
func (d *scriptsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches the list of FleetDM scripts.",
		Attributes: map[string]schema.Attribute{
			"team_id": schema.Int64Attribute{
				Description: "Filter scripts by team ID. If not specified, returns scripts for hosts with no team.",
				Optional:    true,
			},
			"scripts": schema.ListNestedAttribute{
				Description: "The list of scripts.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Description: "The unique identifier of the script.",
							Computed:    true,
						},
						"team_id": schema.Int64Attribute{
							Description: "The ID of the team this script belongs to.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The name of the script.",
							Computed:    true,
						},
						"created_at": schema.StringAttribute{
							Description: "When the script was created.",
							Computed:    true,
						},
						"updated_at": schema.StringAttribute{
							Description: "When the script was last updated.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *scriptsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (d *scriptsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state scriptsDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Prepare team ID filter
	teamID := optionalIntPtr(state.TeamID)

	// Get scripts from FleetDM API
	scripts, err := d.client.ListScripts(ctx, teamID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FleetDM Scripts",
			err.Error(),
		)
		return
	}

	// Map response body to model
	state.Scripts = make([]scriptDataSourceModel, len(scripts))
	for i, script := range scripts {
		state.Scripts[i] = scriptDataSourceModel{
			ID:        types.Int64Value(int64(script.ID)),
			Name:      types.StringValue(script.Name),
			CreatedAt: types.StringValue(script.CreatedAt),
			UpdatedAt: types.StringValue(script.UpdatedAt),
		}
		state.Scripts[i].TeamID = intPtrToInt64(script.TeamID)
	}

	// Set state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
