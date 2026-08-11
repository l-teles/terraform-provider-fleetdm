package fleetdm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newCustomVariableTestClient builds a client pointed at a mock server.
func newCustomVariableTestClient(t *testing.T, serverURL string) *Client {
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

func TestClient_CreateCustomVariable(t *testing.T) {
	var gotBody CreateCustomVariableRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/custom_variables" {
			t.Errorf("expected path /api/v1/fleet/custom_variables, got: %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("expected Content-Type application/json, got: %s", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CreateCustomVariableResponse{ID: 7, Name: "MY_VAR"})
	}))
	defer server.Close()

	client := newCustomVariableTestClient(t, server.URL)

	cv, err := client.CreateCustomVariable(context.Background(), "MY_VAR", "tf-unit-fake-value")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Request shape: exactly {name, value}.
	if gotBody.Name != "MY_VAR" {
		t.Errorf("expected request name MY_VAR, got: %s", gotBody.Name)
	}
	if gotBody.Value != "tf-unit-fake-value" {
		t.Errorf("expected request value to be forwarded verbatim, got: %s", gotBody.Value)
	}

	if cv.ID != 7 {
		t.Errorf("expected id 7, got: %d", cv.ID)
	}
	if cv.Name != "MY_VAR" {
		t.Errorf("expected name MY_VAR, got: %s", cv.Name)
	}
}

func TestClient_CreateCustomVariable_Conflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"message":"Resource Already Exists","errors":[{"name":"base","reason":"name \"MY_VAR\" already exists"}]}`)
	}))
	defer server.Close()

	client := newCustomVariableTestClient(t, server.URL)

	_, err := client.CreateCustomVariable(context.Background(), "MY_VAR", "tf-unit-fake-value")
	if err == nil {
		t.Fatal("expected an error for a duplicate name, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected the error to wrap *APIError, got: %T", err)
	}
	if apiErr.StatusCode != http.StatusConflict {
		t.Errorf("expected status 409, got: %d", apiErr.StatusCode)
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected the API reason to be preserved, got: %v", err)
	}
}

func TestClient_CreateCustomVariable_ValidationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"message":"Validation Failed","errors":[{"name":"name","reason":"secret variable with invalid format: bad-name"}]}`)
	}))
	defer server.Close()

	client := newCustomVariableTestClient(t, server.URL)

	if _, err := client.CreateCustomVariable(context.Background(), "bad-name", "tf-unit-fake-value"); err == nil {
		t.Fatal("expected an error for an invalid name, got nil")
	} else if !strings.Contains(err.Error(), "invalid format") {
		t.Errorf("expected the validation reason to be preserved, got: %v", err)
	}
}

func TestClient_ListCustomVariables(t *testing.T) {
	var gotQueries []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/custom_variables" {
			t.Errorf("expected path /api/v1/fleet/custom_variables, got: %s", r.URL.Path)
		}
		gotQueries = append(gotQueries, r.URL.RawQuery)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListCustomVariablesResponse{
			CustomVariables: []CustomVariable{
				{ID: 1, Name: "ALPHA", CreatedAt: "2026-08-11T09:00:00Z", UpdatedAt: "2026-08-11T09:00:00Z"},
				{ID: 2, Name: "BETA", CreatedAt: "2026-08-11T09:01:00Z", UpdatedAt: "2026-08-11T09:02:00Z"},
			},
			Meta:  &PaginationMeta{HasNextResults: false},
			Count: 2,
		})
	}))
	defer server.Close()

	client := newCustomVariableTestClient(t, server.URL)

	vars, err := client.ListCustomVariables(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(vars) != 2 {
		t.Fatalf("expected 2 custom variables, got: %d", len(vars))
	}
	if vars[0].Name != "ALPHA" || vars[1].Name != "BETA" {
		t.Errorf("unexpected names: %q, %q", vars[0].Name, vars[1].Name)
	}
	if vars[1].UpdatedAt != "2026-08-11T09:02:00Z" {
		t.Errorf("expected updated_at to be decoded, got: %s", vars[1].UpdatedAt)
	}

	// A single request with explicit pagination parameters.
	if len(gotQueries) != 1 {
		t.Fatalf("expected exactly 1 request, got: %d", len(gotQueries))
	}
	for _, want := range []string{"page=0", "per_page=500"} {
		if !strings.Contains(gotQueries[0], want) {
			t.Errorf("expected query to contain %q, got: %s", want, gotQueries[0])
		}
	}
}

// TestClient_ListCustomVariables_Paginates asserts the client walks every page
// rather than silently truncating at the first one. Truncation would make the
// resource's list-and-match read wrongly conclude a variable had been deleted.
func TestClient_ListCustomVariables_Paginates(t *testing.T) {
	var pages []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pages = append(pages, page)

		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "0":
			json.NewEncoder(w).Encode(ListCustomVariablesResponse{
				CustomVariables: []CustomVariable{{ID: 1, Name: "ALPHA"}},
				Meta:            &PaginationMeta{HasNextResults: true},
				Count:           2,
			})
		case "1":
			json.NewEncoder(w).Encode(ListCustomVariablesResponse{
				CustomVariables: []CustomVariable{{ID: 2, Name: "BETA"}},
				Meta:            &PaginationMeta{HasNextResults: false, HasPreviousResults: true},
				Count:           2,
			})
		default:
			t.Errorf("unexpected extra page request: %s", page)
			json.NewEncoder(w).Encode(ListCustomVariablesResponse{})
		}
	}))
	defer server.Close()

	client := newCustomVariableTestClient(t, server.URL)

	vars, err := client.ListCustomVariables(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(vars) != 2 {
		t.Fatalf("expected 2 custom variables across 2 pages, got: %d", len(vars))
	}
	if len(pages) != 2 || pages[0] != "0" || pages[1] != "1" {
		t.Errorf("expected pages 0 then 1, got: %v", pages)
	}
}

// TestClient_ListCustomVariables_StopsOnEmptyPage guards the pagination loop
// against a server that keeps reporting "has next results".
func TestClient_ListCustomVariables_StopsOnEmptyPage(t *testing.T) {
	requests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests > 10 {
			t.Fatal("pagination loop did not stop on an empty page")
		}
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			json.NewEncoder(w).Encode(ListCustomVariablesResponse{
				CustomVariables: []CustomVariable{{ID: 1, Name: "ALPHA"}},
				Meta:            &PaginationMeta{HasNextResults: true},
			})
			return
		}
		json.NewEncoder(w).Encode(ListCustomVariablesResponse{
			CustomVariables: nil,
			Meta:            &PaginationMeta{HasNextResults: true},
		})
	}))
	defer server.Close()

	client := newCustomVariableTestClient(t, server.URL)

	vars, err := client.ListCustomVariables(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(vars) != 1 {
		t.Errorf("expected 1 custom variable, got: %d", len(vars))
	}
	if requests != 2 {
		t.Errorf("expected 2 requests (one page + one empty page), got: %d", requests)
	}
}

// TestClient_ListCustomVariables_NullList covers Fleet's real empty response,
// which sends `"custom_variables": null` rather than an empty array.
func TestClient_ListCustomVariables_NullList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"custom_variables":null,"meta":{"has_next_results":false,"has_previous_results":false},"count":0}`)
	}))
	defer server.Close()

	client := newCustomVariableTestClient(t, server.URL)

	vars, err := client.ListCustomVariables(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(vars) != 0 {
		t.Errorf("expected no custom variables, got: %d", len(vars))
	}
}

func TestClient_ListCustomVariables_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"forbidden"}`)
	}))
	defer server.Close()

	client := newCustomVariableTestClient(t, server.URL)

	if _, err := client.ListCustomVariables(context.Background()); err == nil {
		t.Fatal("expected an error, got nil")
	} else if !strings.Contains(err.Error(), "failed to list custom variables") {
		t.Errorf("expected a wrapped list error, got: %v", err)
	}
}

func TestClient_FindCustomVariableByName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListCustomVariablesResponse{
			CustomVariables: []CustomVariable{
				{ID: 1, Name: "ALPHA"},
				{ID: 2, Name: "BETA", CreatedAt: "2026-08-11T09:00:00Z"},
			},
			Meta: &PaginationMeta{HasNextResults: false},
		})
	}))
	defer server.Close()

	client := newCustomVariableTestClient(t, server.URL)

	found, err := client.FindCustomVariableByName(context.Background(), "BETA")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find BETA, got nil")
	}
	if found.ID != 2 {
		t.Errorf("expected id 2, got: %d", found.ID)
	}
	if found.CreatedAt != "2026-08-11T09:00:00Z" {
		t.Errorf("expected created_at to be returned, got: %s", found.CreatedAt)
	}

	// Name matching must be exact and case-sensitive.
	missing, err := client.FindCustomVariableByName(context.Background(), "beta")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if missing != nil {
		t.Errorf("expected no match for a different case, got: %+v", missing)
	}

	missing, err = client.FindCustomVariableByName(context.Background(), "GAMMA")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if missing != nil {
		t.Errorf("expected nil for an absent name, got: %+v", missing)
	}
}

func TestClient_UpsertCustomVariable(t *testing.T) {
	var gotBody UpsertCustomVariablesRequest
	var gotPath, gotMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	client := newCustomVariableTestClient(t, server.URL)

	if err := client.UpsertCustomVariable(context.Background(), "MY_VAR", "tf-unit-fake-rotated"); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("expected PUT request, got: %s", gotMethod)
	}
	if gotPath != "/api/v1/fleet/spec/secret_variables" {
		t.Errorf("expected path /api/v1/fleet/spec/secret_variables, got: %s", gotPath)
	}
	// Exactly one entry: the spec endpoint upserts only the listed names, so
	// sending more than the managed variable would touch resources Terraform
	// does not own.
	if len(gotBody.Secrets) != 1 {
		t.Fatalf("expected exactly 1 entry in the upsert body, got: %d", len(gotBody.Secrets))
	}
	if gotBody.Secrets[0].Name != "MY_VAR" {
		t.Errorf("expected name MY_VAR, got: %s", gotBody.Secrets[0].Name)
	}
	if gotBody.Secrets[0].Value != "tf-unit-fake-rotated" {
		t.Errorf("expected the new value to be forwarded, got: %s", gotBody.Secrets[0].Value)
	}
}

func TestClient_UpsertCustomVariable_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"message":"Couldn't save secret variables. Missing required private key."}`)
	}))
	defer server.Close()

	client := newCustomVariableTestClient(t, server.URL)

	err := client.UpsertCustomVariable(context.Background(), "MY_VAR", "tf-unit-fake-rotated")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to update custom variable") {
		t.Errorf("expected a wrapped update error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Missing required private key") {
		t.Errorf("expected the server message to be preserved, got: %v", err)
	}
}

func TestClient_DeleteCustomVariable(t *testing.T) {
	var gotPath, gotMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	client := newCustomVariableTestClient(t, server.URL)

	if err := client.DeleteCustomVariable(context.Background(), 42); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("expected DELETE request, got: %s", gotMethod)
	}
	if gotPath != "/api/v1/fleet/custom_variables/42" {
		t.Errorf("expected path /api/v1/fleet/custom_variables/42, got: %s", gotPath)
	}
}

// TestClient_DeleteCustomVariable_InUse pins the 409 Fleet returns when the
// variable is still referenced. The resource surfaces this as an actionable
// error, so the status code and reason must survive wrapping.
func TestClient_DeleteCustomVariable_InUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"message":"Conflict","errors":[{"name":"base","reason":"Couldn't delete. MY_VAR is used by the \"deploy.sh\" script in the \"No team\" team. Please edit or delete the script and try again."}]}`)
	}))
	defer server.Close()

	client := newCustomVariableTestClient(t, server.URL)

	err := client.DeleteCustomVariable(context.Background(), 42)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected the error to wrap *APIError, got: %T", err)
	}
	if apiErr.StatusCode != http.StatusConflict {
		t.Errorf("expected status 409, got: %d", apiErr.StatusCode)
	}
	if !strings.Contains(err.Error(), "is used by") {
		t.Errorf("expected the referencing entity to be named, got: %v", err)
	}
}

func TestClient_DeleteCustomVariable_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Resource Not Found","errors":[{"name":"base","reason":"SecretVariable 42 was not found in the datastore"}]}`)
	}))
	defer server.Close()

	client := newCustomVariableTestClient(t, server.URL)

	err := client.DeleteCustomVariable(context.Background(), 42)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected the error to wrap *APIError, got: %T", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got: %d", apiErr.StatusCode)
	}
}
