package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                   = &UserResource{}
	_ resource.ResourceWithImportState    = &UserResource{}
	_ resource.ResourceWithValidateConfig = &UserResource{}
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
	APIEndpoints       types.Set    `tfsdk:"api_endpoints"`
	ForcePasswordReset types.Bool   `tfsdk:"force_password_reset"`
	Teams              types.List   `tfsdk:"teams"`

	// Computed fields
	GravatarURL types.String `tfsdk:"gravatar_url"`
	Token       types.String `tfsdk:"token"`
}

// UserTeamModel represents a team assignment for a user.
type UserTeamModel struct {
	ID   types.Int64  `tfsdk:"id"`
	Role types.String `tfsdk:"role"`
}

// UserAPIEndpointModel represents one entry of an API-only user's endpoint
// access scope.
type UserAPIEndpointModel struct {
	Method types.String `tfsdk:"method"`
	Path   types.String `tfsdk:"path"`
}

// userAPIEndpointAttrTypes is the object type of an `api_endpoints` element.
var userAPIEndpointAttrTypes = map[string]attr.Type{
	"method": types.StringType,
	"path":   types.StringType,
}

// userAPIEndpointObjectType is the element type of the `api_endpoints` set.
var userAPIEndpointObjectType = types.ObjectType{AttrTypes: userAPIEndpointAttrTypes}

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

~> **Secrets are written to Terraform state.** Both ` + "`password`" + ` and the API ` + "`token`" + ` Fleet mints for API-only users are stored in the state file in cleartext, as Terraform state has no notion of write-only values. Anyone who can read the state file can authenticate as these users. Use a remote backend with encryption at rest and restricted access, never commit state to version control, and prefer passing passwords in through variables sourced from a secrets manager rather than literals in configuration.

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

Fleet mints an API token for API-only users and returns it only once, at
creation. It is exported as the sensitive ` + "`token`" + ` attribute.

` + "```hcl" + `
resource "fleetdm_user" "api_user" {
  name        = "API Service Account"
  email       = "api@example.com"
  password    = "SecurePassword123!"
  global_role = "maintainer"
  api_only    = true
}

output "api_user_token" {
  value     = fleetdm_user.api_user.token
  sensitive = true
}
` + "```" + `

### API-Only User Restricted to Specific Endpoints (Fleet Premium)

` + "`api_endpoints`" + ` narrows an API-only user to the listed endpoints; calls to
anything else are rejected with a 403. Omit it to leave the user able to reach
every endpoint its role allows. Use the ` + "`fleetdm_rest_api_endpoints`" + ` data
source to discover valid method/path pairs.

` + "```hcl" + `
resource "fleetdm_user" "host_reader" {
  name        = "Host Reader"
  email       = "host-reader@example.com"
  password    = "SecurePassword123!"
  global_role = "observer"
  api_only    = true

  api_endpoints = [
    {
      method = "GET"
      path   = "/api/v1/fleet/hosts"
    },
    {
      method = "GET"
      path   = "/api/v1/fleet/hosts/:id"
    },
  ]
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
				Description: "The full name of the user. At most 255 characters — Fleet's API surfaces a longer value as a raw MySQL " +
					"\"Data too long\" error, so the limit is enforced at plan time.",
				MarkdownDescription: "The full name of the user. At most 255 characters — Fleet's API surfaces a longer value as a raw MySQL " +
					"`Data too long` error, so the limit is enforced at plan time.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(fleetdm.MaxNameLength),
				},
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
				Description:         "Whether this user is API-only (cannot use web UI). Immutable after create — Fleet's user-update endpoint rejects api_only, so changing this value forces the user to be destroyed and recreated.",
				MarkdownDescription: "Whether this user is API-only (cannot use web UI). Immutable after create — Fleet's user-update endpoint rejects `api_only`, so changing this value forces the user to be destroyed and recreated.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"api_endpoints": schema.SetNestedAttribute{
				Description:         "Restricts an API-only user to this set of REST API endpoints. Only valid when api_only is true. Omit the attribute to leave the user unrestricted (able to call every endpoint its role allows); removing it later restores that unrestricted access. Each method/path pair must match an entry of the fleetdm_rest_api_endpoints catalog. Fleet Premium only, and requires Fleet 4.90 or later.",
				MarkdownDescription: "Restricts an API-only user to this set of REST API endpoints. Only valid when `api_only` is `true`. Omit the attribute to leave the user unrestricted (able to call every endpoint its role allows); removing it later restores that unrestricted access. Each method/path pair must match an entry of the `fleetdm_rest_api_endpoints` catalog. Fleet Premium only, and requires Fleet 4.90 or later.",
				Optional:            true,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"method": schema.StringAttribute{
							Description:         "The HTTP method of the endpoint. Options: GET, POST, PUT, PATCH, DELETE.",
							MarkdownDescription: "The HTTP method of the endpoint. Options: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`.",
							Required:            true,
							Validators: []validator.String{
								// Mirrors the methods Fleet accepts today
								// (server/fleet/api_endpoints.go,
								// validHTTPMethods). If Fleet ever admits
								// another verb into the catalog, this list has
								// to be widened to match or the new method
								// will be rejected at plan time.
								stringvalidator.OneOf("GET", "POST", "PUT", "PATCH", "DELETE"),
							},
						},
						"path": schema.StringAttribute{
							Description:         "The route template of the endpoint, with :name placeholders for path parameters, for example /api/v1/fleet/hosts/:id.",
							MarkdownDescription: "The route template of the endpoint, with `:name` placeholders for path parameters, for example `/api/v1/fleet/hosts/:id`.",
							Required:            true,
						},
					},
				},
			},
			"token": schema.StringAttribute{
				Description:         "The API token Fleet mints for API-only, non-SSO users. Fleet returns it once, when the user is created, and never again — it is therefore stored in Terraform state and cannot be recovered by re-reading the user. Null for every other user, and for users adopted through terraform import.",
				MarkdownDescription: "The API token Fleet mints for API-only, non-SSO users. Fleet returns it once, when the user is created, and never again — it is therefore stored in Terraform state and cannot be recovered by re-reading the user. Null for every other user, and for users adopted through `terraform import`.",
				Computed:            true,
				Sensitive:           true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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

// ValidateConfig rejects an `api_endpoints` scope on a user that is not
// API-only. Fleet enforces the same rule server-side ("API endpoints can only
// be specified for API only users"); catching it at plan time turns an
// apply-time 422 into a config error.
func (r *UserResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config UserResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.APIEndpoints.IsNull() || config.APIEndpoints.IsUnknown() {
		return
	}
	if config.APIOnly.IsUnknown() {
		return
	}

	// api_only defaults to false, so a null config value is still "not
	// API-only" by the time the request reaches Fleet.
	if !config.APIOnly.ValueBool() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_endpoints"),
			"Invalid Attribute Combination",
			"`api_endpoints` restricts an API-only user to a set of REST API endpoints and can only be "+
				"set when `api_only` is `true`. Set `api_only = true`, or remove `api_endpoints`.",
		)
	}
}

// apiEndpointRefsFromSet converts the `api_endpoints` attribute into the
// client representation. It returns nil when the set is null or unknown.
func apiEndpointRefsFromSet(ctx context.Context, set types.Set, diags *diag.Diagnostics) []fleetdm.APIEndpointRef {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}

	var models []UserAPIEndpointModel
	diags.Append(set.ElementsAs(ctx, &models, false)...)
	if diags.HasError() {
		return nil
	}

	refs := make([]fleetdm.APIEndpointRef, 0, len(models))
	for _, m := range models {
		refs = append(refs, fleetdm.APIEndpointRef{
			Method: m.Method.ValueString(),
			Path:   m.Path.ValueString(),
		})
	}
	return refs
}

// applyAPIEndpoints pushes the desired `api_endpoints` scope to Fleet through
// PATCH /users/api_only/{id}, the only endpoint that accepts the field.
//
// A nil refs slice clears the scope, which Fleet expects as a JSON null rather
// than an empty array.
func (r *UserResource) applyAPIEndpoints(ctx context.Context, id int64, refs []fleetdm.APIEndpointRef) (*fleetdm.User, error) {
	// Fleet rejects an empty array ("at least one API endpoint must be
	// specified") and expects a null to clear the scope, so normalize an empty
	// slice to nil first. A non-nil pointer to a nil slice serializes to
	// `null`; a pointer to a populated slice replaces the scope.
	if len(refs) == 0 {
		refs = nil
	}
	payload := &refs

	tflog.Debug(ctx, "Setting API-only user endpoint scope", map[string]interface{}{
		"id":    id,
		"count": len(refs),
	})

	return r.client.ModifyAPIOnlyUser(ctx, id, fleetdm.ModifyAPIOnlyUserRequest{
		APIEndpoints: payload,
	})
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

	// The endpoint scope cannot travel with the create call: POST /users/admin
	// rejects `api_endpoints` with a 422. Collect it now and apply it as a
	// follow-up PATCH once the user exists.
	endpointRefs := apiEndpointRefsFromSet(ctx, plan.APIEndpoints, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	user, token, err := r.client.CreateUser(ctx, createReq)
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

	if len(endpointRefs) > 0 {
		scoped, err := r.applyAPIEndpoints(ctx, user.ID, endpointRefs)
		if err != nil {
			// The user exists but is unrestricted, which is more permissive
			// than intended. Persist it anyway so the next apply can converge
			// instead of leaking an unmanaged user.
			resp.Diagnostics.AddError(
				"Error Setting FleetDM User API Endpoints",
				"The user was created but its api_endpoints scope could not be applied, so it currently has "+
					"access to every endpoint its role allows. Terraform marks the resource tainted, so the "+
					"next apply will destroy and recreate the user, minting a new API token in the process. "+
					"If the error below is a missing-license error, api_endpoints requires Fleet Premium and "+
					"every retry will fail the same way — remove the attribute or upgrade the license. "+
					"Error: "+err.Error(),
			)
			r.mapUserToModel(ctx, user, &plan, &resp.Diagnostics)
			plan.Token = tokenValue(token)
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			return
		}
		user = scoped
	}

	// Map response to model
	r.mapUserToModel(ctx, user, &plan, &resp.Diagnostics)

	// Fleet returns the API token once, at creation, and never on read.
	plan.Token = tokenValue(token)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// tokenValue wraps the create-time API token, mapping the empty string Fleet
// returns for non-API-only users to null.
func tokenValue(token string) types.String {
	if token == "" {
		return types.StringNull()
	}
	return types.StringValue(token)
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

	// Preserve the write-only password and the create-only API token: neither
	// is returned by a read, so re-deriving them from the response would wipe
	// them from state.
	password := state.Password
	token := state.Token

	r.mapUserToModel(ctx, user, &state, &resp.Diagnostics)

	state.Password = password
	state.Token = token

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

	// `api_only` is intentionally NOT sent on update: Fleet's user-update
	// endpoint rejects the field with a 422 ("api_endpoints: This endpoint
	// does not accept API endpoint values"). Toggling the flag requires a
	// resource replacement, enforced by the RequiresReplace plan modifier
	// on the `api_only` schema attribute.

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

	// `api_endpoints` is not accepted by PATCH /users/{id}; it is applied
	// separately through PATCH /users/api_only/{id}. Only send it when the
	// desired scope actually differs from what is already in state, so that
	// unrelated updates leave the scope untouched.
	scopeChanged := !plan.APIEndpoints.Equal(state.APIEndpoints)
	if scopeChanged {
		refs := apiEndpointRefsFromSet(ctx, plan.APIEndpoints, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}

		scoped, err := r.applyAPIEndpoints(ctx, plan.ID.ValueInt64(), refs)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Updating FleetDM User API Endpoints",
				"Could not update the user's api_endpoints scope, unexpected error: "+err.Error(),
			)
			return
		}
		user = scoped
	}

	r.mapUserToModel(ctx, user, &plan, &resp.Diagnostics)

	if !scopeChanged {
		// An update that leaves the scope alone must not let the shape of the
		// PATCH /users/{id} response decide what lands in state. Fleet does
		// echo `api_endpoints` there today, but relying on that would turn any
		// future change — or a Fleet Free response, which omits the field —
		// into an "inconsistent result after apply" error on an update that
		// never touched the scope. Carry the planned value through instead,
		// the same way the write-only password and create-only token are.
		plan.APIEndpoints = state.APIEndpoints
	}

	// The API token is minted at creation only; carry it across the update.
	plan.Token = state.Token

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

	// Map the API endpoint scope. Fleet omits the field for unrestricted users
	// (and for every non-API-only user), which maps to a null set.
	if len(user.APIEndpoints) > 0 {
		endpointElements := make([]attr.Value, len(user.APIEndpoints))
		for i, e := range user.APIEndpoints {
			endpointObj, d := types.ObjectValue(
				userAPIEndpointAttrTypes,
				map[string]attr.Value{
					"method": types.StringValue(e.Method),
					"path":   types.StringValue(e.Path),
				},
			)
			if d.HasError() {
				diags.Append(d...)
				return
			}
			endpointElements[i] = endpointObj
		}
		endpointSet, d := types.SetValue(userAPIEndpointObjectType, endpointElements)
		if d.HasError() {
			diags.Append(d...)
			return
		}
		model.APIEndpoints = endpointSet
	} else {
		model.APIEndpoints = types.SetNull(userAPIEndpointObjectType)
	}

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
