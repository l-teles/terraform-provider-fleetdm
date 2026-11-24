package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeVPPTokenFleet mocks the App Store app endpoints with Fleet 4.95's
// per-app VPP token: the add body's vpp_token_id is recorded and echoed as
// app_store_app.vpp_token on the title (server/service/software_titles.go).
type fakeVPPTokenFleet struct {
	t   *testing.T
	srv *httptest.Server

	mu           sync.Mutex
	nextTitleID  int
	addBodies    []string
	titleTokens  map[int]*int64
	defaultToken *int64 // token Fleet picks when the add body omits one
	omitEcho     bool   // emulate Fleet < 4.95, which has no vpp_token on the title
	version      string // reported by GET /version; "" makes it fail
	// echoAfterUpdate turns the token echo on once an update arrives, so state
	// can hold null while Fleet holds a token.
	echoAfterUpdate bool
}

func newFakeVPPTokenFleet(t *testing.T) *fakeVPPTokenFleet {
	f := &fakeVPPTokenFleet{t: t, nextTitleID: 100, titleTokens: map[int]*int64{}, version: "4.95.0"}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeVPPTokenFleet) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	const titles = "/api/v1/fleet/software/titles/"

	switch {
	case r.URL.Path == "/api/v1/fleet/version" && r.Method == http.MethodGet:
		if f.version == "" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"version": f.version})

	case strings.HasPrefix(r.URL.Path, titles) && strings.HasSuffix(r.URL.Path, "/app_store_app") && r.Method == http.MethodPatch:
		if f.echoAfterUpdate {
			f.omitEcho = false
		}
		w.WriteHeader(http.StatusOK)

	case r.URL.Path == "/api/v1/fleet/software/app_store_apps" && r.Method == http.MethodPost:
		body, _ := io.ReadAll(r.Body)
		f.addBodies = append(f.addBodies, string(body))
		var req struct {
			VPPTokenID *int64 `json:"vpp_token_id"`
		}
		_ = json.Unmarshal(body, &req)
		id := f.nextTitleID
		f.nextTitleID++
		f.titleTokens[id] = req.VPPTokenID
		if req.VPPTokenID == nil {
			f.titleTokens[id] = f.defaultToken
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"software_title_id": id})

	case strings.HasPrefix(r.URL.Path, titles) && r.Method == http.MethodGet:
		id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, titles))
		token, ok := f.titleTokens[id]
		if err != nil || !ok {
			http.NotFound(w, r)
			return
		}
		app := map[string]any{"app_store_id": "1", "platform": "ios", "self_service": false, "latest_version": "1.0"}
		if !f.omitEcho {
			app["vpp_token"] = nil
			if token != nil {
				app["vpp_token"] = map[string]any{"id": *token, "org_name": "Example Org", "location": "Example Unit", "country_code": "us"}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"software_title": map[string]any{
			"id": id, "name": "Example App", "display_name": "", "source": "ios_apps", "app_store_app": app,
		}})

	case strings.HasPrefix(r.URL.Path, titles) && strings.HasSuffix(r.URL.Path, "/available_for_install") && r.Method == http.MethodDelete:
		id, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, titles), "/available_for_install"))
		delete(f.titleTokens, id)
		w.WriteHeader(http.StatusNoContent)

	case r.URL.Path == "/api/v1/fleet/global/policies" ||
		(strings.HasPrefix(r.URL.Path, "/api/v1/fleet/fleets/") && strings.HasSuffix(r.URL.Path, "/policies")):
		_ = json.NewEncoder(w).Encode(map[string]any{"policies": []any{}})

	default:
		f.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}
}

func (f *fakeVPPTokenFleet) lastAddBody() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.addBodies) == 0 {
		return ""
	}
	return f.addBodies[len(f.addBodies)-1]
}

func (f *fakeVPPTokenFleet) config(platform, extra string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_app_store_app" "test" {
  app_store_id = "1"
  team_id      = 5
  platform     = %[2]q
%[3]s
}
`, f.srv.URL, platform, extra)
}

// expectAddBody asserts the exact JSON body of the most recent add request.
func (f *fakeVPPTokenFleet) expectAddBody(want string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if got := f.lastAddBody(); got != want {
			return fmt.Errorf("add body mismatch:\n got: %s\nwant: %s", got, want)
		}
		return nil
	}
}

// TestAccSoftwareAppStoreAppResource_vppTokenConfigured covers a configured
// vpp_token_id: sent on add, a change replaces the app, and removing it from
// configuration keeps the recorded token without replacing.
func TestAccSoftwareAppStoreAppResource_vppTokenConfigured(t *testing.T) {
	f := newFakeVPPTokenFleet(t)
	const addr = "fleetdm_software_app_store_app.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: f.config("ios", "  vpp_token_id = 3"),
				Check: resource.ComposeAggregateTestCheckFunc(
					f.expectAddBody(`{"app_store_id":"1","team_id":5,"platform":"ios","vpp_token_id":3}`),
					resource.TestCheckResourceAttr(addr, "vpp_token_id", "3"),
					resource.TestCheckResourceAttr(addr, "title_id", "100"),
				),
			},
			{
				Config: f.config("ios", "  vpp_token_id = 4"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(addr, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					f.expectAddBody(`{"app_store_id":"1","team_id":5,"platform":"ios","vpp_token_id":4}`),
					resource.TestCheckResourceAttr(addr, "vpp_token_id", "4"),
					resource.TestCheckResourceAttr(addr, "title_id", "101"),
				),
			},
			{
				Config: f.config("ios", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(addr, plancheck.ResourceActionNoop),
					},
				},
				Check: resource.TestCheckResourceAttr(addr, "vpp_token_id", "4"),
			},
		},
	})
}

// TestAccSoftwareAppStoreAppResource_vppTokenOmitted covers an omitted
// vpp_token_id: the add body has no key, the token Fleet picked is recorded,
// and the next plan is empty.
func TestAccSoftwareAppStoreAppResource_vppTokenOmitted(t *testing.T) {
	f := newFakeVPPTokenFleet(t)
	f.defaultToken = new(int64(7))
	const addr = "fleetdm_software_app_store_app.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: f.config("ios", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					f.expectAddBody(`{"app_store_id":"1","team_id":5,"platform":"ios"}`),
					resource.TestCheckResourceAttr(addr, "vpp_token_id", "7"),
				),
			},
			{
				Config: f.config("ios", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// TestAccSoftwareAppStoreAppResource_vppTokenNoEcho covers Fleet before 4.95,
// which reports no token: an omitted token stays null, and adding one later
// plans an in-place update (not a replacement) that fails with a named error
// instead of being silently ignored.
func TestAccSoftwareAppStoreAppResource_vppTokenNoEcho(t *testing.T) {
	f := newFakeVPPTokenFleet(t)
	f.omitEcho = true
	f.version = "4.94.1"
	const addr = "fleetdm_software_app_store_app.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: f.config("ios", ""),
				Check:  resource.TestCheckNoResourceAttr(addr, "vpp_token_id"),
			},
			{
				Config: f.config("ios", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: f.config("ios", "  vpp_token_id = 3"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				ExpectError: regexp.MustCompile(`vpp_token_id requires Fleet 4\.95\.0 or later`),
			},
		},
	})
	if n := len(f.addBodies); n != 1 {
		t.Errorf("expected exactly one add request (no replacement), got %d", n)
	}
}

// TestAccSoftwareAppStoreAppResource_vppTokenOldFleetCreate checks a configured
// token is refused on Fleet before 4.95 before anything is added.
func TestAccSoftwareAppStoreAppResource_vppTokenOldFleetCreate(t *testing.T) {
	f := newFakeVPPTokenFleet(t)
	f.version = "4.94.1"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      f.config("ios", "  vpp_token_id = 3"),
				ExpectError: regexp.MustCompile(`vpp_token_id requires Fleet 4\.95\.0 or later`),
			},
		},
	})
	if body := f.lastAddBody(); body != "" {
		t.Errorf("no add request expected on Fleet < 4.95, got %s", body)
	}
}

// TestAccSoftwareAppStoreAppResource_vppTokenAndroidRejected checks the
// plan-time error mirroring Fleet's "vpp_token_id isn't supported for Android
// apps" (ee/server/service/vpp.go).
func TestAccSoftwareAppStoreAppResource_vppTokenAndroidRejected(t *testing.T) {
	f := newFakeVPPTokenFleet(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      f.config("android", "  vpp_token_id = 3"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`VPP token not supported for Android apps`),
			},
		},
	})
	if body := f.lastAddBody(); body != "" {
		t.Errorf("no add request expected, got %s", body)
	}
}

// TestAccSoftwareAppStoreAppResource_vppTokenUnknownReplaces resolves the token
// from another resource, so it's unknown at plan time: changing it must plan a
// replacement up front rather than an update that turns into one later.
func TestAccSoftwareAppStoreAppResource_vppTokenUnknownReplaces(t *testing.T) {
	f := newFakeVPPTokenFleet(t)
	const addr = "fleetdm_software_app_store_app.test"
	cfg := func(token int) string {
		return fmt.Sprintf(`
resource "terraform_data" "token" {
  input = %d
}
`, token) + f.config("ios", "  vpp_token_id = terraform_data.token.output")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		ExternalProviders:        map[string]resource.ExternalProvider{"terraform": {Source: "terraform.io/builtin/terraform"}},
		Steps: []resource.TestStep{
			{
				Config: cfg(3),
				Check:  resource.TestCheckResourceAttr(addr, "vpp_token_id", "3"),
			},
			{
				Config: cfg(4),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.TestCheckResourceAttr(addr, "vpp_token_id", "4"),
			},
		},
	})
}

// TestAccSoftwareAppStoreAppResource_vppTokenUnknownVersion fails closed when the
// Fleet version can't be read, since older Fleet would ignore the token.
func TestAccSoftwareAppStoreAppResource_vppTokenUnknownVersion(t *testing.T) {
	f := newFakeVPPTokenFleet(t)
	f.version = ""

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      f.config("ios", "  vpp_token_id = 3"),
				ExpectError: regexp.MustCompile(`(?s)Fleet server version could not\s+be\s+read`),
			},
		},
	})
	if body := f.lastAddBody(); body != "" {
		t.Errorf("no add request expected when the version is unknown, got %s", body)
	}
}

// TestAccSoftwareAppStoreAppResource_vppTokenMismatchOnUpdate covers state with
// no token while Fleet holds a different one: the update must record Fleet's
// token and fail, and the next plan replaces the app.
// When the read-back after an update still reports no token, the configured ID
// is unverified, so Update errors and leaves state without it.
func TestAccSoftwareAppStoreAppResource_vppTokenMissingAfterUpdate(t *testing.T) {
	f := newFakeVPPTokenFleet(t)
	seven := int64(7)
	f.defaultToken = &seven
	f.omitEcho = true
	const addr = "fleetdm_software_app_store_app.test"
	cfg := f.config("ios", "  vpp_token_id = 3")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: f.config("ios", ""),
				Check:  resource.TestCheckNoResourceAttr(addr, "vpp_token_id"),
			},
			{
				Config:      cfg,
				ExpectError: regexp.MustCompile(`(?s)Fleet did not report\s+which VPP token`),
			},
			{
				// State still has no token, so the plan keeps trying instead of going quiet.
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccSoftwareAppStoreAppResource_vppTokenMismatchOnUpdate(t *testing.T) {
	f := newFakeVPPTokenFleet(t)
	seven := int64(7)
	f.defaultToken = &seven
	f.omitEcho = true
	f.echoAfterUpdate = true
	const addr = "fleetdm_software_app_store_app.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: f.config("ios", ""),
				Check:  resource.TestCheckNoResourceAttr(addr, "vpp_token_id"),
			},
			{
				Config:      f.config("ios", "  vpp_token_id = 3"),
				ExpectError: regexp.MustCompile(`(?s)Fleet added this app from VPP token 7`),
			},
			{
				Config: f.config("ios", "  vpp_token_id = 3"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.TestCheckResourceAttr(addr, "vpp_token_id", "3"),
			},
		},
	})
}
