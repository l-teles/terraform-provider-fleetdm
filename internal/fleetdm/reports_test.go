package fleetdm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeQuery(t *testing.T) {
	t.Run("prefers new fields when both present", func(t *testing.T) {
		teamID, fleetID := 1, 2
		q := Query{Report: "SELECT new", Query: "SELECT old", FleetID: &fleetID, TeamID: &teamID}
		normalizeQuery(&q)
		if q.Query != "SELECT new" {
			t.Errorf("expected Query to be overwritten with Report value, got: %s", q.Query)
		}
		if *q.TeamID != fleetID {
			t.Errorf("expected TeamID to be overwritten with FleetID value %d, got: %d", fleetID, *q.TeamID)
		}
	})

	t.Run("falls back to legacy when new fields empty", func(t *testing.T) {
		teamID := 5
		q := Query{Query: "SELECT legacy", TeamID: &teamID}
		normalizeQuery(&q)
		if q.Report != "SELECT legacy" {
			t.Errorf("expected Report to be populated from Query, got: %s", q.Report)
		}
		if q.Query != "SELECT legacy" {
			t.Errorf("expected Query to remain, got: %s", q.Query)
		}
		if q.FleetID == nil || *q.FleetID != teamID {
			t.Errorf("expected FleetID to be populated from TeamID %d, got: %v", teamID, q.FleetID)
		}
	})

	t.Run("handles nil team pointers", func(t *testing.T) {
		q := Query{Report: "SELECT 1"}
		normalizeQuery(&q)
		if q.Query != "SELECT 1" {
			t.Errorf("expected Query synced from Report, got: %s", q.Query)
		}
		if q.TeamID != nil || q.FleetID != nil {
			t.Errorf("expected both team pointers to remain nil")
		}
	})
}

func TestClient_ListQueries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/reports" {
			t.Errorf("expected path /api/v1/fleet/reports, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListQueriesResponse{
			Reports: []Query{
				{ID: 1, Name: "Get OS Version", Report: "SELECT * FROM os_version"},
				{ID: 2, Name: "List Users", Report: "SELECT * FROM users"},
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
	if queries[0].Query != "SELECT * FROM os_version" {
		t.Errorf("expected SQL in Query field, got: %s", queries[0].Query)
	}
}

// TestClient_ListQueries_legacyResponse verifies fallback to the legacy "queries"
// key returned by Fleet <= 4.82.0 (before the reports rename was completed).
func TestClient_ListQueries_legacyResponse(t *testing.T) {
	teamID := 7
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Simulate a legacy Fleet response: only the "queries" key is populated,
		// using "query" for SQL and "team_id" for the team field.
		json.NewEncoder(w).Encode(map[string]interface{}{
			"queries": []map[string]interface{}{
				{"id": 1, "name": "Legacy Query", "query": "SELECT 1", "team_id": teamID},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	queries, err := client.ListQueries(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(queries) != 1 {
		t.Fatalf("expected 1 query, got: %d", len(queries))
	}
	if queries[0].Query != "SELECT 1" {
		t.Errorf("expected SQL 'SELECT 1' via legacy fallback, got: %s", queries[0].Query)
	}
	if queries[0].TeamID == nil || *queries[0].TeamID != teamID {
		t.Errorf("expected TeamID %d via legacy fallback, got: %v", teamID, queries[0].TeamID)
	}
}

func TestClient_GetQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/reports/1" {
			t.Errorf("expected path /api/v1/fleet/reports/1, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GetQueryResponse{
			Report: Query{
				ID:          1,
				Name:        "Get OS Version",
				Description: "Retrieves OS version information",
				Report:      "SELECT * FROM os_version",
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
	if query.Query != "SELECT * FROM os_version" {
		t.Errorf("expected SQL in Query field, got: %s", query.Query)
	}
}

// TestClient_GetQuery_legacyResponse verifies fallback to the legacy "query"
// wrapper key returned by Fleet <= 4.82.0.
func TestClient_GetQuery_legacyResponse(t *testing.T) {
	teamID := 3
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"query": map[string]interface{}{
				"id": 1, "name": "Legacy", "query": "SELECT 1", "team_id": teamID,
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	query, err := client.GetQuery(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if query.Query != "SELECT 1" {
		t.Errorf("expected SQL 'SELECT 1' via legacy fallback, got: %s", query.Query)
	}
	if query.TeamID == nil || *query.TeamID != teamID {
		t.Errorf("expected TeamID %d via legacy fallback, got: %v", teamID, query.TeamID)
	}
}

// TestClient_CreateQuery_legacyResponse verifies fallback to the legacy "query"
// wrapper key returned by Fleet <= 4.82.0.
func TestClient_CreateQuery_legacyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"query": map[string]interface{}{
				"id": 5, "name": "New", "query": "SELECT 2",
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	query, err := client.CreateQuery(context.Background(), CreateQueryRequest{
		Name: "New", Query: "SELECT 2",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if query.ID != 5 {
		t.Errorf("expected ID 5 via legacy fallback, got: %d", query.ID)
	}
	if query.Query != "SELECT 2" {
		t.Errorf("expected SQL 'SELECT 2' via legacy fallback, got: %s", query.Query)
	}
}

func TestClient_CreateQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/reports" {
			t.Errorf("expected path /api/v1/fleet/reports, got: %s", r.URL.Path)
		}

		var req CreateQueryRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Name != "New Query" {
			t.Errorf("expected name 'New Query', got: %s", req.Name)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreateQueryResponse{
			Report: Query{
				ID:          3,
				Name:        req.Name,
				Description: req.Description,
				Report:      req.Query,
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

// TestClient_CreateQuery_LabelSelectors covers the Fleet 4.90 report label
// scoping on create: the selector in use goes out as a name array, and
// omitempty keeps the unused one off the wire (Fleet rejects a request whose
// labels_include_any and labels_include_all are both non-empty).
func TestClient_CreateQuery_LabelSelectors(t *testing.T) {
	var rawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rawBody = string(body)

		var req CreateQueryRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		labels := make([]ReportLabel, 0, len(req.LabelsIncludeAll))
		for i, n := range req.LabelsIncludeAll {
			labels = append(labels, ReportLabel{ID: i + 1, Name: n})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CreateQueryResponse{
			Report: Query{ID: 9, Name: req.Name, Report: req.Query, LabelsIncludeAll: labels},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	query, err := client.CreateQuery(context.Background(), CreateQueryRequest{
		Name:             "Scoped report",
		Query:            "SELECT 1;",
		LabelsIncludeAll: []string{"Macs on Sonoma", "Engineering"},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !strings.Contains(rawBody, `"labels_include_all":["Macs on Sonoma","Engineering"]`) {
		t.Errorf("expected labels_include_all on the wire, body was: %s", rawBody)
	}
	if strings.Contains(rawBody, "labels_include_any") {
		t.Errorf("expected labels_include_any to be omitted, body was: %s", rawBody)
	}
	if len(query.LabelsIncludeAll) != 2 || query.LabelsIncludeAll[0].Name != "Macs on Sonoma" {
		t.Errorf("expected labels_include_all echo, got: %+v", query.LabelsIncludeAll)
	}
}

// TestClient_UpdateQuery_LabelClearVsNoChange asserts the report PATCH
// endpoint's nil-vs-empty label convention, which matches the policy
// endpoints: null preserves Fleet's existing labels, [] clears them.
func TestClient_UpdateQuery_LabelClearVsNoChange(t *testing.T) {
	captured := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured <- string(body)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(UpdateQueryResponse{Report: Query{ID: 3, Name: "ok"}})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})

	if _, err := client.UpdateQuery(context.Background(), 3, UpdateQueryRequest{Name: "ok"}); err != nil {
		t.Fatalf("nil-labels update failed: %v", err)
	}
	body := <-captured
	for _, want := range []string{`"labels_include_any":null`, `"labels_include_all":null`} {
		if !strings.Contains(body, want) {
			t.Errorf("expected request body to contain %q, body was: %s", want, body)
		}
	}

	if _, err := client.UpdateQuery(context.Background(), 3, UpdateQueryRequest{
		Name:             "ok",
		LabelsIncludeAny: []string{"Macs on Sonoma"},
		LabelsIncludeAll: []string{},
	}); err != nil {
		t.Fatalf("selector-swap update failed: %v", err)
	}
	body = <-captured
	for _, want := range []string{`"labels_include_any":["Macs on Sonoma"]`, `"labels_include_all":[]`} {
		if !strings.Contains(body, want) {
			t.Errorf("expected request body to contain %q, body was: %s", want, body)
		}
	}
}

// TestClient_GetQuery_LabelSelectors decodes a report response carrying both
// label arrays, mirroring the shape Fleet 4.90 returns.
func TestClient_GetQuery_LabelSelectors(t *testing.T) {
	body := `{
	  "report": {
	    "id": 93,
	    "name": "Scoped report",
	    "report": "SELECT 1;",
	    "labels_include_any": [{"id": 51, "name": "probe-a"}],
	    "labels_include_all": null
	  }
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key"})
	query, err := client.GetQuery(context.Background(), 93)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(query.LabelsIncludeAny) != 1 || query.LabelsIncludeAny[0].Name != "probe-a" {
		t.Errorf("expected labels_include_any [{id:51,name:\"probe-a\"}], got: %+v", query.LabelsIncludeAny)
	}
	if query.LabelsIncludeAll != nil {
		t.Errorf("expected nil labels_include_all, got: %+v", query.LabelsIncludeAll)
	}
}

func TestClient_UpdateQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH request, got: %s", r.Method)
		}

		if r.URL.Path != "/api/v1/fleet/reports/3" {
			t.Errorf("expected path /api/v1/fleet/reports/3, got: %s", r.URL.Path)
		}

		var req UpdateQueryRequest
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(UpdateQueryResponse{
			Report: Query{
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

		if r.URL.Path != "/api/v1/fleet/reports/id/3" {
			t.Errorf("expected path /api/v1/fleet/reports/id/3, got: %s", r.URL.Path)
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
		if r.URL.Path != "/api/v1/fleet/reports" {
			t.Errorf("expected path /api/v1/fleet/reports, got: %s", r.URL.Path)
		}
		if r.URL.Query().Get("fleet_id") != "5" {
			t.Errorf("expected fleet_id=5, got: %s", r.URL.Query().Get("fleet_id"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListQueriesResponse{
			Reports: []Query{
				{ID: 10, Name: "Team Query", Report: "SELECT 1"},
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
