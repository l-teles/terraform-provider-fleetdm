package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListQueries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/queries" {
			t.Errorf("expected path /api/v1/fleet/queries, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListQueriesResponse{
			Queries: []Query{
				{ID: 1, Name: "Get OS Version", Query: "SELECT * FROM os_version"},
				{ID: 2, Name: "List Users", Query: "SELECT * FROM users"},
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

	queries, err := client.ListQueries(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(queries) != 2 {
		t.Errorf("expected 2 queries, got: %d", len(queries))
	}

	if queries[0].Name != "Get OS Version" {
		t.Errorf("expected first query name 'Get OS Version', got: %s", queries[0].Name)
	}
}

func TestClient_GetQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/queries/1" {
			t.Errorf("expected path /api/v1/fleet/queries/1, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GetQueryResponse{
			Query: Query{
				ID:          1,
				Name:        "Get OS Version",
				Description: "Retrieves OS version information",
				Query:       "SELECT * FROM os_version",
				Platform:    "darwin,linux,windows",
				Interval:    3600,
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

	query, err := client.GetQuery(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if query.ID != 1 {
		t.Errorf("expected query ID 1, got: %d", query.ID)
	}

	if query.Name != "Get OS Version" {
		t.Errorf("expected query name 'Get OS Version', got: %s", query.Name)
	}
}

func TestClient_CreateQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/queries" {
			t.Errorf("expected path /api/v1/fleet/queries, got: %s", r.URL.Path)
		}

		var req CreateQueryRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Name != "New Query" {
			t.Errorf("expected name 'New Query', got: %s", req.Name)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreateQueryResponse{
			Query: Query{
				ID:          3,
				Name:        req.Name,
				Description: req.Description,
				Query:       req.Query,
				Platform:    req.Platform,
				Interval:    req.Interval,
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

	query, err := client.CreateQuery(context.Background(), CreateQueryRequest{
		Name:        "New Query",
		Description: "A new query",
		Query:       "SELECT * FROM system_info",
		Platform:    "darwin",
		Interval:    300,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if query.ID != 3 {
		t.Errorf("expected query ID 3, got: %d", query.ID)
	}

	if query.Name != "New Query" {
		t.Errorf("expected query name 'New Query', got: %s", query.Name)
	}
}

func TestClient_UpdateQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/queries/3" {
			t.Errorf("expected path /api/v1/fleet/queries/3, got: %s", r.URL.Path)
		}

		var req UpdateQueryRequest
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(UpdateQueryResponse{
			Query: Query{
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

	query, err := client.UpdateQuery(context.Background(), 3, UpdateQueryRequest{
		Name:        "Updated Query",
		Description: "Updated description",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if query.Name != "Updated Query" {
		t.Errorf("expected query name 'Updated Query', got: %s", query.Name)
	}
}

func TestClient_DeleteQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/queries/id/3" {
			t.Errorf("expected path /api/v1/fleet/queries/id/3, got: %s", r.URL.Path)
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

	err = client.DeleteQuery(context.Background(), 3)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_ListQueriesByTeam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/queries" {
			t.Errorf("expected path /api/v1/fleet/queries, got: %s", r.URL.Path)
		}
		if r.URL.Query().Get("team_id") != "5" {
			t.Errorf("expected team_id=5, got: %s", r.URL.Query().Get("team_id"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListQueriesResponse{
			Queries: []Query{
				{ID: 10, Name: "Team Query", Query: "SELECT 1"},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	queries, err := client.ListQueriesByTeam(context.Background(), 5)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(queries) != 1 {
		t.Errorf("expected 1 query, got: %d", len(queries))
	}
	if queries[0].Name != "Team Query" {
		t.Errorf("expected name 'Team Query', got: %s", queries[0].Name)
	}
}

