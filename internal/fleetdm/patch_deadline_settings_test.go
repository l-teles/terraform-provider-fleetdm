package fleetdm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestClient_GetPatchDeadlineSettings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/fleet/software/patch_deadline_settings" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.URL.RawQuery != "fleet_id=7" {
			t.Errorf("expected query fleet_id=7, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"patch_deadline_settings":{"fleet_id":7,"days":["monday","friday"],"time":"09:30"}}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test"})
	got, err := client.GetPatchDeadlineSettings(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := &PatchDeadlineSettings{FleetID: 7, Days: []string{"monday", "friday"}, Time: "09:30"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Unassigned is fleet_id 0, which must still be sent: Fleet rejects a request
// without it.
func TestClient_GetPatchDeadlineSettings_Unassigned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "fleet_id=0" {
			t.Errorf("expected query fleet_id=0, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"patch_deadline_settings":{"fleet_id":0,"days":["tuesday"],"time":"17:00"}}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test"})
	got, err := client.GetPatchDeadlineSettings(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := DefaultPatchDeadlineSettings(0); !reflect.DeepEqual(*got, want) {
		t.Errorf("got %+v, want %+v", *got, want)
	}
}

func TestClient_GetPatchDeadlineSettings_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource Not Found","errors":[{"name":"fleet_id","reason":"fleet 99 does not exist"}]}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test"})
	_, err := client.GetPatchDeadlineSettings(context.Background(), 99)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected a 404 APIError, got %v", err)
	}
}

func TestClient_GetPatchDeadlineSettings_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test"})
	if _, err := client.GetPatchDeadlineSettings(context.Background(), 1); err == nil {
		t.Fatal("expected an error for a response without settings")
	}
}

func TestClient_UpdatePatchDeadlineSettings(t *testing.T) {
	for _, fleetID := range []int64{0, 4} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPatch || r.URL.Path != "/api/v1/fleet/software/patch_deadline_settings" {
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}
			if r.URL.RawQuery != "" {
				t.Errorf("fleet_id must travel in the body only, got query %q", r.URL.RawQuery)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			// A map, not a struct, so a missing fleet_id key is caught rather
			// than decoded as 0.
			want := map[string]any{
				"fleet_id": float64(fleetID),
				"days":     []any{"friday", "monday"},
				"time":     "08:15",
			}
			if !reflect.DeepEqual(body, want) {
				t.Errorf("body = %v, want %v", body, want)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"patch_deadline_settings": map[string]any{
				"fleet_id": fleetID, "days": []string{"monday", "friday"}, "time": "08:15",
			}})
		}))

		client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test"})
		got, err := client.UpdatePatchDeadlineSettings(context.Background(), PatchDeadlineSettings{
			FleetID: fleetID, Days: []string{"friday", "monday"}, Time: "08:15",
		})
		server.Close()
		if err != nil {
			t.Fatalf("fleet %d: unexpected error: %v", fleetID, err)
		}
		want := &PatchDeadlineSettings{FleetID: fleetID, Days: []string{"monday", "friday"}, Time: "08:15"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("fleet %d: got %+v, want %+v", fleetID, got, want)
		}
	}
}

func TestClient_UpdatePatchDeadlineSettings_BadRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"Bad request","errors":[{"name":"base","reason":"Couldn't edit patch deadline settings. \"days\" must include at least one day."}]}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test"})
	_, err := client.UpdatePatchDeadlineSettings(context.Background(), PatchDeadlineSettings{FleetID: 1, Time: "17:00"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected a 400 APIError, got %v", err)
	}
}

func TestClient_UpdatePatchDeadlineSettings_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test"})
	if _, err := client.UpdatePatchDeadlineSettings(context.Background(), DefaultPatchDeadlineSettings(1)); err == nil {
		t.Fatal("expected an error for a response without settings")
	}
}
