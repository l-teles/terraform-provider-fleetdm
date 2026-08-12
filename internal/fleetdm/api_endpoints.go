package fleetdm

import (
	"context"
	"fmt"
)

// APIEndpoint is one entry of Fleet's REST API catalog, as returned by
// GET /rest_api. The catalog is the authoritative list of endpoints that an
// API-only user's `api_endpoints` scope can reference: Fleet validates every
// submitted method/path pair against it and rejects unknown pairs with a 422.
//
// Path is a route template using Fleet's `:name` placeholder convention
// (for example "/api/v1/fleet/hosts/:id"), not a concrete request path.
type APIEndpoint struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	DisplayName string `json:"display_name"`
	Deprecated  bool   `json:"deprecated"`
}

// listAPIEndpointsResponse is the API response for listing the REST API catalog.
type listAPIEndpointsResponse struct {
	APIEndpoints []APIEndpoint `json:"api_endpoints"`
}

// ListAPIEndpoints retrieves Fleet's REST API endpoint catalog.
//
// The endpoint is registered by Fleet >= 4.90 but the underlying feature is
// Fleet Premium only: Fleet Free answers with a missing-license error rather
// than an empty catalog.
func (c *Client) ListAPIEndpoints(ctx context.Context) ([]APIEndpoint, error) {
	var resp listAPIEndpointsResponse
	if err := c.Get(ctx, "/rest_api", nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to list API endpoints: %w", err)
	}
	return resp.APIEndpoints, nil
}
