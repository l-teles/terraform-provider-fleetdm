package fleetdm

import (
	"context"
	"fmt"
)

// Certificate authority types accepted by Fleet in the
// /certificate_authorities payloads. These double as the JSON object keys.
const (
	CATypeDigiCert        = "digicert"
	CATypeNDESSCEPProxy   = "ndes_scep_proxy"
	CATypeCustomSCEPProxy = "custom_scep_proxy"
	CATypeCustomESTProxy  = "custom_est_proxy"
	CATypeHydrant         = "hydrant"
	CATypeSmallstep       = "smallstep"
)

// MaskedCASecret is the placeholder Fleet substitutes for stored certificate
// authority secrets. GET /certificate_authorities/{id} always masks them: the
// API layer calls the datastore with includeSecrets=false, so there is no
// parameter that returns the real value. Fleet also accepts this placeholder in
// an update payload to mean "leave the stored secret unchanged".
const MaskedCASecret = "********"

// NDESCAName is the name Fleet assigns to the NDES SCEP proxy certificate
// authority. NDES is a singleton and its name is not settable.
const NDESCAName = "NDES"

// CertificateAuthoritySummary is a certificate authority as returned by
// GET /certificate_authorities. The list endpoint returns identity only.
type CertificateAuthoritySummary struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// CertificateAuthority is a certificate authority as returned by
// GET /certificate_authorities/{id}. Fleet returns a flat object holding the
// fields relevant to the CA's type, with every secret replaced by
// MaskedCASecret. Fields absent for a given type decode as zero values.
type CertificateAuthority struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`

	// Shared by every type.
	URL string `json:"url,omitempty"`

	// DigiCert.
	APIToken                      string   `json:"api_token,omitempty"`
	ProfileID                     string   `json:"profile_id,omitempty"`
	CertificateCommonName         string   `json:"certificate_common_name,omitempty"`
	CertificateUserPrincipalNames []string `json:"certificate_user_principal_names,omitempty"`
	CertificateSeatID             string   `json:"certificate_seat_id,omitempty"`

	// NDES SCEP proxy.
	AdminURL string `json:"admin_url,omitempty"`

	// Custom SCEP proxy.
	Challenge string `json:"challenge,omitempty"`

	// Custom EST proxy, NDES SCEP proxy and Smallstep.
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	// Hydrant.
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`

	// Smallstep.
	ChallengeURL string `json:"challenge_url,omitempty"`

	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// DigiCertCA is the DigiCert certificate authority configuration.
type DigiCertCA struct {
	Name                  *string `json:"name,omitempty"`
	URL                   string  `json:"url"`
	APIToken              string  `json:"api_token"`
	ProfileID             string  `json:"profile_id"`
	CertificateCommonName string  `json:"certificate_common_name"`
	CertificateSeatID     string  `json:"certificate_seat_id"`
	// CertificateUserPrincipalNames is a pointer so an empty list is sent as
	// `[]` rather than omitted: Fleet's update payload treats an absent value
	// as "do not modify", which would make clearing the list impossible.
	CertificateUserPrincipalNames *[]string `json:"certificate_user_principal_names"`
}

// NDESSCEPProxyCA is the NDES SCEP proxy certificate authority configuration.
// It carries no name: Fleet fixes the name to NDESCAName and rejects a name in
// the update payload.
type NDESSCEPProxyCA struct {
	URL      string `json:"url"`
	AdminURL string `json:"admin_url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// CustomSCEPProxyCA is the custom SCEP proxy certificate authority configuration.
type CustomSCEPProxyCA struct {
	Name      *string `json:"name,omitempty"`
	URL       string  `json:"url"`
	Challenge string  `json:"challenge"`
}

// CustomESTProxyCA is the custom EST proxy certificate authority configuration.
type CustomESTProxyCA struct {
	Name     *string `json:"name,omitempty"`
	URL      string  `json:"url"`
	Username string  `json:"username"`
	Password string  `json:"password"`
}

// HydrantCA is the Hydrant certificate authority configuration.
type HydrantCA struct {
	Name         *string `json:"name,omitempty"`
	URL          string  `json:"url"`
	ClientID     string  `json:"client_id"`
	ClientSecret string  `json:"client_secret"`
}

// SmallstepSCEPProxyCA is the Smallstep SCEP certificate authority configuration.
type SmallstepSCEPProxyCA struct {
	Name         *string `json:"name,omitempty"`
	URL          string  `json:"url"`
	ChallengeURL string  `json:"challenge_url"`
	Username     string  `json:"username"`
	Password     string  `json:"password"`
}

// CertificateAuthorityPayload is the request body for creating and updating a
// certificate authority. Exactly one field must be set; Fleet rejects zero or
// more than one.
//
// The same shape serves POST and PATCH. Fleet's update payload uses pointer
// fields, so an omitted field means "do not modify" — but it also refuses to
// change a URL unless the type's secret is supplied in the same request (for
// example custom SCEP requires "challenge" when "url" changes). Sending the
// complete block on every update satisfies those rules unconditionally.
type CertificateAuthorityPayload struct {
	DigiCert        *DigiCertCA           `json:"digicert,omitempty"`
	NDESSCEPProxy   *NDESSCEPProxyCA      `json:"ndes_scep_proxy,omitempty"`
	CustomSCEPProxy *CustomSCEPProxyCA    `json:"custom_scep_proxy,omitempty"`
	CustomESTProxy  *CustomESTProxyCA     `json:"custom_est_proxy,omitempty"`
	Hydrant         *HydrantCA            `json:"hydrant,omitempty"`
	Smallstep       *SmallstepSCEPProxyCA `json:"smallstep,omitempty"`
}

// ClearName drops the name from the payload's block.
//
// Fleet's update endpoint checks name uniqueness without excluding the CA being
// updated, so a PATCH that re-sends the CA's current name is rejected with 409
// "a certificate authority with this name already exists". An update that does
// not rename must therefore omit the field entirely. NDES carries no name and
// is unaffected.
func (p *CertificateAuthorityPayload) ClearName() {
	switch {
	case p.DigiCert != nil:
		p.DigiCert.Name = nil
	case p.CustomSCEPProxy != nil:
		p.CustomSCEPProxy.Name = nil
	case p.CustomESTProxy != nil:
		p.CustomESTProxy.Name = nil
	case p.Hydrant != nil:
		p.Hydrant.Name = nil
	case p.Smallstep != nil:
		p.Smallstep.Name = nil
	}
}

// Name reports the name the payload's block carries, and whether it carries one
// at all. NDES never does.
func (p *CertificateAuthorityPayload) Name() (string, bool) {
	var name *string
	switch {
	case p.DigiCert != nil:
		name = p.DigiCert.Name
	case p.CustomSCEPProxy != nil:
		name = p.CustomSCEPProxy.Name
	case p.CustomESTProxy != nil:
		name = p.CustomESTProxy.Name
	case p.Hydrant != nil:
		name = p.Hydrant.Name
	case p.Smallstep != nil:
		name = p.Smallstep.Name
	}
	if name == nil {
		return "", false
	}
	return *name, true
}

// Type reports the certificate authority type the payload carries, or an empty
// string when no block is set.
func (p *CertificateAuthorityPayload) Type() string {
	switch {
	case p.DigiCert != nil:
		return CATypeDigiCert
	case p.NDESSCEPProxy != nil:
		return CATypeNDESSCEPProxy
	case p.CustomSCEPProxy != nil:
		return CATypeCustomSCEPProxy
	case p.CustomESTProxy != nil:
		return CATypeCustomESTProxy
	case p.Hydrant != nil:
		return CATypeHydrant
	case p.Smallstep != nil:
		return CATypeSmallstep
	default:
		return ""
	}
}

// listCertificateAuthoritiesResponse is the response for listing CAs.
type listCertificateAuthoritiesResponse struct {
	CertificateAuthorities []CertificateAuthoritySummary `json:"certificate_authorities"`
}

// ListCertificateAuthorities retrieves every certificate authority. Fleet
// returns identity only (id, name, type) — never configuration.
func (c *Client) ListCertificateAuthorities(ctx context.Context) ([]CertificateAuthoritySummary, error) {
	var response listCertificateAuthoritiesResponse
	if err := c.Get(ctx, "/certificate_authorities", nil, &response); err != nil {
		return nil, fmt.Errorf("failed to list certificate authorities: %w", err)
	}
	return response.CertificateAuthorities, nil
}

// GetCertificateAuthority retrieves a single certificate authority. Every
// secret in the response is masked as MaskedCASecret.
func (c *Client) GetCertificateAuthority(ctx context.Context, id int64) (*CertificateAuthority, error) {
	var ca CertificateAuthority
	if err := c.Get(ctx, fmt.Sprintf("/certificate_authorities/%d", id), nil, &ca); err != nil {
		return nil, fmt.Errorf("failed to get certificate authority %d: %w", id, err)
	}
	return &ca, nil
}

// CreateCertificateAuthority creates a certificate authority. Fleet validates
// that the configured URL is reachable and speaks the expected protocol before
// storing anything, so unreachable endpoints fail here rather than at use time.
func (c *Client) CreateCertificateAuthority(ctx context.Context, payload *CertificateAuthorityPayload) (*CertificateAuthoritySummary, error) {
	if payload == nil || payload.Type() == "" {
		return nil, fmt.Errorf("failed to create certificate authority: no certificate authority configuration was provided")
	}
	var summary CertificateAuthoritySummary
	if err := c.Post(ctx, "/certificate_authorities", payload, &summary); err != nil {
		return nil, fmt.Errorf("failed to create certificate authority: %w", err)
	}
	return &summary, nil
}

// UpdateCertificateAuthority updates a certificate authority in place. The
// payload's type must match the stored type; Fleet rejects a type change.
func (c *Client) UpdateCertificateAuthority(ctx context.Context, id int64, payload *CertificateAuthorityPayload) error {
	if payload == nil || payload.Type() == "" {
		return fmt.Errorf("failed to update certificate authority %d: no certificate authority configuration was provided", id)
	}
	if err := c.Patch(ctx, fmt.Sprintf("/certificate_authorities/%d", id), payload, nil); err != nil {
		return fmt.Errorf("failed to update certificate authority %d: %w", id, err)
	}
	return nil
}

// DeleteCertificateAuthority deletes a certificate authority. Fleet returns a
// conflict when certificate templates still reference it.
func (c *Client) DeleteCertificateAuthority(ctx context.Context, id int64) error {
	if err := c.Delete(ctx, fmt.Sprintf("/certificate_authorities/%d", id), nil, nil); err != nil {
		return fmt.Errorf("failed to delete certificate authority %d: %w", id, err)
	}
	return nil
}
