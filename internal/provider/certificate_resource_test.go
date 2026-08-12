package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
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
)

// fakeCertificateTemplate is one stored template in the fake server.
type fakeCertificateTemplate struct {
	ID                     int64
	Name                   string
	FleetID                int64
	CertificateAuthorityID int64
	SubjectName            string
	SubjectAlternativeName string
}

// fakeCertificatesServer is a stand-in for Fleet's /certificates endpoints. It
// reproduces the three behaviours the provider has to cope with:
//
//   - the create response echoes only what was sent, so certificate authority
//     name/type and created_at are available from the read routes only;
//   - no response carries the template's fleet, so fleet_id is write-only;
//   - a blank subject alternative name is stored as absent.
type fakeCertificatesServer struct {
	mu        sync.Mutex
	nextID    int64
	templates map[int64]*fakeCertificateTemplate

	// createBodies records every create payload so tests can assert what
	// actually reached Fleet — the only way to observe fleet_id, which no
	// response echoes back.
	createBodies []map[string]any

	// caName and caType are what the read routes report for any referenced
	// certificate authority. rejectCAType makes create fail the way Fleet does
	// for a non-SCEP authority.
	caName       string
	caType       string
	rejectCAType bool

	// failNextGetByID makes the next GET /certificates/{id} fail, so tests can
	// exercise a create whose read-back does not come back. It clears itself.
	failNextGetByID bool

	// getByIDCalls counts GET /certificates/{id} requests.
	getByIDCalls int
}

func newFakeCertificatesServer() (*httptest.Server, *fakeCertificatesServer) {
	f := &fakeCertificatesServer{
		nextID:    1,
		templates: map[int64]*fakeCertificateTemplate{},
		caName:    "SCEP_TEST",
		caType:    "custom_scep_proxy",
	}
	return httptest.NewServer(f), f
}

// wipe removes every stored template, simulating out-of-band deletion.
func (f *fakeCertificatesServer) wipe() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.templates = map[int64]*fakeCertificateTemplate{}
}

// recordedCreateBodies returns a copy of the create payloads seen so far.
func (f *fakeCertificatesServer) recordedCreateBodies() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.createBodies...)
}

const fakeCertificatesBase = "/api/v1/fleet/certificates"

func (f *fakeCertificatesServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.URL.Path == fakeCertificatesBase && r.Method == http.MethodPost:
		f.create(w, r)
	case r.URL.Path == fakeCertificatesBase && r.Method == http.MethodGet:
		f.list(w, r)
	case strings.HasPrefix(r.URL.Path, fakeCertificatesBase+"/"):
		id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, fakeCertificatesBase+"/"), 10, 64)
		if err != nil {
			f.writeError(w, http.StatusNotFound, "bad id")
			return
		}
		switch r.Method {
		case http.MethodGet:
			f.get(w, id)
		case http.MethodDelete:
			f.delete(w, id)
		default:
			// Fleet has no PATCH or PUT for certificate templates.
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	default:
		f.writeError(w, http.StatusNotFound, "unexpected route "+r.Method+" "+r.URL.Path)
	}
}

func (f *fakeCertificatesServer) writeError(w http.ResponseWriter, status int, reason string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"message": "error",
		"errors":  []map[string]string{{"name": "base", "reason": reason}},
	})
}

func (f *fakeCertificatesServer) create(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		f.writeError(w, http.StatusBadRequest, "malformed body")
		return
	}
	f.createBodies = append(f.createBodies, body)

	if f.rejectCAType {
		f.writeError(w, http.StatusBadRequest, "Currently, only the custom_scep_proxy certificate authority is supported.")
		return
	}

	str := func(key string) string {
		v, _ := body[key].(string)
		return v
	}
	num := func(key string) int64 {
		v, _ := body[key].(float64)
		return int64(v)
	}

	name := str("name")
	fleetID := num("fleet_id")

	// Fleet's uniqueness is per (fleet, name).
	for _, existing := range f.templates {
		if existing.FleetID == fleetID && existing.Name == name {
			f.writeError(w, http.StatusConflict, fmt.Sprintf("CertificateTemplate %q already exists", name))
			return
		}
	}

	san := str("subject_alternative_name")
	// Fleet stores a blank subject alternative name as absent.
	if strings.TrimSpace(san) == "" {
		san = ""
	}

	template := &fakeCertificateTemplate{
		ID:                     f.nextID,
		Name:                   name,
		FleetID:                fleetID,
		CertificateAuthorityID: num("certificate_authority_id"),
		SubjectName:            str("subject_name"),
		SubjectAlternativeName: san,
	}
	f.templates[template.ID] = template
	f.nextID++

	// The create response echoes only what was sent: no certificate authority
	// name or type, no created_at, and never the fleet.
	response := map[string]any{
		"id":                       template.ID,
		"name":                     template.Name,
		"certificate_authority_id": template.CertificateAuthorityID,
		"subject_name":             template.SubjectName,
	}
	if template.SubjectAlternativeName != "" {
		response["subject_alternative_name"] = template.SubjectAlternativeName
	}
	_ = json.NewEncoder(w).Encode(response)
}

// summary renders a template the way the list route does.
func (f *fakeCertificatesServer) summary(template *fakeCertificateTemplate) map[string]any {
	out := map[string]any{
		"id":                         template.ID,
		"name":                       template.Name,
		"subject_name":               template.SubjectName,
		"certificate_authority_id":   template.CertificateAuthorityID,
		"certificate_authority_name": f.caName,
		"created_at":                 "2026-08-12T09:10:34Z",
	}
	if template.SubjectAlternativeName != "" {
		out["subject_alternative_name"] = template.SubjectAlternativeName
	}
	return out
}

func (f *fakeCertificatesServer) get(w http.ResponseWriter, id int64) {
	f.getByIDCalls++
	if f.failNextGetByID {
		f.failNextGetByID = false
		f.writeError(w, http.StatusInternalServerError, "transient read failure")
		return
	}

	template, ok := f.templates[id]
	if !ok {
		f.writeError(w, http.StatusNotFound, fmt.Sprintf("CertificateTemplate %d was not found in the datastore", id))
		return
	}
	// The detail route adds the certificate authority type.
	detail := f.summary(template)
	detail["certificate_authority_type"] = f.caType
	_ = json.NewEncoder(w).Encode(map[string]any{"certificate": detail})
}

func (f *fakeCertificatesServer) list(w http.ResponseWriter, r *http.Request) {
	// An absent fleet_id selects fleet 0, not every fleet.
	fleetID, _ := strconv.ParseInt(r.URL.Query().Get("fleet_id"), 10, 64)

	ids := make([]int64, 0, len(f.templates))
	for id, template := range f.templates {
		if template.FleetID == fleetID {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	certificates := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		certificates = append(certificates, f.summary(f.templates[id]))
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"certificates": certificates,
		"meta":         map[string]any{"has_next_results": false, "has_previous_results": false},
	})
}

func (f *fakeCertificatesServer) delete(w http.ResponseWriter, id int64) {
	if _, ok := f.templates[id]; !ok {
		// Fleet looks the template up before authorizing, so a missing row
		// surfaces as a 500 "forbidden" rather than a 404.
		f.writeError(w, http.StatusInternalServerError, "forbidden")
		return
	}
	delete(f.templates, id)
	_, _ = w.Write([]byte(`{}`))
}

func testCertificateProviderBlock(serverURL string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %q
  api_key        = "test-token"
}
`, serverURL)
}

// TestCertificateResource_lifecycle covers create against the no-fleet default,
// the computed attributes that only the read route supplies, and import through
// the composite ID.
func TestCertificateResource_lifecycle(t *testing.T) {
	server, fake := newFakeCertificatesServer()
	defer server.Close()

	config := testCertificateProviderBlock(server.URL) + `
resource "fleetdm_certificate" "test" {
  name                     = "Android_WiFi"
  certificate_authority_id = 3
  subject_name             = "CN=$FLEET_VAR_HOST_HARDWARE_SERIAL,O=Example"
  subject_alternative_name = "DNS=host.example.com"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "id", "1"),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "fleet_id", "0"),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "name", "Android_WiFi"),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "certificate_authority_id", "3"),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "subject_name", "CN=$FLEET_VAR_HOST_HARDWARE_SERIAL,O=Example"),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "subject_alternative_name", "DNS=host.example.com"),
					// Only the read route supplies these, so their presence
					// proves Create followed up with a GET.
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "certificate_authority_name", "SCEP_TEST"),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "certificate_authority_type", "custom_scep_proxy"),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "created_at", "2026-08-12T09:10:34Z"),
					// fleet_id must be on the wire even when it is the default.
					func(*terraform.State) error {
						bodies := fake.recordedCreateBodies()
						if len(bodies) != 1 {
							return fmt.Errorf("expected 1 create call, got %d", len(bodies))
						}
						if got, ok := bodies[0]["fleet_id"]; !ok || got != float64(0) {
							return fmt.Errorf("expected fleet_id 0 in the create body, got %v", bodies[0])
						}
						return nil
					},
				),
			},
			{
				// A no-op plan proves nothing drifts on refresh — in particular
				// that fleet_id survives a read that never returns it.
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "fleetdm_certificate.test",
				ImportState:       true,
				ImportStateId:     "0:1",
				ImportStateVerify: true,
			},
		},
	})
}

// TestCertificateResource_everyAttributeReplaces checks each configurable
// attribute forces replacement. Fleet has no update endpoint, so an in-place
// update would hit the resource's unreachable Update and fail.
func TestCertificateResource_everyAttributeReplaces(t *testing.T) {
	server, fake := newFakeCertificatesServer()
	defer server.Close()

	config := func(fleetID int, name string, caID int, subject, san string) string {
		return testCertificateProviderBlock(server.URL) + fmt.Sprintf(`
resource "fleetdm_certificate" "test" {
  fleet_id                 = %d
  name                     = %q
  certificate_authority_id = %d
  subject_name             = %q
  subject_alternative_name = %q
}
`, fleetID, name, caID, subject, san)
	}

	replaced := resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{
			plancheck.ExpectResourceAction("fleetdm_certificate.test", plancheck.ResourceActionReplace),
		},
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(0, "Base", 3, "CN=base", "DNS=base.example.com"),
				Check:  resource.TestCheckResourceAttr("fleetdm_certificate.test", "id", "1"),
			},
			{
				Config:           config(0, "Renamed", 3, "CN=base", "DNS=base.example.com"),
				ConfigPlanChecks: replaced,
				Check:            resource.TestCheckResourceAttr("fleetdm_certificate.test", "name", "Renamed"),
			},
			{
				Config:           config(0, "Renamed", 9, "CN=base", "DNS=base.example.com"),
				ConfigPlanChecks: replaced,
				Check:            resource.TestCheckResourceAttr("fleetdm_certificate.test", "certificate_authority_id", "9"),
			},
			{
				Config:           config(0, "Renamed", 9, "CN=changed", "DNS=base.example.com"),
				ConfigPlanChecks: replaced,
				Check:            resource.TestCheckResourceAttr("fleetdm_certificate.test", "subject_name", "CN=changed"),
			},
			{
				Config:           config(0, "Renamed", 9, "CN=changed", "DNS=changed.example.com"),
				ConfigPlanChecks: replaced,
				Check:            resource.TestCheckResourceAttr("fleetdm_certificate.test", "subject_alternative_name", "DNS=changed.example.com"),
			},
			{
				Config:           config(7, "Renamed", 9, "CN=changed", "DNS=changed.example.com"),
				ConfigPlanChecks: replaced,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "fleet_id", "7"),
					// No Fleet response echoes the fleet, so the recorded
					// request body is the only proof the new value actually
					// reached Fleet rather than just sitting in state.
					func(*terraform.State) error {
						bodies := fake.recordedCreateBodies()
						if len(bodies) == 0 {
							return fmt.Errorf("expected at least one create call")
						}
						last := bodies[len(bodies)-1]
						if got := last["fleet_id"]; got != float64(7) {
							return fmt.Errorf("expected fleet_id 7 in the last create body, got %v", last)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestCertificateResource_clearingSANReplaces checks that removing the optional
// subject alternative name is a replacement and lands as null, not "".
func TestCertificateResource_clearingSANReplaces(t *testing.T) {
	server, _ := newFakeCertificatesServer()
	defer server.Close()

	withSAN := testCertificateProviderBlock(server.URL) + `
resource "fleetdm_certificate" "test" {
  name                     = "SANTest"
  certificate_authority_id = 3
  subject_name             = "CN=x"
  subject_alternative_name = "DNS=x.example.com"
}
`
	withoutSAN := testCertificateProviderBlock(server.URL) + `
resource "fleetdm_certificate" "test" {
  name                     = "SANTest"
  certificate_authority_id = 3
  subject_name             = "CN=x"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: withSAN,
				Check:  resource.TestCheckResourceAttr("fleetdm_certificate.test", "subject_alternative_name", "DNS=x.example.com"),
			},
			{
				Config: withoutSAN,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_certificate.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.TestCheckNoResourceAttr("fleetdm_certificate.test", "subject_alternative_name"),
			},
			{
				Config: withoutSAN,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// TestCertificateResource_removedOutOfBand checks a template deleted in Fleet is
// dropped from state on refresh rather than erroring.
func TestCertificateResource_removedOutOfBand(t *testing.T) {
	server, fake := newFakeCertificatesServer()
	defer server.Close()

	config := testCertificateProviderBlock(server.URL) + `
resource "fleetdm_certificate" "test" {
  name                     = "Gone"
  certificate_authority_id = 3
  subject_name             = "CN=gone"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("fleetdm_certificate.test", "id", "1"),
			},
			{
				// A RefreshState step must not carry a Config.
				PreConfig:          func() { fake.wipe() },
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestCertificateResource_createSurvivesFailedReadBack checks that a create
// whose follow-up read fails still writes state.
//
// The template exists in Fleet by then, so erroring out of Create would orphan
// it: Terraform would keep no record, and the next apply would fail on the
// duplicate name with no way to reach the template except an out-of-band delete
// or an import. The three read-only attributes are left null instead and the
// next refresh fills them in.
func TestCertificateResource_createSurvivesFailedReadBack(t *testing.T) {
	server, fake := newFakeCertificatesServer()
	defer server.Close()
	fake.failNextGetByID = true

	config := testCertificateProviderBlock(server.URL) + `
resource "fleetdm_certificate" "test" {
  name                     = "ReadBackFails"
  certificate_authority_id = 3
  subject_name             = "CN=x"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					// The apply succeeded and the template is managed.
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "id", "1"),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "name", "ReadBackFails"),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "subject_name", "CN=x"),
					// The read-only attributes the create response never carries
					// are null rather than "" or unknown.
					resource.TestCheckNoResourceAttr("fleetdm_certificate.test", "certificate_authority_name"),
					resource.TestCheckNoResourceAttr("fleetdm_certificate.test", "certificate_authority_type"),
					resource.TestCheckNoResourceAttr("fleetdm_certificate.test", "created_at"),
					// Exactly one template exists: the create was not retried
					// and nothing was orphaned.
					func(*terraform.State) error {
						if bodies := fake.recordedCreateBodies(); len(bodies) != 1 {
							return fmt.Errorf("expected exactly 1 create call, got %d", len(bodies))
						}
						return nil
					},
				),
			},
			{
				// The next refresh backfills them, with no diff to apply.
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "certificate_authority_name", "SCEP_TEST"),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "certificate_authority_type", "custom_scep_proxy"),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "created_at", "2026-08-12T09:10:34Z"),
				),
			},
		},
	})
}

// TestCertificateResource_createRejectsNonSCEPAuthority checks Fleet's save-time
// certificate authority type check surfaces to the practitioner verbatim.
func TestCertificateResource_createRejectsNonSCEPAuthority(t *testing.T) {
	server, fake := newFakeCertificatesServer()
	defer server.Close()
	fake.rejectCAType = true

	config := testCertificateProviderBlock(server.URL) + `
resource "fleetdm_certificate" "test" {
  name                     = "BadCA"
  certificate_authority_id = 4
  subject_name             = "CN=x"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`(?s)Currently,\s+only\s+the\s+custom_scep_proxy\s+certificate\s+authority\s+is\s+supported`),
			},
		},
	})
}

// TestCertificateResource_validationErrors covers the plan-time guards, each of
// which mirrors a rule Fleet enforces server-side.
func TestCertificateResource_validationErrors(t *testing.T) {
	server, _ := newFakeCertificatesServer()
	defer server.Close()

	tests := []struct {
		name        string
		body        string
		expectError *regexp.Regexp
	}{
		{
			name: "name containing a dot",
			body: `
  name                     = "cert.example.com"
  certificate_authority_id = 3
  subject_name             = "CN=x"`,
			expectError: regexp.MustCompile(`(?s)must\s+contain\s+only\s+letters,\s+numbers,\s+spaces,\s+dashes\s+and\s+underscores`),
		},
		{
			name: "name too long",
			body: fmt.Sprintf(`
  name                     = %q
  certificate_authority_id = 3
  subject_name             = "CN=x"`, strings.Repeat("a", 256)),
			expectError: regexp.MustCompile(`(?s)string\s+length\s+must\s+be\s+at\s+most\s+255`),
		},
		{
			name: "blank subject name",
			body: `
  name                     = "BlankSubject"
  certificate_authority_id = 3
  subject_name             = "   "`,
			expectError: regexp.MustCompile(`(?s)must\s+not\s+be\s+empty\s+or\s+consist\s+only\s+of\s+whitespace`),
		},
		{
			name: "blank subject alternative name",
			body: `
  name                     = "BlankSAN"
  certificate_authority_id = 3
  subject_name             = "CN=x"
  subject_alternative_name = " "`,
			expectError: regexp.MustCompile(`(?s)must\s+not\s+be\s+empty\s+or\s+consist\s+only\s+of\s+whitespace`),
		},
		{
			name: "subject alternative name too long",
			body: fmt.Sprintf(`
  name                     = "LongSAN"
  certificate_authority_id = 3
  subject_name             = "CN=x"
  subject_alternative_name = %q`, "DNS="+strings.Repeat("a", 4100)),
			expectError: regexp.MustCompile(`(?s)string\s+length\s+must\s+be\s+at\s+most\s+4096`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: testCertificateProviderBlock(server.URL) + `
resource "fleetdm_certificate" "test" {` + tt.body + `
}
`,
						ExpectError: tt.expectError,
					},
				},
			})
		})
	}
}

// TestCertificateResource_importIDValidation checks the composite import ID is
// rejected when it is not exactly "fleet_id:id".
func TestCertificateResource_importIDValidation(t *testing.T) {
	server, _ := newFakeCertificatesServer()
	defer server.Close()

	config := testCertificateProviderBlock(server.URL) + `
resource "fleetdm_certificate" "test" {
  name                     = "ImportTest"
  certificate_authority_id = 3
  subject_name             = "CN=x"
}
`

	tests := []struct {
		name        string
		importID    string
		expectError *regexp.Regexp
	}{
		{
			name:        "bare id",
			importID:    "1",
			expectError: regexp.MustCompile(`(?s)Import\s+ID\s+must\s+be\s+in\s+format:\s+fleet_id:id`),
		},
		{
			name:        "too many parts",
			importID:    "0:1:2",
			expectError: regexp.MustCompile(`(?s)Import\s+ID\s+must\s+be\s+in\s+format:\s+fleet_id:id`),
		},
		{
			name:        "non-numeric fleet",
			importID:    "notanumber:1",
			expectError: regexp.MustCompile(`(?s)Could\s+not\s+parse\s+Certificate\s+Template\s+Fleet\s+ID`),
		},
		{
			name:        "non-numeric template",
			importID:    "0:notanumber",
			expectError: regexp.MustCompile(`(?s)Could\s+not\s+parse\s+Certificate\s+Template\s+ID`),
		},
		{
			// Nothing in the subsequent Read can contradict the fleet the caller
			// supplied, so a wrong one has to be rejected here. Accepting it
			// would make the next plan replace the template, re-issuing
			// certificates to every host in the fleet.
			name:        "wrong fleet",
			importID:    "9:1",
			expectError: regexp.MustCompile(`(?s)Certificate\s+template\s+1\s+is\s+not\s+on\s+fleet\s+9`),
		},
		{
			name:        "template does not exist",
			importID:    "0:404",
			expectError: regexp.MustCompile(`(?s)Certificate\s+template\s+404\s+is\s+not\s+on\s+fleet\s+0`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: config},
					{
						ResourceName:  "fleetdm_certificate.test",
						ImportState:   true,
						ImportStateId: tt.importID,
						ExpectError:   tt.expectError,
					},
				},
			})
		})
	}
}

// TestCertificateResource_nonBlankStringValidator exercises the validator
// directly, including the Unicode whitespace a \S regex would let through.
func TestCertificateResource_nonBlankStringValidator(t *testing.T) {
	valid := []string{"CN=x", " CN=padded ", "DNS=a.example.com"}
	// U+00A0 NO-BREAK SPACE, U+2028 LINE SEPARATOR and U+3000 IDEOGRAPHIC SPACE
	// are all unicode.IsSpace but outside Go's RE2 \s class.
	invalid := []string{"", " ", "\t", " ", " ", "　", "  \t "}

	for _, value := range valid {
		var resp validator.StringResponse
		nonBlankStringValidator{}.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("subject_name"),
			ConfigValue: types.StringValue(value),
		}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("Expected %q to be accepted, got %v", value, resp.Diagnostics.Errors())
		}
	}

	for _, value := range invalid {
		var resp validator.StringResponse
		nonBlankStringValidator{}.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("subject_name"),
			ConfigValue: types.StringValue(value),
		}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Errorf("Expected %q to be rejected as blank", value)
		}
	}

	// Null and unknown must pass through untouched — an optional attribute that
	// is unset is not a blank value.
	for _, value := range []types.String{types.StringNull(), types.StringUnknown()} {
		var resp validator.StringResponse
		nonBlankStringValidator{}.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("subject_alternative_name"),
			ConfigValue: value,
		}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("Expected %v to be accepted, got %v", value, resp.Diagnostics.Errors())
		}
	}
}

// TestCertificatesDataSource_readsOneFleet checks the data source is scoped to a
// single fleet and reports the list route's fields.
func TestCertificatesDataSource_readsOneFleet(t *testing.T) {
	server, _ := newFakeCertificatesServer()
	defer server.Close()

	config := testCertificateProviderBlock(server.URL) + `
resource "fleetdm_certificate" "no_fleet" {
  name                     = "NoFleet"
  certificate_authority_id = 3
  subject_name             = "CN=nofleet"
}

resource "fleetdm_certificate" "in_fleet" {
  fleet_id                 = 7
  name                     = "InFleet"
  certificate_authority_id = 3
  subject_name             = "CN=infleet"
  subject_alternative_name = "DNS=infleet.example.com"
}

data "fleetdm_certificates" "default_fleet" {
  depends_on = [fleetdm_certificate.no_fleet, fleetdm_certificate.in_fleet]
}

data "fleetdm_certificates" "fleet_seven" {
  fleet_id   = 7
  depends_on = [fleetdm_certificate.no_fleet, fleetdm_certificate.in_fleet]
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Omitting fleet_id reads fleet 0 only, never every fleet.
					resource.TestCheckResourceAttr("data.fleetdm_certificates.default_fleet", "certificates.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_certificates.default_fleet", "certificates.0.name", "NoFleet"),
					resource.TestCheckNoResourceAttr("data.fleetdm_certificates.default_fleet", "certificates.0.subject_alternative_name"),
					resource.TestCheckResourceAttr("data.fleetdm_certificates.default_fleet", "certificates.0.certificate_authority_name", "SCEP_TEST"),

					resource.TestCheckResourceAttr("data.fleetdm_certificates.fleet_seven", "certificates.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_certificates.fleet_seven", "certificates.0.name", "InFleet"),
					resource.TestCheckResourceAttr("data.fleetdm_certificates.fleet_seven", "certificates.0.subject_alternative_name", "DNS=infleet.example.com"),
					resource.TestCheckResourceAttr("data.fleetdm_certificates.fleet_seven", "certificates.0.subject_name", "CN=infleet"),
				),
			},
		},
	})
}
