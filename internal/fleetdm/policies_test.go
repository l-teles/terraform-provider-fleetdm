package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListGlobalPolicies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/global/policies" {
			t.Errorf("expected path /api/v1/fleet/global/policies, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListPoliciesResponse{
			Policies: []Policy{
				{ID: 1, Name: "Disk Encryption", Query: "SELECT 1 FROM disk_encryption WHERE encrypted = 1"},
				{ID: 2, Name: "Firewall Enabled", Query: "SELECT 1 FROM alf WHERE global_state >= 1"},
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

	policies, err := client.ListGlobalPolicies(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(policies) != 2 {
		t.Errorf("expected 2 policies, got: %d", len(policies))
	}

	if policies[0].Name != "Disk Encryption" {
		t.Errorf("expected first policy name 'Disk Encryption', got: %s", policies[0].Name)
	}
}

func TestClient_ListTeamPolicies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/fleets/1/policies" {
			t.Errorf("expected path /api/v1/fleet/fleets/1/policies, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		teamID := 1
		json.NewEncoder(w).Encode(ListPoliciesResponse{
			Policies: []Policy{
				{ID: 3, Name: "Team Policy", Query: "SELECT 1", TeamID: &teamID},
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

	policies, err := client.ListTeamPolicies(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(policies) != 1 {
		t.Errorf("expected 1 policy, got: %d", len(policies))
	}

	if policies[0].TeamID == nil || *policies[0].TeamID != 1 {
		t.Error("expected team ID 1")
	}
}

func TestClient_GetGlobalPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/global/policies/1" {
			t.Errorf("expected path /api/v1/fleet/global/policies/1, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GetPolicyResponse{
			Policy: Policy{
				ID:               1,
				Name:             "Disk Encryption",
				Description:      "Verifies disk encryption is enabled",
				Query:            "SELECT 1 FROM disk_encryption WHERE encrypted = 1",
				Critical:         true,
				Resolution:       "Enable FileVault",
				Platform:         "darwin",
				PassingHostCount: 50,
				FailingHostCount: 5,
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

	policy, err := client.GetGlobalPolicy(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if policy.ID != 1 {
		t.Errorf("expected policy ID 1, got: %d", policy.ID)
	}

	if policy.Name != "Disk Encryption" {
		t.Errorf("expected policy name 'Disk Encryption', got: %s", policy.Name)
	}

	if !policy.Critical {
		t.Error("expected policy to be critical")
	}
}

func TestClient_CreateGlobalPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/global/policies" {
			t.Errorf("expected path /api/v1/fleet/global/policies, got: %s", r.URL.Path)
		}

		var req CreatePolicyRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Name != "New Policy" {
			t.Errorf("expected name 'New Policy', got: %s", req.Name)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreatePolicyResponse{
			Policy: Policy{
				ID:          3,
				Name:        req.Name,
				Description: req.Description,
				Query:       req.Query,
				Critical:    req.Critical,
				Resolution:  req.Resolution,
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

	policy, err := client.CreateGlobalPolicy(context.Background(), CreatePolicyRequest{
		Name:        "New Policy",
		Description: "A new security policy",
		Query:       "SELECT 1 FROM security_check",
		Critical:    true,
		Resolution:  "Fix the issue",
		Platform:    "darwin,linux",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if policy.ID != 3 {
		t.Errorf("expected policy ID 3, got: %d", policy.ID)
	}

	if policy.Name != "New Policy" {
		t.Errorf("expected policy name 'New Policy', got: %s", policy.Name)
	}
}

func TestClient_UpdateGlobalPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/global/policies/3" {
			t.Errorf("expected path /api/v1/fleet/global/policies/3, got: %s", r.URL.Path)
		}

		var req UpdatePolicyRequest
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(UpdatePolicyResponse{
			Policy: Policy{
				ID:          3,
				Name:        req.Name,
				Description: req.Description,
				Critical:    req.Critical,
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

	policy, err := client.UpdateGlobalPolicy(context.Background(), 3, UpdatePolicyRequest{
		Name:        "Updated Policy",
		Description: "Updated description",
		Critical:    false,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if policy.Name != "Updated Policy" {
		t.Errorf("expected policy name 'Updated Policy', got: %s", policy.Name)
	}
}

func TestClient_DeleteGlobalPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/global/policies/delete" {
			t.Errorf("expected path /api/v1/fleet/global/policies/delete, got: %s", r.URL.Path)
		}

		var body struct {
			IDs []int `json:"ids"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		if len(body.IDs) != 1 || body.IDs[0] != 3 {
			t.Errorf("expected IDs [3], got: %v", body.IDs)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string][]int{"deleted": {3}})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.DeleteGlobalPolicy(context.Background(), 3)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_DeleteTeamPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/fleets/1/policies/delete" {
			t.Errorf("expected path /api/v1/fleet/fleets/1/policies/delete, got: %s", r.URL.Path)
		}

		var body struct {
			IDs []int `json:"ids"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		if len(body.IDs) != 1 || body.IDs[0] != 3 {
			t.Errorf("expected IDs [3], got: %v", body.IDs)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string][]int{"deleted": {3}})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.DeleteTeamPolicy(context.Background(), 1, 3)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_CreateTeamPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/fleets/1/policies" {
			t.Errorf("expected path /api/v1/fleet/fleets/1/policies, got: %s", r.URL.Path)
		}

		var req CreatePolicyRequest
		json.NewDecoder(r.Body).Decode(&req)

		teamID := 1
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreatePolicyResponse{
			Policy: Policy{
				ID:     4,
				Name:   req.Name,
				Query:  req.Query,
				TeamID: &teamID,
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

	policy, err := client.CreateTeamPolicy(context.Background(), 1, CreatePolicyRequest{
		Name:  "Team Policy",
		Query: "SELECT 1",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if policy.ID != 4 {
		t.Errorf("expected policy ID 4, got: %d", policy.ID)
	}

	if policy.TeamID == nil || *policy.TeamID != 1 {
		t.Error("expected team ID 1")
	}
}

func TestClient_GetTeamPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/fleets/1/policies/5" {
			t.Errorf("expected path /api/v1/fleet/fleets/1/policies/5, got: %s", r.URL.Path)
		}

		teamID := 1
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GetPolicyResponse{
			Policy: Policy{ID: 5, Name: "Team Policy", Query: "SELECT 1", TeamID: &teamID},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	policy, err := client.GetTeamPolicy(context.Background(), 1, 5)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if policy.ID != 5 {
		t.Errorf("expected policy ID 5, got: %d", policy.ID)
	}
}

func TestClient_UpdateTeamPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH request, got: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/fleets/1/policies/5" {
			t.Errorf("expected path /api/v1/fleet/fleets/1/policies/5, got: %s", r.URL.Path)
		}

		var req UpdatePolicyRequest
		json.NewDecoder(r.Body).Decode(&req)

		teamID := 1
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(UpdatePolicyResponse{
			Policy: Policy{ID: 5, Name: req.Name, TeamID: &teamID},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	policy, err := client.UpdateTeamPolicy(context.Background(), 1, 5, UpdatePolicyRequest{Name: "Updated Team Policy"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if policy.Name != "Updated Team Policy" {
		t.Errorf("expected name 'Updated Team Policy', got: %s", policy.Name)
	}
}

func TestClient_GetPolicy_Global(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/global/policies/1" {
			t.Errorf("expected global path, got: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GetPolicyResponse{Policy: Policy{ID: 1, Name: "Global"}})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	policy, err := client.GetPolicy(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if policy.Name != "Global" {
		t.Errorf("expected name 'Global', got: %s", policy.Name)
	}
}

func TestClient_GetPolicy_Team(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/fleets/2/policies/3" {
			t.Errorf("expected team path, got: %s", r.URL.Path)
		}
		teamID := 2
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GetPolicyResponse{Policy: Policy{ID: 3, Name: "Team", TeamID: &teamID}})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	teamID := 2
	policy, err := client.GetPolicy(context.Background(), 3, &teamID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if policy.Name != "Team" {
		t.Errorf("expected name 'Team', got: %s", policy.Name)
	}
}

func TestClient_CreatePolicy_Global(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/global/policies" {
			t.Errorf("expected global path, got: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CreatePolicyResponse{Policy: Policy{ID: 10, Name: "New Global"}})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	policy, err := client.CreatePolicy(context.Background(), nil, CreatePolicyRequest{Name: "New Global", Query: "SELECT 1"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if policy.ID != 10 {
		t.Errorf("expected ID 10, got: %d", policy.ID)
	}
}

func TestClient_CreatePolicy_Team(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/fleets/3/policies" {
			t.Errorf("expected team path, got: %s", r.URL.Path)
		}
		teamID := 3
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CreatePolicyResponse{Policy: Policy{ID: 11, Name: "New Team", TeamID: &teamID}})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	teamID := 3
	policy, err := client.CreatePolicy(context.Background(), &teamID, CreatePolicyRequest{Name: "New Team", Query: "SELECT 1"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if policy.ID != 11 {
		t.Errorf("expected ID 11, got: %d", policy.ID)
	}
}

func TestClient_UpdatePolicy_Global(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/global/policies/1" {
			t.Errorf("expected global path, got: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(UpdatePolicyResponse{Policy: Policy{ID: 1, Name: "Updated"}})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	policy, err := client.UpdatePolicy(context.Background(), 1, nil, UpdatePolicyRequest{Name: "Updated"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if policy.Name != "Updated" {
		t.Errorf("expected name 'Updated', got: %s", policy.Name)
	}
}

func TestClient_UpdatePolicy_Team(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/fleets/2/policies/5" {
			t.Errorf("expected team path, got: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(UpdatePolicyResponse{Policy: Policy{ID: 5, Name: "Updated Team"}})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	teamID := 2
	policy, err := client.UpdatePolicy(context.Background(), 5, &teamID, UpdatePolicyRequest{Name: "Updated Team"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if policy.Name != "Updated Team" {
		t.Errorf("expected name 'Updated Team', got: %s", policy.Name)
	}
}

func TestClient_DeletePolicy_Global(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/global/policies/delete" {
			t.Errorf("expected global delete path, got: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string][]int{"deleted": {1}})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	err := client.DeletePolicy(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_DeletePolicy_Team(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/fleets/2/policies/delete" {
			t.Errorf("expected team delete path, got: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string][]int{"deleted": {5}})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	teamID := 2
	err := client.DeletePolicy(context.Background(), 5, &teamID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_ListPolicies_Global(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/global/policies" {
			t.Errorf("expected global path, got: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListPoliciesResponse{
			Policies: []Policy{{ID: 1, Name: "P1"}, {ID: 2, Name: "P2"}},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	policies, err := client.ListPolicies(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(policies) != 2 {
		t.Errorf("expected 2 policies, got: %d", len(policies))
	}
}

func TestClient_ListPolicies_Team(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/fleets/3/policies" {
			t.Errorf("expected team path, got: %s", r.URL.Path)
		}
		teamID := 3
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListPoliciesResponse{
			Policies: []Policy{{ID: 10, Name: "TP", TeamID: &teamID}},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	teamID := 3
	policies, err := client.ListPolicies(context.Background(), &teamID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(policies) != 1 {
		t.Errorf("expected 1 policy, got: %d", len(policies))
	}
}
