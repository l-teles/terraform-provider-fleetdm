package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListTeams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/teams" {
			t.Errorf("Expected path '/api/v1/fleet/teams', got '%s'", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Expected method 'GET', got '%s'", r.Method)
		}

		response := ListTeamsResponse{
			Teams: []Team{
				{ID: 1, Name: "Team 1", Description: "First team", UserCount: 5, HostCount: 10},
				{ID: 2, Name: "Team 2", Description: "Second team", UserCount: 3, HostCount: 5},
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

	teams, err := client.ListTeams(context.Background(), 0, 100)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(teams) != 2 {
		t.Errorf("Expected 2 teams, got: %d", len(teams))
	}

	if teams[0].Name != "Team 1" {
		t.Errorf("Expected team name 'Team 1', got: %s", teams[0].Name)
	}
}

func TestClient_GetTeam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/teams/1" {
			t.Errorf("Expected path '/api/v1/fleet/teams/1', got '%s'", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Expected method 'GET', got '%s'", r.Method)
		}

		response := GetTeamResponse{
			Team: Team{ID: 1, Name: "Team 1", Description: "First team", UserCount: 5, HostCount: 10},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	team, err := client.GetTeam(context.Background(), 1)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if team.ID != 1 {
		t.Errorf("Expected team ID 1, got: %d", team.ID)
	}

	if team.Name != "Team 1" {
		t.Errorf("Expected team name 'Team 1', got: %s", team.Name)
	}
}

func TestClient_CreateTeam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/teams" {
			t.Errorf("Expected path '/api/v1/fleet/teams', got '%s'", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("Expected method 'POST', got '%s'", r.Method)
		}

		var req CreateTeamRequest
		json.NewDecoder(r.Body).Decode(&req)

		response := struct {
			Team Team `json:"team"`
		}{
			Team: Team{ID: 1, Name: req.Name, Description: req.Description},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	team, err := client.CreateTeam(context.Background(), CreateTeamRequest{
		Name:        "New Team",
		Description: "A new team",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if team.Name != "New Team" {
		t.Errorf("Expected team name 'New Team', got: %s", team.Name)
	}
}

func TestClient_UpdateTeam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/teams/1" {
			t.Errorf("Expected path '/api/v1/fleet/teams/1', got '%s'", r.URL.Path)
		}
		if r.Method != "PATCH" {
			t.Errorf("Expected method 'PATCH', got '%s'", r.Method)
		}

		var req UpdateTeamRequest
		json.NewDecoder(r.Body).Decode(&req)

		response := struct {
			Team Team `json:"team"`
		}{
			Team: Team{ID: 1, Name: req.Name, Description: req.Description},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	team, err := client.UpdateTeam(context.Background(), 1, UpdateTeamRequest{
		Name:        "Updated Team",
		Description: "Updated description",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if team.Name != "Updated Team" {
		t.Errorf("Expected team name 'Updated Team', got: %s", team.Name)
	}
}

func TestClient_DeleteTeam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/teams/1" {
			t.Errorf("Expected path '/api/v1/fleet/teams/1', got '%s'", r.URL.Path)
		}
		if r.Method != "DELETE" {
			t.Errorf("Expected method 'DELETE', got '%s'", r.Method)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	err := client.DeleteTeam(context.Background(), 1)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

func TestClient_GetTeamEnrollSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/teams/1/secrets" {
			t.Errorf("Expected path '/api/v1/fleet/teams/1/secrets', got '%s'", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Expected method 'GET', got '%s'", r.Method)
		}

		response := struct {
			Secrets []EnrollSecret `json:"secrets"`
		}{
			Secrets: []EnrollSecret{
				{Secret: "secret1", CreatedAt: "2024-01-01T00:00:00Z"},
				{Secret: "secret2", CreatedAt: "2024-01-02T00:00:00Z"},
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

	secrets, err := client.GetTeamEnrollSecrets(context.Background(), 1)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(secrets) != 2 {
		t.Errorf("Expected 2 secrets, got: %d", len(secrets))
	}

	if secrets[0].Secret != "secret1" {
		t.Errorf("Expected secret 'secret1', got: %s", secrets[0].Secret)
	}
}

func TestClient_ModifyTeamEnrollSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/teams/1/secrets" {
			t.Errorf("Expected path '/api/v1/fleet/teams/1/secrets', got '%s'", r.URL.Path)
		}
		if r.Method != "PATCH" {
			t.Errorf("Expected method 'PATCH', got '%s'", r.Method)
		}

		response := struct {
			Secrets []EnrollSecret `json:"secrets"`
		}{
			Secrets: []EnrollSecret{
				{Secret: "new-secret", CreatedAt: "2024-01-15T00:00:00Z"},
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

	secrets, err := client.ModifyTeamEnrollSecrets(context.Background(), 1, []EnrollSecret{
		{Secret: "new-secret"},
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(secrets) != 1 {
		t.Errorf("Expected 1 secret, got: %d", len(secrets))
	}

	if secrets[0].Secret != "new-secret" {
		t.Errorf("Expected secret 'new-secret', got: %s", secrets[0].Secret)
	}
}
