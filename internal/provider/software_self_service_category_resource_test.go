package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// selfServiceCategoryMock is an in-memory stand-in for Fleet's self-service
// category endpoints, faithful enough to drive a full Terraform lifecycle:
// fleet-scoped listing that rejects a missing fleet_id with 422, unique names
// per fleet, PATCH rename and DELETE.
type selfServiceCategoryMock struct {
	mu         sync.Mutex
	nextID     int64
	categories map[int64]mockCategory
}

type mockCategory struct {
	ID      int64
	FleetID int64
	Name    string
}

func newSelfServiceCategoryMock() *selfServiceCategoryMock {
	return &selfServiceCategoryMock{
		nextID:     1,
		categories: map[int64]mockCategory{},
	}
}

func (m *selfServiceCategoryMock) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	const basePath = "/api/v1/fleet/software/self_service_categories"

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		m.mu.Lock()
		defer m.mu.Unlock()

		switch {
		case r.URL.Path == basePath && r.Method == http.MethodGet:
			raw := r.URL.Query().Get("fleet_id")
			if raw == "" {
				// Fleet's own behaviour: fleet_id is mandatory on the list
				// endpoint.
				w.WriteHeader(http.StatusUnprocessableEntity)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"message": "Validation Failed",
					"errors":  []map[string]string{{"name": "fleet_id", "reason": "fleet_id is required"}},
				})
				return
			}
			fleetID, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				t.Errorf("Unexpected non-numeric fleet_id %q", raw)
			}

			out := []map[string]interface{}{}
			for _, c := range m.categories {
				if c.FleetID == fleetID {
					out = append(out, m.serialize(c))
				}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"self_service_categories": out})

		case r.URL.Path == basePath && r.Method == http.MethodPost:
			var body struct {
				FleetID *int64 `json:"fleet_id"`
				Name    string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("Failed to decode create body: %v", err)
			}
			if body.FleetID == nil {
				w.WriteHeader(http.StatusUnprocessableEntity)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"message": "Validation Failed",
					"errors":  []map[string]string{{"name": "fleet_id", "reason": "fleet_id is required"}},
				})
				return
			}
			for _, c := range m.categories {
				if c.FleetID == *body.FleetID && strings.EqualFold(c.Name, body.Name) {
					w.WriteHeader(http.StatusConflict)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"message": "Validation Failed",
						"errors":  []map[string]string{{"name": "name", "reason": "category name already exists"}},
					})
					return
				}
			}

			created := mockCategory{ID: m.nextID, FleetID: *body.FleetID, Name: body.Name}
			m.nextID++
			m.categories[created.ID] = created
			json.NewEncoder(w).Encode(map[string]interface{}{"self_service_category": m.serialize(created)})

		case strings.HasPrefix(r.URL.Path, basePath+"/") && r.Method == http.MethodPatch:
			id, ok := m.lookupID(w, r, basePath)
			if !ok {
				return
			}
			var body struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("Failed to decode update body: %v", err)
			}
			updated := m.categories[id]
			updated.Name = body.Name
			m.categories[id] = updated
			json.NewEncoder(w).Encode(map[string]interface{}{"self_service_category": m.serialize(updated)})

		case strings.HasPrefix(r.URL.Path, basePath+"/") && r.Method == http.MethodDelete:
			id, ok := m.lookupID(w, r, basePath)
			if !ok {
				return
			}
			delete(m.categories, id)
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}
}

// lookupID resolves the trailing path segment to a stored category, writing a
// 404 when it is unknown.
func (m *selfServiceCategoryMock) lookupID(w http.ResponseWriter, r *http.Request, basePath string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, basePath+"/"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return 0, false
	}
	if _, ok := m.categories[id]; !ok {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Resource Not Found"})
		return 0, false
	}
	return id, true
}

// serialize emits both the new "fleet_id" and legacy "team_id" keys, matching
// what Fleet 4.90 returns.
func (m *selfServiceCategoryMock) serialize(c mockCategory) map[string]interface{} {
	return map[string]interface{}{
		"id":         c.ID,
		"name":       c.Name,
		"fleet_id":   c.FleetID,
		"team_id":    c.FleetID,
		"created_at": "2026-05-12T18:45:00Z",
		"updated_at": "2026-05-12T18:45:00Z",
	}
}

// TestAccSoftwareSelfServiceCategoryResource_mockLifecycle exercises create,
// in-place rename, composite import and destroy against the mock server.
func TestAccSoftwareSelfServiceCategoryResource_mockLifecycle(t *testing.T) {
	mock := newSelfServiceCategoryMock()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: testAccSelfServiceCategoryMockConfig(server.URL, 3, "💼 Engineering"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_self_service_category.test", "fleet_id", "3"),
					resource.TestCheckResourceAttr("fleetdm_software_self_service_category.test", "name", "💼 Engineering"),
					resource.TestCheckResourceAttr("fleetdm_software_self_service_category.test", "id", "1"),
				),
			},
			// ImportState with the composite fleet_id:id
			{
				ResourceName:      "fleetdm_software_self_service_category.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccSelfServiceCategoryImportID("fleetdm_software_self_service_category.test"),
			},
			// Rename in place — the ID must survive
			{
				Config: testAccSelfServiceCategoryMockConfig(server.URL, 3, "💼 Engineering tools"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_self_service_category.test", "name", "💼 Engineering tools"),
					resource.TestCheckResourceAttr("fleetdm_software_self_service_category.test", "id", "1"),
				),
			},
		},
	})
}

// A fleet_id change is not something Fleet can do in place, so the resource
// must be replaced (new ID) rather than updated.
func TestAccSoftwareSelfServiceCategoryResource_mockFleetIDRequiresReplace(t *testing.T) {
	mock := newSelfServiceCategoryMock()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSelfServiceCategoryMockConfig(server.URL, 3, "🔐 Security"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_self_service_category.test", "id", "1"),
					resource.TestCheckResourceAttr("fleetdm_software_self_service_category.test", "fleet_id", "3"),
				),
			},
			{
				Config: testAccSelfServiceCategoryMockConfig(server.URL, 4, "🔐 Security"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_self_service_category.test", "fleet_id", "4"),
					resource.TestCheckResourceAttr("fleetdm_software_self_service_category.test", "id", "2"),
				),
			},
		},
	})
}

// A category deleted outside Terraform must be dropped from state rather than
// erroring, so the next apply recreates it.
func TestAccSoftwareSelfServiceCategoryResource_mockDeletedOutOfBand(t *testing.T) {
	mock := newSelfServiceCategoryMock()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSelfServiceCategoryMockConfig(server.URL, 3, "🛟 Support"),
				Check:  resource.TestCheckResourceAttr("fleetdm_software_self_service_category.test", "id", "1"),
			},
			{
				PreConfig: func() {
					mock.mu.Lock()
					defer mock.mu.Unlock()
					mock.categories = map[int64]mockCategory{}
				},
				Config: testAccSelfServiceCategoryMockConfig(server.URL, 3, "🛟 Support"),
				Check:  resource.TestCheckResourceAttr("fleetdm_software_self_service_category.test", "id", "2"),
			},
		},
	})
}

func TestAccSoftwareSelfServiceCategoryResource_mockImportIDInvalid(t *testing.T) {
	mock := newSelfServiceCategoryMock()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSelfServiceCategoryMockConfig(server.URL, 3, "🧰 Developer tools"),
			},
			{
				ResourceName:  "fleetdm_software_self_service_category.test",
				ImportState:   true,
				ImportStateId: "1",
				ExpectError:   regexp.MustCompile("Import ID must be in format: fleet_id:id"),
			},
			{
				ResourceName:  "fleetdm_software_self_service_category.test",
				ImportState:   true,
				ImportStateId: "abc:1",
				ExpectError:   regexp.MustCompile("Could not parse Self-Service Category Fleet ID"),
			},
		},
	})
}

// Fleet measures the 255-character name limit in runes
// (utf8.RuneCountInString), so 255 emoji are a valid name even though they are
// four times that many bytes. A byte-counting validator would reject this.
func TestAccSoftwareSelfServiceCategoryResource_mockNameMaxRunes(t *testing.T) {
	mock := newSelfServiceCategoryMock()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	maxRunes := strings.Repeat("🌎", 255)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSelfServiceCategoryMockConfig(server.URL, 3, maxRunes),
				Check:  resource.TestCheckResourceAttr("fleetdm_software_self_service_category.test", "name", maxRunes),
			},
		},
	})
}

// A name over the limit is rejected during validation, before any request is
// made. The step is kept in its own test case so no resource is ever created
// and the post-test destroy has nothing to plan against an invalid config.
func TestAccSoftwareSelfServiceCategoryResource_mockNameTooLong(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccSelfServiceCategoryMockConfig("http://localhost:0", 3, strings.Repeat("a", 256)),
				ExpectError: regexp.MustCompile(`Invalid Attribute Value Length`),
			},
		},
	})
}

// Fleet trims the name server-side with strings.TrimSpace (unicode.IsSpace), so
// a padded or empty name is rejected at plan time rather than applying to a
// value Terraform did not plan.
//
// The non-ASCII cases are the ones that matter: Go's RE2 defines \s as the
// ASCII set [\t\n\f\r ], so a `^\S(.*\S)?$` pattern would accept every entry
// below except the plain-space ones and let Fleet silently trim them.
func TestAccSoftwareSelfServiceCategoryResource_mockNameWhitespaceRejected(t *testing.T) {
	const base = "\U0001F4BC Engineering"

	// Every entry is written with explicit escapes: invisible whitespace in
	// source is unreviewable, and the point of the table is exactly which
	// code points are involved.
	cases := map[string]string{
		"trailing ascii space":      base + " ",
		"leading ascii space":       " " + base,
		"leading tab":               "\t" + base,
		"empty":                     "",
		"only ascii spaces":         "   ",
		"leading vertical tab":      "\u000B" + base,
		"trailing vertical tab":     base + "\u000B",
		"leading form feed":         "\u000C" + base,
		"leading next line":         "\u0085" + base,
		"leading nbsp":              "\u00A0" + base,
		"trailing nbsp":             base + "\u00A0",
		"leading ogham space":       "\u1680" + base,
		"leading line separator":    "\u2028" + base,
		"trailing narrow nbsp":      base + "\u202F",
		"leading ideographic space": "\u3000" + base,
		"only ideographic space":    "\u3000\u3000",
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      testAccSelfServiceCategoryMockConfig("http://localhost:0", 3, value),
						ExpectError: regexp.MustCompile(`leading/trailing whitespace`),
					},
				},
			})
		})
	}
}

// The validator must reject only surrounding whitespace. Fleet's TrimSpace
// leaves interior whitespace alone, so names containing it — including exotic
// spaces — are valid and must still apply.
func TestAccSoftwareSelfServiceCategoryResource_mockInteriorWhitespaceAllowed(t *testing.T) {
	mock := newSelfServiceCategoryMock()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	name := "\U0001F4BC Engineering \u00A0&\u3000Platform"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSelfServiceCategoryMockConfig(server.URL, 3, name),
				Check:  resource.TestCheckResourceAttr("fleetdm_software_self_service_category.test", "name", name),
			},
		},
	})
}

// TestAccSoftwareSelfServiceCategoryResource_basic runs the create/rename/
// import/destroy cycle against a live Fleet instance. Self-service categories
// are Fleet Premium; CI's Fleet runs with --dev_license so the endpoints are
// available.
func TestAccSoftwareSelfServiceCategoryResource_basic(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	fleetName := "tf-acc-test-" + suffix
	// A random suffix keeps the name clear of the defaults Fleet seeds on
	// every new fleet, which would otherwise collide.
	categoryName := "🧰 tf-acc-test-" + suffix
	renamedName := "🔐 tf-acc-test-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: testAccSelfServiceCategoryConfig(fleetName, categoryName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_self_service_category.test", "name", categoryName),
					resource.TestCheckResourceAttrSet("fleetdm_software_self_service_category.test", "id"),
					resource.TestCheckResourceAttrPair(
						"fleetdm_software_self_service_category.test", "fleet_id",
						"fleetdm_fleet.test", "id",
					),
				),
			},
			// ImportState
			{
				ResourceName:      "fleetdm_software_self_service_category.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccSelfServiceCategoryImportID("fleetdm_software_self_service_category.test"),
			},
			// Rename in place
			{
				Config: testAccSelfServiceCategoryConfig(fleetName, renamedName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_self_service_category.test", "name", renamedName),
				),
			},
		},
	})
}

// testAccSelfServiceCategoryImportID builds the "fleet_id:id" composite import
// ID from the resource's current state.
func testAccSelfServiceCategoryImportID(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", resourceName)
		}
		return fmt.Sprintf("%s:%s", rs.Primary.Attributes["fleet_id"], rs.Primary.Attributes["id"]), nil
	}
}

// hclQuote renders s as an HCL quoted-string literal.
//
// fmt's %q cannot be used for names carrying exotic whitespace: it emits Go
// escape sequences, and HCL rejects the ones Go has but HCL does not (`\v`,
// `\f`, `\a`, `\b`) with "The symbol \"v\" is not a valid escape sequence
// selector". Everything outside printable ASCII is therefore emitted as
// \uNNNN / \UNNNNNNNN, which HCL does understand.
func hclQuote(s string) string {
	// ${ and %{ open an HCL template interpolation; doubling the sigil is the
	// documented way to keep it literal.
	s = strings.ReplaceAll(s, "${", "$${")
	s = strings.ReplaceAll(s, "%{", "%%{")

	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '\\' || r == '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r >= 0x20 && r <= 0x7E:
			b.WriteRune(r)
		case r > 0xFFFF:
			fmt.Fprintf(&b, `\U%08X`, r)
		default:
			fmt.Fprintf(&b, `\u%04X`, r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func testAccSelfServiceCategoryMockConfig(serverURL string, fleetID int, name string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_self_service_category" "test" {
  fleet_id = %[2]d
  name     = %[3]s
}
`, serverURL, fleetID, hclQuote(name))
}

func testAccSelfServiceCategoryConfig(fleetName, categoryName string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %[1]q
}

resource "fleetdm_software_self_service_category" "test" {
  fleet_id = fleetdm_fleet.test.id
  name     = %[2]s
}
`, fleetName, hclQuote(categoryName))
}
