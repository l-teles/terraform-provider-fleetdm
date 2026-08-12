package fleetdm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// newTestClient builds a client pointed at a mock server.
func newCustomHostVitalTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	client, err := NewClient(ClientConfig{
		ServerAddress: serverURL,
		APIKey:        "test-api-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return client
}

func TestClient_ListCustomHostVitals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/custom_host_vitals" {
			t.Errorf("expected path /api/v1/fleet/custom_host_vitals, got: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("expected per_page=100, got: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListCustomHostVitalsResponse{
			CustomHostVitals: []CustomHostVital{
				{ID: 1, Name: "asset_tag", CreatedAt: "2026-08-11T18:17:24Z", UpdatedAt: "2026-08-11T18:17:24Z"},
				{ID: 2, Name: "cost_centre", CreatedAt: "2026-08-11T18:18:16Z", UpdatedAt: "2026-08-11T18:18:16Z"},
			},
			Meta:  &PaginationMeta{HasNextResults: false},
			Count: 2,
		})
	}))
	defer server.Close()

	vitals, err := newCustomHostVitalTestClient(t, server.URL).ListCustomHostVitals(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(vitals) != 2 {
		t.Fatalf("expected 2 custom host vitals, got: %d", len(vitals))
	}
	if vitals[0].Name != "asset_tag" {
		t.Errorf("expected first vital name 'asset_tag', got: %s", vitals[0].Name)
	}
	if vitals[1].ID != 2 {
		t.Errorf("expected second vital id 2, got: %d", vitals[1].ID)
	}
}

// TestClient_ListCustomHostVitals_Paginates asserts every page is walked. A
// single-page read would silently truncate the list, which would in turn make
// GetCustomHostVital report a 404 for a vital that exists.
func TestClient_ListCustomHostVitals_Paginates(t *testing.T) {
	var pagesSeen []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pagesSeen = append(pagesSeen, page)

		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "0":
			_ = json.NewEncoder(w).Encode(ListCustomHostVitalsResponse{
				CustomHostVitals: []CustomHostVital{{ID: 1, Name: "a"}},
				Meta:             &PaginationMeta{HasNextResults: true},
				Count:            3,
			})
		case "1":
			_ = json.NewEncoder(w).Encode(ListCustomHostVitalsResponse{
				CustomHostVitals: []CustomHostVital{{ID: 2, Name: "b"}, {ID: 3, Name: "c"}},
				Meta:             &PaginationMeta{HasNextResults: false},
				Count:            3,
			})
		default:
			t.Errorf("unexpected page requested: %q", page)
		}
	}))
	defer server.Close()

	vitals, err := newCustomHostVitalTestClient(t, server.URL).ListCustomHostVitals(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(vitals) != 3 {
		t.Fatalf("expected 3 custom host vitals across pages, got: %d", len(vitals))
	}
	if len(pagesSeen) != 2 || pagesSeen[0] != "0" || pagesSeen[1] != "1" {
		t.Errorf("expected pages [0 1] to be requested, got: %v", pagesSeen)
	}
}

// TestClient_ListCustomHostVitals_PaginationGuard makes sure a server that
// always claims another page is available terminates with an error instead of
// looping forever.
func TestClient_ListCustomHostVitals_PaginationGuard(t *testing.T) {
	var requests int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListCustomHostVitalsResponse{
			CustomHostVitals: []CustomHostVital{{ID: requests, Name: "v" + strconv.Itoa(requests)}},
			Meta:             &PaginationMeta{HasNextResults: true},
		})
	}))
	defer server.Close()

	_, err := newCustomHostVitalTestClient(t, server.URL).ListCustomHostVitals(context.Background())
	if err == nil {
		t.Fatal("expected an error when has_next_results never turns false")
	}
	if requests != maxCustomHostVitalListPages {
		t.Errorf("expected exactly %d requests before giving up, got: %d", maxCustomHostVitalListPages, requests)
	}
}

func TestClient_GetCustomHostVital(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fleet has no GET /custom_host_vitals/{id} (it answers 405), so the
		// client must resolve a single vital through the list endpoint.
		if r.URL.Path != "/api/v1/fleet/custom_host_vitals" {
			t.Errorf("expected the list endpoint, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListCustomHostVitalsResponse{
			CustomHostVitals: []CustomHostVital{
				{ID: 1, Name: "asset_tag", CreatedAt: "2026-08-11T18:17:24Z"},
				{ID: 7, Name: "room", CreatedAt: "2026-08-11T18:19:24Z"},
			},
			Meta: &PaginationMeta{HasNextResults: false},
		})
	}))
	defer server.Close()

	vital, err := newCustomHostVitalTestClient(t, server.URL).GetCustomHostVital(context.Background(), 7)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if vital.ID != 7 {
		t.Errorf("expected id 7, got: %d", vital.ID)
	}
	if vital.Name != "room" {
		t.Errorf("expected name 'room', got: %s", vital.Name)
	}
}

// TestClient_GetCustomHostVital_NotFound pins the synthetic 404 that lets
// resource Read treat an out-of-band deletion as "gone" and drop it from state.
func TestClient_GetCustomHostVital_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListCustomHostVitalsResponse{
			CustomHostVitals: []CustomHostVital{{ID: 1, Name: "asset_tag"}},
			Meta:             &PaginationMeta{HasNextResults: false},
		})
	}))
	defer server.Close()

	_, err := newCustomHostVitalTestClient(t, server.URL).GetCustomHostVital(context.Background(), 42)
	if err == nil {
		t.Fatal("expected an error for a missing custom host vital")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status 404, got: %d", apiErr.StatusCode)
	}
}

// TestClient_GetCustomHostVital_RouteNotFound covers a 404 from the collection
// endpoint itself — a pre-4.90 Fleet or a proxy misroute.
//
// This must NOT unwrap to a 404: the provider removes a resource from state on
// a not-found, and conflating the two would make a single refresh against an
// unsupported server drop every custom host vital and propose recreating them.
func TestClient_GetCustomHostVital_RouteNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource Not Found","errors":[{"name":"base","reason":"Unknown endpoint"}]}`))
	}))
	defer server.Close()

	_, err := newCustomHostVitalTestClient(t, server.URL).GetCustomHostVital(context.Background(), 1)
	if err == nil {
		t.Fatal("expected an error when the collection endpoint 404s")
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		t.Fatal("a route-level 404 must not unwrap to a 404 *APIError, or callers will treat the vital as deleted")
	}
	if !strings.Contains(err.Error(), "4.90") {
		t.Errorf("expected the error to point at the Fleet version requirement, got: %s", err.Error())
	}
}

func TestClient_CreateCustomHostVital(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/custom_host_vitals" {
			t.Errorf("expected path /api/v1/fleet/custom_host_vitals, got: %s", r.URL.Path)
		}

		// Pin the request body: Fleet's create endpoint takes `name` and
		// nothing else, and its decoder discards unknown fields silently. Any
		// extra key we send would be a fabricated field that looks accepted.
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if len(body) != 1 {
			t.Errorf("expected exactly one field in the request body, got: %v", body)
		}
		if body["name"] != "asset_tag" {
			t.Errorf("expected name 'asset_tag', got: %v", body["name"])
		}

		w.Header().Set("Content-Type", "application/json")
		// Fleet really does answer with empty timestamps here.
		_ = json.NewEncoder(w).Encode(CustomHostVitalResponse{
			CustomHostVital: CustomHostVital{ID: 3, Name: "asset_tag"},
		})
	}))
	defer server.Close()

	vital, err := newCustomHostVitalTestClient(t, server.URL).CreateCustomHostVital(
		context.Background(),
		CreateCustomHostVitalRequest{Name: "asset_tag"},
	)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if vital.ID != 3 {
		t.Errorf("expected id 3, got: %d", vital.ID)
	}
	if vital.CreatedAt != "" {
		t.Errorf("expected an empty created_at from the create endpoint, got: %q", vital.CreatedAt)
	}
}

func TestClient_CreateCustomHostVital_DuplicateName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"Resource Already Exists","errors":[{"name":"base","reason":"name \"asset_tag\" already exists"}]}`))
	}))
	defer server.Close()

	_, err := newCustomHostVitalTestClient(t, server.URL).CreateCustomHostVital(
		context.Background(),
		CreateCustomHostVitalRequest{Name: "asset_tag"},
	)
	if err == nil {
		t.Fatal("expected an error for a duplicate name")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected a wrapped *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusConflict {
		t.Errorf("expected status 409, got: %d", apiErr.StatusCode)
	}
}

func TestClient_UpdateCustomHostVital(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH request, got: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/custom_host_vitals/5" {
			t.Errorf("expected path /api/v1/fleet/custom_host_vitals/5, got: %s", r.URL.Path)
		}

		// `name` is mandatory on PATCH — Fleet 422s a body without it rather
		// than treating the field as optional, so it must always be sent.
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if len(body) != 1 {
			t.Errorf("expected exactly one field in the request body, got: %v", body)
		}
		if body["name"] != "asset_tag_v2" {
			t.Errorf("expected name 'asset_tag_v2', got: %v", body["name"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CustomHostVitalResponse{
			CustomHostVital: CustomHostVital{ID: 5, Name: "asset_tag_v2"},
		})
	}))
	defer server.Close()

	vital, err := newCustomHostVitalTestClient(t, server.URL).UpdateCustomHostVital(
		context.Background(), 5,
		UpdateCustomHostVitalRequest{Name: "asset_tag_v2"},
	)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if vital.Name != "asset_tag_v2" {
		t.Errorf("expected name 'asset_tag_v2', got: %s", vital.Name)
	}
}

func TestClient_DeleteCustomHostVital(t *testing.T) {
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE request, got: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/custom_host_vitals/9" {
			t.Errorf("expected path /api/v1/fleet/custom_host_vitals/9, got: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	if err := newCustomHostVitalTestClient(t, server.URL).DeleteCustomHostVital(context.Background(), 9); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !called {
		t.Error("expected the delete endpoint to be called")
	}
}

// TestClient_DeleteCustomHostVital_InUse covers Fleet's 409 for a vital still
// referenced by a profile, script, installer or label. The reason text is what
// tells the user which reference to remove, so it must survive wrapping.
func TestClient_DeleteCustomHostVital_InUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"Conflict","errors":[{"name":"base","reason":"Couldn't delete. Custom host vital \"asset_tag\" (used as $FLEET_HOST_VITAL_1) is used by the \"tagged\" label in the \"Unassigned\" fleet. Please edit or delete the label and try again."}]}`))
	}))
	defer server.Close()

	err := newCustomHostVitalTestClient(t, server.URL).DeleteCustomHostVital(context.Background(), 1)
	if err == nil {
		t.Fatal("expected an error when the vital is still referenced")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected a wrapped *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusConflict {
		t.Errorf("expected status 409, got: %d", apiErr.StatusCode)
	}
	if want := "is used by the"; !strings.Contains(err.Error(), want) {
		t.Errorf("expected the error to keep Fleet's reason text containing %q, got: %s", want, err.Error())
	}
}

func TestClient_CustomHostVitalErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"Validation Failed","errors":[{"name":"name","reason":"custom host vital name cannot be empty"}]}`))
	}))
	defer server.Close()

	client := newCustomHostVitalTestClient(t, server.URL)

	if _, err := client.CreateCustomHostVital(context.Background(), CreateCustomHostVitalRequest{}); err == nil {
		t.Error("expected an error from create with an empty name")
	}
	if _, err := client.UpdateCustomHostVital(context.Background(), 1, UpdateCustomHostVitalRequest{}); err == nil {
		t.Error("expected an error from update with an empty name")
	}
	if _, err := client.ListCustomHostVitals(context.Background()); err == nil {
		t.Error("expected an error from list")
	}
}

func TestClient_CustomHostVitalEndpointPaths(t *testing.T) {
	// Guards against a typo'd path silently hitting a different endpoint.
	for _, tc := range []struct {
		name string
		id   int
		want string
	}{
		{name: "single digit", id: 1, want: "/api/v1/fleet/custom_host_vitals/1"},
		{name: "multi digit", id: 12345, want: "/api/v1/fleet/custom_host_vitals/12345"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			if err := newCustomHostVitalTestClient(t, server.URL).DeleteCustomHostVital(context.Background(), tc.id); err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if gotPath != tc.want {
				t.Errorf("expected path %s, got: %s", tc.want, gotPath)
			}
		})
	}
}

// TestHostVitalCriteriaMarshalling pins the label criteria wire format: the
// optional fields must drop out when unset, because Fleet rejects a
// custom_host_vital_id on a non-custom vital.
func TestHostVitalCriteriaMarshalling(t *testing.T) {
	id := 4

	for _, tc := range []struct {
		name     string
		criteria HostVitalCriteria
		want     string
	}{
		{
			name:     "idp group without operator",
			criteria: HostVitalCriteria{Vital: HostVitalEndUserIDPGroup, Value: "Engineering"},
			want:     `{"vital":"end_user_idp_group","value":"Engineering"}`,
		},
		{
			name:     "custom host vital with operator",
			criteria: HostVitalCriteria{Vital: HostVitalCustomHostVital, Value: "1234", Operator: HostVitalOperatorLike, CustomHostVitalID: &id},
			want:     `{"vital":"custom_host_vital","value":"1234","operator":"LIKE","custom_host_vital_id":4}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.criteria)
			if err != nil {
				t.Fatalf("failed to marshal criteria: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("expected %s, got: %s", tc.want, got)
			}
		})
	}
}

// TestClient_UpdateLabel_CriteriaEcho pins the modify-label response decode for
// a host-vitals label.
//
// Renaming such a label goes through PATCH, whose response carries the
// (immutable) criteria back. If this decode broke, the provider would map an
// empty criteria over good state and abort a plain rename with an
// inconsistent-result error.
func TestClient_UpdateLabel_CriteriaEcho(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH request, got: %s", r.Method)
		}

		// Fleet ignores criteria on modify, so the provider must not send it.
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if _, ok := body["criteria"]; ok {
			t.Errorf("criteria must not be sent on modify (Fleet ignores it), got: %v", body)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"label":{"id":7,"name":"renamed","description":"d","query":"",` +
			`"criteria":{"vital":"custom_host_vital","value":"1234","operator":"=","custom_host_vital_id":3},` +
			`"platform":"","label_type":"regular","label_membership_type":"host_vitals"}}`))
	}))
	defer server.Close()

	label, err := newCustomHostVitalTestClient(t, server.URL).UpdateLabel(
		context.Background(), 7,
		UpdateLabelRequest{Name: "renamed", Description: "d"},
	)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if label.Criteria == nil {
		t.Fatal("expected criteria to be decoded from the modify response")
	}
	if label.Criteria.Vital != HostVitalCustomHostVital {
		t.Errorf("expected vital %q, got: %q", HostVitalCustomHostVital, label.Criteria.Vital)
	}
	if label.Criteria.Value != "1234" {
		t.Errorf("expected value '1234', got: %q", label.Criteria.Value)
	}
	if label.Criteria.CustomHostVitalID == nil || *label.Criteria.CustomHostVitalID != 3 {
		t.Errorf("expected custom_host_vital_id 3, got: %v", label.Criteria.CustomHostVitalID)
	}
	if label.LabelMembershipType != "host_vitals" {
		t.Errorf("expected membership type 'host_vitals', got: %q", label.LabelMembershipType)
	}
}

// TestCreateLabelRequestMarshalling asserts a criteria label sends no criteria
// key when unset, and that an empty query is still sent (Fleet only counts a
// non-empty query toward its "only one of query/criteria/hosts" rule).
func TestCreateLabelRequestMarshalling(t *testing.T) {
	withCriteria, err := json.Marshal(CreateLabelRequest{
		Name:     "tagged",
		Criteria: &HostVitalCriteria{Vital: HostVitalEndUserIDPDepartment, Value: "Security"},
	})
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}
	want := `{"name":"tagged","query":"","criteria":{"vital":"end_user_idp_department","value":"Security"}}`
	if string(withCriteria) != want {
		t.Errorf("expected %s, got: %s", want, withCriteria)
	}

	dynamic, err := json.Marshal(CreateLabelRequest{Name: "macs", Query: "SELECT 1"})
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}
	if strings.Contains(string(dynamic), "criteria") {
		t.Errorf("expected no criteria key for a dynamic label, got: %s", dynamic)
	}
}
