package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &UserDataSource{}

// NewUserDataSource creates a new user data source.
func NewUserDataSource() datasource.DataSource {
	return &UserDataSource{}
}

// UserDataSource defines the data source implementation.
type UserDataSource struct {
	client *fleetdm.Client
}

// UserDataSourceModel describes the data source data model.
type UserDataSourceModel struct {
	ID                 types.Int64  `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Email              types.String `tfsdk:"email"`
	GlobalRole         types.String `tfsdk:"global_role"`
	SSOEnabled         types.Bool   `tfsdk:"sso_enabled"`
	MFAEnabled         types.Bool   `tfsdk:"mfa_enabled"`
	APIOnly            types.Bool   `tfsdk:"api_only"`
	ForcePasswordReset types.Bool   `tfsdk:"force_password_reset"`
	GravatarURL        types.String `tfsdk:"gravatar_url"`
	Teams              types.List   `tfsdk:"teams"`
}

// Metadata returns the data source type name.
func (ds *UserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

// Schema defines the schema for the data source.
func (ds *UserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves information about a FleetDM user.",
		MarkdownDescription: `Retrieves information about a FleetDM user.

## Example Usage

` + "```hcl" + `
data "fleetdm_user" "admin" {
  id = 1
}

output "admin_email" {
  value = data.fleetdm_user.admin.email
}
` + "```",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description:         "The unique identifier of the user.",
				MarkdownDescription: "The unique identifier of the user.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				Description:         "The full name of the user.",
				MarkdownDescription: "The full name of the user.",
				Computed:            true,
			},
			"email": schema.StringAttribute{
				Description:         "The email address of the user.",
				MarkdownDescription: "The email address of the user.",
				Computed:            true,
			},
			"global_role": schema.StringAttribute{
				Description:         "The global role assigned to the user.",
				MarkdownDescription: "The global role assigned to the user.",
				Computed:            true,
			},
			"sso_enabled": schema.BoolAttribute{
				Description:         "Whether SSO is enabled for this user.",
				MarkdownDescription: "Whether SSO is enabled for this user.",
				Computed:            true,
			},
			"mfa_enabled": schema.BoolAttribute{
				Description:         "Whether MFA is enabled for this user.",
				MarkdownDescription: "Whether MFA is enabled for this user.",
				Computed:            true,
			},
			"api_only": schema.BoolAttribute{
				Description:         "Whether this user is API-only.",
				MarkdownDescription: "Whether this user is API-only.",
				Computed:            true,
			},
			"force_password_reset": schema.BoolAttribute{
				Description:         "Whether the user must reset their password on next login.",
				MarkdownDescription: "Whether the user must reset their password on next login.",
				Computed:            true,
			},
			"gravatar_url": schema.StringAttribute{
				Description:         "The Gravatar URL for the user.",
				MarkdownDescription: "The Gravatar URL for the user.",
				Computed:            true,
			},
			"teams": schema.ListNestedAttribute{
				Description:         "Team assignments for this user.",
				MarkdownDescription: "Team assignments for this user.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Description:         "The ID of the team.",
							MarkdownDescription: "The ID of the team.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							Description:         "The name of the team.",
							MarkdownDescription: "The name of the team.",
							Computed:            true,
						},
						"role": schema.StringAttribute{
							Description:         "The role for this team.",
							MarkdownDescription: "The role for this team.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (ds *UserDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	ds.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (ds *UserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config UserDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading user", map[string]interface{}{
		"id": config.ID.ValueInt64(),
	})

	user, err := ds.client.GetUser(ctx, config.ID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading FleetDM User",
			"Could not read user ID "+strconv.FormatInt(config.ID.ValueInt64(), 10)+": "+err.Error(),
		)
		return
	}

	// Map response to model
	config.ID = types.Int64Value(user.ID)
	config.Name = types.StringValue(user.Name)
	config.Email = types.StringValue(user.Email)
	config.SSOEnabled = types.BoolValue(user.SSOEnabled)
	config.MFAEnabled = types.BoolValue(user.MFAEnabled)
	config.APIOnly = types.BoolValue(user.APIOnly)
	config.ForcePasswordReset = types.BoolValue(user.ForcePasswordReset)
	config.GravatarURL = types.StringValue(user.GravatarURL)

	config.GlobalRole = stringPtrToString(user.GlobalRole)

	// Map teams
	config.Teams = mapUserTeamsToList(ctx, user.Teams, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

