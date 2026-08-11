package fleetdm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// newTestCAClient returns a client pointed at the given test server.
func newTestCAClient(t *testing.T, serverURL string) *Client {
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

func TestListCertificateAuthorities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/certificate_authorities" {
			t.Errorf("Expected path /api/v1/fleet/certificate_authorities, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"certificate_authorities": []map[string]interface{}{
				{"id": 7, "name": "SCEP_TEST", "type": CATypeCustomSCEPProxy},
				{"id": 8, "name": NDESCAName, "type": CATypeNDESSCEPProxy},
			},
		})
	}))
	defer server.Close()

	cas, err := newTestCAClient(t, server.URL).ListCertificateAuthorities(context.Background())
	if err != nil {
		t.Fatalf("ListCertificateAuthorities failed: %v", err)
	}
	if len(cas) != 2 {
		t.Fatalf("Expected 2 certificate authorities, got %d", len(cas))
	}
	if cas[0].ID != 7 || cas[0].Name != "SCEP_TEST" || cas[0].Type != CATypeCustomSCEPProxy {
		t.Errorf("Unexpected first CA: %+v", cas[0])
	}
	if cas[1].Name != NDESCAName {
		t.Errorf("Expected NDES CA name %q, got %q", NDESCAName, cas[1].Name)
	}
}

func TestListCertificateAuthoritiesEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"certificate_authorities": []interface{}{}})
	}))
	defer server.Close()

	cas, err := newTestCAClient(t, server.URL).ListCertificateAuthorities(context.Background())
	if err != nil {
		t.Fatalf("ListCertificateAuthorities failed: %v", err)
	}
	if len(cas) != 0 {
		t.Fatalf("Expected 0 certificate authorities, got %d", len(cas))
	}
}

// TestGetCertificateAuthorityMasksSecrets pins the observed Fleet 4.90
// behaviour: the detail endpoint returns non-secret configuration but replaces
// every secret with MaskedCASecret.
func TestGetCertificateAuthorityMasksSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/certificate_authorities/12" {
			t.Errorf("Expected path /api/v1/fleet/certificate_authorities/12, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":            12,
			"type":          CATypeCustomSCEPProxy,
			"name":          "SCEP_TEST",
			"url":           "https://scep.example.test/scep",
			"challenge":     MaskedCASecret,
			"created_at":    "2026-01-01T00:00:00Z",
			"updated_at":    "2026-01-02T00:00:00Z",
			"unknown_field": "ignored",
		})
	}))
	defer server.Close()

	ca, err := newTestCAClient(t, server.URL).GetCertificateAuthority(context.Background(), 12)
	if err != nil {
		t.Fatalf("GetCertificateAuthority failed: %v", err)
	}
	if ca.ID != 12 || ca.Name != "SCEP_TEST" || ca.Type != CATypeCustomSCEPProxy {
		t.Errorf("Unexpected CA identity: %+v", ca)
	}
	if ca.URL != "https://scep.example.test/scep" {
		t.Errorf("Expected url to be refreshed, got %q", ca.URL)
	}
	if ca.Challenge != MaskedCASecret {
		t.Errorf("Expected challenge to be the mask %q, got %q", MaskedCASecret, ca.Challenge)
	}
	if ca.UpdatedAt != "2026-01-02T00:00:00Z" {
		t.Errorf("Expected updated_at to decode, got %q", ca.UpdatedAt)
	}
}

func TestGetCertificateAuthorityNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Resource Not Found",
			"errors":  []map[string]string{{"name": "base", "reason": "CertificateAuthority 99 was not found in the datastore"}},
		})
	}))
	defer server.Close()

	_, err := newTestCAClient(t, server.URL).GetCertificateAuthority(context.Background(), 99)
	if err == nil {
		t.Fatal("Expected an error for a missing certificate authority")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Expected an *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", apiErr.StatusCode)
	}
}

// TestCreateCertificateAuthorityRequestBodies pins the exact JSON sent for each
// CA type. The invariants that matter:
//   - the payload nests the configuration under the type's key;
//   - ndes_scep_proxy carries no "name" (Fleet fixes it and rejects one);
//   - digicert always sends certificate_user_principal_names so an empty list
//     clears the stored value instead of leaving it untouched.
func TestCreateCertificateAuthorityRequestBodies(t *testing.T) {
	noUPNs := []string{}
	oneUPN := []string{"probe-upn@example.test"}

	tests := []struct {
		name     string
		payload  *CertificateAuthorityPayload
		wantType string
		wantBody map[string]interface{}
	}{
		{
			name: "custom_scep_proxy",
			payload: &CertificateAuthorityPayload{CustomSCEPProxy: &CustomSCEPProxyCA{
				Name:      strPtr("SCEP_TEST"),
				URL:       "https://scep.example.test/scep",
				Challenge: "fake-challenge",
			}},
			wantType: CATypeCustomSCEPProxy,
			wantBody: map[string]interface{}{
				"custom_scep_proxy": map[string]interface{}{
					"name":      "SCEP_TEST",
					"url":       "https://scep.example.test/scep",
					"challenge": "fake-challenge",
				},
			},
		},
		{
			name: "custom_est_proxy",
			payload: &CertificateAuthorityPayload{CustomESTProxy: &CustomESTProxyCA{
				Name:     strPtr("EST_TEST"),
				URL:      "https://est.example.test/.well-known/est",
				Username: "fake-user",
				Password: "fake-password",
			}},
			wantType: CATypeCustomESTProxy,
			wantBody: map[string]interface{}{
				"custom_est_proxy": map[string]interface{}{
					"name":     "EST_TEST",
					"url":      "https://est.example.test/.well-known/est",
					"username": "fake-user",
					"password": "fake-password",
				},
			},
		},
		{
			name: "ndes_scep_proxy omits name",
			payload: &CertificateAuthorityPayload{NDESSCEPProxy: &NDESSCEPProxyCA{
				URL:      "https://ndes.example.test/certsrv/mscep/mscep.dll",
				AdminURL: "https://ndes.example.test/certsrv/mscep_admin/",
				Username: "fake-user@example.test",
				Password: "fake-password",
			}},
			wantType: CATypeNDESSCEPProxy,
			wantBody: map[string]interface{}{
				"ndes_scep_proxy": map[string]interface{}{
					"url":       "https://ndes.example.test/certsrv/mscep/mscep.dll",
					"admin_url": "https://ndes.example.test/certsrv/mscep_admin/",
					"username":  "fake-user@example.test",
					"password":  "fake-password",
				},
			},
		},
		{
			name: "hydrant",
			payload: &CertificateAuthorityPayload{Hydrant: &HydrantCA{
				Name:         strPtr("HYDRANT_TEST"),
				URL:          "https://hydrant.example.test",
				ClientID:     "fake-client-id",
				ClientSecret: "fake-client-secret",
			}},
			wantType: CATypeHydrant,
			wantBody: map[string]interface{}{
				"hydrant": map[string]interface{}{
					"name":          "HYDRANT_TEST",
					"url":           "https://hydrant.example.test",
					"client_id":     "fake-client-id",
					"client_secret": "fake-client-secret",
				},
			},
		},
		{
			name: "smallstep",
			payload: &CertificateAuthorityPayload{Smallstep: &SmallstepSCEPProxyCA{
				Name:         strPtr("SMALLSTEP_TEST"),
				URL:          "https://smallstep.example.test/scep/agents",
				ChallengeURL: "https://smallstep.example.test/challenge",
				Username:     "fake-user",
				Password:     "fake-password",
			}},
			wantType: CATypeSmallstep,
			wantBody: map[string]interface{}{
				"smallstep": map[string]interface{}{
					"name":          "SMALLSTEP_TEST",
					"url":           "https://smallstep.example.test/scep/agents",
					"challenge_url": "https://smallstep.example.test/challenge",
					"username":      "fake-user",
					"password":      "fake-password",
				},
			},
		},
		{
			name: "digicert sends empty upn list",
			payload: &CertificateAuthorityPayload{DigiCert: &DigiCertCA{
				Name:                          strPtr("DIGICERT_TEST"),
				URL:                           "https://digicert.example.test",
				APIToken:                      "fake-api-token",
				ProfileID:                     "fake-profile-id",
				CertificateCommonName:         "$FLEET_VAR_HOST_HARDWARE_SERIAL",
				CertificateSeatID:             "$FLEET_VAR_HOST_HARDWARE_SERIAL",
				CertificateUserPrincipalNames: &noUPNs,
			}},
			wantType: CATypeDigiCert,
			wantBody: map[string]interface{}{
				"digicert": map[string]interface{}{
					"name":                             "DIGICERT_TEST",
					"url":                              "https://digicert.example.test",
					"api_token":                        "fake-api-token",
					"profile_id":                       "fake-profile-id",
					"certificate_common_name":          "$FLEET_VAR_HOST_HARDWARE_SERIAL",
					"certificate_seat_id":              "$FLEET_VAR_HOST_HARDWARE_SERIAL",
					"certificate_user_principal_names": []interface{}{},
				},
			},
		},
		{
			name: "digicert sends populated upn list",
			payload: &CertificateAuthorityPayload{DigiCert: &DigiCertCA{
				Name:                          strPtr("DIGICERT_TEST"),
				URL:                           "https://digicert.example.test",
				APIToken:                      "fake-api-token",
				ProfileID:                     "fake-profile-id",
				CertificateCommonName:         "probe-cn",
				CertificateSeatID:             "probe-seat",
				CertificateUserPrincipalNames: &oneUPN,
			}},
			wantType: CATypeDigiCert,
			wantBody: map[string]interface{}{
				"digicert": map[string]interface{}{
					"name":                             "DIGICERT_TEST",
					"url":                              "https://digicert.example.test",
					"api_token":                        "fake-api-token",
					"profile_id":                       "fake-profile-id",
					"certificate_common_name":          "probe-cn",
					"certificate_seat_id":              "probe-seat",
					"certificate_user_principal_names": []interface{}{"probe-upn@example.test"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody map[string]interface{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/fleet/certificate_authorities" {
					t.Errorf("Expected path /api/v1/fleet/certificate_authorities, got %s", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST method, got %s", r.Method)
				}
				raw, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(raw, &gotBody); err != nil {
					t.Errorf("Failed to decode request body: %v", err)
				}
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id": 3, "name": "CREATED", "type": tt.wantType,
				})
			}))
			defer server.Close()

			if got := tt.payload.Type(); got != tt.wantType {
				t.Errorf("Payload.Type() = %q, want %q", got, tt.wantType)
			}

			summary, err := newTestCAClient(t, server.URL).CreateCertificateAuthority(context.Background(), tt.payload)
			if err != nil {
				t.Fatalf("CreateCertificateAuthority failed: %v", err)
			}
			if summary.ID != 3 || summary.Type != tt.wantType {
				t.Errorf("Unexpected summary: %+v", summary)
			}
			if !reflect.DeepEqual(gotBody, tt.wantBody) {
				t.Errorf("Request body mismatch\n got: %#v\nwant: %#v", gotBody, tt.wantBody)
			}
		})
	}
}

func TestCreateCertificateAuthorityNoConfiguration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("Expected no HTTP request when no CA block is set")
	}))
	defer server.Close()

	client := newTestCAClient(t, server.URL)
	if _, err := client.CreateCertificateAuthority(context.Background(), &CertificateAuthorityPayload{}); err == nil {
		t.Error("Expected an error for an empty payload")
	}
	if _, err := client.CreateCertificateAuthority(context.Background(), nil); err == nil {
		t.Error("Expected an error for a nil payload")
	}
}

// TestCreateCertificateAuthorityValidationError covers Fleet rejecting the
// configuration, for example because it could not reach the configured URL.
func TestCreateCertificateAuthorityValidationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Validation Failed",
			"errors": []map[string]string{{
				"name":   "url",
				"reason": "Couldn't add certificate authority. Invalid SCEP URL. Please correct and try again.",
			}},
		})
	}))
	defer server.Close()

	_, err := newTestCAClient(t, server.URL).CreateCertificateAuthority(context.Background(),
		&CertificateAuthorityPayload{CustomSCEPProxy: &CustomSCEPProxyCA{
			Name: strPtr("SCEP_TEST"), URL: "https://scep.example.test/scep", Challenge: "fake-challenge",
		}})
	if err == nil {
		t.Fatal("Expected an error when Fleet rejects the configuration")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Expected an *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("Expected status 422, got %d", apiErr.StatusCode)
	}
	if len(apiErr.Errors) != 1 || apiErr.Errors[0].Name != "url" {
		t.Errorf("Expected the url validation detail to be preserved, got %+v", apiErr.Errors)
	}
}

// TestUpdateCertificateAuthority pins that updates PATCH the id-scoped path
// with the complete block. Fleet refuses to change a URL unless the type's
// secret accompanies it, so the client never sends a partial block.
func TestUpdateCertificateAuthority(t *testing.T) {
	var gotBody map[string]interface{}
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	err := newTestCAClient(t, server.URL).UpdateCertificateAuthority(context.Background(), 12,
		&CertificateAuthorityPayload{CustomSCEPProxy: &CustomSCEPProxyCA{
			Name: strPtr("SCEP_RENAMED"), URL: "https://scep.example.test/scep2", Challenge: "fake-challenge",
		}})
	if err != nil {
		t.Fatalf("UpdateCertificateAuthority failed: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("Expected PATCH method, got %s", gotMethod)
	}
	if gotPath != "/api/v1/fleet/certificate_authorities/12" {
		t.Errorf("Expected path /api/v1/fleet/certificate_authorities/12, got %s", gotPath)
	}
	want := map[string]interface{}{
		"custom_scep_proxy": map[string]interface{}{
			"name":      "SCEP_RENAMED",
			"url":       "https://scep.example.test/scep2",
			"challenge": "fake-challenge",
		},
	}
	if !reflect.DeepEqual(gotBody, want) {
		t.Errorf("Request body mismatch\n got: %#v\nwant: %#v", gotBody, want)
	}
}

func TestUpdateCertificateAuthorityNoConfiguration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("Expected no HTTP request when no CA block is set")
	}))
	defer server.Close()

	client := newTestCAClient(t, server.URL)
	if err := client.UpdateCertificateAuthority(context.Background(), 1, &CertificateAuthorityPayload{}); err == nil {
		t.Error("Expected an error for an empty payload")
	}
	if err := client.UpdateCertificateAuthority(context.Background(), 1, nil); err == nil {
		t.Error("Expected an error for a nil payload")
	}
}

// TestUpdateCertificateAuthorityTypeMismatch covers Fleet refusing to change a
// CA's type, which the provider plans as a replacement instead.
func TestUpdateCertificateAuthorityTypeMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Bad request",
			"errors": []map[string]string{{
				"name":   "base",
				"reason": "Couldn't edit certificate authority. The certificate authority types must be the same.",
			}},
		})
	}))
	defer server.Close()

	err := newTestCAClient(t, server.URL).UpdateCertificateAuthority(context.Background(), 12,
		&CertificateAuthorityPayload{Hydrant: &HydrantCA{
			Name: strPtr("HYDRANT_TEST"), URL: "https://hydrant.example.test",
			ClientID: "fake-client-id", ClientSecret: "fake-client-secret",
		}})
	if err == nil {
		t.Fatal("Expected an error when the types differ")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Expected an *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", apiErr.StatusCode)
	}
}

func TestDeleteCertificateAuthority(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := newTestCAClient(t, server.URL).DeleteCertificateAuthority(context.Background(), 12); err != nil {
		t.Fatalf("DeleteCertificateAuthority failed: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("Expected DELETE method, got %s", gotMethod)
	}
	if gotPath != "/api/v1/fleet/certificate_authorities/12" {
		t.Errorf("Expected path /api/v1/fleet/certificate_authorities/12, got %s", gotPath)
	}
}

// TestDeleteCertificateAuthorityConflict covers Fleet refusing to delete a CA
// that certificate templates still reference.
func TestDeleteCertificateAuthorityConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Couldn't delete certificate authority. It is referenced by certificate templates. Please remove the certificate templates first.",
		})
	}))
	defer server.Close()

	err := newTestCAClient(t, server.URL).DeleteCertificateAuthority(context.Background(), 12)
	if err == nil {
		t.Fatal("Expected an error when the CA is still referenced")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Expected an *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusConflict {
		t.Errorf("Expected status 409, got %d", apiErr.StatusCode)
	}
}

// TestCertificateAuthorityPayloadClearName pins the name-omission contract.
// Fleet checks name uniqueness on update without excluding the CA being
// updated, so an update that is not a rename must leave the field out entirely
// rather than re-send the current value.
func TestCertificateAuthorityPayloadClearName(t *testing.T) {
	payload := &CertificateAuthorityPayload{CustomSCEPProxy: &CustomSCEPProxyCA{
		Name:      strPtr("SCEP_TEST"),
		URL:       "https://scep.example.test/scep",
		Challenge: "fake-challenge",
	}}

	if name, ok := payload.Name(); !ok || name != "SCEP_TEST" {
		t.Fatalf("Name() = (%q, %v), want (\"SCEP_TEST\", true)", name, ok)
	}

	payload.ClearName()
	if _, ok := payload.Name(); ok {
		t.Error("Name() still reports a name after ClearName()")
	}

	// The cleared name must vanish from the wire format, not serialize as null.
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}
	var decoded map[string]map[string]interface{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Failed to decode payload: %v", err)
	}
	block := decoded[CATypeCustomSCEPProxy]
	if _, present := block["name"]; present {
		t.Errorf("Cleared name still present in the request body: %s", encoded)
	}
	if block["challenge"] != "fake-challenge" || block["url"] != "https://scep.example.test/scep" {
		t.Errorf("ClearName() disturbed the rest of the block: %s", encoded)
	}
}

// TestCertificateAuthorityPayloadNameNDES checks NDES reports no name: Fleet
// fixes it and rejects one in the update payload.
func TestCertificateAuthorityPayloadNameNDES(t *testing.T) {
	payload := &CertificateAuthorityPayload{NDESSCEPProxy: &NDESSCEPProxyCA{
		URL: "https://ndes.example.test/certsrv/mscep/mscep.dll",
	}}
	if name, ok := payload.Name(); ok {
		t.Errorf("Name() = (%q, true), want no name for NDES", name)
	}
	// Must be a no-op rather than a panic.
	payload.ClearName()
}

func TestCertificateAuthorityPayloadType(t *testing.T) {
	tests := []struct {
		name    string
		payload CertificateAuthorityPayload
		want    string
	}{
		{"empty", CertificateAuthorityPayload{}, ""},
		{"digicert", CertificateAuthorityPayload{DigiCert: &DigiCertCA{}}, CATypeDigiCert},
		{"ndes", CertificateAuthorityPayload{NDESSCEPProxy: &NDESSCEPProxyCA{}}, CATypeNDESSCEPProxy},
		{"custom_scep", CertificateAuthorityPayload{CustomSCEPProxy: &CustomSCEPProxyCA{}}, CATypeCustomSCEPProxy},
		{"custom_est", CertificateAuthorityPayload{CustomESTProxy: &CustomESTProxyCA{}}, CATypeCustomESTProxy},
		{"hydrant", CertificateAuthorityPayload{Hydrant: &HydrantCA{}}, CATypeHydrant},
		{"smallstep", CertificateAuthorityPayload{Smallstep: &SmallstepSCEPProxyCA{}}, CATypeSmallstep},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.payload.Type(); got != tt.want {
				t.Errorf("Type() = %q, want %q", got, tt.want)
			}
		})
	}
}
