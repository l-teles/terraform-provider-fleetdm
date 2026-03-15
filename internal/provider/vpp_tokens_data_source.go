package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &vppTokensDataSource{}
	_ datasource.DataSourceWithConfigure = &vppTokensDataSource{}
)

// NewVPPTokensDataSource is a helper function to simplify the provider implementation.
func NewVPPTokensDataSource() datasource.DataSource {
	return &vppTokensDataSource{}
}

// vppTokensDataSource is the data source implementation.
type vppTokensDataSource struct {
	client *fleetdm.Client
}

// vppTokensDataSourceModel maps the data source schema data.
type vppTokensDataSourceModel struct {
	Tokens []vppTokenModel `tfsdk:"tokens"`
}

// vppTokenModel maps individual VPP token data.
type vppTokenModel struct {
	ID               types.Int64  `tfsdk:"id"`
	OrganizationName types.String `tfsdk:"organization_name"`
	Location         types.String `tfsdk:"location"`
	RenewDate        types.String `tfsdk:"renew_date"`
	Teams            types.List   `tfsdk:"teams"`
}

// vppTokenTeamModel maps team data within a VPP token.
type vppTokenTeamModel struct {
	ID   types.Int64  `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

// Metadata returns the data source type name.
func (d *vppTokensDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vpp_tokens"
}

// Schema defines the schema for the data source.
func (d *vppTokensDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves all Volume Purchase Program (VPP) tokens from FleetDM. This is a Premium feature.",
		Attributes: map[string]schema.Attribute{
			"tokens": schema.ListNestedAttribute{
				Description: "List of VPP tokens.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Description: "The unique identifier of the VPP token.",
							Computed:    true,
						},
						"organization_name": schema.StringAttribute{
							Description: "The organization name associated with the VPP token.",
							Computed:    true,
						},
						"location": schema.StringAttribute{
							Description: "The location/content token name in Apple Business Manager.",
							Computed:    true,
						},
						"renew_date": schema.StringAttribute{
							Description: "When the token needs to be renewed.",
							Computed:    true,
						},
						"teams": schema.ListNestedAttribute{
							Description: "Teams associated with this VPP token.",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"id": schema.Int64Attribute{
										Description: "The team ID.",
										Computed:    true,
									},
									"name": schema.StringAttribute{
										Description: "The team name.",
										Computed:    true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *vppTokensDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (d *vppTokensDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state vppTokensDataSourceModel

	// Get all VPP tokens
	tokens, err := d.client.ListVPPTokens(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read VPP Tokens",
			err.Error(),
		)
		return
	}

	// Map response to state
	for _, token := range tokens {
		// Build teams list
		var teams []vppTokenTeamModel
		for _, team := range token.Teams {
			teams = append(teams, vppTokenTeamModel{
				ID:   types.Int64Value(int64(team.ID)),
				Name: types.StringValue(team.Name),
			})
		}

		// Convert teams to types.List
		teamsList, diags := types.ListValueFrom(ctx, types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"id":   types.Int64Type,
				"name": types.StringType,
			},
		}, teams)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		tokenModel := vppTokenModel{
			ID:               types.Int64Value(int64(token.ID)),
			OrganizationName: types.StringValue(token.OrganizationName),
			Location:         types.StringValue(token.Location),
			RenewDate:        types.StringValue(token.RenewDate),
			Teams:            teamsList,
		}

		state.Tokens = append(state.Tokens, tokenModel)
	}

	// Set state
	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}
