package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListActivities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/activities" {
			t.Errorf("Expected path '/api/v1/fleet/activities', got '%s'", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Expected method 'GET', got '%s'", r.Method)
		}

		actorID := 1
		response := listActivitiesResponse{
			Activities: []Activity{
				{
					ID:             1,
					CreatedAt:      "2024-01-15T10:30:00Z",
					ActorFullName:  "Admin User",
					ActorID:        &actorID,
					ActorEmail:     "admin@example.com",
					Type:           "user_logged_in",
					FleetInitiated: false,
				},
				{
					ID:             2,
					CreatedAt:      "2024-01-15T10:25:00Z",
					ActorFullName:  "Admin User",
					ActorID:        &actorID,
					ActorEmail:     "admin@example.com",
					Type:           "created_team",
					FleetInitiated: false,
					Details: map[string]any{
						"team_id":   2,
						"team_name": "Workstations",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	activities, err := client.ListActivities(context.Background(), nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(activities) != 2 {
		t.Errorf("Expected 2 activities, got: %d", len(activities))
	}

	if activities[0].Type != "user_logged_in" {
		t.Errorf("Expected activity type 'user_logged_in', got: %s", activities[0].Type)
	}

	if activities[0].ActorFullName != "Admin User" {
		t.Errorf("Expected actor name 'Admin User', got: %s", activities[0].ActorFullName)
	}
}

func TestClient_ListActivitiesWithFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("activity_type") != "created_team" {
			t.Errorf("Expected activity_type 'created_team', got '%s'", query.Get("activity_type"))
		}
		if query.Get("per_page") != "10" {
			t.Errorf("Expected per_page '10', got '%s'", query.Get("per_page"))
		}
		if query.Get("order_key") != "created_at" {
			t.Errorf("Expected order_key 'created_at', got '%s'", query.Get("order_key"))
		}
		if query.Get("order_direction") != "desc" {
			t.Errorf("Expected order_direction 'desc', got '%s'", query.Get("order_direction"))
		}

		actorID := 1
		response := listActivitiesResponse{
			Activities: []Activity{
				{
					ID:             2,
					CreatedAt:      "2024-01-15T10:25:00Z",
					ActorFullName:  "Admin User",
					ActorID:        &actorID,
					ActorEmail:     "admin@example.com",
					Type:           "created_team",
					FleetInitiated: false,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	opts := &ListActivitiesOptions{
		ActivityType:   "created_team",
		PerPage:        10,
		OrderKey:       "created_at",
		OrderDirection: "desc",
	}
	activities, err := client.ListActivities(context.Background(), opts)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(activities) != 1 {
		t.Errorf("Expected 1 activity, got: %d", len(activities))
	}

	if activities[0].Type != "created_team" {
		t.Errorf("Expected activity type 'created_team', got: %s", activities[0].Type)
	}
}

func TestClient_ListActivitiesWithDateFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("start_created_at") != "2024-01-01T00:00:00Z" {
			t.Errorf("Expected start_created_at '2024-01-01T00:00:00Z', got '%s'", query.Get("start_created_at"))
		}
		if query.Get("end_created_at") != "2024-01-31T23:59:59Z" {
			t.Errorf("Expected end_created_at '2024-01-31T23:59:59Z', got '%s'", query.Get("end_created_at"))
		}

		response := listActivitiesResponse{
			Activities: []Activity{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	opts := &ListActivitiesOptions{
		StartCreatedAt: "2024-01-01T00:00:00Z",
		EndCreatedAt:   "2024-01-31T23:59:59Z",
	}
	activities, err := client.ListActivities(context.Background(), opts)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(activities) != 0 {
		t.Errorf("Expected 0 activities, got: %d", len(activities))
	}
}

func TestClient_ListActivitiesFleetInitiated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := listActivitiesResponse{
			Activities: []Activity{
				{
					ID:             3,
					CreatedAt:      "2024-01-15T10:30:00Z",
					ActorFullName:  "",
					ActorID:        nil,
					ActorEmail:     "",
					Type:           "installed_software",
					FleetInitiated: true,
					Details: map[string]any{
						"status":           "installed",
						"host_id":          1272,
						"software_package": "ZoomInstallerIT.pkg",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	activities, err := client.ListActivities(context.Background(), nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(activities) != 1 {
		t.Errorf("Expected 1 activity, got: %d", len(activities))
	}

	if !activities[0].FleetInitiated {
		t.Error("Expected FleetInitiated to be true")
	}

	if activities[0].ActorID != nil {
		t.Error("Expected ActorID to be nil for fleet-initiated activity")
	}
}

func TestClient_ListActivitiesWithQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("query") != "admin@example.com" {
			t.Errorf("Expected query 'admin@example.com', got '%s'", query.Get("query"))
		}

		actorID := 1
		response := listActivitiesResponse{
			Activities: []Activity{
				{
					ID:             1,
					CreatedAt:      "2024-01-15T10:30:00Z",
					ActorFullName:  "Admin User",
					ActorID:        &actorID,
					ActorEmail:     "admin@example.com",
					Type:           "user_logged_in",
					FleetInitiated: false,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	opts := &ListActivitiesOptions{
		Query: "admin@example.com",
	}
	activities, err := client.ListActivities(context.Background(), opts)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(activities) != 1 {
		t.Errorf("Expected 1 activity, got: %d", len(activities))
	}

	if activities[0].ActorEmail != "admin@example.com" {
		t.Errorf("Expected actor email 'admin@example.com', got: %s", activities[0].ActorEmail)
	}
}
