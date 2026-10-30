package fleetdm

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// MDM EULA platforms. Fleet keeps one EULA per platform.
const (
	EULAPlatformDarwin  = "darwin"
	EULAPlatformWindows = "windows"
)

// MaxEULASize is Fleet's request body limit for an EULA upload (25 MiB).
const MaxEULASize int64 = 25 << 20

// MaxWindowsEULASize is Fleet's limit for a Windows Markdown EULA (512 KB).
const MaxWindowsEULASize int64 = 512 << 10

// ErrEULAPlatformUnsupported means the server answered for a different
// platform than the one asked for: Fleet before 4.94 ignores the platform
// parameter and always serves the macOS EULA.
var ErrEULAPlatformUnsupported = errors.New("this Fleet server does not support Windows EULAs (Fleet 4.94 or later is required)")

// EULA is the metadata Fleet reports for an uploaded EULA.
type EULA struct {
	Name      string `json:"name"`
	Token     string `json:"token"`
	CreatedAt string `json:"created_at"`
	// Platform is empty on Fleet before 4.94, which only had macOS EULAs.
	Platform string `json:"platform"`
	// Sha256 is the raw digest; Fleet sends it base64-encoded.
	Sha256 []byte `json:"sha256"`
}

// Sha256Hex returns the digest as lowercase hex, or "" when Fleet sent none.
func (e *EULA) Sha256Hex() string {
	if len(e.Sha256) == 0 {
		return ""
	}
	return hex.EncodeToString(e.Sha256)
}

// UploadEULA uploads an EULA for a platform. Fleet answers 409 when one
// already exists for that platform; there is no in-place update.
func (c *Client) UploadEULA(ctx context.Context, platform, fileName string, content []byte) error {
	// The platform goes in the form only: Fleet rejects a form and query
	// string that name different platforms.
	_, err := c.doMultipartRequest(ctx, http.MethodPost, "/setup_experience/eula", "eula", fileName, content,
		map[string]string{"platform": platform})
	if err != nil {
		return fmt.Errorf("failed to upload %s EULA: %w", platform, err)
	}
	return nil
}

// GetEULAMetadata returns the EULA metadata for a platform. A 404 means no
// EULA is uploaded. It returns ErrEULAPlatformUnsupported when the server
// answers for another platform.
func (c *Client) GetEULAMetadata(ctx context.Context, platform string) (*EULA, error) {
	var eula EULA
	if err := c.Get(ctx, "/setup_experience/eula/metadata", map[string]string{"platform": platform}, &eula); err != nil {
		return nil, fmt.Errorf("failed to get %s EULA metadata: %w", platform, err)
	}
	if !eulaPlatformMatches(platform, eula.Platform) {
		return nil, ErrEULAPlatformUnsupported
	}
	return &eula, nil
}

// eulaPlatformMatches reports whether the metadata belongs to the requested
// platform. An empty reported platform is a pre-4.94 server, which only
// serves the macOS EULA.
func eulaPlatformMatches(requested, reported string) bool {
	if reported == "" {
		return requested == EULAPlatformDarwin
	}
	return requested == reported
}

// DeleteEULA deletes the EULA with the given token.
func (c *Client) DeleteEULA(ctx context.Context, token string) error {
	if err := c.Delete(ctx, "/setup_experience/eula/"+url.PathEscape(token), nil, nil); err != nil {
		return fmt.Errorf("failed to delete EULA: %w", err)
	}
	return nil
}
