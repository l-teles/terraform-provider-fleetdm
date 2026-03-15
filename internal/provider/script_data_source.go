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
	_ datasource.DataSource              = &scriptDataSource{}
	_ datasource.DataSourceWithConfigure = &scriptDataSource{}
)

// NewScriptDataSource is a helper function to simplify the provider implementation.
func NewScriptDataSource() datasource.DataSource {
	return &scriptDataSource{}
}

// scriptDataSource is the data source implementation.
type scriptDataSource struct {
	client *fleetdm.Client
}

// scriptDataSourceModel maps the data source schema data.
type scriptDataSourceModel struct {
	ID        types.Int64  `tfsdk:"id"`
	TeamID    types.Int64  `tfsdk:"team_id"`
	Name      types.String `tfsdk:"name"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

// Metadata returns the data source type name.
func (d *scriptDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_script"
}

// Schema defines the schema for the data source.
func (d *scriptDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a FleetDM script by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "The unique identifier of the script.",
				Required:    true,
			},
			"team_id": schema.Int64Attribute{
				Description: "The ID of the team this script belongs to. Null for global scripts.",
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
	}
}

// Configure adds the provider configured client to the data source.
func (d *scriptDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (d *scriptDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state scriptDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get script from FleetDM API
	script, err := d.client.GetScript(ctx, int(state.ID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FleetDM Script",
			err.Error(),
		)
		return
	}

	// Map response body to model
	state.ID = types.Int64Value(int64(script.ID))
	state.Name = types.StringValue(script.Name)
	state.CreatedAt = types.StringValue(script.CreatedAt)
	state.UpdatedAt = types.StringValue(script.UpdatedAt)

	state.TeamID = intPtrToInt64(script.TeamID)

	// Set state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
