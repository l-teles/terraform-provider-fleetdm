package fleetdm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
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

// TestClient_CreateTeamPolicy_WithType verifies that the new fleet-only
// fields (type, patch_software_title_id, software_title_id, script_id,
// labels_*) round-trip through the Create endpoint.
func TestClient_CreateTeamPolicy_WithType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req CreatePolicyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Type != "patch" {
			t.Errorf("expected type 'patch', got: %q", req.Type)
		}
		if req.PatchSoftwareTitleID == nil || *req.PatchSoftwareTitleID != 99 {
			t.Errorf("expected patch_software_title_id 99, got: %v", req.PatchSoftwareTitleID)
		}
		if req.ScriptID == nil || *req.ScriptID != 7 {
			t.Errorf("expected script_id 7, got: %v", req.ScriptID)
		}
		if len(req.LabelsIncludeAny) != 1 || req.LabelsIncludeAny[0] != "Macs on Sonoma" {
			t.Errorf("expected labels_include_any [Macs on Sonoma], got: %v", req.LabelsIncludeAny)
		}

		teamID := 1
		echoLabels := make([]PolicyLabel, 0, len(req.LabelsIncludeAny))
		for i, n := range req.LabelsIncludeAny {
			echoLabels = append(echoLabels, PolicyLabel{ID: i + 1, Name: n})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CreatePolicyResponse{
			Policy: Policy{
				ID:               100,
				Name:             req.Name,
				Query:            req.Query,
				Type:             req.Type,
				LabelsIncludeAny: echoLabels,
				TeamID:           &teamID,
				RunScript:        &PolicyAutomationScript{Name: "do-thing", ID: *req.ScriptID},
				PatchSoftware:    &PolicyAutomationPatchSoftware{Name: "Adobe Acrobat.app", SoftwareTitleID: *req.PatchSoftwareTitleID},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})

	patchID := 99
	scriptID := 7
	// Query intentionally left empty — Fleet rejects query together with
	// type=patch. The dedicated TestClient_CreateTeamPolicy_PatchOmitsQuery
	// test asserts the wire-level omission.
	policy, err := client.CreateTeamPolicy(context.Background(), 1, CreatePolicyRequest{
		Name:                 "Patch Acrobat",
		Type:                 "patch",
		PatchSoftwareTitleID: &patchID,
		ScriptID:             &scriptID,
		LabelsIncludeAny:     []string{"Macs on Sonoma"},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if policy.Type != "patch" {
		t.Errorf("expected response type 'patch', got: %q", policy.Type)
	}
	if policy.PatchSoftware == nil || policy.PatchSoftware.SoftwareTitleID != 99 {
		t.Errorf("expected patch_software echo with id 99, got: %+v", policy.PatchSoftware)
	}
	if policy.RunScript == nil || policy.RunScript.ID != 7 {
		t.Errorf("expected run_script echo with id 7, got: %+v", policy.RunScript)
	}
}

// TestClient_CreateTeamPolicy_PatchOmitsQuery is the regression guard for
// patch-policy creation: Fleet rejects `query` together with `type=patch`,
// so an empty Query string on the request struct must serialize as an
// omitted field (via the `omitempty` JSON tag) rather than `"query": ""`.
func TestClient_CreateTeamPolicy_PatchOmitsQuery(t *testing.T) {
	var rawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		rawBody = string(body)

		teamID := 1
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CreatePolicyResponse{
			Policy: Policy{ID: 200, Name: "Patch Acrobat", TeamID: &teamID, Type: "patch"},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	patchID := 99
	if _, err := client.CreateTeamPolicy(context.Background(), 1, CreatePolicyRequest{
		Name:                 "Patch Acrobat",
		Type:                 "patch",
		PatchSoftwareTitleID: &patchID,
		// Query intentionally left empty.
	}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if strings.Contains(rawBody, `"query"`) {
		t.Errorf("expected request body to omit query for patch policy, body was: %s", rawBody)
	}
	if !strings.Contains(rawBody, `"type":"patch"`) {
		t.Errorf("expected request body to include type=patch, body was: %s", rawBody)
	}
}

// TestClient_UpdateTeamPolicy_PointerFieldsSerializeNullToClear is the
// regression guard for the no-omitempty decision on the *pointer* fields
// of UpdatePolicyRequest (script_id, software_title_id, the calendar/CA
// bools). Fleet treats JSON null on these fields as "clear / reset to
// default"; omitempty would suppress the null and silently leave the
// previous server-side value in place. Label slice fields use a different
// convention (null = no change, [] = clear) and are tested separately.
func TestClient_UpdateTeamPolicy_PointerFieldsSerializeNullToClear(t *testing.T) {
	var rawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		rawBody = string(body)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(UpdatePolicyResponse{Policy: Policy{ID: 5, Name: "Cleared"}})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	if _, err := client.UpdateTeamPolicy(context.Background(), 1, 5, UpdatePolicyRequest{
		Name:                           "Cleared",
		Query:                          "SELECT 1",
		SoftwareTitleID:                nil,
		ScriptID:                       nil,
		CalendarEventsEnabled:          nil,
		ConditionalAccessEnabled:       nil,
		ConditionalAccessBypassEnabled: nil,
	}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	for _, want := range []string{
		`"software_title_id":null`,
		`"script_id":null`,
		`"calendar_events_enabled":null`,
		`"conditional_access_enabled":null`,
		`"conditional_access_bypass_enabled":null`,
	} {
		if !strings.Contains(rawBody, want) {
			t.Errorf("expected request body to contain %q, body was: %s", want, rawBody)
		}
	}
}

// TestClient_UpdateTeamPolicy_LabelClearVsNoChange documents and asserts
// the asymmetric label semantics: a nil slice serializes as JSON null
// (Fleet treats this as "no change" — keep existing labels), while an
// empty slice serializes as JSON [] (Fleet clears the labels). The
// provider relies on this distinction to preserve UI-set labels when a
// label set is Unknown at plan time, and to actually clear labels when
// the user removes them from HCL.
func TestClient_UpdateTeamPolicy_LabelClearVsNoChange(t *testing.T) {
	captured := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured <- string(body)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(UpdatePolicyResponse{Policy: Policy{ID: 5, Name: "ok"}})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})

	// nil slice => null (no change semantics).
	if _, err := client.UpdateTeamPolicy(context.Background(), 1, 5, UpdatePolicyRequest{
		Name:             "ok",
		LabelsIncludeAny: nil,
	}); err != nil {
		t.Fatalf("nil-labels update failed: %v", err)
	}
	body := <-captured
	if !strings.Contains(body, `"labels_include_any":null`) {
		t.Errorf("nil slice should serialize as JSON null; got: %s", body)
	}

	// Empty slice => [] (clear semantics).
	if _, err := client.UpdateTeamPolicy(context.Background(), 1, 5, UpdatePolicyRequest{
		Name:             "ok",
		LabelsIncludeAny: []string{},
	}); err != nil {
		t.Fatalf("empty-labels update failed: %v", err)
	}
	body = <-captured
	if !strings.Contains(body, `"labels_include_any":[]`) {
		t.Errorf("empty slice should serialize as JSON []; got: %s", body)
	}
}

// TestClient_GetTeamPolicy_FullResponse decodes a fixture that mirrors the
// Get fleet policy example in the upstream REST API docs (rest-api.md
// lines 8362-8401) and asserts every new field flows through.
func TestClient_GetTeamPolicy_FullResponse(t *testing.T) {
	body := `{
	  "policy": {
	    "id": 43,
	    "name": "Gatekeeper enabled",
	    "query": "SELECT 1 FROM gatekeeper WHERE assessments_enabled = 1;",
	    "description": "Checks if gatekeeper is enabled on macOS devices",
	    "critical": true,
	    "type": "dynamic",
	    "author_id": 42,
	    "author_name": "John",
	    "author_email": "john@example.com",
	    "team_id": 1,
	    "resolution": "Resolution steps",
	    "platform": "darwin",
	    "created_at": "2021-12-16T14:37:37Z",
	    "updated_at": "2021-12-16T16:39:00Z",
	    "passing_host_count": 0,
	    "failing_host_count": 0,
	    "host_count_updated_at": null,
	    "calendar_events_enabled": true,
	    "conditional_access_enabled": false,
	    "fleet_maintained": false,
	    "labels_include_any": [{"id": 11, "name": "Macs on Sonoma"}],
	    "patch_software": {
	      "display_name": "",
	      "name": "Adobe Acrobat.app",
	      "software_title_id": 1234
	    },
	    "install_software": {
	      "name": "Adobe Acrobat.app",
	      "software_title_id": 1234
	    },
	    "run_script": {
	      "name": "Enable gatekeeper",
	      "id": 1337
	    }
	  }
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	policy, err := client.GetTeamPolicy(context.Background(), 1, 43)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if policy.Type != "dynamic" {
		t.Errorf("expected type 'dynamic', got: %q", policy.Type)
	}
	if !policy.CalendarEventsEnabled {
		t.Error("expected calendar_events_enabled true")
	}
	if policy.ConditionalAccessEnabled {
		t.Error("expected conditional_access_enabled false")
	}
	if policy.FleetMaintained {
		t.Error("expected fleet_maintained false")
	}
	if len(policy.LabelsIncludeAny) != 1 || policy.LabelsIncludeAny[0].Name != "Macs on Sonoma" || policy.LabelsIncludeAny[0].ID != 11 {
		t.Errorf("expected labels_include_any [{id:11,name:\"Macs on Sonoma\"}], got: %+v", policy.LabelsIncludeAny)
	}
	if policy.InstallSoftware == nil || policy.InstallSoftware.SoftwareTitleID != 1234 {
		t.Errorf("expected install_software.software_title_id 1234, got: %+v", policy.InstallSoftware)
	}
	if policy.RunScript == nil || policy.RunScript.ID != 1337 {
		t.Errorf("expected run_script.id 1337, got: %+v", policy.RunScript)
	}
	if policy.PatchSoftware == nil || policy.PatchSoftware.SoftwareTitleID != 1234 {
		t.Errorf("expected patch_software.software_title_id 1234, got: %+v", policy.PatchSoftware)
	}
}

func TestClient_ListPoliciesByInstallSoftwareTitleID_Global(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/global/policies" {
			t.Errorf("expected global policies path, got: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListPoliciesResponse{
			Policies: []Policy{
				{ID: 1, Name: "No automation"},
				{ID: 2, Name: "Installs Other", InstallSoftware: &PolicyAutomationSoftware{SoftwareTitleID: 99}},
				{ID: 3, Name: "Installs Target A", InstallSoftware: &PolicyAutomationSoftware{SoftwareTitleID: 42}},
				{ID: 4, Name: "Installs Target B", InstallSoftware: &PolicyAutomationSoftware{SoftwareTitleID: 42}},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	matches, err := client.ListPoliciesByInstallSoftwareTitleID(context.Background(), 42, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got: %d (%+v)", len(matches), matches)
	}
	if matches[0].ID != 3 || matches[1].ID != 4 {
		t.Errorf("expected policy IDs [3, 4], got: [%d, %d]", matches[0].ID, matches[1].ID)
	}
}

func TestClient_ListPoliciesByInstallSoftwareTitleID_Team(t *testing.T) {
	var sawTeamPath bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/fleet/fleets/7/policies" {
			sawTeamPath = true
		} else {
			t.Errorf("expected team policies path, got: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListPoliciesResponse{
			Policies: []Policy{
				{ID: 10, Name: "Installs Target", InstallSoftware: &PolicyAutomationSoftware{SoftwareTitleID: 42}},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	teamID := 7
	matches, err := client.ListPoliciesByInstallSoftwareTitleID(context.Background(), 42, &teamID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !sawTeamPath {
		t.Error("expected ListPoliciesByInstallSoftwareTitleID to hit the team-scoped endpoint")
	}
	if len(matches) != 1 || matches[0].ID != 10 {
		t.Errorf("expected one match with ID 10, got: %+v", matches)
	}
}

// TestClient_ListPoliciesByInstallSoftwareTitleID_Paginates verifies the
// helper walks every page of /global/policies until has_next_results=false,
// finding matches across pages. Without pagination, Fleet's default
// per_page=20 would cause matches on later pages to be missed and the
// caller would hit the 409 "Policy automation uses this software" error
// that this helper exists to prevent.
func TestClient_ListPoliciesByInstallSoftwareTitleID_Paginates(t *testing.T) {
	pagesServed := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/global/policies" {
			t.Errorf("expected /global/policies, got %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("expected per_page=100, got %s", got)
		}

		pagesServed++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "": // page 0 — first call omits the page param
			_ = json.NewEncoder(w).Encode(ListPoliciesResponse{
				Policies: []Policy{
					{ID: 1, Name: "Other", InstallSoftware: &PolicyAutomationSoftware{SoftwareTitleID: 99}},
					{ID: 2, Name: "Match page 0", InstallSoftware: &PolicyAutomationSoftware{SoftwareTitleID: 42}},
				},
				Meta: &PaginationMeta{HasNextResults: true},
			})
		case "1":
			_ = json.NewEncoder(w).Encode(ListPoliciesResponse{
				Policies: []Policy{
					{ID: 3, Name: "Match page 1", InstallSoftware: &PolicyAutomationSoftware{SoftwareTitleID: 42}},
				},
				Meta: &PaginationMeta{HasNextResults: true},
			})
		case "2":
			_ = json.NewEncoder(w).Encode(ListPoliciesResponse{
				Policies: []Policy{
					{ID: 4, Name: "No automation"},
					{ID: 5, Name: "Match page 2", InstallSoftware: &PolicyAutomationSoftware{SoftwareTitleID: 42}},
				},
				Meta: &PaginationMeta{HasNextResults: false},
			})
		default:
			t.Errorf("unexpected page=%s; pagination loop should have stopped", r.URL.Query().Get("page"))
		}
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	matches, err := client.ListPoliciesByInstallSoftwareTitleID(context.Background(), 42, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if pagesServed != 3 {
		t.Errorf("expected exactly 3 pages served, got %d", pagesServed)
	}
	wantIDs := []int{2, 3, 5}
	gotIDs := make([]int, len(matches))
	for i, p := range matches {
		gotIDs[i] = p.ID
	}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Errorf("expected matches %v, got %v", wantIDs, gotIDs)
	}
}

func TestClient_ListPoliciesByInstallSoftwareTitleID_NoMatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListPoliciesResponse{
			Policies: []Policy{
				{ID: 1, Name: "No automation"},
				{ID: 2, Name: "Installs Other", InstallSoftware: &PolicyAutomationSoftware{SoftwareTitleID: 99}},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	matches, err := client.ListPoliciesByInstallSoftwareTitleID(context.Background(), 42, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected zero matches, got: %d", len(matches))
	}
}

func TestClient_SetPolicyInstallSoftwareTitleID_DetachPreservesFields(t *testing.T) {
	// Existing policy state: software_title_id=42, plus all the other fields
	// we expect to be preserved across the PATCH.
	teamID := 7
	existing := Policy{
		ID:                       55,
		Name:                     "Install Slack",
		Description:              "Auto-install on failing hosts",
		Query:                    "SELECT 1 FROM osquery_info",
		Critical:                 true,
		Resolution:               "Install the latest Slack",
		Platform:                 "darwin",
		TeamID:                   &teamID,
		CalendarEventsEnabled:    true,
		ConditionalAccessEnabled: false,
		LabelsIncludeAny:         []PolicyLabel{{ID: 11, Name: "Macs on Sonoma"}, {ID: 12, Name: "Macs in Engineering"}},
		LabelsExcludeAny:         []PolicyLabel{{ID: 13, Name: "Exempt"}},
		InstallSoftware:          &PolicyAutomationSoftware{SoftwareTitleID: 42},
		RunScript:                &PolicyAutomationScript{ID: 9001},
	}

	var sawGet, sawPatch bool
	var patchBody UpdatePolicyRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			sawGet = true
			if r.URL.Path != "/api/v1/fleet/fleets/7/policies/55" {
				t.Errorf("unexpected GET path: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(GetPolicyResponse{Policy: existing})
		case http.MethodPatch:
			sawPatch = true
			if r.URL.Path != "/api/v1/fleet/fleets/7/policies/55" {
				t.Errorf("unexpected PATCH path: %s", r.URL.Path)
			}
			if err := json.NewDecoder(r.Body).Decode(&patchBody); err != nil {
				t.Fatalf("decode patch body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(UpdatePolicyResponse{Policy: existing})
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})

	if err := client.SetPolicyInstallSoftwareTitleID(context.Background(), 55, &teamID, nil); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !sawGet {
		t.Error("expected SetPolicyInstallSoftwareTitleID to issue a GET first")
	}
	if !sawPatch {
		t.Error("expected SetPolicyInstallSoftwareTitleID to issue a PATCH")
	}
	if patchBody.SoftwareTitleID != nil {
		t.Errorf("expected software_title_id to be cleared (nil), got: %v", *patchBody.SoftwareTitleID)
	}
	if patchBody.Name != existing.Name {
		t.Errorf("expected name to be preserved as %q, got: %q", existing.Name, patchBody.Name)
	}
	if patchBody.Description != existing.Description {
		t.Errorf("expected description to be preserved, got: %q", patchBody.Description)
	}
	if patchBody.Query != existing.Query {
		t.Errorf("expected query to be preserved, got: %q", patchBody.Query)
	}
	if patchBody.Critical != existing.Critical {
		t.Errorf("expected critical=true to be preserved, got: %v", patchBody.Critical)
	}
	if patchBody.Resolution != existing.Resolution {
		t.Errorf("expected resolution to be preserved, got: %q", patchBody.Resolution)
	}
	if patchBody.Platform != existing.Platform {
		t.Errorf("expected platform to be preserved, got: %q", patchBody.Platform)
	}
	if patchBody.CalendarEventsEnabled == nil || *patchBody.CalendarEventsEnabled != true {
		t.Errorf("expected calendar_events_enabled true to be preserved, got: %v", patchBody.CalendarEventsEnabled)
	}
	if patchBody.ConditionalAccessEnabled == nil || *patchBody.ConditionalAccessEnabled != false {
		t.Errorf("expected conditional_access_enabled false to be preserved, got: %v", patchBody.ConditionalAccessEnabled)
	}
	if patchBody.ScriptID == nil || *patchBody.ScriptID != 9001 {
		t.Errorf("expected script_id 9001 to be preserved, got: %v", patchBody.ScriptID)
	}
	wantLabelsInc := []string{"Macs on Sonoma", "Macs in Engineering"}
	if !slices.Equal(patchBody.LabelsIncludeAny, wantLabelsInc) {
		t.Errorf("expected labels_include_any %v, got: %v", wantLabelsInc, patchBody.LabelsIncludeAny)
	}
	wantLabelsExc := []string{"Exempt"}
	if !slices.Equal(patchBody.LabelsExcludeAny, wantLabelsExc) {
		t.Errorf("expected labels_exclude_any %v, got: %v", wantLabelsExc, patchBody.LabelsExcludeAny)
	}
}

func TestClient_SetPolicyInstallSoftwareTitleID_Reattach(t *testing.T) {
	existing := Policy{
		ID:   55,
		Name: "Install Slack",
	}
	var patchBody UpdatePolicyRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(GetPolicyResponse{Policy: existing})
		case http.MethodPatch:
			if err := json.NewDecoder(r.Body).Decode(&patchBody); err != nil {
				t.Fatalf("decode patch body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(UpdatePolicyResponse{Policy: existing})
		}
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})

	newID := 77
	if err := client.SetPolicyInstallSoftwareTitleID(context.Background(), 55, nil, &newID); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if patchBody.SoftwareTitleID == nil {
		t.Fatal("expected software_title_id to be set, got nil")
	}
	if *patchBody.SoftwareTitleID != 77 {
		t.Errorf("expected software_title_id 77, got: %d", *patchBody.SoftwareTitleID)
	}
}

func TestPolicyLabelsToStrings(t *testing.T) {
	if got := policyLabelsToStrings(nil); got != nil {
		t.Errorf("expected nil for nil input, got: %v", got)
	}
	if got := policyLabelsToStrings([]PolicyLabel{}); got != nil {
		t.Errorf("expected nil for empty input, got: %v", got)
	}
	in := []PolicyLabel{{ID: 1, Name: "Alpha"}, {ID: 2, Name: "Beta"}}
	got := policyLabelsToStrings(in)
	want := []string{"Alpha", "Beta"}
	if !slices.Equal(got, want) {
		t.Errorf("expected %v, got: %v", want, got)
	}
}

// TestClient_ListPoliciesByPatchSoftwareTitleID_Global verifies that the
// helper filters on Policy.PatchSoftware (not InstallSoftware), and ignores
// policies that don't reference the target title or have only install_software
// attached.
func TestClient_ListPoliciesByPatchSoftwareTitleID_Global(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/global/policies" {
			t.Errorf("expected global policies path, got: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListPoliciesResponse{
			Policies: []Policy{
				{ID: 1, Name: "No automation"},
				{ID: 2, Name: "Install only", InstallSoftware: &PolicyAutomationSoftware{SoftwareTitleID: 42}},
				{ID: 3, Name: "Patch other", PatchSoftware: &PolicyAutomationPatchSoftware{SoftwareTitleID: 99}},
				{ID: 4, Name: "Patch target A", PatchSoftware: &PolicyAutomationPatchSoftware{SoftwareTitleID: 42}},
				{ID: 5, Name: "Patch target B", PatchSoftware: &PolicyAutomationPatchSoftware{SoftwareTitleID: 42}},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	matches, err := client.ListPoliciesByPatchSoftwareTitleID(context.Background(), 42, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches (4 and 5), got: %d (%+v)", len(matches), matches)
	}
	if matches[0].ID != 4 || matches[1].ID != 5 {
		t.Errorf("expected policy IDs [4, 5], got: [%d, %d]", matches[0].ID, matches[1].ID)
	}
}

// TestClient_ListPoliciesByPatchSoftwareTitleID_FallbackInstallSoftware
// verifies the fallback path: when Fleet's list response omits the
// `patch_software` block on a type=patch policy (a documented Fleet
// behavior — see mapPatchSoftware in the policy resource), the helper
// still recognizes the policy via the always-echoed install_software
// field. Without this fallback, detachPoliciesBeforeTitleDelete would
// miss the policy and Fleet would 409 on DeleteSoftwarePackage with
// "This software has a patch policy".
func TestClient_ListPoliciesByPatchSoftwareTitleID_FallbackInstallSoftware(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListPoliciesResponse{
			Policies: []Policy{
				// type=patch policy with NO patch_software echoed — Fleet
				// sometimes omits it; install_software is always present.
				{ID: 11, Name: "Patch via install fallback", Type: "patch", InstallSoftware: &PolicyAutomationSoftware{SoftwareTitleID: 42}},
				// type=patch policy that DOES echo patch_software.
				{ID: 12, Name: "Patch explicit", Type: "patch", PatchSoftware: &PolicyAutomationPatchSoftware{SoftwareTitleID: 42}},
				// type=dynamic install_software policy — must NOT match here.
				{ID: 13, Name: "Install only (dynamic)", Type: "dynamic", InstallSoftware: &PolicyAutomationSoftware{SoftwareTitleID: 42}},
				// Different title.
				{ID: 14, Name: "Patch other title", Type: "patch", PatchSoftware: &PolicyAutomationPatchSoftware{SoftwareTitleID: 99}},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	matches, err := client.ListPoliciesByPatchSoftwareTitleID(context.Background(), 42, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches (id 11 via fallback, id 12 explicit), got %d: %+v", len(matches), matches)
	}
	gotIDs := []int{matches[0].ID, matches[1].ID}
	wantIDs := []int{11, 12}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Errorf("expected ids %v, got %v", wantIDs, gotIDs)
	}
}

// TestClient_ListPoliciesByPatchSoftwareTitleID_Team verifies team scoping
// hits the team-scoped endpoint.
func TestClient_ListPoliciesByPatchSoftwareTitleID_Team(t *testing.T) {
	var sawTeamPath bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/fleet/fleets/7/policies" {
			sawTeamPath = true
		} else {
			t.Errorf("expected team policies path, got: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListPoliciesResponse{
			Policies: []Policy{
				{ID: 10, Name: "Patch", PatchSoftware: &PolicyAutomationPatchSoftware{SoftwareTitleID: 42}},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	teamID := 7
	matches, err := client.ListPoliciesByPatchSoftwareTitleID(context.Background(), 42, &teamID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !sawTeamPath {
		t.Error("expected ListPoliciesByPatchSoftwareTitleID to hit the team-scoped endpoint")
	}
	if len(matches) != 1 || matches[0].ID != 10 {
		t.Errorf("expected one match with ID 10, got: %+v", matches)
	}
}

// TestClient_SetPolicyPatchSoftwareTitleID_Detach verifies the helper sends
// a single-field PATCH body with patch_software_title_id=null and no other
// fields — Fleet's PATCH semantics treat absent fields as "no change", so
// the minimal body is enough to detach without round-tripping every field.
func TestClient_SetPolicyPatchSoftwareTitleID_Detach(t *testing.T) {
	var sawPatch bool
	var rawBody map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/fleets/7/policies/55" {
			t.Errorf("unexpected PATCH path: %s", r.URL.Path)
		}
		sawPatch = true
		if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
			t.Fatalf("decode patch body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	teamID := 7
	if err := client.SetPolicyPatchSoftwareTitleID(context.Background(), 55, &teamID, nil); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !sawPatch {
		t.Fatal("expected a PATCH request")
	}
	if len(rawBody) != 1 {
		t.Errorf("expected single-field PATCH body, got %d fields: %v", len(rawBody), rawBody)
	}
	raw, ok := rawBody["patch_software_title_id"]
	if !ok {
		t.Fatal("expected patch_software_title_id field in body")
	}
	if string(raw) != "null" {
		t.Errorf("expected patch_software_title_id=null, got: %s", raw)
	}
}

// TestClient_SetPolicyPatchSoftwareTitleID_Reattach verifies the helper
// sends patch_software_title_id=<value> when reattaching.
func TestClient_SetPolicyPatchSoftwareTitleID_Reattach(t *testing.T) {
	var rawBody map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/global/policies/55" {
			t.Errorf("unexpected PATCH path (expected global): %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
			t.Fatalf("decode patch body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	newID := 77
	if err := client.SetPolicyPatchSoftwareTitleID(context.Background(), 55, nil, &newID); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if string(rawBody["patch_software_title_id"]) != "77" {
		t.Errorf("expected patch_software_title_id=77, got: %s", rawBody["patch_software_title_id"])
	}
}
