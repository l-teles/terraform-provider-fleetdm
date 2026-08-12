package fleetdm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_ListAPIEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/rest_api" {
			t.Errorf("Expected path '/api/v1/fleet/rest_api', got '%s'", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Expected method 'GET', got '%s'", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"api_endpoints":[
			{"method":"GET","path":"/api/v1/fleet/hosts","display_name":"List hosts","deprecated":false},
			{"method":"GET","path":"/api/v1/fleet/hosts/:id","display_name":"Get host","deprecated":false},
			{"method":"GET","path":"/api/v1/fleet/mdm/apple/profiles","display_name":"List custom macOS settings (configuration profiles)","deprecated":true}
		]}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	endpoints, err := client.ListAPIEndpoints(context.Background())
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(endpoints) != 3 {
		t.Fatalf("Expected 3 endpoints, got %d", len(endpoints))
	}

	if endpoints[0].Method != "GET" {
		t.Errorf("Expected method 'GET', got %q", endpoints[0].Method)
	}
	if endpoints[0].Path != "/api/v1/fleet/hosts" {
		t.Errorf("Expected path '/api/v1/fleet/hosts', got %q", endpoints[0].Path)
	}
	if endpoints[0].DisplayName != "List hosts" {
		t.Errorf("Expected display name 'List hosts', got %q", endpoints[0].DisplayName)
	}
	if endpoints[0].Deprecated {
		t.Error("Expected the first endpoint not to be deprecated")
	}

	// Path templates keep Fleet's `:name` placeholder convention verbatim —
	// the value has to round-trip unchanged to be usable in an api_endpoints
	// scope, which Fleet matches by exact route template.
	if endpoints[1].Path != "/api/v1/fleet/hosts/:id" {
		t.Errorf("Expected the :id route template, got %q", endpoints[1].Path)
	}

	if !endpoints[2].Deprecated {
		t.Error("Expected the third endpoint to be flagged deprecated")
	}
}

// TestClient_ListAPIEndpointsMissingLicense covers Fleet Free, where the route
// is registered but the feature is Premium-only, so the server answers with a
// missing-license error rather than an empty catalog.
func TestClient_ListAPIEndpointsMissingLicense(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(`{"message":"Requires Fleet Premium license"}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	_, err := client.ListAPIEndpoints(context.Background())
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to list API endpoints") {
		t.Errorf("Expected the error to be wrapped by ListAPIEndpoints, got: %v", err)
	}
}
