// Package fleetdm provides a Go client for the FleetDM API.

package fleetdm

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client represents a FleetDM API client.
type Client struct {
	// BaseURL is the base URL for the FleetDM API.
	BaseURL string

	// APIKey is the API key used for authentication.
	APIKey string

	// HTTPClient is the HTTP client used for making requests.
	HTTPClient *http.Client

	// UserAgent is the user agent string sent with each request.
	UserAgent string

	// setupExperienceMu holds a *sync.Mutex per (teamID, platform) so the
	// read-modify-write pattern on PUT /setup_experience/software (a
	// replace-the-whole-list endpoint) is serialized across concurrent
	// Terraform resources within a single apply. Cross-process races
	// against the Fleet UI remain a user-facing concern — documented on
	// the install_during_setup attribute.
	setupExperienceMu sync.Map
}

// ClientConfig holds configuration options for creating a new Client.
type ClientConfig struct {
	// ServerAddress is the address of the FleetDM server (e.g., "https://fleet.example.com").
	ServerAddress string

	// APIKey is the API key for authentication.
	APIKey string

	// VerifyTLS determines whether to verify TLS certificates. Defaults to true.
	VerifyTLS bool

	// Timeout is the timeout for HTTP requests in seconds. Defaults to 30.
	Timeout int
}

// NewClient creates a new FleetDM API client.
func NewClient(config ClientConfig) (*Client, error) {
	if config.ServerAddress == "" {
		return nil, fmt.Errorf("server address is required")
	}

	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	// Ensure the server address has a scheme
	serverURL := config.ServerAddress
	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		serverURL = "https://" + serverURL
	}

	// Parse and validate the URL
	parsedURL, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("invalid server address: %w", err)
	}

	baseURL := fmt.Sprintf("%s://%s/api/v1/fleet", parsedURL.Scheme, parsedURL.Host)

	// Set default timeout
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 30
	}

	// Configure TLS. MinVersion is pinned rather than left to the Go default
	// so the negotiated version never depends on the toolchain's current
	// default, and so a server offering only TLS 1.0/1.1 is refused instead of
	// silently downgraded. This is the only tls.Config the client builds — the
	// same transport serves both the verify_tls=true and verify_tls=false
	// paths, so the floor applies either way.
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: !config.VerifyTLS, // #nosec G402 -- user-controlled provider config //nolint:gosec
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	httpClient := &http.Client{
		Timeout:   time.Duration(timeout) * time.Second,
		Transport: transport,
	}

	return &Client{
		BaseURL:    baseURL,
		APIKey:     config.APIKey,
		HTTPClient: httpClient,
		UserAgent:  "terraform-provider-fleetdm",
	}, nil
}

// APIError represents an error response from the FleetDM API.
type APIError struct {
	StatusCode int
	Message    string
	Errors     []ErrorDetail `json:"errors,omitempty"`
}

// ErrorDetail represents a detailed error from the FleetDM API.
type ErrorDetail struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

func (e *APIError) Error() string {
	if len(e.Errors) > 0 {
		var errMsgs []string
		for _, err := range e.Errors {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %s", err.Name, err.Reason))
		}
		return fmt.Sprintf("FleetDM API error (status %d): %s - %s", e.StatusCode, e.Message, strings.Join(errMsgs, "; "))
	}
	return fmt.Sprintf("FleetDM API error (status %d): %s", e.StatusCode, e.Message)
}

// newAPIError builds an *APIError from a >=400 response, preferring Fleet's
// JSON {message, errors} envelope and falling back to the raw body when the
// response isn't JSON.
func newAPIError(statusCode int, respBody []byte) *APIError {
	apiErr := &APIError{
		StatusCode: statusCode,
		Message:    string(respBody),
	}

	var errResp struct {
		Message string        `json:"message"`
		Errors  []ErrorDetail `json:"errors"`
	}
	if json.Unmarshal(respBody, &errResp) == nil {
		if errResp.Message != "" {
			apiErr.Message = errResp.Message
		}
		apiErr.Errors = errResp.Errors
	}

	return apiErr
}

// maxResponseBytes caps how much of a JSON API response body the client will
// buffer. Without a cap, io.ReadAll grows without limit, so a compromised or
// misbehaving Fleet — or anything else answering on the configured server
// address — can drive the provider to exhaust memory during a plan or apply.
//
// The cap only has to clear the largest legitimate JSON envelope Fleet
// produces. The biggest ones in practice are full host, software and activity
// listings, which run to a few MB even on large deployments; script and
// configuration profile *content* is fetched through dedicated helpers
// (GetScriptContent, GetConfigProfileContent) that do not go through
// doRequest, so their sizes are not a factor here. 100MiB leaves several
// orders of magnitude of headroom, which is why exceeding it is treated as a
// broken server rather than a limit worth tuning.
const maxResponseBytes = 100 << 20

// readResponseBody buffers a response body up to maxResponseBytes, returning
// an error rather than a truncated body when the server sends more.
func readResponseBody(body io.Reader) ([]byte, error) {
	return readResponseBodyLimit(body, maxResponseBytes)
}

// readResponseBodyLimit is readResponseBody with the cap injected, so the
// boundary can be tested without moving 100MiB through a test server. It reads
// one byte past the limit so "exactly at the limit" stays a success and only a
// genuine overrun fails.
func readResponseBodyLimit(body io.Reader, limit int) ([]byte, error) {
	buf, err := io.ReadAll(io.LimitReader(body, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if len(buf) > limit {
		return nil, fmt.Errorf("response body exceeds the %d byte limit; refusing to buffer it", limit)
	}
	return buf, nil
}

// doRequest performs an HTTP request to the FleetDM API.
func (c *Client) doRequest(ctx context.Context, method, endpoint string, body interface{}, result interface{}) error {
	reqURL := c.BaseURL + endpoint

	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		return newAPIError(resp.StatusCode, respBody)
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// Get performs a GET request to the specified endpoint.
func (c *Client) Get(ctx context.Context, endpoint string, params map[string]string, result interface{}) error {
	if len(params) > 0 {
		queryParams := url.Values{}
		for k, v := range params {
			if v != "" {
				queryParams.Add(k, v)
			}
		}
		if encoded := queryParams.Encode(); encoded != "" {
			endpoint = endpoint + "?" + encoded
		}
	}
	return c.doRequest(ctx, http.MethodGet, endpoint, nil, result)
}

// Post performs a POST request to the specified endpoint.
func (c *Client) Post(ctx context.Context, endpoint string, body interface{}, result interface{}) error {
	return c.doRequest(ctx, http.MethodPost, endpoint, body, result)
}

// Patch performs a PATCH request to the specified endpoint.
func (c *Client) Patch(ctx context.Context, endpoint string, body interface{}, result interface{}) error {
	return c.doRequest(ctx, http.MethodPatch, endpoint, body, result)
}

// Put performs a PUT request to the specified endpoint.
func (c *Client) Put(ctx context.Context, endpoint string, body interface{}, result interface{}) error {
	return c.doRequest(ctx, http.MethodPut, endpoint, body, result)
}

// Delete performs a DELETE request to the specified endpoint.
func (c *Client) Delete(ctx context.Context, endpoint string, body interface{}, result interface{}) error {
	return c.doRequest(ctx, http.MethodDelete, endpoint, body, result)
}

// PaginationMeta represents pagination metadata in API responses.
type PaginationMeta struct {
	HasPreviousResults bool `json:"has_previous_results"`
	HasNextResults     bool `json:"has_next_results"`
	TotalResults       int  `json:"total_results,omitempty"`
}
