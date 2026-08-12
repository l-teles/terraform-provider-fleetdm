package fleetdm

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClient_ValidConfig(t *testing.T) {
	config := ClientConfig{
		ServerAddress: "https://fleet.example.com",
		APIKey:        "test-api-key",
		VerifyTLS:     true,
		Timeout:       30,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if client.BaseURL != "https://fleet.example.com/api/v1/fleet" {
		t.Errorf("expected BaseURL 'https://fleet.example.com/api/v1/fleet', got: %s", client.BaseURL)
	}

	if client.APIKey != "test-api-key" {
		t.Errorf("expected APIKey 'test-api-key', got: %s", client.APIKey)
	}
}

func TestNewClient_MissingServerAddress(t *testing.T) {
	config := ClientConfig{
		APIKey: "test-api-key",
	}

	_, err := NewClient(config)
	if err == nil {
		t.Fatal("expected error for missing server address")
	}
}

func TestNewClient_MissingAPIKey(t *testing.T) {
	config := ClientConfig{
		ServerAddress: "https://fleet.example.com",
	}

	_, err := NewClient(config)
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestNewClient_AddScheme(t *testing.T) {
	config := ClientConfig{
		ServerAddress: "fleet.example.com",
		APIKey:        "test-api-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if client.BaseURL != "https://fleet.example.com/api/v1/fleet" {
		t.Errorf("expected BaseURL with https scheme, got: %s", client.BaseURL)
	}
}

func TestClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("expected Authorization header, got: %s", r.Header.Get("Authorization"))
		}

		if r.URL.Path != "/api/v1/fleet/test" {
			t.Errorf("expected path /api/v1/fleet/test, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	config := ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	var result map[string]string
	err = client.Get(context.Background(), "/test", nil, &result)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("expected status 'ok', got: %s", result["status"])
	}
}

func TestClient_GetWithParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "1" {
			t.Errorf("expected page=1, got: %s", r.URL.Query().Get("page"))
		}
		if r.URL.Query().Get("per_page") != "10" {
			t.Errorf("expected per_page=10, got: %s", r.URL.Query().Get("per_page"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	config := ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	params := map[string]string{
		"page":     "1",
		"per_page": "10",
	}

	var result map[string]string
	err = client.Get(context.Background(), "/test", params, &result)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_Post(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got: %s", r.Method)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got: %s", r.Header.Get("Content-Type"))
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "test-team" {
			t.Errorf("expected name 'test-team', got: %s", body["name"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{"team": map[string]interface{}{"id": 1, "name": "test-team"}})
	}))
	defer server.Close()

	config := ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	requestBody := map[string]string{"name": "test-team"}
	var result map[string]interface{}
	err = client.Post(context.Background(), "/teams", requestBody, &result)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_Patch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH request, got: %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
	}))
	defer server.Close()

	config := ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	var result map[string]string
	err = client.Patch(context.Background(), "/teams/1", map[string]string{"name": "updated"}, &result)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_Put(t *testing.T) {
	var gotBody map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT request, got: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/spec/teams" {
			t.Errorf("expected path /api/v1/fleet/spec/teams, got: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "replaced"})
	}))
	defer server.Close()

	config := ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	var result map[string]string
	err = client.Put(context.Background(), "/spec/teams", map[string]string{"name": "replaced"}, &result)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if gotBody["name"] != "replaced" {
		t.Errorf("expected the request body to be forwarded, got: %v", gotBody)
	}
	if result["status"] != "replaced" {
		t.Errorf("expected the response to be decoded, got: %v", result)
	}
}

func TestClient_Delete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE request, got: %s", r.Method)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.Delete(context.Background(), "/teams/1", nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Team not found",
			"errors": []map[string]string{
				{"name": "id", "reason": "Team with ID 999 not found"},
			},
		})
	}))
	defer server.Close()

	config := ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	var result map[string]string
	err = client.Get(context.Background(), "/teams/999", nil, &result)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got: %T", err)
	}

	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("expected status code 404, got: %d", apiErr.StatusCode)
	}

	if apiErr.Message != "Team not found" {
		t.Errorf("expected message 'Team not found', got: %s", apiErr.Message)
	}
}

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *APIError
		expected string
	}{
		{
			name: "with message only",
			err: &APIError{
				StatusCode: 404,
				Message:    "Not found",
			},
			expected: "FleetDM API error (status 404): Not found",
		},
		{
			name: "with error details",
			err: &APIError{
				StatusCode: 400,
				Message:    "Validation error",
				Errors: []ErrorDetail{
					{Name: "name", Reason: "is required"},
				},
			},
			expected: "FleetDM API error (status 400): Validation error - name: is required",
		},
		{
			name: "with multiple error details",
			err: &APIError{
				StatusCode: 400,
				Message:    "Validation error",
				Errors: []ErrorDetail{
					{Name: "name", Reason: "is required"},
					{Name: "email", Reason: "is invalid"},
				},
			},
			expected: "FleetDM API error (status 400): Validation error - name: is required; email: is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("expected: %s, got: %s", tt.expected, result)
			}
		})
	}
}

// TestNewClient_PinsTLSMinVersion asserts the constructed transport refuses to
// negotiate below TLS 1.2 on both the verify_tls=true and verify_tls=false
// paths. The same tls.Config serves both, so a regression on either would be a
// silent downgrade rather than a visible failure.
func TestNewClient_PinsTLSMinVersion(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		verifyTLS              bool
		wantInsecureSkipVerify bool
	}{
		{name: "verify TLS enabled", verifyTLS: true, wantInsecureSkipVerify: false},
		{name: "verify TLS disabled", verifyTLS: false, wantInsecureSkipVerify: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewClient(ClientConfig{
				ServerAddress: "https://fleet.example.com",
				APIKey:        "test-api-key",
				VerifyTLS:     tc.verifyTLS,
			})
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}

			transport, ok := client.HTTPClient.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("expected *http.Transport, got %T", client.HTTPClient.Transport)
			}
			if transport.TLSClientConfig == nil {
				t.Fatal("expected an explicit TLSClientConfig; a nil one leaves MinVersion up to the Go default")
			}
			if got := transport.TLSClientConfig.MinVersion; got != tls.VersionTLS12 {
				t.Errorf("expected MinVersion TLS 1.2 (%#x), got %#x", uint16(tls.VersionTLS12), got)
			}
			if got := transport.TLSClientConfig.InsecureSkipVerify; got != tc.wantInsecureSkipVerify {
				t.Errorf("expected InsecureSkipVerify=%v, got %v", tc.wantInsecureSkipVerify, got)
			}
		})
	}
}

// TestReadResponseBodyLimit covers the cap's boundary at a small injected
// limit: at the limit is a success, one byte past it is an error naming the
// cap. Doing it here rather than over HTTP keeps the exact-boundary assertions
// from having to move 100MiB per case.
func TestReadResponseBodyLimit(t *testing.T) {
	const limit = 16

	for _, tc := range []struct {
		name    string
		size    int
		wantErr bool
	}{
		{name: "empty", size: 0},
		{name: "under the limit", size: limit - 1},
		{name: "exactly at the limit", size: limit},
		{name: "one byte over the limit", size: limit + 1, wantErr: true},
		{name: "far over the limit", size: limit * 10, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Repeat("a", tc.size)

			got, err := readResponseBodyLimit(strings.NewReader(body), limit)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for a %d byte body, got nil", tc.size)
				}
				// The error must name the cap so an operator can tell a
				// too-large response from a transport failure.
				if !strings.Contains(err.Error(), fmt.Sprintf("%d byte limit", limit)) {
					t.Errorf("expected the error to name the %d byte limit, got: %v", limit, err)
				}
				if got != nil {
					t.Errorf("expected no body on error, got %d bytes", len(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error for a %d byte body, got: %v", tc.size, err)
			}
			if string(got) != body {
				t.Errorf("expected the body to round-trip intact, got %d of %d bytes", len(got), tc.size)
			}
		})
	}
}

// TestClient_RejectsOversizeResponse checks the real maxResponseBytes cap is
// actually wired into doRequest: a server that streams more than the cap gets
// an error rather than having its body buffered, while a body just under it
// still succeeds.
//
// The bodies are streamed and the success case is read with a nil result, so no
// case holds more than one copy of the payload in memory.
// streamOversizeBody writes size bytes in fixed chunks so neither side
// allocates the whole payload. Shared by the oversize tests for doRequest and
// for the raw-content helpers that bypass it.
func streamOversizeBody(w http.ResponseWriter, size int) {
	chunk := make([]byte, 1<<20)
	for i := range chunk {
		chunk[i] = 'a'
	}
	for remaining := size; remaining > 0; {
		n := min(remaining, len(chunk))
		if _, err := w.Write(chunk[:n]); err != nil {
			return
		}
		remaining -= n
	}
}

func TestClient_RejectsOversizeResponse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		size    int
		wantErr bool
	}{
		{name: "just under the cap", size: maxResponseBytes - 1},
		{name: "just over the cap", size: maxResponseBytes + 1, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				streamOversizeBody(w, tc.size)
			}))
			defer server.Close()

			client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
			if err != nil {
				t.Fatalf("expected no error creating client, got: %v", err)
			}

			// nil result skips json.Unmarshal, which this payload is not
			// valid JSON for anyway — the cap is enforced before decoding.
			err = client.Get(context.Background(), "/anything", nil, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for a %d byte body, got nil", tc.size)
				}
				if !strings.Contains(err.Error(), "byte limit") {
					t.Errorf("expected a body-limit error, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error for a %d byte body, got: %v", tc.size, err)
			}
		})
	}
}
