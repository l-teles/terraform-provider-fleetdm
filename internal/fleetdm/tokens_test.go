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

// TestListVPPTokens_495Shape decodes Fleet's VPPTokenDB as 4.95 serializes it:
// team tuples keyed `team_id`, plus `country_code` and `created_at`.
func TestListVPPTokens_495Shape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"vpp_tokens":[
			{"id":3,"org_name":"Example Org","location":"Example Unit","renew_date":"2030-01-01T00:00:00Z",
			 "country_code":"us","created_at":"2026-01-01T00:00:00Z",
			 "teams":[{"team_id":0,"name":"No team"},{"team_id":7,"name":"Workstations"}],
			 "fleets":[{"fleet_id":0,"name":"No team"},{"fleet_id":7,"name":"Workstations"}]},
			{"id":4,"org_name":"Example Org","location":"Other Unit","renew_date":"2030-01-01T00:00:00Z","teams":[]},
			{"id":5,"org_name":"Example Org","location":"Unassigned","renew_date":"2030-01-01T00:00:00Z","teams":null}
		]}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-token"})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	tokens, err := client.ListVPPTokens(context.Background())
	if err != nil {
		t.Fatalf("ListVPPTokens failed: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(tokens))
	}
	got := tokens[0]
	if got.CountryCode != "us" || got.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("country_code/created_at not decoded: %+v", got)
	}
	if len(got.Teams) != 2 || got.Teams[1].ID != 7 || got.Teams[1].Name != "Workstations" || got.Teams[0].ID != 0 {
		t.Errorf("teams not decoded from team_id: %+v", got.Teams)
	}
	if tokens[1].Teams == nil || len(tokens[1].Teams) != 0 {
		t.Errorf("all-teams token must decode as an empty, non-nil list: %#v", tokens[1].Teams)
	}
	if tokens[2].Teams != nil {
		t.Errorf("unassigned token must decode as nil teams: %#v", tokens[2].Teams)
	}
}
