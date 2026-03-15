package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient_ValidConfig(t *testing.T) {
	config := ClientConfig{
		ServerAddress: "https://fleet.example.com",
		APIKey:        "test-api-key",
		VerifyTLS:     true,
		Timeout:       30,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if client.BaseURL != "https://fleet.example.com/api/v1/fleet" {
		t.Errorf("expected BaseURL 'https://fleet.example.com/api/v1/fleet', got: %s", client.BaseURL)
	}

	if client.APIKey != "test-api-key" {
		t.Errorf("expected APIKey 'test-api-key', got: %s", client.APIKey)
	}
}

func TestNewClient_MissingServerAddress(t *testing.T) {
	config := ClientConfig{
		APIKey: "test-api-key",
	}

	_, err := NewClient(config)
	if err == nil {
		t.Fatal("expected error for missing server address")
	}
}

func TestNewClient_MissingAPIKey(t *testing.T) {
	config := ClientConfig{
		ServerAddress: "https://fleet.example.com",
	}

	_, err := NewClient(config)
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestNewClient_AddScheme(t *testing.T) {
	config := ClientConfig{
		ServerAddress: "fleet.example.com",
		APIKey:        "test-api-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if client.BaseURL != "https://fleet.example.com/api/v1/fleet" {
		t.Errorf("expected BaseURL with https scheme, got: %s", client.BaseURL)
	}
}

func TestClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("expected Authorization header, got: %s", r.Header.Get("Authorization"))
		}

		if r.URL.Path != "/api/v1/fleet/test" {
			t.Errorf("expected path /api/v1/fleet/test, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	config := ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	var result map[string]string
	err = client.Get(context.Background(), "/test", nil, &result)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("expected status 'ok', got: %s", result["status"])
	}
}

func TestClient_GetWithParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "1" {
			t.Errorf("expected page=1, got: %s", r.URL.Query().Get("page"))
		}
		if r.URL.Query().Get("per_page") != "10" {
			t.Errorf("expected per_page=10, got: %s", r.URL.Query().Get("per_page"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	config := ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	params := map[string]string{
		"page":     "1",
		"per_page": "10",
	}

	var result map[string]string
	err = client.Get(context.Background(), "/test", params, &result)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_Post(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got: %s", r.Method)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got: %s", r.Header.Get("Content-Type"))
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "test-team" {
			t.Errorf("expected name 'test-team', got: %s", body["name"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{"team": map[string]interface{}{"id": 1, "name": "test-team"}})
	}))
	defer server.Close()

	config := ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	requestBody := map[string]string{"name": "test-team"}
	var result map[string]interface{}
	err = client.Post(context.Background(), "/teams", requestBody, &result)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_Patch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH request, got: %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
	}))
	defer server.Close()

	config := ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	var result map[string]string
	err = client.Patch(context.Background(), "/teams/1", map[string]string{"name": "updated"}, &result)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_Delete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE request, got: %s", r.Method)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.Delete(context.Background(), "/teams/1", nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Team not found",
			"errors": []map[string]string{
				{"name": "id", "reason": "Team with ID 999 not found"},
			},
		})
	}))
	defer server.Close()

	config := ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	var result map[string]string
	err = client.Get(context.Background(), "/teams/999", nil, &result)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got: %T", err)
	}

	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("expected status code 404, got: %d", apiErr.StatusCode)
	}

	if apiErr.Message != "Team not found" {
		t.Errorf("expected message 'Team not found', got: %s", apiErr.Message)
	}
}

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *APIError
		expected string
	}{
		{
			name: "with message only",
			err: &APIError{
				StatusCode: 404,
				Message:    "Not found",
			},
			expected: "FleetDM API error (status 404): Not found",
		},
		{
			name: "with error details",
			err: &APIError{
				StatusCode: 400,
				Message:    "Validation error",
				Errors: []ErrorDetail{
					{Name: "name", Reason: "is required"},
				},
			},
			expected: "FleetDM API error (status 400): Validation error - name: is required",
		},
		{
			name: "with multiple error details",
			err: &APIError{
				StatusCode: 400,
				Message:    "Validation error",
				Errors: []ErrorDetail{
					{Name: "name", Reason: "is required"},
					{Name: "email", Reason: "is invalid"},
				},
			},
			expected: "FleetDM API error (status 400): Validation error - name: is required; email: is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("expected: %s, got: %s", tt.expected, result)
			}
		})
	}
}
