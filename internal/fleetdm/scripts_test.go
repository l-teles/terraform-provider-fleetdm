package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListScripts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/scripts" {
			t.Errorf("expected path /api/v1/fleet/scripts, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListScriptsResponse{
			Scripts: []*Script{
				{ID: 1, Name: "install-app.sh"},
				{ID: 2, Name: "configure-settings.ps1"},
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

	scripts, err := client.ListScripts(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(scripts) != 2 {
		t.Errorf("expected 2 scripts, got: %d", len(scripts))
	}

	if scripts[0].Name != "install-app.sh" {
		t.Errorf("expected first script name 'install-app.sh', got: %s", scripts[0].Name)
	}
}

func TestClient_ListScriptsWithTeam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		teamID := r.URL.Query().Get("team_id")
		if teamID != "5" {
			t.Errorf("expected team_id=5, got: %s", teamID)
		}

		w.Header().Set("Content-Type", "application/json")
		teamIDVal := 5
		json.NewEncoder(w).Encode(ListScriptsResponse{
			Scripts: []*Script{
				{ID: 3, Name: "team-script.sh", TeamID: &teamIDVal},
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

	teamID := 5
	scripts, err := client.ListScripts(context.Background(), &teamID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(scripts) != 1 {
		t.Errorf("expected 1 script, got: %d", len(scripts))
	}

	if scripts[0].TeamID == nil || *scripts[0].TeamID != 5 {
		t.Error("expected script to have team ID 5")
	}
}

func TestClient_GetScript(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/scripts/1" {
			t.Errorf("expected path /api/v1/fleet/scripts/1, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Script{
			ID:        1,
			Name:      "install-app.sh",
			CreatedAt: "2026-01-31T00:00:00Z",
			UpdatedAt: "2026-01-31T00:00:00Z",
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

	script, err := client.GetScript(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if script.ID != 1 {
		t.Errorf("expected script ID 1, got: %d", script.ID)
	}

	if script.Name != "install-app.sh" {
		t.Errorf("expected script name 'install-app.sh', got: %s", script.Name)
	}
}

func TestClient_CreateScript(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First request: POST to create
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/fleet/scripts" {
			// Verify content type is multipart
			contentType := r.Header.Get("Content-Type")
			if contentType == "" {
				t.Error("expected Content-Type header")
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(CreateScriptResponse{ScriptID: 10})
			return
		}

		// Second request: GET to fetch created script
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/fleet/scripts/10" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Script{
				ID:        10,
				Name:      "new-script.sh",
				CreatedAt: "2026-01-31T00:00:00Z",
				UpdatedAt: "2026-01-31T00:00:00Z",
			})
			return
		}

		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	script, err := client.CreateScript(context.Background(), nil, "new-script.sh", []byte("#!/bin/bash\necho 'Hello'"))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if script.ID != 10 {
		t.Errorf("expected script ID 10, got: %d", script.ID)
	}

	if script.Name != "new-script.sh" {
		t.Errorf("expected script name 'new-script.sh', got: %s", script.Name)
	}
}

func TestClient_UpdateScript(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First request: PATCH to update
		if r.Method == http.MethodPatch && r.URL.Path == "/api/v1/fleet/scripts/5" {
			// Verify content type is multipart
			contentType := r.Header.Get("Content-Type")
			if contentType == "" {
				t.Error("expected Content-Type header")
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(UpdateScriptResponse{ScriptID: 5})
			return
		}

		// Second request: GET to fetch updated script
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/fleet/scripts/5" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Script{
				ID:        5,
				Name:      "updated-script.sh",
				CreatedAt: "2026-01-30T00:00:00Z",
				UpdatedAt: "2026-01-31T12:00:00Z",
			})
			return
		}

		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	script, err := client.UpdateScript(context.Background(), 5, "updated-script.sh", []byte("#!/bin/bash\necho 'Updated'"))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if script.ID != 5 {
		t.Errorf("expected script ID 5, got: %d", script.ID)
	}

	if script.Name != "updated-script.sh" {
		t.Errorf("expected script name 'updated-script.sh', got: %s", script.Name)
	}
}

func TestClient_DeleteScript(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/scripts/3" {
			t.Errorf("expected path /api/v1/fleet/scripts/3, got: %s", r.URL.Path)
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

	err = client.DeleteScript(context.Background(), 3)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_GetScriptContent(t *testing.T) {
	const wantContent = "#!/bin/bash\necho hello"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/scripts/7" {
			t.Errorf("expected path /api/v1/fleet/scripts/7, got: %s", r.URL.Path)
		}
		if r.URL.Query().Get("alt") != "media" {
			t.Errorf("expected alt=media query param, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(wantContent))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content, err := client.GetScriptContent(context.Background(), 7)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if content != wantContent {
		t.Errorf("expected content %q, got: %q", wantContent, content)
	}
}

func TestClient_GetScriptContent_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("script not found"))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})

	_, err := client.GetScriptContent(context.Background(), 99)
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

