package fleetdm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Software title icons (Fleet 4.90+).
//
// Fleet 4.90 added a per-(title, fleet) custom icon that replaces the
// auto-derived one in Fleet Desktop / the self-service catalog. The API is
// three routes hanging off the title:
//
//	PUT    /software/titles/{id}/icon?fleet_id={n}   multipart, "icon" file part
//	GET    /software/titles/{id}/icon?fleet_id={n}   raw PNG bytes
//	DELETE /software/titles/{id}/icon?fleet_id={n}   clears the icon
//
// Probed against Fleet v4.90.0; the notes below record behaviour that is not
// in the published docs and that the resource layer depends on:
//
//   - fleet_id MUST be in the query string. Sending it only as a multipart
//     form field is rejected with "team_id is required" — Fleet parses the
//     scope before it parses the body.
//   - The file part field name is "icon". Any other name yields "either icon
//     multipart field or hashSHA256 and filename are required". (The
//     hashSHA256+filename alternative in that message is not reachable over
//     multipart — it belongs to Fleet's internal GitOps path — so this client
//     always sends the bytes.)
//   - The part's filename is not validated; Fleet sniffs the content. A PNG
//     named "icon.txt" is accepted, a JPEG named "icon.png" is not.
//   - Constraints, all enforced server-side with clear messages: PNG only,
//     at least 120x120 px, at most 1024x1024 px, under 100KB. Larger bodies
//     hit the server's max-request-body limit (413) first.
//   - PUT over an existing icon replaces it; there is no need to DELETE first.
//   - The title must be installable. Pointing at the wrong fleet_id, or at an
//     inventory-only title, returns "Software title has no software
//     installer, VPP app, or in-house app: {id}".
//   - GET returns the uploaded bytes verbatim (verified byte-identical), so
//     hashing the response is a sound drift check. Fleet does NOT echo an
//     icon hash anywhere in the title JSON — icon_url is the only marker.
//   - DELETE is NOT idempotent: with no icon stored it returns HTTP 500
//     "sql: no rows in result set" rather than a 404. See
//     IsTitleIconAbsent.

// UploadTitleIconRequest contains the parameters for PUT
// /software/titles/{id}/icon.
type UploadTitleIconRequest struct {
	TitleID  int    // Software title the icon belongs to
	FleetID  int    // Fleet (team) scope; 0 is valid and means "No team"
	Icon     []byte // PNG bytes; 120x120..1024x1024, under 100KB
	Filename string // Multipart part filename; cosmetic, Fleet sniffs content
}

// uploadTitleIconResponse is Fleet's reply to a successful icon upload.
type uploadTitleIconResponse struct {
	IconURL string `json:"icon_url"`
}

// titleIconEndpoint builds the icon route for a (title, fleet) pair. fleet_id
// is always emitted, including the 0 ("No team") case, because Fleet treats a
// missing fleet_id as an error rather than as "No team".
func titleIconEndpoint(titleID, fleetID int) string {
	return fmt.Sprintf("/software/titles/%d/icon?fleet_id=%d", titleID, fleetID)
}

// UploadTitleIcon uploads (or replaces) the custom icon for a software title
// in a given fleet and returns the icon_url Fleet reports for it.
func (c *Client) UploadTitleIcon(ctx context.Context, req *UploadTitleIconRequest) (string, error) {
	if req == nil {
		return "", fmt.Errorf("upload title icon request is nil")
	}
	if len(req.Icon) == 0 {
		return "", fmt.Errorf("icon content is empty")
	}

	filename := req.Filename
	if filename == "" {
		filename = "icon.png"
	}

	// fleet_id rides in the query string only — see the package notes.
	respBody, err := c.doMultipartRequest(
		ctx,
		http.MethodPut,
		titleIconEndpoint(req.TitleID, req.FleetID),
		"icon",
		filename,
		req.Icon,
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("failed to upload icon for software title %d: %w", req.TitleID, err)
	}

	var uploadResp uploadTitleIconResponse
	if err := json.Unmarshal(respBody, &uploadResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal icon upload response: %w, body: %s", err, string(respBody))
	}

	return uploadResp.IconURL, nil
}

// MaxTitleIconSize bounds how many bytes GetTitleIcon will buffer from a
// response.
//
// Fleet rejects icon uploads over 100KB, so a legitimate icon is always far
// under this. The bound protects the provider from an unbounded read of
// server-controlled data on every refresh — a compromised or misbehaving
// Fleet, or something else answering on that URL, should not be able to grow
// the provider's memory without limit. 1MiB leaves generous headroom over
// Fleet's own cap.
const MaxTitleIconSize = 1 << 20

// GetTitleIcon fetches the raw icon bytes for a software title. Returns an
// *APIError with StatusCode 404 when the title has no icon, so callers can
// use isNotFound-style checks. The bytes are byte-identical to what was
// uploaded, which makes SHA256 over the response a real drift signal.
//
// Responses larger than MaxTitleIconSize are rejected rather than buffered.
func (c *Client) GetTitleIcon(ctx context.Context, titleID, fleetID int) ([]byte, error) {
	reqURL := c.BaseURL + titleIconEndpoint(titleID, fleetID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read one byte past the cap so an over-size response is detectable
	// rather than silently truncated into a wrong hash.
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxTitleIconSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, newAPIError(resp.StatusCode, body)
	}

	if len(body) > MaxTitleIconSize {
		return nil, fmt.Errorf(
			"icon for software title %d in fleet %d exceeds %d bytes; Fleet rejects icons over 100KB, "+
				"so this response is not a valid icon",
			titleID, fleetID, MaxTitleIconSize)
	}

	return body, nil
}

// DeleteTitleIcon removes the custom icon for a software title.
//
// Fleet returns 200 with an empty object on success. When the title has no
// icon it returns 500 "sql: no rows in result set" instead of a 404, so
// callers that want delete to be idempotent should funnel the error through
// IsTitleIconAbsent.
func (c *Client) DeleteTitleIcon(ctx context.Context, titleID, fleetID int) error {
	if err := c.Delete(ctx, titleIconEndpoint(titleID, fleetID), nil, nil); err != nil {
		return fmt.Errorf("failed to delete icon for software title %d: %w", titleID, err)
	}
	return nil
}

// IsTitleIconAbsent reports whether err means "this title has no icon".
//
// Two shapes count. A 404 is the honest answer Fleet gives on GET. The 500
// carrying "sql: no rows in result set" is what DELETE gives when no icon row
// exists — a Fleet bug (it should be a 404 or a no-op 200), but one the
// provider must absorb so `terraform destroy` still converges after someone
// clears the icon in the UI. The match is narrowed to that exact sentinel so
// genuine 500s still surface as errors.
func IsTitleIconAbsent(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode == http.StatusNotFound {
		return true
	}
	if apiErr.StatusCode != http.StatusInternalServerError {
		return false
	}
	if strings.Contains(apiErr.Message, "no rows in result set") {
		return true
	}
	for _, detail := range apiErr.Errors {
		if strings.Contains(detail.Reason, "no rows in result set") {
			return true
		}
	}
	return false
}
