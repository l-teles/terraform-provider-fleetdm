package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListABMTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/ab_tokens" {
			t.Errorf("Expected path /api/v1/fleet/ab_tokens, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Expected GET method, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ab_tokens": []map[string]interface{}{
				{
					"id":       1,
					"apple_id": "admin@example.com",
					"org_name": "Test Corp",
				},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-token",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	tokens, err := client.ListABMTokens(context.Background())
	if err != nil {
		t.Fatalf("ListABMTokens failed: %v", err)
	}

	if len(tokens) != 1 {
		t.Fatalf("Expected 1 ABM token, got %d", len(tokens))
	}

	if tokens[0].AppleID != "admin@example.com" {
		t.Errorf("Expected apple_id 'admin@example.com', got '%s'", tokens[0].AppleID)
	}

	if tokens[0].OrganizationName != "Test Corp" {
		t.Errorf("Expected org_name 'Test Corp', got '%s'", tokens[0].OrganizationName)
	}
}

func TestListABMTokensLegacyKeyOnNewPath(t *testing.T) {
	// A Fleet version may serve /ab_tokens but still respond with the legacy
	// "abm_tokens" key; the client must decode either.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/ab_tokens" {
			t.Errorf("Expected path /api/v1/fleet/ab_tokens, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"abm_tokens": []map[string]interface{}{
				{"id": 1, "apple_id": "admin@example.com", "org_name": "Test Corp"},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-token",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	tokens, err := client.ListABMTokens(context.Background())
	if err != nil {
		t.Fatalf("ListABMTokens failed: %v", err)
	}
	if len(tokens) != 1 || tokens[0].OrganizationName != "Test Corp" {
		t.Fatalf("Expected 1 token for 'Test Corp', got %+v", tokens)
	}
}

func TestListABMTokensFallbackToLegacyPath(t *testing.T) {
	// Fleet < 4.87 has no /ab_tokens; the client must fall back to /abm_tokens.
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/fleet/ab_tokens":
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{"message": "Resource Not Found"})
		case "/api/v1/fleet/abm_tokens":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"abm_tokens": []map[string]interface{}{
					{"id": 2, "apple_id": "legacy@example.com", "org_name": "Legacy Corp"},
				},
			})
		default:
			t.Errorf("Unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-token",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	tokens, err := client.ListABMTokens(context.Background())
	if err != nil {
		t.Fatalf("ListABMTokens failed: %v", err)
	}
	if len(tokens) != 1 || tokens[0].OrganizationName != "Legacy Corp" {
		t.Fatalf("Expected 1 token for 'Legacy Corp', got %+v", tokens)
	}
	want := []string{"/api/v1/fleet/ab_tokens", "/api/v1/fleet/abm_tokens"}
	if len(requests) != 2 || requests[0] != want[0] || requests[1] != want[1] {
		t.Errorf("Expected request order %v, got %v", want, requests)
	}
}

func TestListABMTokensBothPathsNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Resource Not Found"})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-token",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if _, err := client.ListABMTokens(context.Background()); err == nil {
		t.Fatal("Expected error when both endpoints return 404, got nil")
	}
}

func TestListVPPTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/vpp_tokens" {
			t.Errorf("Expected path /api/v1/fleet/vpp_tokens, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Expected GET method, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"vpp_tokens": []map[string]interface{}{
				{
					"id":       1,
					"org_name": "VPP Corp",
					"location": "US",
				},
				{
					"id":       2,
					"org_name": "VPP Corp EU",
					"location": "EU",
				},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-token",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	tokens, err := client.ListVPPTokens(context.Background())
	if err != nil {
		t.Fatalf("ListVPPTokens failed: %v", err)
	}

	if len(tokens) != 2 {
		t.Fatalf("Expected 2 VPP tokens, got %d", len(tokens))
	}

	if tokens[0].OrganizationName != "VPP Corp" {
		t.Errorf("Expected org_name 'VPP Corp', got '%s'", tokens[0].OrganizationName)
	}

	if tokens[1].Location != "EU" {
		t.Errorf("Expected location 'EU', got '%s'", tokens[1].Location)
	}
}
