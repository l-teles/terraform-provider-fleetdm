package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &UsersDataSource{}

// NewUsersDataSource creates a new users data source.
func NewUsersDataSource() datasource.DataSource {
	return &UsersDataSource{}
}

// UsersDataSource defines the data source implementation.
type UsersDataSource struct {
	client *fleetdm.Client
}

// UsersDataSourceModel describes the data source data model.
type UsersDataSourceModel struct {
	Query  types.String `tfsdk:"query"`
	TeamID types.Int64  `tfsdk:"team_id"`
	Users  types.List   `tfsdk:"users"`
}

// Metadata returns the data source type name.
func (d *UsersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_users"
}

// Schema defines the schema for the data source.
func (d *UsersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves a list of FleetDM users.",
		MarkdownDescription: `Retrieves a list of FleetDM users.

## Example Usage

### List All Users

` + "```hcl" + `
data "fleetdm_users" "all" {}

output "user_count" {
  value = length(data.fleetdm_users.all.users)
}
` + "```" + `

### Search Users by Name or Email

` + "```hcl" + `
data "fleetdm_users" "admins" {
  query = "admin"
}
` + "```" + `

### List Users in a Team (Fleet Premium)

` + "```hcl" + `
data "fleetdm_users" "team_users" {
  team_id = fleetdm_team.workstations.id
}
` + "```",

		Attributes: map[string]schema.Attribute{
			"query": schema.StringAttribute{
				Description:         "Search query keywords. Searchable fields include name and email.",
				MarkdownDescription: "Search query keywords. Searchable fields include `name` and `email`.",
				Optional:            true,
			},
			"team_id": schema.Int64Attribute{
				Description:         "Filter users by team ID (Fleet Premium).",
				MarkdownDescription: "Filter users by team ID (Fleet Premium).",
				Optional:            true,
			},
			"users": schema.ListNestedAttribute{
				Description:         "List of users matching the filter criteria.",
				MarkdownDescription: "List of users matching the filter criteria.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Description:         "The unique identifier of the user.",
							MarkdownDescription: "The unique identifier of the user.",
							Computed:            true,
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
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *UsersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (d *UsersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config UsersDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build query parameters
	params := make(map[string]string)
	if !config.Query.IsNull() && !config.Query.IsUnknown() {
		params["query"] = config.Query.ValueString()
	}
	if !config.TeamID.IsNull() && !config.TeamID.IsUnknown() {
		params["team_id"] = strconv.FormatInt(config.TeamID.ValueInt64(), 10)
	}

	tflog.Debug(ctx, "Reading users", map[string]interface{}{
		"query":   config.Query.ValueString(),
		"team_id": config.TeamID.ValueInt64(),
	})

	users, err := d.client.ListUsers(ctx, params)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading FleetDM Users",
			"Could not read users: "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Found users", map[string]interface{}{
		"count": len(users),
	})

	// Map response to model
	config.Users = d.mapUsersToList(ctx, users, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// mapUsersToList converts FleetDM User slice to Terraform List.
func (d *UsersDataSource) mapUsersToList(ctx context.Context, users []fleetdm.User, diags *diag.Diagnostics) types.List {
	teamAttrTypes := map[string]attr.Type{
		"id":   types.Int64Type,
		"name": types.StringType,
		"role": types.StringType,
	}

	userAttrTypes := map[string]attr.Type{
		"id":                   types.Int64Type,
		"name":                 types.StringType,
		"email":                types.StringType,
		"global_role":          types.StringType,
		"sso_enabled":          types.BoolType,
		"mfa_enabled":          types.BoolType,
		"api_only":             types.BoolType,
		"force_password_reset": types.BoolType,
		"gravatar_url":         types.StringType,
		"teams":                types.ListType{ElemType: types.ObjectType{AttrTypes: teamAttrTypes}},
	}

	if len(users) == 0 {
		return types.ListNull(types.ObjectType{AttrTypes: userAttrTypes})
	}

	userElements := make([]attr.Value, len(users))
	for i, u := range users {
		// Map teams
		teamsList := mapUserTeamsToList(ctx, u.Teams, diags)
		if diags.HasError() {
			return types.ListNull(types.ObjectType{AttrTypes: userAttrTypes})
		}

		userObj, dd := types.ObjectValue(
			userAttrTypes,
			map[string]attr.Value{
				"id":                   types.Int64Value(u.ID),
				"name":                 types.StringValue(u.Name),
				"email":                types.StringValue(u.Email),
				"global_role":          stringPtrToString(u.GlobalRole),
				"sso_enabled":          types.BoolValue(u.SSOEnabled),
				"mfa_enabled":          types.BoolValue(u.MFAEnabled),
				"api_only":             types.BoolValue(u.APIOnly),
				"force_password_reset": types.BoolValue(u.ForcePasswordReset),
				"gravatar_url":         types.StringValue(u.GravatarURL),
				"teams":                teamsList,
			},
		)
		if dd.HasError() {
			diags.Append(dd...)
			return types.ListNull(types.ObjectType{AttrTypes: userAttrTypes})
		}
		userElements[i] = userObj
	}

	userList, dd := types.ListValue(types.ObjectType{AttrTypes: userAttrTypes}, userElements)
	if dd.HasError() {
		diags.Append(dd...)
		return types.ListNull(types.ObjectType{AttrTypes: userAttrTypes})
	}

	return userList
}
