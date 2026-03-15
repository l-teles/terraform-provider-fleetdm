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
	_ datasource.DataSource              = &abmTokensDataSource{}
	_ datasource.DataSourceWithConfigure = &abmTokensDataSource{}
)

// NewABMTokensDataSource is a helper function to simplify the provider implementation.
func NewABMTokensDataSource() datasource.DataSource {
	return &abmTokensDataSource{}
}

// abmTokensDataSource is the data source implementation.
type abmTokensDataSource struct {
	client *fleetdm.Client
}

// abmTokensDataSourceModel maps the data source schema data.
type abmTokensDataSourceModel struct {
	Tokens []abmTokenModel `tfsdk:"tokens"`
}

// abmTokenModel maps individual ABM token data.
type abmTokenModel struct {
	ID               types.Int64  `tfsdk:"id"`
	AppleID          types.String `tfsdk:"apple_id"`
	OrganizationName types.String `tfsdk:"organization_name"`
	MDMServerURL     types.String `tfsdk:"mdm_server_url"`
	RenewDate        types.String `tfsdk:"renew_date"`
	TermsExpired     types.Bool   `tfsdk:"terms_expired"`
	MacOSTeamID      types.Int64  `tfsdk:"macos_team_id"`
	IOSTeamID        types.Int64  `tfsdk:"ios_team_id"`
	IPadOSTeamID     types.Int64  `tfsdk:"ipados_team_id"`
	MacOSTeamName    types.String `tfsdk:"macos_team_name"`
	IOSTeamName      types.String `tfsdk:"ios_team_name"`
	IPadOSTeamName   types.String `tfsdk:"ipados_team_name"`
}

// Metadata returns the data source type name.
func (d *abmTokensDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_abm_tokens"
}

// Schema defines the schema for the data source.
func (d *abmTokensDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves all Apple Business Manager (ABM) tokens from FleetDM. This is a Premium feature.",
		Attributes: map[string]schema.Attribute{
			"tokens": schema.ListNestedAttribute{
				Description: "List of ABM tokens.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Description: "The unique identifier of the ABM token.",
							Computed:    true,
						},
						"apple_id": schema.StringAttribute{
							Description: "The Apple ID associated with the token.",
							Computed:    true,
						},
						"organization_name": schema.StringAttribute{
							Description: "The organization name in Apple Business Manager.",
							Computed:    true,
						},
						"mdm_server_url": schema.StringAttribute{
							Description: "The MDM server URL.",
							Computed:    true,
						},
						"renew_date": schema.StringAttribute{
							Description: "When the token needs to be renewed.",
							Computed:    true,
						},
						"terms_expired": schema.BoolAttribute{
							Description: "Whether the ABM terms and conditions have expired.",
							Computed:    true,
						},
						"macos_team_id": schema.Int64Attribute{
							Description: "The team ID for macOS devices enrolled via this token.",
							Computed:    true,
						},
						"ios_team_id": schema.Int64Attribute{
							Description: "The team ID for iOS devices enrolled via this token.",
							Computed:    true,
						},
						"ipados_team_id": schema.Int64Attribute{
							Description: "The team ID for iPadOS devices enrolled via this token.",
							Computed:    true,
						},
						"macos_team_name": schema.StringAttribute{
							Description: "The team name for macOS devices.",
							Computed:    true,
						},
						"ios_team_name": schema.StringAttribute{
							Description: "The team name for iOS devices.",
							Computed:    true,
						},
						"ipados_team_name": schema.StringAttribute{
							Description: "The team name for iPadOS devices.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *abmTokensDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (d *abmTokensDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state abmTokensDataSourceModel

	// Get all ABM tokens
	tokens, err := d.client.ListABMTokens(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read ABM Tokens",
			err.Error(),
		)
		return
	}

	// Map response to state
	for _, token := range tokens {
		tokenModel := abmTokenModel{
			ID:               types.Int64Value(int64(token.ID)),
			AppleID:          types.StringValue(token.AppleID),
			OrganizationName: types.StringValue(token.OrganizationName),
			MDMServerURL:     types.StringValue(token.MDMServerURL),
			RenewDate:        types.StringValue(token.RenewDate),
			TermsExpired:     types.BoolValue(token.TermsExpired),
			MacOSTeamName:    types.StringValue(token.MacOSTeamName),
			IOSTeamName:      types.StringValue(token.IOSTeamName),
			IPadOSTeamName:   types.StringValue(token.IPadOSTeamName),
		}

		// Handle nullable team IDs
		if token.MacOSTeamID != nil {
			tokenModel.MacOSTeamID = types.Int64Value(int64(*token.MacOSTeamID))
		} else {
			tokenModel.MacOSTeamID = types.Int64Null()
		}
		if token.IOSTeamID != nil {
			tokenModel.IOSTeamID = types.Int64Value(int64(*token.IOSTeamID))
		} else {
			tokenModel.IOSTeamID = types.Int64Null()
		}
		if token.IPadOSTeamID != nil {
			tokenModel.IPadOSTeamID = types.Int64Value(int64(*token.IPadOSTeamID))
		} else {
			tokenModel.IPadOSTeamID = types.Int64Null()
		}

		state.Tokens = append(state.Tokens, tokenModel)
	}

	// Set state
	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}
