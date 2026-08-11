package fleetdm

import (
	"context"
	"fmt"
	"strconv"
)

// selfServiceCategoriesPath is the base path for the self-service category
// endpoints. All four verbs (GET/POST/PATCH/DELETE) are registered by Fleet
// >= 4.90; the feature itself is Fleet Premium only.
const selfServiceCategoriesPath = "/software/self_service_categories"

// SelfServiceCategoryNameMaxLength mirrors Fleet's server-side limit on
// category names. Fleet counts runes (utf8.RuneCountInString), not bytes, so
// emoji-prefixed names such as "🌎 Browsers" are measured by character.
const SelfServiceCategoryNameMaxLength = 255

// SelfServiceCategory represents a self-service software category scoped to a
// fleet (team). Categories group self-service software so end users can browse
// it by category on the "My device > Self-service" page.
//
// Fleet is transitioning the team field name: the server struct declares
// `team_id` with a `renameto:"fleet_id"` tag, so a response may carry either
// key (or both, as observed on Fleet 4.90). Both are deserialized and
// consolidated by normalizeSelfServiceCategory, which prefers the new
// "fleet_id" name.
type SelfServiceCategory struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	FleetID   *int64 `json:"fleet_id,omitempty"` // new name for the team field
	TeamID    *int64 `json:"team_id,omitempty"`  // legacy name for the team field
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// normalizeSelfServiceCategory consolidates the dual team field names after
// deserialization, preferring "fleet_id" and falling back to "team_id".
func normalizeSelfServiceCategory(c *SelfServiceCategory) {
	if c.FleetID != nil {
		c.TeamID = c.FleetID
	} else {
		c.FleetID = c.TeamID
	}
}

// listSelfServiceCategoriesResponse is the API response for listing
// self-service categories.
type listSelfServiceCategoriesResponse struct {
	SelfServiceCategories []SelfServiceCategory `json:"self_service_categories"`
}

// selfServiceCategoryResponse is the API response wrapper returned by the
// create and update endpoints.
type selfServiceCategoryResponse struct {
	SelfServiceCategory *SelfServiceCategory `json:"self_service_category"`
}

// CreateSelfServiceCategoryRequest is the request body for creating a
// self-service category.
type CreateSelfServiceCategoryRequest struct {
	FleetID int64  `json:"fleet_id"`
	Name    string `json:"name"`
}

// UpdateSelfServiceCategoryRequest is the request body for renaming a
// self-service category. Name is the only mutable field.
type UpdateSelfServiceCategoryRequest struct {
	Name string `json:"name"`
}

// ListSelfServiceCategories retrieves the self-service categories on a fleet.
//
// Fleet requires the fleet_id query parameter and rejects the request with 422
// ("fleet_id is required") when it is absent, so it is always sent — including
// the fleetID == 0 case, which selects the categories for hosts that are not
// assigned to a fleet.
func (c *Client) ListSelfServiceCategories(ctx context.Context, fleetID int64) ([]SelfServiceCategory, error) {
	params := map[string]string{
		"fleet_id": strconv.FormatInt(fleetID, 10),
	}

	var response listSelfServiceCategoriesResponse
	if err := c.Get(ctx, selfServiceCategoriesPath, params, &response); err != nil {
		return nil, fmt.Errorf("failed to list self-service categories for fleet %d: %w", fleetID, err)
	}

	categories := response.SelfServiceCategories
	for i := range categories {
		normalizeSelfServiceCategory(&categories[i])
	}
	return categories, nil
}

// GetSelfServiceCategory retrieves a single self-service category by ID.
//
// Fleet exposes no per-category GET endpoint, so this lists the fleet's
// categories and matches on ID. A category that is absent from the list is not
// an error: (nil, nil) is returned so callers can distinguish "deleted out of
// band" from a transport or authorization failure.
func (c *Client) GetSelfServiceCategory(ctx context.Context, fleetID, id int64) (*SelfServiceCategory, error) {
	categories, err := c.ListSelfServiceCategories(ctx, fleetID)
	if err != nil {
		return nil, err
	}

	for i := range categories {
		if categories[i].ID == id {
			return &categories[i], nil
		}
	}
	return nil, nil
}

// CreateSelfServiceCategory creates a new self-service category on a fleet.
// Names must be unique within the fleet (case-insensitive).
func (c *Client) CreateSelfServiceCategory(ctx context.Context, req CreateSelfServiceCategoryRequest) (*SelfServiceCategory, error) {
	var response selfServiceCategoryResponse
	if err := c.Post(ctx, selfServiceCategoriesPath, req, &response); err != nil {
		return nil, fmt.Errorf("failed to create self-service category: %w", err)
	}
	if response.SelfServiceCategory == nil {
		return nil, fmt.Errorf("failed to create self-service category: response contained no category")
	}

	normalizeSelfServiceCategory(response.SelfServiceCategory)
	return response.SelfServiceCategory, nil
}

// UpdateSelfServiceCategory renames an existing self-service category.
func (c *Client) UpdateSelfServiceCategory(ctx context.Context, id int64, req UpdateSelfServiceCategoryRequest) (*SelfServiceCategory, error) {
	endpoint := fmt.Sprintf("%s/%d", selfServiceCategoriesPath, id)

	var response selfServiceCategoryResponse
	if err := c.Patch(ctx, endpoint, req, &response); err != nil {
		return nil, fmt.Errorf("failed to update self-service category %d: %w", id, err)
	}
	if response.SelfServiceCategory == nil {
		return nil, fmt.Errorf("failed to update self-service category %d: response contained no category", id)
	}

	normalizeSelfServiceCategory(response.SelfServiceCategory)
	return response.SelfServiceCategory, nil
}

// DeleteSelfServiceCategory deletes a self-service category by ID. Software
// assigned to the category is removed from it but otherwise unaffected.
func (c *Client) DeleteSelfServiceCategory(ctx context.Context, id int64) error {
	endpoint := fmt.Sprintf("%s/%d", selfServiceCategoriesPath, id)
	if err := c.Delete(ctx, endpoint, nil, nil); err != nil {
		return fmt.Errorf("failed to delete self-service category %d: %w", id, err)
	}
	return nil
}
