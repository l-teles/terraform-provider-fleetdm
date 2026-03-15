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

	// Configure TLS
	tlsConfig := &tls.Config{
		InsecureSkipVerify: !config.VerifyTLS, //nolint:gosec
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr APIError
		apiErr.StatusCode = resp.StatusCode
		apiErr.Message = string(respBody)

		// Try to parse as JSON error
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

		return &apiErr
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
