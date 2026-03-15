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
		if r.URL.Path != "/api/v1/fleet/abm_tokens" {
			t.Errorf("Expected path /api/v1/fleet/abm_tokens, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Expected GET method, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"abm_tokens": []map[string]interface{}{
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

