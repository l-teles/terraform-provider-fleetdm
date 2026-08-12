package fleetdm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// customHostVitalsUnsupportedMessage explains a 404 from the collection
// endpoint, which means the API route is absent rather than the vital.
const customHostVitalsUnsupportedMessage = "custom host vitals are not supported by this Fleet server: " +
	"GET /custom_host_vitals returned 404, so the API route is absent. " +
	"The fleetdm_custom_host_vital resource requires Fleet 4.90 or later. " +
	"This is a missing route, not a missing vital — a vital that no longer exists comes back as an " +
	"empty result from a successful list, and the provider removes it from state in that case."

const (
	// customHostVitalListPerPage is the page size used when walking
	// GET /custom_host_vitals. Fleet applies no LIMIT when per_page is
	// omitted, but relying on that would break the moment Fleet adds a
	// default cap, so we page explicitly.
	customHostVitalListPerPage = 100

	// maxCustomHostVitalListPages bounds the pagination loop so a server that
	// keeps reporting has_next_results can't spin forever.
	maxCustomHostVitalListPages = 100
)

// CustomHostVital represents a FleetDM custom host vital.
//
// A custom host vital is a named slot whose per-host values are pushed into
// Fleet out-of-band (PUT /hosts/{host_id}/custom_host_vitals/{id}) and then
// referenced as a `$FLEET_HOST_VITAL_<id>` variable from configuration
// profiles, scripts and software installers, or matched by a host-vitals
// label's criteria.
//
// The entity itself carries no query: Fleet 4.90's
// `custom_host_vitals` table is (id, name, created_at, updated_at) and the
// create/update endpoints accept `name` only. Fleet's JSON decoder silently
// drops unknown request fields, so sending anything else looks like it
// succeeds while being discarded.
type CustomHostVital struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name"`
	// CreatedAt and UpdatedAt are populated by the list endpoint. Fleet's
	// create/update responses return them as empty strings because the
	// datastore doesn't read the row back after writing it.
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// ListCustomHostVitalsResponse represents the response from the list custom host vitals endpoint.
type ListCustomHostVitalsResponse struct {
	CustomHostVitals []CustomHostVital `json:"custom_host_vitals"`
	Meta             *PaginationMeta   `json:"meta"`
	Count            int               `json:"count"`
}

// CustomHostVitalResponse represents the response from the create and update
// custom host vital endpoints.
type CustomHostVitalResponse struct {
	CustomHostVital CustomHostVital `json:"custom_host_vital"`
}

// CreateCustomHostVitalRequest represents the request to create a custom host vital.
type CreateCustomHostVitalRequest struct {
	Name string `json:"name"`
}

// UpdateCustomHostVitalRequest represents the request to rename a custom host
// vital. Name is the only mutable field, and Fleet rejects an empty one with a
// 422 — this is a full replace of the name, not a partial patch.
type UpdateCustomHostVitalRequest struct {
	Name string `json:"name"`
}

// ListCustomHostVitals retrieves all custom host vitals, walking every page.
//
// Fleet returns them sorted by name ascending; callers that need a stable
// order by id must sort themselves.
func (c *Client) ListCustomHostVitals(ctx context.Context) ([]CustomHostVital, error) {
	var vitals []CustomHostVital

	for page := range maxCustomHostVitalListPages {
		params := map[string]string{
			"per_page": strconv.Itoa(customHostVitalListPerPage),
			"page":     strconv.Itoa(page),
		}

		var resp ListCustomHostVitalsResponse
		if err := c.Get(ctx, "/custom_host_vitals", params, &resp); err != nil {
			return nil, fmt.Errorf("failed to list custom host vitals (page %d): %w", page, err)
		}

		vitals = append(vitals, resp.CustomHostVitals...)

		if resp.Meta == nil || !resp.Meta.HasNextResults {
			return vitals, nil
		}
	}

	return nil, fmt.Errorf("custom host vital pagination exceeded %d pages — Fleet API may be returning has_next_results=true indefinitely", maxCustomHostVitalListPages)
}

// GetCustomHostVital retrieves a single custom host vital by ID.
//
// Fleet 4.90 exposes no GET /custom_host_vitals/{id} (that path is
// PATCH/DELETE only and answers 405 to a GET), so this filters the list
// endpoint. A missing id yields a synthetic 404 *APIError so callers can treat
// it like any other not-found response.
func (c *Client) GetCustomHostVital(ctx context.Context, id int) (*CustomHostVital, error) {
	vitals, err := c.ListCustomHostVitals(ctx)
	if err != nil {
		// A 404 from the collection endpoint is the *route* missing (Fleet
		// older than 4.90, or a proxy misroute), never the vital missing — an
		// absent vital comes back as an empty result from a successful 200
		// list. Deliberately reported without wrapping the *APIError, so it
		// cannot unwrap to a 404: callers key resource removal off the
		// not-in-list 404 synthesized below, and letting a route 404 reach them
		// would silently drop every vital from state and then propose
		// recreating them on a server that cannot accept them.
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			// err.Error() rather than %w: carrying the text forward without
			// the error value is what keeps errors.As from finding the 404.
			return nil, fmt.Errorf("%s Fleet reported: %s", customHostVitalsUnsupportedMessage, err.Error())
		}
		return nil, fmt.Errorf("failed to get custom host vital %d: %w", id, err)
	}

	for _, v := range vitals {
		if v.ID == id {
			return &v, nil
		}
	}

	return nil, &APIError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("custom host vital %d not found", id),
	}
}

// CreateCustomHostVital creates a new custom host vital.
//
// The response echoes id and name but leaves created_at/updated_at empty;
// callers that need timestamps should follow up with GetCustomHostVital.
func (c *Client) CreateCustomHostVital(ctx context.Context, req CreateCustomHostVitalRequest) (*CustomHostVital, error) {
	var resp CustomHostVitalResponse
	if err := c.Post(ctx, "/custom_host_vitals", req, &resp); err != nil {
		return nil, fmt.Errorf("failed to create custom host vital: %w", err)
	}
	return &resp.CustomHostVital, nil
}

// UpdateCustomHostVital renames an existing custom host vital in place.
//
// As with create, the response leaves created_at/updated_at empty.
func (c *Client) UpdateCustomHostVital(ctx context.Context, id int, req UpdateCustomHostVitalRequest) (*CustomHostVital, error) {
	var resp CustomHostVitalResponse
	endpoint := fmt.Sprintf("/custom_host_vitals/%d", id)
	if err := c.Patch(ctx, endpoint, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to update custom host vital %d: %w", id, err)
	}
	return &resp.CustomHostVital, nil
}

// DeleteCustomHostVital deletes a custom host vital by ID.
//
// Fleet answers 409 Conflict when the vital is still referenced by a
// configuration profile, script, software installer or host-vitals label; the
// error message names the referencing entity.
func (c *Client) DeleteCustomHostVital(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/custom_host_vitals/%d", id)
	if err := c.Delete(ctx, endpoint, nil, nil); err != nil {
		return fmt.Errorf("failed to delete custom host vital %d: %w", id, err)
	}
	return nil
}
