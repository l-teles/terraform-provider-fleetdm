package fleetdm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newSelfServiceCategoryTestClient wires a client to the supplied test server.
func newSelfServiceCategoryTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	client, err := NewClient(ClientConfig{
		ServerAddress: serverURL,
		APIKey:        "test-token",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	return client
}

func TestListSelfServiceCategories(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/software/self_service_categories" {
			t.Errorf("Expected path /api/v1/fleet/software/self_service_categories, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got %s", r.Method)
		}
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(map[string]interface{}{
			"self_service_categories": []map[string]interface{}{
				{
					"id":         12,
					"name":       "🌎 Browsers",
					"fleet_id":   3,
					"team_id":    3,
					"created_at": "2026-05-01T14:22:58Z",
					"updated_at": "2026-05-01T14:22:58Z",
				},
				{
					"id":       13,
					"name":     "🧰 Developer tools",
					"fleet_id": 3,
					"team_id":  3,
				},
			},
		})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	categories, err := client.ListSelfServiceCategories(context.Background(), 3)
	if err != nil {
		t.Fatalf("ListSelfServiceCategories failed: %v", err)
	}

	if gotQuery != "fleet_id=3" {
		t.Errorf("Expected query 'fleet_id=3', got %q", gotQuery)
	}
	if len(categories) != 2 {
		t.Fatalf("Expected 2 categories, got %d", len(categories))
	}
	if categories[0].ID != 12 {
		t.Errorf("Expected id 12, got %d", categories[0].ID)
	}
	if categories[0].Name != "🌎 Browsers" {
		t.Errorf("Expected name '🌎 Browsers', got %q", categories[0].Name)
	}
	if categories[0].FleetID == nil || *categories[0].FleetID != 3 {
		t.Errorf("Expected fleet_id 3, got %v", categories[0].FleetID)
	}
	if categories[0].TeamID == nil || *categories[0].TeamID != 3 {
		t.Errorf("Expected team_id 3, got %v", categories[0].TeamID)
	}
	if categories[0].CreatedAt != "2026-05-01T14:22:58Z" {
		t.Errorf("Expected created_at '2026-05-01T14:22:58Z', got %q", categories[0].CreatedAt)
	}
}

// The fleet_id query parameter must be sent even when it is 0 — Fleet uses 0
// for hosts that are not assigned to a fleet, and omitting the parameter
// entirely is a 422.
func TestListSelfServiceCategoriesSendsZeroFleetID(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(map[string]interface{}{
			"self_service_categories": []map[string]interface{}{},
		})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	categories, err := client.ListSelfServiceCategories(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListSelfServiceCategories failed: %v", err)
	}
	if gotQuery != "fleet_id=0" {
		t.Errorf("Expected query 'fleet_id=0', got %q", gotQuery)
	}
	if len(categories) != 0 {
		t.Errorf("Expected 0 categories, got %d", len(categories))
	}
}

// Fleet may serve only the legacy "team_id" key; the client must still resolve
// the fleet ID.
func TestListSelfServiceCategoriesLegacyTeamIDKeyOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"self_service_categories": []map[string]interface{}{
				{"id": 7, "name": "🔐 Security", "team_id": 5},
			},
		})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	categories, err := client.ListSelfServiceCategories(context.Background(), 5)
	if err != nil {
		t.Fatalf("ListSelfServiceCategories failed: %v", err)
	}
	if len(categories) != 1 {
		t.Fatalf("Expected 1 category, got %d", len(categories))
	}
	if categories[0].FleetID == nil || *categories[0].FleetID != 5 {
		t.Errorf("Expected fleet_id to fall back to team_id 5, got %v", categories[0].FleetID)
	}
}

// Fleet answers 422 with an "fleet_id is required" invalid-argument error when
// the query parameter is missing. The client always sends it, but the error
// must surface intact when the server rejects a request for any reason.
func TestListSelfServiceCategoriesMissingFleetIDError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fleet_id") != "" {
			t.Errorf("Expected the test server to be probed without fleet_id, got %q", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Validation Failed",
			"errors": []map[string]string{
				{"name": "fleet_id", "reason": "fleet_id is required"},
			},
		})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	// Bypass ListSelfServiceCategories so the request genuinely omits fleet_id,
	// reproducing Fleet's 422 shape end to end.
	var response listSelfServiceCategoriesResponse
	err := client.Get(context.Background(), selfServiceCategoriesPath, nil, &response)
	if err == nil {
		t.Fatal("Expected an error for a request without fleet_id, got nil")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("Expected status 422, got %d", apiErr.StatusCode)
	}
	if len(apiErr.Errors) != 1 || apiErr.Errors[0].Name != "fleet_id" {
		t.Errorf("Expected a fleet_id error detail, got %+v", apiErr.Errors)
	}
}

func TestListSelfServiceCategoriesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "forbidden"})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	if _, err := client.ListSelfServiceCategories(context.Background(), 3); err == nil {
		t.Fatal("Expected error on 403 response, got nil")
	}
}

func TestGetSelfServiceCategory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("fleet_id"); got != "3" {
			t.Errorf("Expected fleet_id=3, got %q", got)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"self_service_categories": []map[string]interface{}{
				{"id": 12, "name": "🌎 Browsers", "fleet_id": 3},
				{"id": 42, "name": "💼 Engineering", "fleet_id": 3},
			},
		})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	category, err := client.GetSelfServiceCategory(context.Background(), 3, 42)
	if err != nil {
		t.Fatalf("GetSelfServiceCategory failed: %v", err)
	}
	if category == nil {
		t.Fatal("Expected category 42 to be found, got nil")
	}
	if category.Name != "💼 Engineering" {
		t.Errorf("Expected name '💼 Engineering', got %q", category.Name)
	}
}

func TestGetSelfServiceCategoryNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"self_service_categories": []map[string]interface{}{
				{"id": 12, "name": "🌎 Browsers", "fleet_id": 3},
			},
		})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	category, err := client.GetSelfServiceCategory(context.Background(), 3, 999)
	if err != nil {
		t.Fatalf("Expected no error for an absent category, got %v", err)
	}
	if category != nil {
		t.Fatalf("Expected nil for an absent category, got %+v", category)
	}
}

func TestGetSelfServiceCategoryListError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "fleet 9 does not exist"})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	if _, err := client.GetSelfServiceCategory(context.Background(), 9, 1); err == nil {
		t.Fatal("Expected error when the fleet does not exist, got nil")
	}
}

func TestCreateSelfServiceCategory(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/software/self_service_categories" {
			t.Errorf("Expected path /api/v1/fleet/software/self_service_categories, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"self_service_category": map[string]interface{}{
				"id":         42,
				"name":       "💼 Engineering",
				"fleet_id":   3,
				"team_id":    3,
				"created_at": "2026-05-12T18:45:00Z",
				"updated_at": "2026-05-12T18:45:00Z",
			},
		})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	category, err := client.CreateSelfServiceCategory(context.Background(), CreateSelfServiceCategoryRequest{
		FleetID: 3,
		Name:    "💼 Engineering",
	})
	if err != nil {
		t.Fatalf("CreateSelfServiceCategory failed: %v", err)
	}

	if gotBody["name"] != "💼 Engineering" {
		t.Errorf("Expected request name '💼 Engineering', got %v", gotBody["name"])
	}
	if gotBody["fleet_id"] != float64(3) {
		t.Errorf("Expected request fleet_id 3, got %v", gotBody["fleet_id"])
	}
	if _, ok := gotBody["team_id"]; ok {
		t.Errorf("Expected no legacy team_id key in the request body, got %v", gotBody)
	}
	if category.ID != 42 {
		t.Errorf("Expected id 42, got %d", category.ID)
	}
	if category.FleetID == nil || *category.FleetID != 3 {
		t.Errorf("Expected fleet_id 3, got %v", category.FleetID)
	}
}

// fleet_id 0 ("Unassigned" hosts) must be present in the request body, not
// dropped as a zero value.
func TestCreateSelfServiceCategoryZeroFleetID(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"self_service_category": map[string]interface{}{
				"id": 1, "name": "🛟 Support", "fleet_id": 0,
			},
		})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	category, err := client.CreateSelfServiceCategory(context.Background(), CreateSelfServiceCategoryRequest{
		FleetID: 0,
		Name:    "🛟 Support",
	})
	if err != nil {
		t.Fatalf("CreateSelfServiceCategory failed: %v", err)
	}
	if v, ok := gotBody["fleet_id"]; !ok || v != float64(0) {
		t.Errorf("Expected request fleet_id 0 to be present, got %v", gotBody)
	}
	if category.FleetID == nil || *category.FleetID != 0 {
		t.Errorf("Expected fleet_id 0, got %v", category.FleetID)
	}
}

func TestCreateSelfServiceCategoryDuplicateNameError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Validation Failed",
			"errors": []map[string]string{
				{"name": "name", "reason": "category name already exists"},
			},
		})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	_, err := client.CreateSelfServiceCategory(context.Background(), CreateSelfServiceCategoryRequest{
		FleetID: 3,
		Name:    "🌎 Browsers",
	})
	if err == nil {
		t.Fatal("Expected error on duplicate name, got nil")
	}
}

// A 200 with no category in the wrapper must not be reported as success — the
// caller would otherwise dereference a nil category.
func TestCreateSelfServiceCategoryEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	if _, err := client.CreateSelfServiceCategory(context.Background(), CreateSelfServiceCategoryRequest{
		FleetID: 3,
		Name:    "💼 Engineering",
	}); err == nil {
		t.Fatal("Expected error when the response contains no category, got nil")
	}
}

func TestUpdateSelfServiceCategory(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/software/self_service_categories/42" {
			t.Errorf("Expected path /api/v1/fleet/software/self_service_categories/42, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPatch {
			t.Errorf("Expected PATCH method, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"self_service_category": map[string]interface{}{
				"id":         42,
				"name":       "💼 Engineering tools",
				"fleet_id":   3,
				"created_at": "2026-05-12T18:45:00Z",
				"updated_at": "2026-05-12T19:01:00Z",
			},
		})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	category, err := client.UpdateSelfServiceCategory(context.Background(), 42, UpdateSelfServiceCategoryRequest{
		Name: "💼 Engineering tools",
	})
	if err != nil {
		t.Fatalf("UpdateSelfServiceCategory failed: %v", err)
	}

	if gotBody["name"] != "💼 Engineering tools" {
		t.Errorf("Expected request name '💼 Engineering tools', got %v", gotBody["name"])
	}
	// The rename endpoint takes only a name; sending the fleet field would be
	// rejected as an unknown parameter.
	if _, ok := gotBody["fleet_id"]; ok {
		t.Errorf("Expected no fleet_id in the PATCH body, got %v", gotBody)
	}
	if category.Name != "💼 Engineering tools" {
		t.Errorf("Expected name '💼 Engineering tools', got %q", category.Name)
	}
	if category.UpdatedAt != "2026-05-12T19:01:00Z" {
		t.Errorf("Expected updated_at '2026-05-12T19:01:00Z', got %q", category.UpdatedAt)
	}
}

func TestUpdateSelfServiceCategoryNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Resource Not Found"})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	_, err := client.UpdateSelfServiceCategory(context.Background(), 999, UpdateSelfServiceCategoryRequest{Name: "gone"})
	if err == nil {
		t.Fatal("Expected error on 404 response, got nil")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Expected a wrapped *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", apiErr.StatusCode)
	}
}

func TestUpdateSelfServiceCategoryEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	if _, err := client.UpdateSelfServiceCategory(context.Background(), 42, UpdateSelfServiceCategoryRequest{
		Name: "💼 Engineering tools",
	}); err == nil {
		t.Fatal("Expected error when the response contains no category, got nil")
	}
}

func TestDeleteSelfServiceCategory(t *testing.T) {
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	if err := client.DeleteSelfServiceCategory(context.Background(), 42); err != nil {
		t.Fatalf("DeleteSelfServiceCategory failed: %v", err)
	}
	if gotPath != "/api/v1/fleet/software/self_service_categories/42" {
		t.Errorf("Expected path /api/v1/fleet/software/self_service_categories/42, got %s", gotPath)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("Expected DELETE method, got %s", gotMethod)
	}
}

func TestDeleteSelfServiceCategoryNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Resource Not Found"})
	}))
	defer server.Close()

	client := newSelfServiceCategoryTestClient(t, server.URL)

	err := client.DeleteSelfServiceCategory(context.Background(), 999)
	if err == nil {
		t.Fatal("Expected error on 404 response, got nil")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Expected a wrapped *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", apiErr.StatusCode)
	}
}

func TestNormalizeSelfServiceCategory(t *testing.T) {
	fleetID := int64(3)
	teamID := int64(7)

	tests := []struct {
		name     string
		in       SelfServiceCategory
		wantBoth *int64
	}{
		{"fleet_id only", SelfServiceCategory{FleetID: &fleetID}, &fleetID},
		{"team_id only", SelfServiceCategory{TeamID: &teamID}, &teamID},
		{"fleet_id wins", SelfServiceCategory{FleetID: &fleetID, TeamID: &teamID}, &fleetID},
		{"neither", SelfServiceCategory{}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in
			normalizeSelfServiceCategory(&got)

			if tt.wantBoth == nil {
				if got.FleetID != nil || got.TeamID != nil {
					t.Fatalf("Expected both IDs to stay nil, got fleet=%v team=%v", got.FleetID, got.TeamID)
				}
				return
			}
			if got.FleetID == nil || *got.FleetID != *tt.wantBoth {
				t.Errorf("Expected fleet_id %d, got %v", *tt.wantBoth, got.FleetID)
			}
			if got.TeamID == nil || *got.TeamID != *tt.wantBoth {
				t.Errorf("Expected team_id %d, got %v", *tt.wantBoth, got.TeamID)
			}
		})
	}
}
