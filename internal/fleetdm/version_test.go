package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_GetVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/version" {
			t.Errorf("expected path '/api/v1/fleet/version', got: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got: %s", r.Method)
		}

		resp := VersionInfo{
			Version:   "4.50.0",
			Branch:    "main",
			Revision:  "abc123",
			GoVersion: "go1.21.0",
			BuildDate: "2024-01-15T10:00:00Z",
			BuildUser: "builder",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	version, err := client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("failed to get version: %v", err)
	}

	if version.Version != "4.50.0" {
		t.Errorf("expected version '4.50.0', got: %s", version.Version)
	}
	if version.Branch != "main" {
		t.Errorf("expected branch 'main', got: %s", version.Branch)
	}
	if version.Revision != "abc123" {
		t.Errorf("expected revision 'abc123', got: %s", version.Revision)
	}
	if version.GoVersion != "go1.21.0" {
		t.Errorf("expected go_version 'go1.21.0', got: %s", version.GoVersion)
	}
	if version.BuildDate != "2024-01-15T10:00:00Z" {
		t.Errorf("expected build_date '2024-01-15T10:00:00Z', got: %s", version.BuildDate)
	}
	if version.BuildUser != "builder" {
		t.Errorf("expected build_user 'builder', got: %s", version.BuildUser)
	}
}
