package fleetdm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestCertificateTemplateClient returns a client pointed at the given test server.
func newTestCertificateTemplateClient(t *testing.T, serverURL string) *Client {
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

// TestCreateCertificateTemplateRequestBody pins the exact JSON the client sends.
// Fleet silently discards unknown request fields, so a typo in a key would look
// like a successful create while the value was thrown away.
func TestCreateCertificateTemplateRequestBody(t *testing.T) {
	var body map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/certificates" {
			t.Errorf("Expected path /api/v1/fleet/certificates, got %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                       12,
			"name":                     "Android_WiFi",
			"certificate_authority_id": 3,
			"subject_name":             "CN=$FLEET_VAR_HOST_HARDWARE_SERIAL",
			"subject_alternative_name": "DNS=host.example.com",
		})
	}))
	defer server.Close()

	template, err := newTestCertificateTemplateClient(t, server.URL).CreateCertificateTemplate(
		context.Background(),
		CreateCertificateTemplateRequest{
			Name:                   "Android_WiFi",
			FleetID:                5,
			CertificateAuthorityID: 3,
			SubjectName:            "CN=$FLEET_VAR_HOST_HARDWARE_SERIAL",
			SubjectAlternativeName: "DNS=host.example.com",
		},
	)
	if err != nil {
		t.Fatalf("CreateCertificateTemplate failed: %v", err)
	}

	want := map[string]any{
		"name":                     "Android_WiFi",
		"fleet_id":                 float64(5),
		"certificate_authority_id": float64(3),
		"subject_name":             "CN=$FLEET_VAR_HOST_HARDWARE_SERIAL",
		"subject_alternative_name": "DNS=host.example.com",
	}
	if len(body) != len(want) {
		t.Errorf("Expected exactly %d request keys, got %d: %v", len(want), len(body), body)
	}
	for key, expected := range want {
		if got, ok := body[key]; !ok {
			t.Errorf("Request body is missing key %q: %v", key, body)
		} else if got != expected {
			t.Errorf("Request body key %q = %v, want %v", key, got, expected)
		}
	}

	if template.ID != 12 {
		t.Errorf("Expected ID 12, got %d", template.ID)
	}
	if template.SubjectAlternativeName != "DNS=host.example.com" {
		t.Errorf("Unexpected subject alternative name %q", template.SubjectAlternativeName)
	}
}

// TestCreateCertificateTemplateSendsZeroFleetID checks the no-fleet case still
// puts fleet_id on the wire rather than relying on Fleet's server-side default.
func TestCreateCertificateTemplateSendsZeroFleetID(t *testing.T) {
	var raw string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read request body: %v", err)
		}
		raw = string(payload)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": "NoFleet", "subject_name": "CN=x"})
	}))
	defer server.Close()

	_, err := newTestCertificateTemplateClient(t, server.URL).CreateCertificateTemplate(
		context.Background(),
		CreateCertificateTemplateRequest{Name: "NoFleet", FleetID: 0, CertificateAuthorityID: 2, SubjectName: "CN=x"},
	)
	if err != nil {
		t.Fatalf("CreateCertificateTemplate failed: %v", err)
	}

	if !strings.Contains(raw, `"fleet_id":0`) {
		t.Errorf("Expected fleet_id:0 in the request body, got %s", raw)
	}
	// An absent SAN must be omitted rather than sent as an empty string.
	if strings.Contains(raw, "subject_alternative_name") {
		t.Errorf("Expected subject_alternative_name to be omitted, got %s", raw)
	}
}

// TestCreateCertificateTemplateCAErrors covers the two save-time certificate
// authority checks Fleet performs.
func TestCreateCertificateTemplateCAErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		reason     string
	}{
		{
			name:       "wrong CA type",
			statusCode: http.StatusBadRequest,
			reason:     "Currently, only the custom_scep_proxy certificate authority is supported.",
		},
		{
			name:       "CA does not exist",
			statusCode: http.StatusNotFound,
			reason:     "CertificateAuthority 999 was not found in the datastore",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"message": "error",
					"errors":  []map[string]string{{"name": "base", "reason": tt.reason}},
				})
			}))
			defer server.Close()

			_, err := newTestCertificateTemplateClient(t, server.URL).CreateCertificateTemplate(
				context.Background(),
				CreateCertificateTemplateRequest{Name: "X", CertificateAuthorityID: 999, SubjectName: "CN=x"},
			)
			if err == nil {
				t.Fatal("Expected an error, got nil")
			}
			if !strings.Contains(err.Error(), "failed to create certificate template") {
				t.Errorf("Expected the client's wrapping context, got %v", err)
			}

			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("Expected the error to unwrap to *APIError, got %T", err)
			}
			if apiErr.StatusCode != tt.statusCode {
				t.Errorf("Expected status %d, got %d", tt.statusCode, apiErr.StatusCode)
			}
		})
	}
}

// TestCreateCertificateTemplateDuplicateName covers the 409 Fleet returns when
// the name is already taken within the fleet.
func TestCreateCertificateTemplateDuplicateName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Resource Already Exists",
			"errors":  []map[string]string{{"name": "base", "reason": `CertificateTemplate "Dup" already exists`}},
		})
	}))
	defer server.Close()

	_, err := newTestCertificateTemplateClient(t, server.URL).CreateCertificateTemplate(
		context.Background(),
		CreateCertificateTemplateRequest{Name: "Dup", CertificateAuthorityID: 1, SubjectName: "CN=x"},
	)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		t.Errorf("Expected a 409 APIError, got %v", err)
	}
}

// TestCreateCertificateTemplateDegenerateResponse checks a 200 whose body
// carries no template is rejected rather than decoded to a zero-value
// template. Accepting it would write id 0 into state, and the next read
// would 404 on /certificates/0 and drop the resource — orphaning the
// template Fleet actually stored.
func TestCreateCertificateTemplateDegenerateResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	_, err := newTestCertificateTemplateClient(t, server.URL).CreateCertificateTemplate(
		context.Background(),
		CreateCertificateTemplateRequest{Name: "Degenerate", CertificateAuthorityID: 1, SubjectName: "CN=x"},
	)
	if err == nil || !strings.Contains(err.Error(), "response contained no certificate template") {
		t.Fatalf("Expected a no-certificate-template error, got %v", err)
	}
}

// TestGetCertificateTemplate checks the wrapped response shape and that every
// read-only field lands on the struct.
func TestGetCertificateTemplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/certificates/12" {
			t.Errorf("Expected path /api/v1/fleet/certificates/12, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"certificate": map[string]any{
				"id":                         12,
				"name":                       "Android_WiFi",
				"subject_name":               "CN=probe",
				"subject_alternative_name":   "DNS=host.example.com",
				"certificate_authority_id":   3,
				"certificate_authority_name": "SCEP_TEST",
				"certificate_authority_type": CATypeCustomSCEPProxy,
				"created_at":                 "2026-08-12T09:10:34Z",
			},
		})
	}))
	defer server.Close()

	template, err := newTestCertificateTemplateClient(t, server.URL).GetCertificateTemplate(context.Background(), 12)
	if err != nil {
		t.Fatalf("GetCertificateTemplate failed: %v", err)
	}
	if template.ID != 12 || template.Name != "Android_WiFi" {
		t.Errorf("Unexpected identity: %+v", template)
	}
	if template.CertificateAuthorityID != 3 || template.CertificateAuthorityName != "SCEP_TEST" {
		t.Errorf("Unexpected certificate authority fields: %+v", template)
	}
	if template.CertificateAuthorityType != CATypeCustomSCEPProxy {
		t.Errorf("Expected type %q, got %q", CATypeCustomSCEPProxy, template.CertificateAuthorityType)
	}
	if template.CreatedAt != "2026-08-12T09:10:34Z" {
		t.Errorf("Unexpected created_at %q", template.CreatedAt)
	}
}

// TestGetCertificateTemplateOmittedSAN checks that a template with no subject
// alternative name decodes as an empty string rather than erroring — Fleet omits
// the key entirely.
func TestGetCertificateTemplateOmittedSAN(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"certificate": map[string]any{
				"id": 3, "name": "NoSAN", "subject_name": "CN=x", "certificate_authority_id": 1,
			},
		})
	}))
	defer server.Close()

	template, err := newTestCertificateTemplateClient(t, server.URL).GetCertificateTemplate(context.Background(), 3)
	if err != nil {
		t.Fatalf("GetCertificateTemplate failed: %v", err)
	}
	if template.SubjectAlternativeName != "" {
		t.Errorf("Expected an empty subject alternative name, got %q", template.SubjectAlternativeName)
	}
}

// TestGetCertificateTemplateNotFound checks the 404 unwraps to an APIError so
// the resource can remove a deleted template from state.
func TestGetCertificateTemplateNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Resource Not Found",
			"errors":  []map[string]string{{"name": "base", "reason": "CertificateTemplate 7 was not found in the datastore"}},
		})
	}))
	defer server.Close()

	_, err := newTestCertificateTemplateClient(t, server.URL).GetCertificateTemplate(context.Background(), 7)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("Expected a 404 APIError, got %v", err)
	}
}

// TestGetCertificateTemplateEmptyEnvelope guards against a 200 whose body has no
// certificate object being reported as a zero-valued template.
func TestGetCertificateTemplateEmptyEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	_, err := newTestCertificateTemplateClient(t, server.URL).GetCertificateTemplate(context.Background(), 4)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "response contained no certificate") {
		t.Errorf("Unexpected error %v", err)
	}
}

// TestListCertificateTemplatesScopesToFleet checks the fleet_id query parameter
// is always sent, including for fleet 0 — an absent fleet_id means "no team"
// rather than "every fleet", so dropping it would silently change the scope.
func TestListCertificateTemplatesScopesToFleet(t *testing.T) {
	for _, fleetID := range []int64{0, 42} {
		t.Run(fmt.Sprintf("fleet_%d", fleetID), func(t *testing.T) {
			var gotFleetID string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotFleetID = r.URL.Query().Get("fleet_id")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"certificates": []map[string]any{{"id": 1, "name": "A", "subject_name": "CN=a", "certificate_authority_id": 1}},
					"meta":         map[string]any{"has_next_results": false, "has_previous_results": false},
				})
			}))
			defer server.Close()

			templates, err := newTestCertificateTemplateClient(t, server.URL).ListCertificateTemplates(context.Background(), fleetID)
			if err != nil {
				t.Fatalf("ListCertificateTemplates failed: %v", err)
			}
			if want := fmt.Sprintf("%d", fleetID); gotFleetID != want {
				t.Errorf("Expected fleet_id=%s in the query string, got %q", want, gotFleetID)
			}
			if len(templates) != 1 {
				t.Fatalf("Expected 1 template, got %d", len(templates))
			}
		})
	}
}

// TestListCertificateTemplatesPaginates walks two pages, checking the client
// follows has_next_results and preserves Fleet's id-ascending order.
func TestListCertificateTemplatesPaginates(t *testing.T) {
	var pages []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		if r.URL.Query().Get("per_page") == "" {
			t.Error("Expected an explicit per_page in the query string")
		}

		switch page {
		case "0":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"certificates": []map[string]any{
					{"id": 1, "name": "A", "subject_name": "CN=a", "certificate_authority_id": 1},
					{"id": 2, "name": "B", "subject_name": "CN=b", "certificate_authority_id": 1},
				},
				"meta": map[string]any{"has_next_results": true, "has_previous_results": false},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"certificates": []map[string]any{
					{"id": 3, "name": "C", "subject_name": "CN=c", "certificate_authority_id": 1},
				},
				"meta": map[string]any{"has_next_results": false, "has_previous_results": true},
			})
		}
	}))
	defer server.Close()

	templates, err := newTestCertificateTemplateClient(t, server.URL).ListCertificateTemplates(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListCertificateTemplates failed: %v", err)
	}
	if len(pages) != 2 || pages[0] != "0" || pages[1] != "1" {
		t.Errorf("Expected pages 0 then 1, got %v", pages)
	}
	if len(templates) != 3 {
		t.Fatalf("Expected 3 templates across both pages, got %d", len(templates))
	}
	for i, wantID := range []int64{1, 2, 3} {
		if templates[i].ID != wantID {
			t.Errorf("templates[%d].ID = %d, want %d", i, templates[i].ID, wantID)
		}
	}
}

// TestListCertificateTemplatesRunawayPagination checks the loop is bounded when
// the server never flips has_next_results to false.
func TestListCertificateTemplatesRunawayPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"certificates": []map[string]any{{"id": 1, "name": "A", "subject_name": "CN=a", "certificate_authority_id": 1}},
			"meta":         map[string]any{"has_next_results": true},
		})
	}))
	defer server.Close()

	_, err := newTestCertificateTemplateClient(t, server.URL).ListCertificateTemplates(context.Background(), 0)
	if err == nil {
		t.Fatal("Expected a pagination bound error, got nil")
	}
	if !strings.Contains(err.Error(), "pagination exceeded") {
		t.Errorf("Unexpected error %v", err)
	}
}

// TestListCertificateTemplatesEmpty checks an empty fleet yields no templates
// and no error.
func TestListCertificateTemplatesEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"certificates": []map[string]any{},
			"meta":         map[string]any{"has_next_results": false, "has_previous_results": false},
		})
	}))
	defer server.Close()

	templates, err := newTestCertificateTemplateClient(t, server.URL).ListCertificateTemplates(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListCertificateTemplates failed: %v", err)
	}
	if len(templates) != 0 {
		t.Errorf("Expected no templates, got %d", len(templates))
	}
}

// TestDeleteCertificateTemplate checks the route and method.
func TestDeleteCertificateTemplate(t *testing.T) {
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/certificates/12" {
			t.Errorf("Expected path /api/v1/fleet/certificates/12, got %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	if err := newTestCertificateTemplateClient(t, server.URL).DeleteCertificateTemplate(context.Background(), 12); err != nil {
		t.Fatalf("DeleteCertificateTemplate failed: %v", err)
	}
	if !called {
		t.Error("Expected the delete endpoint to be called")
	}
}

// TestDeleteCertificateTemplateAlreadyGone pins the surprising response Fleet
// gives for a template that no longer exists: it looks the template up before
// authorizing, so the missing row escapes as a 500 "forbidden" rather than a
// 404. The client must surface that as an error — a real permission failure is
// indistinguishable from it, and treating it as success would report a destroy
// that never happened.
func TestDeleteCertificateTemplateAlreadyGone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "forbidden",
			"errors":  []map[string]string{{"name": "base", "reason": "forbidden"}},
		})
	}))
	defer server.Close()

	err := newTestCertificateTemplateClient(t, server.URL).DeleteCertificateTemplate(context.Background(), 7)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Expected the error to unwrap to *APIError, got %T", err)
	}
	// The status must survive intact. If it were mapped to 404 the resource's
	// isNotFound branch would treat a failed delete as an already-deleted
	// template and report a destroy that never happened.
	if apiErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected the 500 to be preserved, got %d", apiErr.StatusCode)
	}
}

// TestCertificateTemplateNamePattern pins the name character set against the
// cases Fleet accepts and rejects, so a drift in the regex is caught here rather
// than at apply time.
func TestCertificateTemplateNamePattern(t *testing.T) {
	valid := []string{"Android_WiFi", "cert 1", "a-b_c 9", "A", strings.Repeat("b", 255)}
	invalid := []string{"", "cert.dot", "cert@host", "café", "cert/slash", "cert:colon"}

	for _, name := range valid {
		if !CertificateTemplateNamePattern.MatchString(name) {
			t.Errorf("Expected %q to be a valid certificate template name", name)
		}
	}
	for _, name := range invalid {
		if CertificateTemplateNamePattern.MatchString(name) {
			t.Errorf("Expected %q to be an invalid certificate template name", name)
		}
	}
}
