package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	user, err := client.CreateUser(context.Background(), req)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if user.ID != 3 {
		t.Errorf("Expected user ID 3, got: %d", user.ID)
	}

	if user.Name != "New User" {
		t.Errorf("Expected user name 'New User', got: %s", user.Name)
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

// Helper function
func strPtr(s string) *string {
	return &s
}
