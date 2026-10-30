package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestClient_SoftwareTypeFilterQuery checks that both software list calls
// send Fleet 4.94's `source` / `extension_for` as one comma-separated
// parameter each, and leave them out when unset.
func TestClient_SoftwareTypeFilterQuery(t *testing.T) {
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/fleet/software/titles":
			_ = json.NewEncoder(w).Encode(listSoftwareTitlesResponse{})
		case "/api/v1/fleet/software/versions":
			_ = json.NewEncoder(w).Encode(listSoftwareVersionsResponse{})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	filter := SoftwareTypeFilter{
		Sources:      []string{"chrome_extensions", "firefox_addons"},
		ExtensionFor: []string{"brave", "edge"},
	}

	calls := map[string]func(SoftwareTypeFilter) error{
		"titles": func(f SoftwareTypeFilter) error {
			_, _, err := client.ListSoftwareTitles(ctx, SoftwareTitleListOptions{SoftwareTypeFilter: f})
			return err
		},
		"versions": func(f SoftwareTypeFilter) error {
			_, _, err := client.ListSoftwareVersions(ctx, SoftwareVersionListOptions{SoftwareTypeFilter: f})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(filter); err != nil {
				t.Fatal(err)
			}
			if v := got.Get("source"); v != "chrome_extensions,firefox_addons" {
				t.Errorf("source = %q", v)
			}
			if v := got.Get("extension_for"); v != "brave,edge" {
				t.Errorf("extension_for = %q", v)
			}
			if len(got["source"]) != 1 || len(got["extension_for"]) != 1 {
				t.Errorf("expected one parameter each, got %v", got)
			}

			if err := call(SoftwareTypeFilter{}); err != nil {
				t.Fatal(err)
			}
			if got.Has("source") || got.Has("extension_for") {
				t.Errorf("expected no filter parameters when unset, got %v", got)
			}
		})
	}
}
