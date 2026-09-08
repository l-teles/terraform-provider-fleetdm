package fleetdm

import (
	"context"
	"errors"
	"fmt"
)

// ABMToken represents an Apple Business Manager token in FleetDM.
type ABMToken struct {
	ID               int    `json:"id"`
	AppleID          string `json:"apple_id"`
	OrganizationName string `json:"org_name"`
	MDMServerURL     string `json:"mdm_server_url,omitempty"`
	RenewDate        string `json:"renew_date,omitempty"`
	TermsExpired     bool   `json:"terms_expired,omitempty"`
	// TokenInvalid reports that Apple rejected the token itself (revoked,
	// invalid signature, or a server-side error), as opposed to TermsExpired
	// which means the Apple Business terms need re-accepting. Fleet 4.91+.
	TokenInvalid   bool   `json:"token_invalid,omitempty"`
	MacOSTeamID    *int   `json:"macos_team_id,omitempty"`
	IOSTeamID      *int   `json:"ios_team_id,omitempty"`
	IPadOSTeamID   *int   `json:"ipados_team_id,omitempty"`
	MacOSTeamName  string `json:"macos_team_name,omitempty"`
	IOSTeamName    string `json:"ios_team_name,omitempty"`
	IPadOSTeamName string `json:"ipados_team_name,omitempty"`
}

// listABMTokensResponse is the API response for listing ABM tokens. Fleet 4.87
// renamed the response key to "ab_tokens"; both keys are decoded and resolved.
type listABMTokensResponse struct {
	ABTokens  []ABMToken `json:"ab_tokens"`
	ABMTokens []ABMToken `json:"abm_tokens"`
}

// resolve returns the token list, preferring the new "ab_tokens" key
// and falling back to "abm_tokens".
func (r *listABMTokensResponse) resolve() []ABMToken {
	if len(r.ABTokens) > 0 {
		return r.ABTokens
	}
	return r.ABMTokens
}

// ListABMTokens retrieves all Apple Business (formerly Apple Business Manager)
// tokens. Fleet 4.87 deprecated GET /abm_tokens in favor of GET /ab_tokens, so
// the new path is tried first with a fallback to the legacy path on 404 to
// keep supporting Fleet >= 4.82.
func (c *Client) ListABMTokens(ctx context.Context) ([]ABMToken, error) {
	var response listABMTokensResponse
	err := c.Get(ctx, "/ab_tokens", nil, &response)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			response = listABMTokensResponse{}
			err = c.Get(ctx, "/abm_tokens", nil, &response)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list ABM tokens: %w", err)
	}
	return response.resolve(), nil
}

// VPPToken represents a Volume Purchase Program token in FleetDM.
type VPPToken struct {
	ID               int    `json:"id"`
	OrganizationName string `json:"org_name"`
	Location         string `json:"location,omitempty"`
	RenewDate        string `json:"renew_date,omitempty"`
	Teams            []Team `json:"teams,omitempty"`
}

// listVPPTokensResponse is the API response for listing VPP tokens.
type listVPPTokensResponse struct {
	VPPTokens []VPPToken `json:"vpp_tokens"`
}

// ListVPPTokens retrieves all VPP tokens.
func (c *Client) ListVPPTokens(ctx context.Context) ([]VPPToken, error) {
	var response listVPPTokensResponse
	err := c.Get(ctx, "/vpp_tokens", nil, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to list VPP tokens: %w", err)
	}
	return response.VPPTokens, nil
}
