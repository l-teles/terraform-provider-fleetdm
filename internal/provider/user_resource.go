package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &UserResource{}
	_ resource.ResourceWithImportState = &UserResource{}
)

// NewUserResource creates a new user resource.
func NewUserResource() resource.Resource {
	return &UserResource{}
}

// UserResource defines the resource implementation.
type UserResource struct {
	client *fleetdm.Client
}

// UserResourceModel describes the resource data model.
type UserResourceModel struct {
	ID                 types.Int64  `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Email              types.String `tfsdk:"email"`
	Password           types.String `tfsdk:"password"`
	GlobalRole         types.String `tfsdk:"global_role"`
	SSOEnabled         types.Bool   `tfsdk:"sso_enabled"`
	MFAEnabled         types.Bool   `tfsdk:"mfa_enabled"`
	APIOnly            types.Bool   `tfsdk:"api_only"`
	ForcePasswordReset types.Bool   `tfsdk:"force_password_reset"`
	Teams              types.List   `tfsdk:"teams"`

	// Computed fields
	GravatarURL types.String `tfsdk:"gravatar_url"`
}

// UserTeamModel represents a team assignment for a user.
type UserTeamModel struct {
	ID   types.Int64  `tfsdk:"id"`
	Role types.String `tfsdk:"role"`
}

// Metadata returns the resource type name.
func (r *UserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

// Schema defines the schema for the resource.
func (r *UserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a FleetDM user.",
		MarkdownDescription: `Manages a FleetDM user.

Users can have either a global role or team-specific roles. Use ` + "`global_role`" + ` for global access or ` + "`teams`" + ` for team-based access.

## Example Usage

### Global Admin User

` + "```hcl" + `
resource "fleetdm_user" "admin" {
  name        = "Admin User"
  email       = "admin@example.com"
  password    = "SecurePassword123!"
  global_role = "admin"
}
` + "```" + `

### API-Only User

` + "```hcl" + `
resource "fleetdm_user" "api_user" {
  name        = "API Service Account"
  email       = "api@example.com"
  password    = "SecurePassword123!"
  global_role = "maintainer"
  api_only    = true
}
` + "```" + `

### Team-Based User (Fleet Premium)

` + "```hcl" + `
resource "fleetdm_user" "team_user" {
  name     = "Team User"
  email    = "teamuser@example.com"
  password = "SecurePassword123!"

  teams = [
    {
      id   = fleetdm_team.workstations.id
      role = "maintainer"
    },
    {
      id   = fleetdm_team.servers.id
      role = "observer"
    }
  ]
}
` + "```" + `

## Import

Users can be imported using the user ID:

` + "```shell" + `
terraform import fleetdm_user.admin 123
` + "```",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description:         "The unique identifier of the user.",
				MarkdownDescription: "The unique identifier of the user.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description:         "The full name of the user.",
				MarkdownDescription: "The full name of the user.",
				Required:            true,
			},
			"email": schema.StringAttribute{
				Description:         "The email address of the user.",
				MarkdownDescription: "The email address of the user.",
				Required:            true,
			},
			"password": schema.StringAttribute{
				Description:         "The password for the user. Required for non-SSO users.",
				MarkdownDescription: "The password for the user. Required for non-SSO users.",
				Optional:            true,
				Sensitive:           true,
			},
			"global_role": schema.StringAttribute{
				Description:         "The global role assigned to the user. Options: admin, maintainer, observer, observer_plus, gitops. Mutually exclusive with teams.",
				MarkdownDescription: "The global role assigned to the user. Options: `admin`, `maintainer`, `observer`, `observer_plus`, `gitops`. Mutually exclusive with `teams`.",
				Optional:            true,
			},
			"sso_enabled": schema.BoolAttribute{
				Description:         "Whether SSO is enabled for this user.",
				MarkdownDescription: "Whether SSO is enabled for this user.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"mfa_enabled": schema.BoolAttribute{
				Description:         "Whether MFA is enabled for this user (Fleet Premium). Incompatible with SSO and API-only users.",
				MarkdownDescription: "Whether MFA is enabled for this user (Fleet Premium). Incompatible with SSO and API-only users.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"api_only": schema.BoolAttribute{
				Description:         "Whether this user is API-only (cannot use web UI).",
				MarkdownDescription: "Whether this user is API-only (cannot use web UI).",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"force_password_reset": schema.BoolAttribute{
				Description:         "Whether the user is required to reset their password on next login.",
				MarkdownDescription: "Whether the user is required to reset their password on next login.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"teams": schema.ListNestedAttribute{
				Description:         "Team assignments for this user (Fleet Premium). Mutually exclusive with global_role.",
				MarkdownDescription: "Team assignments for this user (Fleet Premium). Mutually exclusive with `global_role`.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Description:         "The ID of the team.",
							MarkdownDescription: "The ID of the team.",
							Required:            true,
						},
						"role": schema.StringAttribute{
							Description:         "The role for this team. Options: admin, maintainer, observer, observer_plus, gitops.",
							MarkdownDescription: "The role for this team. Options: `admin`, `maintainer`, `observer`, `observer_plus`, `gitops`.",
							Required:            true,
						},
					},
				},
			},
			"gravatar_url": schema.StringAttribute{
				Description:         "The Gravatar URL for the user.",
				MarkdownDescription: "The Gravatar URL for the user.",
				Computed:            true,
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *UserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// Create creates the resource and sets the initial Terraform state.
func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan UserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating user", map[string]interface{}{
		"name":  plan.Name.ValueString(),
		"email": plan.Email.ValueString(),
	})

	// Build create request
	createReq := fleetdm.CreateUserRequest{
		Name:       plan.Name.ValueString(),
		Email:      plan.Email.ValueString(),
		SSOEnabled: plan.SSOEnabled.ValueBool(),
		MFAEnabled: plan.MFAEnabled.ValueBool(),
		APIOnly:    plan.APIOnly.ValueBool(),
	}

	// Set password if provided
	if !plan.Password.IsNull() && !plan.Password.IsUnknown() {
		createReq.Password = plan.Password.ValueString()
	}

	// Set global role if provided
	if !plan.GlobalRole.IsNull() && !plan.GlobalRole.IsUnknown() {
		role := plan.GlobalRole.ValueString()
		createReq.GlobalRole = &role
	}

	// Set teams if provided
	if !plan.Teams.IsNull() && !plan.Teams.IsUnknown() {
		var teams []UserTeamModel
		resp.Diagnostics.Append(plan.Teams.ElementsAs(ctx, &teams, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for _, t := range teams {
			createReq.Teams = append(createReq.Teams, fleetdm.UserTeam{
				ID:   t.ID.ValueInt64(),
				Role: t.Role.ValueString(),
			})
		}
	}

	// Set force password reset
	if !plan.ForcePasswordReset.IsNull() && !plan.ForcePasswordReset.IsUnknown() {
		forceReset := plan.ForcePasswordReset.ValueBool()
		createReq.AdminForcedPasswordReset = &forceReset
	}

	user, err := r.client.CreateUser(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating FleetDM User",
			"Could not create user, unexpected error: "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "User created", map[string]interface{}{
		"id":    user.ID,
		"email": user.Email,
	})

	// Map response to model
	r.mapUserToModel(ctx, user, &plan, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state UserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading user", map[string]interface{}{
		"id": state.ID.ValueInt64(),
	})

	user, err := r.client.GetUser(ctx, state.ID.ValueInt64())
	if err != nil {
		if isNotFound(err) {
			tflog.Warn(ctx, "User not found, removing from state", map[string]interface{}{
				"id": state.ID.ValueInt64(),
			})
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error Reading FleetDM User",
			"Could not read user ID "+strconv.FormatInt(state.ID.ValueInt64(), 10)+": "+err.Error(),
		)
		return
	}

	// Preserve password from state (it's write-only)
	password := state.Password

	r.mapUserToModel(ctx, user, &state, &resp.Diagnostics)

	// Restore password from state
	state.Password = password

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan UserResourceModel
	var state UserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating user", map[string]interface{}{
		"id":    plan.ID.ValueInt64(),
		"email": plan.Email.ValueString(),
	})

	// Build update request
	updateReq := fleetdm.UpdateUserRequest{
		Name:  plan.Name.ValueString(),
		Email: plan.Email.ValueString(),
	}

	// Set SSO/MFA settings
	ssoEnabled := plan.SSOEnabled.ValueBool()
	updateReq.SSOEnabled = &ssoEnabled

	mfaEnabled := plan.MFAEnabled.ValueBool()
	updateReq.MFAEnabled = &mfaEnabled

	apiOnly := plan.APIOnly.ValueBool()
	updateReq.APIOnly = &apiOnly

	// Set global role if provided
	if !plan.GlobalRole.IsNull() && !plan.GlobalRole.IsUnknown() {
		role := plan.GlobalRole.ValueString()
		updateReq.GlobalRole = &role
	}

	// Set teams if provided
	if !plan.Teams.IsNull() && !plan.Teams.IsUnknown() {
		var teams []UserTeamModel
		resp.Diagnostics.Append(plan.Teams.ElementsAs(ctx, &teams, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for _, t := range teams {
			updateReq.Teams = append(updateReq.Teams, fleetdm.UserTeam{
				ID:   t.ID.ValueInt64(),
				Role: t.Role.ValueString(),
			})
		}
	}

	user, err := r.client.UpdateUser(ctx, plan.ID.ValueInt64(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating FleetDM User",
			"Could not update user, unexpected error: "+err.Error(),
		)
		return
	}

	r.mapUserToModel(ctx, user, &plan, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state UserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting user", map[string]interface{}{
		"id": state.ID.ValueInt64(),
	})

	err := r.client.DeleteUser(ctx, state.ID.ValueInt64())
	if err != nil {
		if isNotFound(err) {
			tflog.Warn(ctx, "User already deleted", map[string]interface{}{
				"id": state.ID.ValueInt64(),
			})
			return
		}

		resp.Diagnostics.AddError(
			"Error Deleting FleetDM User",
			"Could not delete user, unexpected error: "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "User deleted", map[string]interface{}{
		"id": state.ID.ValueInt64(),
	})
}

// ImportState imports an existing resource by ID.
func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, ok := parseIDFromString(req.ID, "User", &resp.Diagnostics)
	if !ok {
		return
	}

	tflog.Debug(ctx, "Importing user", map[string]interface{}{
		"id": id,
	})

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// mapUserToModel maps a FleetDM User to the Terraform model.
func (r *UserResource) mapUserToModel(ctx context.Context, user *fleetdm.User, model *UserResourceModel, diags *diag.Diagnostics) {
	model.ID = types.Int64Value(user.ID)
	model.Name = types.StringValue(user.Name)
	model.Email = types.StringValue(user.Email)
	model.SSOEnabled = types.BoolValue(user.SSOEnabled)
	model.MFAEnabled = types.BoolValue(user.MFAEnabled)
	model.APIOnly = types.BoolValue(user.APIOnly)
	model.ForcePasswordReset = types.BoolValue(user.ForcePasswordReset)
	model.GravatarURL = types.StringValue(user.GravatarURL)

	model.GlobalRole = stringPtrToString(user.GlobalRole)

	// Map teams
	if len(user.Teams) > 0 {
		teamElements := make([]attr.Value, len(user.Teams))
		for i, t := range user.Teams {
			teamObj, d := types.ObjectValue(
				map[string]attr.Type{
					"id":   types.Int64Type,
					"role": types.StringType,
				},
				map[string]attr.Value{
					"id":   types.Int64Value(t.ID),
					"role": types.StringValue(t.Role),
				},
			)
			if d.HasError() {
				diags.Append(d...)
				return
			}
			teamElements[i] = teamObj
		}
		teamList, d := types.ListValue(
			types.ObjectType{
				AttrTypes: map[string]attr.Type{
					"id":   types.Int64Type,
					"role": types.StringType,
				},
			},
			teamElements,
		)
		if d.HasError() {
			diags.Append(d...)
			return
		}
		model.Teams = teamList
	} else {
		model.Teams = types.ListNull(types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"id":   types.Int64Type,
				"role": types.StringType,
			},
		})
	}
}
