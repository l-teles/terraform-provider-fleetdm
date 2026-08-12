package fleetdm

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
)

// certificateTemplatesPath is the base route for certificate templates.
const certificateTemplatesPath = "/certificates"

// Certificate templates ("certificates" in Fleet's UI and API route) bind a
// certificate authority to a subject name so Fleet can issue a client
// certificate to each enrolled Android host in a fleet.
//
// Fleet 4.90 exposes only four routes for them, and there is no PATCH or PUT —
// confirmed against server/service/handler.go at tag fleet-v4.90.0 and by
// probing a live 4.90 server, which answers 405 to both:
//
//	POST   /certificates        create
//	GET    /certificates        list, scoped to one fleet, paginated
//	GET    /certificates/{id}   read one
//	DELETE /certificates/{id}   delete
//
// Every field is therefore immutable: a change of any kind means delete and
// recreate. The GitOps batch routes (POST and DELETE /spec/certificates) are
// deliberately not wrapped — they replace a whole fleet's set of templates,
// which does not map onto per-resource lifecycle.
const (
	// CertificateTemplateNameMaxLength is the longest name Fleet accepts.
	// Fleet measures it with len(), so this is a byte count — but
	// CertificateTemplateNamePattern already restricts names to ASCII, which
	// makes bytes and characters equivalent for any name that validates.
	CertificateTemplateNameMaxLength = 255

	// CertificateTemplateSubjectAlternativeNameMaxBytes is the longest
	// subject alternative name Fleet accepts, measured in bytes.
	CertificateTemplateSubjectAlternativeNameMaxBytes = 4096

	// certificateTemplateListPerPage is the page size used when walking
	// GET /certificates. Fleet applies no LIMIT when per_page is omitted, but
	// relying on that would break the moment Fleet adds a default cap, so we
	// page explicitly.
	certificateTemplateListPerPage = 100

	// maxCertificateTemplateListPages bounds the pagination loop so a server
	// that keeps reporting has_next_results cannot spin forever.
	maxCertificateTemplateListPages = 100
)

// CertificateTemplateNamePattern is the name character set Fleet enforces
// server-side (server/service/certificates.go, certificateTemplateNameRegex).
// Note that dots are not permitted, so a DNS-style name is rejected.
var CertificateTemplateNamePattern = regexp.MustCompile(`^[a-zA-Z0-9 \-_]+$`)

// CertificateTemplateSubjectAlternativeNameKeys are the SAN attribute keys
// Fleet accepts. Fleet upper-cases each key before checking membership, so a
// lower-case key in the configuration is accepted and stored verbatim.
var CertificateTemplateSubjectAlternativeNameKeys = []string{"DNS", "EMAIL", "UPN", "IP", "URI"}

// CertificateTemplate is a certificate template as returned by Fleet.
//
// The three response shapes are supersets of one another, so one struct covers
// all of them and absent fields decode as zero values:
//
//   - POST /certificates returns id, name, certificate_authority_id,
//     subject_name and subject_alternative_name.
//   - GET /certificates adds certificate_authority_name and created_at.
//   - GET /certificates/{id} adds certificate_authority_type.
//
// The fleet (team) the template belongs to is NOT part of any response: Fleet's
// response struct tags it `json:"-"` (server/fleet/certificate_templates.go).
// It is a create-time input only, which is why the Terraform resource has to
// carry fleet_id in state rather than refresh it, and why importing needs the
// fleet ID supplied alongside the template ID.
type CertificateTemplate struct {
	ID                     int64  `json:"id"`
	Name                   string `json:"name"`
	SubjectName            string `json:"subject_name"`
	SubjectAlternativeName string `json:"subject_alternative_name,omitempty"`
	CertificateAuthorityID int64  `json:"certificate_authority_id"`

	// CertificateAuthorityName is returned by both GET routes, never by POST.
	CertificateAuthorityName string `json:"certificate_authority_name,omitempty"`

	// CertificateAuthorityType is returned by GET /certificates/{id} only.
	CertificateAuthorityType string `json:"certificate_authority_type,omitempty"`

	// CreatedAt is returned by both GET routes, never by POST.
	CreatedAt string `json:"created_at,omitempty"`
}

// CreateCertificateTemplateRequest is the body for POST /certificates.
//
// FleetID has no omitempty: Fleet reads an absent fleet as 0 ("no team"), so
// omitting the key when it is 0 would work but would make the request depend on
// a server-side default. Sending it explicitly states the intent.
//
// Fleet is renaming the team field: the server struct declares `team_id` with a
// `renameto:"fleet_id"` tag. Both keys are accepted on 4.90 (verified live —
// a template created with `fleet_id` lands in that fleet and is absent from the
// no-fleet listing), so the request uses the new name to match the rest of this
// provider.
type CreateCertificateTemplateRequest struct {
	Name                   string `json:"name"`
	FleetID                int64  `json:"fleet_id"`
	CertificateAuthorityID int64  `json:"certificate_authority_id"`
	SubjectName            string `json:"subject_name"`
	SubjectAlternativeName string `json:"subject_alternative_name,omitempty"`
}

// listCertificateTemplatesResponse is the response for GET /certificates.
type listCertificateTemplatesResponse struct {
	Certificates []CertificateTemplate `json:"certificates"`
	Meta         *PaginationMeta       `json:"meta"`
}

// certificateTemplateResponse is the response wrapper for
// GET /certificates/{id}. The create route returns the object unwrapped.
type certificateTemplateResponse struct {
	Certificate *CertificateTemplate `json:"certificate"`
}

// CreateCertificateTemplate creates a certificate template.
//
// Fleet validates the request against the certificate authority before storing
// anything: the CA must exist (404 otherwise) and its type must be
// custom_scep_proxy (400 "Currently, only the custom_scep_proxy certificate
// authority is supported."). It does not re-check that the CA's URL is
// reachable — that happens when the CA itself is saved.
//
// The response echoes only the fields that were sent. Callers that need
// certificate_authority_name, certificate_authority_type or created_at must
// follow up with GetCertificateTemplate.
func (c *Client) CreateCertificateTemplate(ctx context.Context, req CreateCertificateTemplateRequest) (*CertificateTemplate, error) {
	var template CertificateTemplate
	if err := c.Post(ctx, certificateTemplatesPath, req, &template); err != nil {
		return nil, fmt.Errorf("failed to create certificate template: %w", err)
	}
	return &template, nil
}

// GetCertificateTemplate retrieves a single certificate template. Fleet answers
// 404 once the template is gone, which is what lets the provider detect
// out-of-band deletion.
func (c *Client) GetCertificateTemplate(ctx context.Context, id int64) (*CertificateTemplate, error) {
	var response certificateTemplateResponse
	if err := c.Get(ctx, fmt.Sprintf("%s/%d", certificateTemplatesPath, id), nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get certificate template %d: %w", id, err)
	}
	if response.Certificate == nil {
		return nil, fmt.Errorf("failed to get certificate template %d: response contained no certificate", id)
	}
	return response.Certificate, nil
}

// ListCertificateTemplates retrieves the certificate templates on one fleet,
// walking every page.
//
// The listing is always scoped to a single fleet: Fleet has no "all fleets"
// mode for this route, and an absent fleet_id selects fleet 0 ("no team")
// rather than everything. Results come back ordered by id ascending — Fleet
// pins the sort server-side and ignores any ordering parameters.
func (c *Client) ListCertificateTemplates(ctx context.Context, fleetID int64) ([]CertificateTemplate, error) {
	var templates []CertificateTemplate

	for page := range maxCertificateTemplateListPages {
		params := map[string]string{
			"fleet_id": strconv.FormatInt(fleetID, 10),
			"per_page": strconv.Itoa(certificateTemplateListPerPage),
			"page":     strconv.Itoa(page),
		}

		var response listCertificateTemplatesResponse
		if err := c.Get(ctx, certificateTemplatesPath, params, &response); err != nil {
			return nil, fmt.Errorf("failed to list certificate templates for fleet %d (page %d): %w", fleetID, page, err)
		}

		templates = append(templates, response.Certificates...)

		// An empty page ends the walk regardless of the metadata, so a server
		// that keeps reporting has_next_results costs one request rather than
		// the full bound.
		if len(response.Certificates) == 0 || response.Meta == nil || !response.Meta.HasNextResults {
			return templates, nil
		}
	}

	return nil, fmt.Errorf("certificate template pagination exceeded %d pages — Fleet API may be returning has_next_results=true indefinitely", maxCertificateTemplateListPages)
}

// DeleteCertificateTemplate deletes a certificate template.
//
// Deleting a template is always allowed: the reference runs the other way, so
// it is the certificate authority that cannot be deleted while templates point
// at it (409 "Couldn't delete certificate authority. Certificate templates
// still reference it. Please remove the certificate templates first.").
//
// Deleting a template that is already gone does NOT answer 404. Fleet looks the
// template up before authorizing, so the lookup failure escapes before any
// authorization check runs and Fleet's authz middleware turns it into a 500
// "forbidden" (verified live on 4.90). That response is deliberately not
// treated as success here: a genuine permission failure looks identical, and
// swallowing it would report a destroy that never happened. The resource's Read
// removes a missing template from state first, so the double-delete path is not
// reached in normal use.
func (c *Client) DeleteCertificateTemplate(ctx context.Context, id int64) error {
	if err := c.Delete(ctx, fmt.Sprintf("%s/%d", certificateTemplatesPath, id), nil, nil); err != nil {
		return fmt.Errorf("failed to delete certificate template %d: %w", id, err)
	}
	return nil
}
