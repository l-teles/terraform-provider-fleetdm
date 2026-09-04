package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// caSecretFields are the per-type secret keys Fleet masks in its responses.
var caSecretFields = map[string]bool{
	"challenge":     true,
	"password":      true,
	"api_token":     true,
	"client_secret": true,
}

// fakeCAServer is a stateful stand-in for Fleet's /certificate_authorities
// endpoints. It reproduces the two behaviours the provider has to cope with:
// the detail endpoint returns non-secret configuration, and every secret comes
// back as the mask.
type fakeCAServer struct {
	mu     sync.Mutex
	nextID int
	cas    map[int]map[string]interface{}
	// writes records the block of every POST and PATCH, so tests can assert
	// what actually reached Fleet — the only way to see a write-only secret,
	// which by design appears in neither plan nor state.
	writes []caWrite
}

type caWrite struct {
	method string
	caType string
	block  map[string]interface{}
}

func newFakeCAServer() (*httptest.Server, *fakeCAServer) {
	f := &fakeCAServer{nextID: 1, cas: map[int]map[string]interface{}{}}
	return httptest.NewServer(f), f
}

// nameTaken reports whether any stored CA of the given type already carries the
// name. Fleet's uniqueness check does not exclude the CA being updated, so this
// deliberately matches the row itself too.
func (f *fakeCAServer) nameTaken(caType, name string) bool {
	for _, detail := range f.cas {
		if detail["type"] == caType && detail["name"] == name {
			return true
		}
	}
	return false
}

// recordedWrites returns a copy of the writes seen so far.
func (f *fakeCAServer) recordedWrites() []caWrite {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]caWrite(nil), f.writes...)
}

func (f *fakeCAServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	const base = "/api/v1/fleet/certificate_authorities"
	switch {
	case r.URL.Path == base && r.Method == http.MethodPost:
		f.create(w, r)
	case r.URL.Path == base && r.Method == http.MethodGet:
		f.list(w)
	case strings.HasPrefix(r.URL.Path, base+"/"):
		id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, base+"/"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			f.get(w, id)
		case http.MethodPatch:
			f.patch(w, r, id)
		case http.MethodDelete:
			f.delete(w, id)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	default:
		http.NotFound(w, r)
	}
}

// decodeBlock extracts the single CA type key and its configuration.
func decodeBlock(r *http.Request) (string, map[string]interface{}, error) {
	var body map[string]map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", nil, err
	}
	if len(body) != 1 {
		return "", nil, fmt.Errorf("expected exactly one CA type key, got %d", len(body))
	}
	for caType, block := range body {
		return caType, block, nil
	}
	return "", nil, fmt.Errorf("unreachable")
}

func (f *fakeCAServer) create(w http.ResponseWriter, r *http.Request) {
	caType, block, err := decodeBlock(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": err.Error()})
		return
	}

	f.writes = append(f.writes, caWrite{method: http.MethodPost, caType: caType, block: block})

	id := f.nextID
	f.nextID++

	detail := map[string]interface{}{"id": id, "type": caType}
	for k, v := range block {
		detail[k] = v
	}
	// Fleet fixes the NDES name and does not accept one.
	if caType == "ndes_scep_proxy" {
		detail["name"] = "NDES"
	}
	f.cas[id] = detail

	name, _ := detail["name"].(string)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "name": name, "type": caType})
}

func (f *fakeCAServer) patch(w http.ResponseWriter, r *http.Request, id int) {
	detail, ok := f.cas[id]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": fmt.Sprintf("Certificate authority with ID %d does not exist.", id),
		})
		return
	}
	caType, block, err := decodeBlock(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": err.Error()})
		return
	}
	if caType != detail["type"] {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Couldn't edit certificate authority. The certificate authority types must be the same.",
		})
		return
	}
	// Fleet checks name uniqueness on update without excluding the CA being
	// updated, so a PATCH that re-sends the CA's current name is rejected.
	// Reproduced here so the provider's "omit an unchanged name" behaviour is
	// covered by the mock tests and not only by the live ones.
	if name, ok := block["name"].(string); ok && f.nameTaken(caType, name) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Conflict",
			"errors": []map[string]string{{
				"name": "base", "reason": "a certificate authority with this name already exists",
			}},
		})
		return
	}

	f.writes = append(f.writes, caWrite{method: http.MethodPatch, caType: caType, block: block})

	for k, v := range block {
		detail[k] = v
	}
	if caType == "ndes_scep_proxy" {
		detail["name"] = "NDES"
	}
	w.Write([]byte(`{}`))
}

// get returns the stored detail with secrets masked, as Fleet does.
func (f *fakeCAServer) get(w http.ResponseWriter, id int) {
	detail, ok := f.cas[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Resource Not Found",
			"errors":  []map[string]string{{"name": "base", "reason": "not found"}},
		})
		return
	}
	masked := map[string]interface{}{}
	for k, v := range detail {
		if caSecretFields[k] {
			masked[k] = "********"
			continue
		}
		masked[k] = v
	}
	json.NewEncoder(w).Encode(masked)
}

func (f *fakeCAServer) list(w http.ResponseWriter) {
	summaries := []map[string]interface{}{}
	for id, detail := range f.cas {
		name, _ := detail["name"].(string)
		summaries = append(summaries, map[string]interface{}{
			"id": id, "name": name, "type": detail["type"],
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"certificate_authorities": summaries})
}

func (f *fakeCAServer) delete(w http.ResponseWriter, id int) {
	if _, ok := f.cas[id]; !ok {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Resource Not Found"})
		return
	}
	delete(f.cas, id)
	w.WriteHeader(http.StatusNoContent)
}

func testCAProviderBlock(serverURL string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %q
  api_key        = "test-token"
}
`, serverURL)
}

// TestCertificateAuthorityResource_customSCEPLifecycle covers create, an
// in-place update that renames and re-points the CA, and a degraded import.
func TestCertificateAuthorityResource_customSCEPLifecycle(t *testing.T) {
	server, _ := newFakeCAServer()
	defer server.Close()

	config := func(name, url, challenge string) string {
		return testCAProviderBlock(server.URL) + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  custom_scep_proxy = {
    name      = %q
    url       = %q
    challenge = %q
  }
}
`, name, url, challenge)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("SCEP_TEST", "https://scep.example.test/scep", "fake-challenge"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "id", "1"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "name", "SCEP_TEST"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "type", "custom_scep_proxy"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.url", "https://scep.example.test/scep"),
					// The secret must survive the masked read, not become "********".
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.challenge", "fake-challenge"),
				),
			},
			{
				// In-place update: same type, new name, url and secret. The id
				// must be preserved, proving Fleet's PATCH path was used.
				Config: config("SCEP_RENAMED", "https://scep2.example.test/scep", "fake-challenge-rotated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "id", "1"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "name", "SCEP_RENAMED"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.url", "https://scep2.example.test/scep"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.challenge", "fake-challenge-rotated"),
				),
			},
			{
				// Degraded import: identity and non-secret config come back,
				// the secret cannot and is imported as null.
				ResourceName:      "fleetdm_certificate_authority.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"custom_scep_proxy.challenge",
				},
			},
		},
	})
}

// TestCertificateAuthorityResource_secretsWOVersionForcesUpdate checks that
// bumping the rotation trigger re-sends the block without replacing the CA.
func TestCertificateAuthorityResource_secretsWOVersionForcesUpdate(t *testing.T) {
	server, _ := newFakeCAServer()
	defer server.Close()

	config := func(version int) string {
		return testCAProviderBlock(server.URL) + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  secrets_wo_version = %d

  custom_scep_proxy = {
    name      = "SCEP_TEST"
    url       = "https://scep.example.test/scep"
    challenge = "fake-challenge"
  }
}
`, version)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "id", "1"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "secrets_wo_version", "1"),
				),
			},
			{
				Config: config(2),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Same id: the bump is an update, never a replacement.
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "id", "1"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "secrets_wo_version", "2"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.challenge", "fake-challenge"),
				),
			},
		},
	})
}

// TestCertificateAuthorityResource_typeChangeReplaces checks that switching CA
// type replaces the resource, since Fleet cannot change a type in place.
func TestCertificateAuthorityResource_typeChangeReplaces(t *testing.T) {
	server, _ := newFakeCAServer()
	defer server.Close()

	scepConfig := testCAProviderBlock(server.URL) + `
resource "fleetdm_certificate_authority" "test" {
  custom_scep_proxy = {
    name      = "SCEP_TEST"
    url       = "https://scep.example.test/scep"
    challenge = "fake-challenge"
  }
}
`
	hydrantConfig := testCAProviderBlock(server.URL) + `
resource "fleetdm_certificate_authority" "test" {
  hydrant = {
    name          = "HYDRANT_TEST"
    url           = "https://hydrant.example.test"
    client_id     = "fake-client-id"
    client_secret = "fake-client-secret"
  }
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: scepConfig,
				Check:  resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "id", "1"),
			},
			{
				Config: hydrantConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					// A new id proves the resource was replaced rather than patched.
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "id", "2"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "type", "hydrant"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "name", "HYDRANT_TEST"),
					resource.TestCheckNoResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.name"),
				),
			},
		},
	})
}

// TestCertificateAuthorityResource_ndesFixedName checks that the NDES block
// takes no name and that Fleet's fixed name surfaces as the computed name.
func TestCertificateAuthorityResource_ndesFixedName(t *testing.T) {
	server, _ := newFakeCAServer()
	defer server.Close()

	config := testCAProviderBlock(server.URL) + `
resource "fleetdm_certificate_authority" "test" {
  ndes_scep_proxy = {
    url       = "https://ndes.example.test/certsrv/mscep/mscep.dll"
    admin_url = "https://ndes.example.test/certsrv/mscep_admin/"
    username  = "fake-user@example.test"
    password  = "fake-password"
  }
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "name", "NDES"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "type", "ndes_scep_proxy"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "ndes_scep_proxy.username", "fake-user@example.test"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "ndes_scep_proxy.password", "fake-password"),
				),
			},
		},
	})
}

// TestCertificateAuthorityResource_digicertUPNs covers the DigiCert block,
// including the optional user principal name list.
func TestCertificateAuthorityResource_digicertUPNs(t *testing.T) {
	server, _ := newFakeCAServer()
	defer server.Close()

	config := testCAProviderBlock(server.URL) + `
resource "fleetdm_certificate_authority" "test" {
  digicert = {
    name                             = "DIGICERT_TEST"
    url                              = "https://digicert.example.test"
    api_token                        = "fake-api-token"
    profile_id                       = "fake-profile-id"
    certificate_common_name          = "probe-cn"
    certificate_seat_id              = "probe-seat"
    certificate_user_principal_names = ["probe-upn@example.test"]
  }
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "type", "digicert"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "digicert.profile_id", "fake-profile-id"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "digicert.api_token", "fake-api-token"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "digicert.certificate_user_principal_names.#", "1"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "digicert.certificate_user_principal_names.0", "probe-upn@example.test"),
				),
			},
		},
	})
}

// TestCertificateAuthorityResource_digicertUPNFormsAreStable pins that neither
// an omitted nor an explicitly empty user principal name list produces a
// perpetual diff. Fleet reports both as empty, so a naive refresh would rewrite
// one form into the other on every read. resource.Test fails a step whose apply
// leaves a non-empty plan, which is what catches the regression.
func TestCertificateAuthorityResource_digicertUPNFormsAreStable(t *testing.T) {
	for _, tc := range []struct {
		name      string
		upnAttr   string
		wantCount string
	}{
		{name: "omitted", upnAttr: "", wantCount: ""},
		{name: "explicitly empty", upnAttr: "certificate_user_principal_names = []", wantCount: "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := newFakeCAServer()
			defer server.Close()

			config := testCAProviderBlock(server.URL) + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  digicert = {
    name                    = "DIGICERT_TEST"
    url                     = "https://digicert.example.test"
    api_token               = "fake-api-token"
    profile_id              = "fake-profile-id"
    certificate_common_name = "probe-cn"
    certificate_seat_id     = "probe-seat"
    %s
  }
}
`, tc.upnAttr)

			checks := []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "type", "digicert"),
			}
			if tc.wantCount == "" {
				checks = append(checks, resource.TestCheckNoResourceAttr(
					"fleetdm_certificate_authority.test", "digicert.certificate_user_principal_names"))
			} else {
				checks = append(checks, resource.TestCheckResourceAttr(
					"fleetdm_certificate_authority.test", "digicert.certificate_user_principal_names.#", tc.wantCount))
			}

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: config,
						Check:  resource.ComposeAggregateTestCheckFunc(checks...),
					},
					{
						// A refresh must not rewrite the configured form.
						Config:   config,
						PlanOnly: true,
					},
				},
			})
		})
	}
}

// TestCertificateAuthorityResource_digicertUPNDriftDetected checks that
// preserving the configured empty form does not blind the provider to a real
// out-of-band change to the list.
func TestCertificateAuthorityResource_digicertUPNDriftDetected(t *testing.T) {
	server, _ := newFakeCAServer()
	defer server.Close()

	config := testCAProviderBlock(server.URL) + `
resource "fleetdm_certificate_authority" "test" {
  digicert = {
    name                             = "DIGICERT_TEST"
    url                              = "https://digicert.example.test"
    api_token                        = "fake-api-token"
    profile_id                       = "fake-profile-id"
    certificate_common_name          = "probe-cn"
    certificate_seat_id              = "probe-seat"
    certificate_user_principal_names = ["probe-upn@example.test"]
  }
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.TestCheckResourceAttr("fleetdm_certificate_authority.test",
					"digicert.certificate_user_principal_names.0", "probe-upn@example.test"),
			},
			{
				// Clear the list behind Terraform's back; the refresh must
				// notice and plan to restore it.
				PreConfig: func() {
					body := strings.NewReader(`{"digicert":{"api_token":"fake-api-token","certificate_user_principal_names":[]}}`)
					req, _ := http.NewRequest(http.MethodPatch,
						server.URL+"/api/v1/fleet/certificate_authorities/1", body)
					req.Header.Set("Content-Type", "application/json")
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatalf("Failed to clear the UPN list out of band: %v", err)
					}
					resp.Body.Close()
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// lastWriteBlock returns the block of the most recent write of the given
// method, failing the test when there is none.
func lastWriteBlock(t *testing.T, f *fakeCAServer, method string) map[string]interface{} {
	t.Helper()
	writes := f.recordedWrites()
	for i := len(writes) - 1; i >= 0; i-- {
		if writes[i].method == method {
			return writes[i].block
		}
	}
	t.Fatalf("No %s write was recorded", method)
	return nil
}

// checkWriteCarried asserts the last write of the given method sent the whole
// block, including the secret. A write-only secret is in neither plan nor
// state, so what reached Fleet is the only place it can be observed.
func checkWriteCarried(t *testing.T, f *fakeCAServer, method string, want map[string]interface{}) resource.TestCheckFunc {
	return func(*terraform.State) error {
		t.Helper()
		block := lastWriteBlock(t, f, method)
		for k, v := range want {
			if got := block[k]; got != v {
				return fmt.Errorf("%s block field %q = %v, want %v (full block: %v)", method, k, got, v, block)
			}
		}
		return nil
	}
}

// checkWriteOmitted asserts the last write of the given method did not carry
// the named field.
func checkWriteOmitted(t *testing.T, f *fakeCAServer, method, field string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		t.Helper()
		block := lastWriteBlock(t, f, method)
		if _, present := block[field]; present {
			return fmt.Errorf("%s block unexpectedly carried %q (full block: %v)", method, field, block)
		}
		return nil
	}
}

// TestCertificateAuthorityResource_writeOnlyLifecycle exercises the write-only
// credential path: the secret never lands in plan or state, editing it alone is
// invisible to Terraform, and bumping secrets_wo_version re-sends the complete
// block with the value read from configuration.
//
// Skipped below Terraform 1.11, the first release supporting write-only
// attributes — the CI matrix still includes 1.5.
func TestCertificateAuthorityResource_writeOnlyLifecycle(t *testing.T) {
	server, fake := newFakeCAServer()
	defer server.Close()

	config := func(challenge string, version int) string {
		return testCAProviderBlock(server.URL) + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  secrets_wo_version = %d

  custom_scep_proxy = {
    name         = "SCEP_TEST"
    url          = "https://scep.example.test/scep"
    challenge_wo = %q
  }
}
`, version, challenge)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: config("fake-challenge-wo", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "id", "1"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "name", "SCEP_TEST"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "secrets_wo_version", "1"),
					// The point of the write-only path: neither variant is persisted.
					resource.TestCheckNoResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.challenge_wo"),
					resource.TestCheckNoResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.challenge"),
					// ...but Fleet did receive it.
					checkWriteCarried(t, fake, http.MethodPost, map[string]interface{}{
						"name":      "SCEP_TEST",
						"url":       "https://scep.example.test/scep",
						"challenge": "fake-challenge-wo",
					}),
				),
			},
			{
				// Editing challenge_wo alone must not produce a diff: Terraform
				// cannot see a write-only value.
				Config: config("fake-challenge-wo-changed", 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Bumping the version rotates in place, re-sending the whole
				// block with the config-sourced secret.
				Config: config("fake-challenge-wo-rotated", 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_certificate_authority.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "id", "1"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "secrets_wo_version", "2"),
					resource.TestCheckNoResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.challenge_wo"),
					// The name is unchanged, so it must be absent from the
					// PATCH: Fleet rejects a re-sent current name with 409.
					checkWriteOmitted(t, fake, http.MethodPatch, "name"),
					checkWriteCarried(t, fake, http.MethodPatch, map[string]interface{}{
						"url":       "https://scep.example.test/scep",
						"challenge": "fake-challenge-wo-rotated",
					}),
				),
			},
			{
				ResourceName:      "fleetdm_certificate_authority.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Nothing secret-shaped can come back from Fleet.
				ImportStateVerifyIgnore: []string{
					"custom_scep_proxy.challenge",
					"custom_scep_proxy.challenge_wo",
					"secrets_wo_version",
				},
			},
		},
	})
}

// TestCertificateAuthorityResource_writeOnlyNonSecretUpdate covers a
// non-secret change with an unchanged write-only credential and an unchanged
// secrets_wo_version.
//
// This is the interaction Fleet forces: it refuses to change a URL unless the
// type's secret accompanies it, while the secret is write-only and therefore
// absent from both plan and state. The update has to recover it from
// req.Config, and the only place that is observable is the request Fleet
// received — a regression here would send an empty challenge and silently
// destroy the credential.
func TestCertificateAuthorityResource_writeOnlyNonSecretUpdate(t *testing.T) {
	server, fake := newFakeCAServer()
	defer server.Close()

	config := func(url string) string {
		return testCAProviderBlock(server.URL) + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  secrets_wo_version = 1

  custom_scep_proxy = {
    name         = "SCEP_TEST"
    url          = %q
    challenge_wo = "fake-challenge-wo"
  }
}
`, url)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: config("https://scep.example.test/scep"),
			},
			{
				Config: config("https://scep2.example.test/scep"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_certificate_authority.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "id", "1"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test",
						"custom_scep_proxy.url", "https://scep2.example.test/scep"),
					// The version did not move, but the secret still has to
					// travel with the URL change.
					checkWriteCarried(t, fake, http.MethodPatch, map[string]interface{}{
						"url":       "https://scep2.example.test/scep",
						"challenge": "fake-challenge-wo",
					}),
					checkWriteOmitted(t, fake, http.MethodPatch, "name"),
				),
			},
		},
	})
}

// TestCertificateAuthorityResource_writeOnlyAcrossTypes checks the write-only
// variant is wired for every CA type that carries a credential, not just SCEP.
func TestCertificateAuthorityResource_writeOnlyAcrossTypes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		block     string
		secretKey string
	}{
		{
			name: "digicert", secretKey: "api_token",
			block: `digicert = {
    name                    = "DIGICERT_TEST"
    url                     = "https://digicert.example.test"
    api_token_wo            = "fake-secret-wo"
    profile_id              = "fake-profile-id"
    certificate_common_name = "probe-cn"
    certificate_seat_id     = "probe-seat"
  }`,
		},
		{
			name: "ndes_scep_proxy", secretKey: "password",
			block: `ndes_scep_proxy = {
    url         = "https://ndes.example.test/certsrv/mscep/mscep.dll"
    admin_url   = "https://ndes.example.test/certsrv/mscep_admin/"
    username    = "fake-user@example.test"
    password_wo = "fake-secret-wo"
  }`,
		},
		{
			name: "custom_est_proxy", secretKey: "password",
			block: `custom_est_proxy = {
    name        = "EST_TEST"
    url         = "https://est.example.test/.well-known/est"
    username    = "fake-user"
    password_wo = "fake-secret-wo"
  }`,
		},
		{
			name: "hydrant", secretKey: "client_secret",
			block: `hydrant = {
    name             = "HYDRANT_TEST"
    url              = "https://hydrant.example.test"
    client_id        = "fake-client-id"
    client_secret_wo = "fake-secret-wo"
  }`,
		},
		{
			name: "smallstep", secretKey: "password",
			block: `smallstep = {
    name          = "SMALLSTEP_TEST"
    url           = "https://smallstep.example.test/scep/agents"
    challenge_url = "https://smallstep.example.test/challenge"
    username      = "fake-user"
    password_wo   = "fake-secret-wo"
  }`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, fake := newFakeCAServer()
			defer server.Close()

			config := testCAProviderBlock(server.URL) + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  secrets_wo_version = 1

  %s
}
`, tc.block)

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				TerraformVersionChecks: []tfversion.TerraformVersionCheck{
					tfversion.SkipBelow(tfversion.Version1_11_0),
				},
				Steps: []resource.TestStep{
					{
						Config: config,
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "type", tc.name),
							resource.TestCheckNoResourceAttr("fleetdm_certificate_authority.test",
								tc.name+"."+tc.secretKey+"_wo"),
							resource.TestCheckNoResourceAttr("fleetdm_certificate_authority.test",
								tc.name+"."+tc.secretKey),
							checkWriteCarried(t, fake, http.MethodPost, map[string]interface{}{
								tc.secretKey: "fake-secret-wo",
							}),
						),
					},
				},
			})
		})
	}
}

// TestCertificateAuthorityResource_secretExactlyOneOf checks that a block must
// carry exactly one of the in-state and write-only forms of its credential.
func TestCertificateAuthorityResource_secretExactlyOneOf(t *testing.T) {
	server, _ := newFakeCAServer()
	defer server.Close()

	both := testCAProviderBlock(server.URL) + `
resource "fleetdm_certificate_authority" "test" {
  custom_scep_proxy = {
    name         = "SCEP_TEST"
    url          = "https://scep.example.test/scep"
    challenge    = "fake-challenge"
    challenge_wo = "fake-challenge-wo"
  }
}
`
	neither := testCAProviderBlock(server.URL) + `
resource "fleetdm_certificate_authority" "test" {
  custom_scep_proxy = {
    name = "SCEP_TEST"
    url  = "https://scep.example.test/scep"
  }
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config:      both,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			{
				Config:      neither,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
		},
	})
}

// TestCertificateAuthorityResource_rejectsMaskedSecret checks that Fleet's
// redaction placeholder is refused as a configured credential, in both the
// in-state and the write-only form.
//
// Fleet shows `********` wherever a stored secret would appear, so pasting it
// back is an easy post-import mistake. Fleet then reads the mask in an update
// as "leave the secret unchanged", so without this guard the apply succeeds
// while state records the mask as the credential — and a later replacement
// would send the literal mask as the real secret.
func TestCertificateAuthorityResource_rejectsMaskedSecret(t *testing.T) {
	for _, tc := range []struct {
		name  string
		attr  string
		tfVer []tfversion.TerraformVersionCheck
	}{
		{name: "in-state", attr: "challenge"},
		{
			name: "write-only", attr: "challenge_wo",
			tfVer: []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := newFakeCAServer()
			defer server.Close()

			config := testCAProviderBlock(server.URL) + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  custom_scep_proxy = {
    name = "SCEP_TEST"
    url  = "https://scep.example.test/scep"
    %s   = %q
  }
}
`, tc.attr, fleetdm.MaskedCASecret)

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				TerraformVersionChecks:   tc.tfVer,
				Steps: []resource.TestStep{
					{
						Config:      config,
						ExpectError: regexp.MustCompile(`Invalid Certificate Authority Secret`),
					},
				},
			})
		})
	}
}

// TestCertificateAuthorityResource_unknownBlock checks that a CA block whose
// whole value is unknown at plan time — because it comes from another
// resource's output — is planned rather than crashing.
//
// ModifyPlan must not decode the plan into the typed model for this: an unknown
// object cannot convert into a *struct, which fails with a Value Conversion
// Error before any of the resource's own logic runs.
func TestCertificateAuthorityResource_unknownBlock(t *testing.T) {
	server, _ := newFakeCAServer()
	defer server.Close()

	config := testCAProviderBlock(server.URL) + `
resource "terraform_data" "cfg" {
  input = {
    name      = "SCEP_TEST"
    url       = "https://scep.example.test/scep"
    challenge = "fake-challenge"
  }
}

resource "fleetdm_certificate_authority" "test" {
  custom_scep_proxy = terraform_data.cfg.output
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "type", "custom_scep_proxy"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "name", "SCEP_TEST"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.challenge", "fake-challenge"),
				),
			},
		},
	})
}

// TestCertificateAuthorityResource_exactlyOneOf checks the schema rejects zero
// and multiple CA type blocks.
func TestCertificateAuthorityResource_exactlyOneOf(t *testing.T) {
	server, _ := newFakeCAServer()
	defer server.Close()

	noBlocks := testCAProviderBlock(server.URL) + `
resource "fleetdm_certificate_authority" "test" {}
`
	twoBlocks := testCAProviderBlock(server.URL) + `
resource "fleetdm_certificate_authority" "test" {
  custom_scep_proxy = {
    name      = "SCEP_TEST"
    url       = "https://scep.example.test/scep"
    challenge = "fake-challenge"
  }

  hydrant = {
    name          = "HYDRANT_TEST"
    url           = "https://hydrant.example.test"
    client_id     = "fake-client-id"
    client_secret = "fake-client-secret"
  }
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      noBlocks,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			{
				Config:      twoBlocks,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
		},
	})
}

// TestCertificateAuthorityResource_rejectsUnnormalizedValues checks the
// validator that prevents a permanent diff against Fleet's server-side
// preprocessing, which trims whitespace and normalizes to Unicode NFC.
//
// For a name the consequence is worse than a cosmetic diff: the resulting
// update carries a name Fleet considers unchanged, which Fleet answers with 409.
func TestCertificateAuthorityResource_rejectsUnnormalizedValues(t *testing.T) {
	for _, tc := range []struct {
		name      string
		attr      string
		value     string
		wantError string
	}{
		{
			name: "untrimmed name", attr: "name", value: " SCEP_TEST ",
			wantError: `must not have leading or trailing whitespace`,
		},
		{
			name: "untrimmed url", attr: "url", value: " https://scep.example.test/scep ",
			wantError: `must not have leading or trailing whitespace`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := newFakeCAServer()
			defer server.Close()

			fields := map[string]string{
				"name":      "SCEP_TEST",
				"url":       "https://scep.example.test/scep",
				"challenge": "fake-challenge",
			}
			fields[tc.attr] = tc.value

			config := testCAProviderBlock(server.URL) + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  custom_scep_proxy = {
    name      = %q
    url       = %q
    challenge = %q
  }
}
`, fields["name"], fields["url"], fields["challenge"])

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      config,
						ExpectError: regexp.MustCompile(tc.wantError),
					},
				},
			})
		})
	}
}

// TestCertificateAuthorityResource_recreatesWhenDeletedOutOfBand checks the
// 404 handling in Read removes the resource from state.
func TestCertificateAuthorityResource_recreatesWhenDeletedOutOfBand(t *testing.T) {
	server, _ := newFakeCAServer()
	defer server.Close()

	config := testCAProviderBlock(server.URL) + `
resource "fleetdm_certificate_authority" "test" {
  custom_est_proxy = {
    name     = "EST_TEST"
    url      = "https://est.example.test/.well-known/est"
    username = "fake-user"
    password = "fake-password"
  }
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "id", "1"),
			},
			{
				// Delete behind Terraform's back, then refresh: the plan must
				// be non-empty because the resource left state.
				PreConfig: func() {
					req, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/fleet/certificate_authorities/1", nil)
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatalf("Failed to delete out of band: %v", err)
					}
					resp.Body.Close()
				},
				Config:             config,
				ExpectNonEmptyPlan: true,
				PlanOnly:           true,
			},
		},
	})
}

// TestPreserveSecret covers the state-preserving rule for CA credentials.
//
// Fleet's contract is that it never returns a secret, so the last case is
// defense in depth: if a future Fleet ever did leak one, it must not be written
// into state for a resource whose credential is deliberately absent from it —
// the write-only path, and an import before the credential is supplied.
func TestPreserveSecret(t *testing.T) {
	for _, tc := range []struct {
		name     string
		apiValue string
		state    types.String
		want     types.String
	}{
		{
			name:     "mask keeps configured value",
			apiValue: fleetdm.MaskedCASecret, state: types.StringValue("fake-challenge"),
			want: types.StringValue("fake-challenge"),
		},
		{
			name:     "empty keeps configured value",
			apiValue: "", state: types.StringValue("fake-challenge"),
			want: types.StringValue("fake-challenge"),
		},
		{
			name:     "mask keeps null null",
			apiValue: fleetdm.MaskedCASecret, state: types.StringNull(),
			want: types.StringNull(),
		},
		{
			name:     "leaked secret never populates a null state",
			apiValue: "unexpectedly-returned-secret", state: types.StringNull(),
			want: types.StringNull(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := preserveSecret(tc.apiValue, tc.state); !got.Equal(tc.want) {
				t.Errorf("preserveSecret(%q, %v) = %v, want %v", tc.apiValue, tc.state, got, tc.want)
			}
		})
	}
}

// TestFleetNormalizedValidatorNFC covers the NFC branch of the name/URL
// validator directly.
//
// It cannot be reached through an HCL literal: Terraform normalizes string
// literals to NFC before a provider ever sees them, which was confirmed by
// feeding a decomposed literal through a full plan and observing no diagnostic
// even though the same bytes fail norm.NFC.IsNormalString in Go. The guard
// still matters for values that do not come from a literal — a variable loaded
// from a JSON file, a data source attribute, or another resource's output —
// where no such normalization happens.
func TestFleetNormalizedValidatorNFC(t *testing.T) {
	for _, tc := range []struct {
		name      string
		value     string
		wantError bool
	}{
		{name: "composed", value: "CAF\u00c9_SCEP", wantError: false},
		{name: "decomposed", value: "CAFE\u0301_SCEP", wantError: true},
		{name: "plain ascii", value: "SCEP_TEST", wantError: false},
		{name: "null", value: "", wantError: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := validator.StringRequest{
				Path:        path.Root("custom_scep_proxy").AtName("name"),
				ConfigValue: types.StringValue(tc.value),
			}
			if tc.value == "" {
				req.ConfigValue = types.StringNull()
			}
			resp := &validator.StringResponse{}

			fleetNormalizedValidator{}.ValidateString(context.Background(), req, resp)

			if got := resp.Diagnostics.HasError(); got != tc.wantError {
				t.Errorf("HasError() = %v, want %v (diagnostics: %v)", got, tc.wantError, resp.Diagnostics)
			}
			if tc.wantError && !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "NFC") {
				t.Errorf("Expected an NFC diagnostic, got: %v", resp.Diagnostics.Errors()[0].Detail())
			}
		})
	}
}

// TestAccCertificateAuthorityResource_nameLength pins the 255-character cap
// Fleet enforces on a certificate authority name ("CA name cannot be longer
// than 255 characters").
func TestAccCertificateAuthorityResource_nameLength(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fakeFleetProviderConfig("http://127.0.0.1:1") + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  custom_scep_proxy = {
    name      = %[1]q
    url       = "https://scep.example.test/scep"
    challenge = "challenge-value"
  }
}
`, strings.Repeat("c", 256)),
				ExpectError: regexp.MustCompile(`(?s)at\s+most\s+255`),
			},
		},
	})
}
