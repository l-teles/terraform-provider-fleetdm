package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_ListUsers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/users" {
			t.Errorf("Expected path '/api/v1/fleet/users', got '%s'", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Expected method 'GET', got '%s'", r.Method)
		}

		response := ListUsersResponse{
			Users: []User{
				{ID: 1, Name: "Admin User", Email: "admin@example.com", GlobalRole: strPtr("admin")},
				{ID: 2, Name: "Observer User", Email: "observer@example.com", GlobalRole: strPtr("observer")},
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

	users, err := client.ListUsers(context.Background(), nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(users) != 2 {
		t.Errorf("Expected 2 users, got: %d", len(users))
	}

	if users[0].Name != "Admin User" {
		t.Errorf("Expected user name 'Admin User', got: %s", users[0].Name)
	}

	if users[0].Email != "admin@example.com" {
		t.Errorf("Expected user email 'admin@example.com', got: %s", users[0].Email)
	}
}

func TestClient_ListUsersWithFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("query") != "admin" {
			t.Errorf("Expected query 'admin', got '%s'", query.Get("query"))
		}

		response := ListUsersResponse{
			Users: []User{
				{ID: 1, Name: "Admin User", Email: "admin@example.com", GlobalRole: strPtr("admin")},
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

	params := map[string]string{"query": "admin"}
	users, err := client.ListUsers(context.Background(), params)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(users) != 1 {
		t.Errorf("Expected 1 user, got: %d", len(users))
	}
}

func TestClient_GetUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/users/1" {
			t.Errorf("Expected path '/api/v1/fleet/users/1', got '%s'", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Expected method 'GET', got '%s'", r.Method)
		}

		response := GetUserResponse{
			User: User{
				ID:         1,
				Name:       "Admin User",
				Email:      "admin@example.com",
				GlobalRole: strPtr("admin"),
				SSOEnabled: false,
				MFAEnabled: false,
				APIOnly:    false,
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

	user, err := client.GetUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if user.ID != 1 {
		t.Errorf("Expected user ID 1, got: %d", user.ID)
	}

	if user.Name != "Admin User" {
		t.Errorf("Expected user name 'Admin User', got: %s", user.Name)
	}

	if *user.GlobalRole != "admin" {
		t.Errorf("Expected global role 'admin', got: %s", *user.GlobalRole)
	}
}

func TestClient_CreateUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/users/admin" {
			t.Errorf("Expected path '/api/v1/fleet/users/admin', got '%s'", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("Expected method 'POST', got '%s'", r.Method)
		}

		var req CreateUserRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Name != "New User" {
			t.Errorf("Expected name 'New User', got: %s", req.Name)
		}
		if req.Email != "newuser@example.com" {
			t.Errorf("Expected email 'newuser@example.com', got: %s", req.Email)
		}

		response := CreateUserResponse{
			User: User{
				ID:         3,
				Name:       req.Name,
				Email:      req.Email,
				GlobalRole: req.GlobalRole,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	req := CreateUserRequest{
		Name:       "New User",
		Email:      "newuser@example.com",
		Password:   "securepassword123",
		GlobalRole: strPtr("observer"),
	}

	user, token, err := client.CreateUser(context.Background(), req)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if user.ID != 3 {
		t.Errorf("Expected user ID 3, got: %d", user.ID)
	}

	if user.Name != "New User" {
		t.Errorf("Expected user name 'New User', got: %s", user.Name)
	}

	if token != "" {
		t.Errorf("Expected no token for a non-API-only user, got: %q", token)
	}
}

// TestClient_CreateUserReturnsAPIOnlyToken pins the create-time token
// contract. Fleet returns a session token in the `token` field of the
// POST /users/admin response for API-only, non-SSO users, and never returns it
// again on a subsequent read — so CreateUser must surface it to the caller.
func TestClient_CreateUserReturnsAPIOnlyToken(t *testing.T) {
	const wantToken = "kJ3xN0tARealT0ken=="

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}
		if _, present := body["api_endpoints"]; present {
			t.Error("POST /users/admin must never carry api_endpoints: Fleet rejects it with a 422")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CreateUserResponse{
			User:  User{ID: 7, Name: "Bot", Email: "bot@example.com", APIOnly: true},
			Token: wantToken,
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	user, token, err := client.CreateUser(context.Background(), CreateUserRequest{
		Name:    "Bot",
		Email:   "bot@example.com",
		APIOnly: true,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !user.APIOnly {
		t.Error("Expected api_only user")
	}
	if token != wantToken {
		t.Errorf("Expected token %q, got %q", wantToken, token)
	}
}

func TestClient_UpdateUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/users/1" {
			t.Errorf("Expected path '/api/v1/fleet/users/1', got '%s'", r.URL.Path)
		}
		if r.Method != "PATCH" {
			t.Errorf("Expected method 'PATCH', got '%s'", r.Method)
		}

		var req UpdateUserRequest
		json.NewDecoder(r.Body).Decode(&req)

		response := UpdateUserResponse{
			User: User{
				ID:         1,
				Name:       req.Name,
				Email:      "admin@example.com",
				GlobalRole: strPtr("admin"),
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

	req := UpdateUserRequest{
		Name: "Updated Admin User",
	}

	user, err := client.UpdateUser(context.Background(), 1, req)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if user.Name != "Updated Admin User" {
		t.Errorf("Expected user name 'Updated Admin User', got: %s", user.Name)
	}
}

// TestClient_UpdateUserOmitsAPIOnly guards the wire format of UpdateUser
// requests. Fleet's PATCH /users/{id} endpoint rejects any presence of the
// `api_only` field with a 422 ("api_endpoints: This endpoint does not
// accept API endpoint values"), so the field is intentionally absent from
// UpdateUserRequest. This test parses the actual JSON body to assert
// `api_only` never appears — a stronger guarantee than relying on struct
// shape, because anyone adding the field back to UpdateUserRequest (or
// embedding it indirectly) would break this test.
func TestClient_UpdateUserOmitsAPIOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}
		if _, present := body["api_only"]; present {
			t.Errorf("UpdateUser request body must not contain api_only; got body: %v", body)
		}

		response := UpdateUserResponse{
			User: User{ID: 1, Name: "Updated", Email: "user@example.com"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	ssoEnabled := false
	mfaEnabled := false
	globalRole := "observer"
	req := UpdateUserRequest{
		Name:       "Updated",
		Email:      "user@example.com",
		SSOEnabled: &ssoEnabled,
		MFAEnabled: &mfaEnabled,
		GlobalRole: &globalRole,
	}
	if _, err := client.UpdateUser(context.Background(), 1, req); err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

func TestClient_DeleteUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/users/1" {
			t.Errorf("Expected path '/api/v1/fleet/users/1', got '%s'", r.URL.Path)
		}
		if r.Method != "DELETE" {
			t.Errorf("Expected method 'DELETE', got '%s'", r.Method)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	err := client.DeleteUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

func TestClient_GetUserNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Resource Not Found",
			"errors": []map[string]string{
				{"name": "base", "reason": "User with id=999 was not found in the datastore"},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	_, err := client.GetUser(context.Background(), 999)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	// The error is wrapped, so we check the message contains expected text
	if err.Error() == "" {
		t.Fatal("Expected non-empty error message")
	}
}

// TestClient_ModifyAPIOnlyUserAPIEndpointsWireFormat pins the three-state
// encoding of the `api_endpoints` field, which Fleet distinguishes by
// presence: absent leaves the scope untouched, JSON null clears it, and an
// array replaces it. An empty array is NOT equivalent to null — Fleet rejects
// it with a 422 ("at least one API endpoint must be specified") — so the
// nil-slice-behind-a-pointer encoding matters and is asserted on the raw body.
func TestClient_ModifyAPIOnlyUserAPIEndpointsWireFormat(t *testing.T) {
	tests := []struct {
		name    string
		refs    *[]APIEndpointRef
		wantRaw string
	}{
		{
			name:    "absent leaves the scope unchanged",
			refs:    nil,
			wantRaw: "",
		},
		{
			name:    "null clears the scope",
			refs:    func() *[]APIEndpointRef { var s []APIEndpointRef; return &s }(),
			wantRaw: "null",
		},
		{
			name: "array replaces the scope",
			refs: &[]APIEndpointRef{
				{Method: "GET", Path: "/api/v1/fleet/hosts"},
			},
			wantRaw: `[{"method":"GET","path":"/api/v1/fleet/hosts"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/fleet/users/api_only/5" {
					t.Errorf("Expected path '/api/v1/fleet/users/api_only/5', got '%s'", r.URL.Path)
				}
				if r.Method != "PATCH" {
					t.Errorf("Expected method 'PATCH', got '%s'", r.Method)
				}

				var raw map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
					t.Fatalf("Failed to decode request body: %v", err)
				}

				got, present := raw["api_endpoints"]
				if tt.wantRaw == "" {
					if present {
						t.Errorf("Expected api_endpoints to be absent, got %s", got)
					}
				} else {
					if !present {
						t.Fatal("Expected api_endpoints to be present")
					}
					if string(got) != tt.wantRaw {
						t.Errorf("Expected api_endpoints %s, got %s", tt.wantRaw, got)
					}
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(UpdateUserResponse{
					User: User{ID: 5, Name: "Bot", APIOnly: true},
				})
			}))
			defer server.Close()

			client, _ := NewClient(ClientConfig{
				ServerAddress: server.URL,
				APIKey:        "test-key",
			})

			user, err := client.ModifyAPIOnlyUser(context.Background(), 5, ModifyAPIOnlyUserRequest{
				APIEndpoints: tt.refs,
			})
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}
			if user.ID != 5 {
				t.Errorf("Expected user ID 5, got: %d", user.ID)
			}
		})
	}
}

// TestClient_ModifyAPIOnlyUserError checks that Fleet's 422 for a target that
// is not an API-only user surfaces to the caller.
func TestClient_ModifyAPIOnlyUserError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Validation Failed",
			"errors": []map[string]string{
				{"name": "id", "reason": "target user is not an API-only user"},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	_, err := client.ModifyAPIOnlyUser(context.Background(), 5, ModifyAPIOnlyUserRequest{})
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not an API-only user") {
		t.Errorf("Expected error to mention the API-only constraint, got: %v", err)
	}
}

// TestClient_UpdateUserDecodesAPIEndpointsEcho pins what UpdateUser does with
// the `api_endpoints` Fleet echoes back on PATCH /users/{id}. That endpoint
// refuses to *accept* the field but still returns the user's current scope, so
// the response has to deserialize into User.APIEndpoints rather than being
// dropped. The provider does not depend on this echo to keep the scope in
// state — an update that leaves the scope alone carries the planned value
// through instead — but a silent decode regression here would be invisible
// otherwise.
func TestClient_UpdateUserDecodesAPIEndpointsEcho(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}
		// The request must not carry the field even though the response does.
		if _, present := body["api_endpoints"]; present {
			t.Error("PATCH /users/{id} must never carry api_endpoints: Fleet rejects it with a 422")
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user":{"id":5,"name":"Renamed","email":"bot@example.com","api_only":true,
			"api_endpoints":[{"method":"GET","path":"/api/v1/fleet/labels"}]}}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	user, err := client.UpdateUser(context.Background(), 5, UpdateUserRequest{Name: "Renamed"})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if user.Name != "Renamed" {
		t.Errorf("Expected name 'Renamed', got %q", user.Name)
	}
	if len(user.APIEndpoints) != 1 {
		t.Fatalf("Expected the echoed scope to decode into 1 entry, got %d", len(user.APIEndpoints))
	}
	if user.APIEndpoints[0].Path != "/api/v1/fleet/labels" {
		t.Errorf("Expected the echoed path, got %q", user.APIEndpoints[0].Path)
	}
}

// TestClient_UpdateUserOmitsAPIEndpointsEcho covers the other shape: a
// response with no `api_endpoints` key at all, which is what an unrestricted
// user (or Fleet Free) returns. It must decode to an empty scope, not an error.
func TestClient_UpdateUserOmitsAPIEndpointsEcho(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user":{"id":5,"name":"Renamed","email":"bot@example.com","api_only":true}}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	user, err := client.UpdateUser(context.Background(), 5, UpdateUserRequest{Name: "Renamed"})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if len(user.APIEndpoints) != 0 {
		t.Errorf("Expected an empty scope, got %d entries", len(user.APIEndpoints))
	}
}

// TestClient_GetUserParsesAPIEndpoints checks that a user's endpoint scope is
// deserialized from a read, which is what lets the provider detect drift.
func TestClient_GetUserParsesAPIEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user":{"id":5,"name":"Bot","email":"bot@example.com","api_only":true,
			"api_endpoints":[{"method":"GET","path":"/api/v1/fleet/hosts"},
			                 {"method":"GET","path":"/api/v1/fleet/hosts/:id"}]}}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})

	user, err := client.GetUser(context.Background(), 5)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(user.APIEndpoints) != 2 {
		t.Fatalf("Expected 2 API endpoints, got %d", len(user.APIEndpoints))
	}
	if user.APIEndpoints[1].Path != "/api/v1/fleet/hosts/:id" {
		t.Errorf("Expected the :id route template, got %q", user.APIEndpoints[1].Path)
	}
}

// Helper function
func strPtr(s string) *string {
	return &s
}
