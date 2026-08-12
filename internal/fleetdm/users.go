package fleetdm

import (
	"context"
	"fmt"
	"strconv"
)

// User represents a FleetDM user.
type User struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Email              string     `json:"email"`
	Password           string     `json:"password,omitempty"`
	GlobalRole         *string    `json:"global_role"`
	Enabled            bool       `json:"enabled,omitempty"`
	ForcePasswordReset bool       `json:"force_password_reset,omitempty"`
	GravatarURL        string     `json:"gravatar_url,omitempty"`
	SSOEnabled         bool       `json:"sso_enabled,omitempty"`
	MFAEnabled         bool       `json:"mfa_enabled,omitempty"`
	APIOnly            bool       `json:"api_only,omitempty"`
	CreatedAt          string     `json:"created_at,omitempty"`
	UpdatedAt          string     `json:"updated_at,omitempty"`
	Teams              []UserTeam `json:"teams,omitempty"`

	// APIEndpoints is the set of endpoints an API-only user is restricted to.
	// Fleet only populates it for api_only users that have a scope configured;
	// an empty value means the user may call every registered endpoint,
	// subject to its role.
	APIEndpoints []APIEndpointRef `json:"api_endpoints,omitempty"`
}

// APIEndpointRef identifies one endpoint in an API-only user's access scope.
// The pair must match an entry of Fleet's REST API catalog (see
// Client.ListAPIEndpoints); Path is a route template using Fleet's `:name`
// placeholder convention, e.g. "/api/v1/fleet/hosts/:id".
type APIEndpointRef struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// UserTeam represents a team assignment for a user.
type UserTeam struct {
	ID          int64  `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Role        string `json:"role"`
}

// ListUsersResponse represents the response from listing users.
type ListUsersResponse struct {
	Users []User `json:"users"`
}

// GetUserResponse represents the response from getting a user.
type GetUserResponse struct {
	User User `json:"user"`
}

// CreateUserRequest represents the request to create a user.
type CreateUserRequest struct {
	Name                     string     `json:"name"`
	Email                    string     `json:"email"`
	Password                 string     `json:"password,omitempty"`
	SSOEnabled               bool       `json:"sso_enabled,omitempty"`
	MFAEnabled               bool       `json:"mfa_enabled,omitempty"`
	APIOnly                  bool       `json:"api_only,omitempty"`
	GlobalRole               *string    `json:"global_role,omitempty"`
	Teams                    []UserTeam `json:"teams,omitempty"`
	AdminForcedPasswordReset *bool      `json:"admin_forced_password_reset,omitempty"`
}

// CreateUserResponse represents the response from creating a user.
type CreateUserResponse struct {
	User  User   `json:"user"`
	Token string `json:"token,omitempty"` // Only returned for API-only users
}

// UpdateUserRequest represents the request to update a user.
//
// `api_only` and `api_endpoints` are intentionally absent: Fleet's
// PATCH /users/{id} endpoint rejects both fields outright (422
// "api_endpoints: This endpoint does not accept API endpoint values"). The
// api-only flag can only be set at user creation — the provider enforces this
// via a RequiresReplace plan modifier on the `api_only` schema attribute — and
// the endpoint scope is managed through ModifyAPIOnlyUser instead.
type UpdateUserRequest struct {
	Name        string     `json:"name,omitempty"`
	Email       string     `json:"email,omitempty"`
	Position    string     `json:"position,omitempty"`
	SSOEnabled  *bool      `json:"sso_enabled,omitempty"`
	MFAEnabled  *bool      `json:"mfa_enabled,omitempty"`
	GlobalRole  *string    `json:"global_role,omitempty"`
	Teams       []UserTeam `json:"teams,omitempty"`
	Password    string     `json:"password,omitempty"`     // Current password (required for email/password changes)
	NewPassword string     `json:"new_password,omitempty"` // New password
}

// UpdateUserResponse represents the response from updating a user.
type UpdateUserResponse struct {
	User User `json:"user"`
}

// ListUsers returns a list of all users.
func (c *Client) ListUsers(ctx context.Context, params map[string]string) ([]User, error) {
	var resp ListUsersResponse
	err := c.Get(ctx, "/users", params, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	return resp.Users, nil
}

// GetUser returns a user by ID.
func (c *Client) GetUser(ctx context.Context, id int64) (*User, error) {
	var resp GetUserResponse
	err := c.Get(ctx, "/users/"+strconv.FormatInt(id, 10), nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &resp.User, nil
}

// CreateUser creates a new user.
//
// The second return value is the API session token Fleet mints for API-only,
// non-SSO users; it is empty for every other user. Fleet returns it exactly
// once, at creation, and never again on read — callers that need it must
// persist it themselves.
//
// Note that `api_endpoints` cannot be supplied here: POST /users/admin rejects
// the field with a 422. Set an API-only user's endpoint scope with a follow-up
// ModifyAPIOnlyUser call.
func (c *Client) CreateUser(ctx context.Context, req CreateUserRequest) (*User, string, error) {
	var resp CreateUserResponse
	err := c.Post(ctx, "/users/admin", req, &resp)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create user: %w", err)
	}
	return &resp.User, resp.Token, nil
}

// ModifyAPIOnlyUserRequest represents the request to update an API-only user
// via PATCH /users/api_only/{id}, the only endpoint that accepts
// `api_endpoints`.
//
// Fleet rejects the call with a 422 when the target user is not API-only
// ("target user is not an API-only user") or when it is the caller's own user.
type ModifyAPIOnlyUserRequest struct {
	Name       string     `json:"name,omitempty"`
	GlobalRole *string    `json:"global_role,omitempty"`
	Teams      []UserTeam `json:"teams,omitempty"`

	// APIEndpoints reproduces Fleet's three-state semantics for the field:
	//
	//   nil pointer                  → key omitted: leave the scope unchanged.
	//   pointer to a nil slice       → JSON null: clear the scope, restoring
	//                                  access to every endpoint.
	//   pointer to a non-empty slice → replace the scope with those entries.
	//
	// A pointer to an empty (but non-nil) slice serializes to `[]`, which
	// Fleet rejects with a 422 ("at least one API endpoint must be
	// specified"); use the nil-slice form to clear instead.
	APIEndpoints *[]APIEndpointRef `json:"api_endpoints,omitempty"`
}

// ModifyAPIOnlyUser updates an API-only user, including its `api_endpoints`
// access scope.
func (c *Client) ModifyAPIOnlyUser(ctx context.Context, id int64, req ModifyAPIOnlyUserRequest) (*User, error) {
	var resp UpdateUserResponse
	err := c.Patch(ctx, "/users/api_only/"+strconv.FormatInt(id, 10), req, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to update API-only user: %w", err)
	}
	return &resp.User, nil
}

// UpdateUser updates an existing user.
func (c *Client) UpdateUser(ctx context.Context, id int64, req UpdateUserRequest) (*User, error) {
	var resp UpdateUserResponse
	err := c.Patch(ctx, "/users/"+strconv.FormatInt(id, 10), req, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}
	return &resp.User, nil
}

// DeleteUser deletes a user.
func (c *Client) DeleteUser(ctx context.Context, id int64) error {
	err := c.Delete(ctx, "/users/"+strconv.FormatInt(id, 10), nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}
