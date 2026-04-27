package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/labels" {
			t.Errorf("expected path /api/v1/fleet/labels, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListLabelsResponse{
			Labels: []Label{
				{ID: 1, Name: "All Hosts", Query: "SELECT 1"},
				{ID: 2, Name: "macOS", Query: "SELECT 1 WHERE platform = 'darwin'"},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	labels, err := client.ListLabels(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(labels) != 2 {
		t.Errorf("expected 2 labels, got: %d", len(labels))
	}

	if labels[0].Name != "All Hosts" {
		t.Errorf("expected first label name 'All Hosts', got: %s", labels[0].Name)
	}
}

func TestClient_GetLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/labels/1" {
			t.Errorf("expected path /api/v1/fleet/labels/1, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GetLabelResponse{
			Label: Label{
				ID:          1,
				Name:        "All Hosts",
				Description: "All hosts in Fleet",
				Query:       "SELECT 1",
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	label, err := client.GetLabel(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if label.ID != 1 {
		t.Errorf("expected label ID 1, got: %d", label.ID)
	}

	if label.Name != "All Hosts" {
		t.Errorf("expected label name 'All Hosts', got: %s", label.Name)
	}
}

func TestClient_CreateLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/labels" {
			t.Errorf("expected path /api/v1/fleet/labels, got: %s", r.URL.Path)
		}

		var req CreateLabelRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Name != "Windows Servers" {
			t.Errorf("expected name 'Windows Servers', got: %s", req.Name)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreateLabelResponse{
			Label: Label{
				ID:          3,
				Name:        req.Name,
				Description: req.Description,
				Query:       req.Query,
				Platform:    req.Platform,
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	label, err := client.CreateLabel(context.Background(), CreateLabelRequest{
		Name:        "Windows Servers",
		Description: "All Windows Server machines",
		Query:       "SELECT 1 WHERE platform = 'windows'",
		Platform:    "windows",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if label.ID != 3 {
		t.Errorf("expected label ID 3, got: %d", label.ID)
	}

	if label.Name != "Windows Servers" {
		t.Errorf("expected label name 'Windows Servers', got: %s", label.Name)
	}
}

func TestClient_UpdateLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/labels/3" {
			t.Errorf("expected path /api/v1/fleet/labels/3, got: %s", r.URL.Path)
		}

		var req UpdateLabelRequest
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(UpdateLabelResponse{
			Label: Label{
				ID:          3,
				Name:        req.Name,
				Description: req.Description,
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	label, err := client.UpdateLabel(context.Background(), 3, UpdateLabelRequest{
		Name:        "Windows Servers Updated",
		Description: "Updated description",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if label.Name != "Windows Servers Updated" {
		t.Errorf("expected label name 'Windows Servers Updated', got: %s", label.Name)
	}
}

func TestClient_DeleteLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/labels/id/3" {
			t.Errorf("expected path /api/v1/fleet/labels/id/3, got: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.DeleteLabel(context.Background(), 3)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}
