package fleetdm

import (
	"context"
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
	MacOSTeamID      *int   `json:"macos_team_id,omitempty"`
	IOSTeamID        *int   `json:"ios_team_id,omitempty"`
	IPadOSTeamID     *int   `json:"ipados_team_id,omitempty"`
	MacOSTeamName    string `json:"macos_team_name,omitempty"`
	IOSTeamName      string `json:"ios_team_name,omitempty"`
	IPadOSTeamName   string `json:"ipados_team_name,omitempty"`
}

// listABMTokensResponse is the API response for listing ABM tokens.
type listABMTokensResponse struct {
	ABMTokens []ABMToken `json:"abm_tokens"`
}

// ListABMTokens retrieves all ABM tokens.
func (c *Client) ListABMTokens(ctx context.Context) ([]ABMToken, error) {
	var response listABMTokensResponse
	err := c.Get(ctx, "/abm_tokens", nil, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to list ABM tokens: %w", err)
	}
	return response.ABMTokens, nil
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
