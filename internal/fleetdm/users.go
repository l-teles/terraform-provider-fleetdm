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
type UpdateUserRequest struct {
	Name        string     `json:"name,omitempty"`
	Email       string     `json:"email,omitempty"`
	Position    string     `json:"position,omitempty"`
	SSOEnabled  *bool      `json:"sso_enabled,omitempty"`
	MFAEnabled  *bool      `json:"mfa_enabled,omitempty"`
	APIOnly     *bool      `json:"api_only,omitempty"`
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
func (c *Client) CreateUser(ctx context.Context, req CreateUserRequest) (*User, error) {
	var resp CreateUserResponse
	err := c.Post(ctx, "/users/admin", req, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
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
