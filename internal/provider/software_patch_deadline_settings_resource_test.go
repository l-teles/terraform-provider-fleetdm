package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

const patchDeadlineResourceName = "fleetdm_software_patch_deadline_settings.test"

// patchDeadlineMock mirrors Fleet 4.95's patch deadline endpoints: fleet_id is
// required, unknown fleets answer 404, unset fleets report the defaults, and
// days are stored in week order.
type patchDeadlineMock struct {
	mu       sync.Mutex
	fleets   map[int64]bool
	settings map[int64]fleetdm.PatchDeadlineSettings
	patches  int
	// patchNotFound makes PATCH answer 404, as when the fleet is deleted
	// between refresh and destroy.
	patchNotFound bool
	// patchNotFoundHits counts PATCHes answered 404 because of patchNotFound.
	patchNotFoundHits int
	// version is reported by GET /version; empty answers 404 (version unknown).
	version string
	// noRoute removes the endpoint, as on Fleet before 4.95.
	noRoute bool
}

func newPatchDeadlineMock(fleetIDs ...int64) *patchDeadlineMock {
	m := &patchDeadlineMock{fleets: map[int64]bool{0: true}, settings: map[int64]fleetdm.PatchDeadlineSettings{}}
	for _, id := range fleetIDs {
		m.fleets[id] = true
	}
	return m
}

// current returns the settings Fleet would report for a fleet.
func (m *patchDeadlineMock) current(fleetID int64) fleetdm.PatchDeadlineSettings {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.settings[fleetID]; ok {
		return s
	}
	return fleetdm.DefaultPatchDeadlineSettings(fleetID)
}

func (m *patchDeadlineMock) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	const basePath = "/api/v1/fleet/software/patch_deadline_settings"

	writeErr := func(w http.ResponseWriter, status int, name, reason string) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Validation Failed",
			"errors":  []map[string]string{{"name": name, "reason": reason}},
		})
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		m.mu.Lock()
		version, noRoute := m.version, m.noRoute
		m.mu.Unlock()
		if r.URL.Path == "/api/v1/fleet/version" && version != "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"version": version})
			return
		}
		if r.URL.Path != basePath || noRoute {
			http.NotFound(w, r)
			return
		}

		var fleetID *int64
		var days []string
		var tm string
		switch r.Method {
		case http.MethodGet:
			if raw := r.URL.Query().Get("fleet_id"); raw != "" {
				id, err := strconv.ParseInt(raw, 10, 64)
				if err != nil {
					t.Errorf("non-numeric fleet_id %q", raw)
				}
				fleetID = &id
			}
		case http.MethodPatch:
			var body struct {
				FleetID *int64   `json:"fleet_id"`
				Days    []string `json:"days"`
				Time    string   `json:"time"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode PATCH body: %v", err)
			}
			fleetID, days, tm = body.FleetID, body.Days, body.Time
		default:
			http.NotFound(w, r)
			return
		}

		if fleetID == nil {
			writeErr(w, http.StatusUnprocessableEntity, "fleet_id", "fleet_id is required")
			return
		}

		m.mu.Lock()
		exists := m.fleets[*fleetID]
		forced404 := exists && r.Method == http.MethodPatch && m.patchNotFound
		if forced404 {
			m.patchNotFoundHits++
		}
		m.mu.Unlock()
		if !exists || forced404 {
			writeErr(w, http.StatusNotFound, "fleet_id", fmt.Sprintf("fleet %d does not exist", *fleetID))
			return
		}

		if r.Method == http.MethodPatch {
			if len(days) == 0 {
				writeErr(w, http.StatusBadRequest, "base", `Couldn't edit patch deadline settings. "days" must include at least one day.`)
				return
			}
			sorted := []string{}
			for _, d := range fleetdm.PatchDeadlineDays {
				if slices.Contains(days, d) {
					sorted = append(sorted, d)
				}
			}
			m.mu.Lock()
			m.settings[*fleetID] = fleetdm.PatchDeadlineSettings{FleetID: *fleetID, Days: sorted, Time: tm}
			m.patches++
			m.mu.Unlock()
		}

		_ = json.NewEncoder(w).Encode(map[string]any{"patch_deadline_settings": m.current(*fleetID)})
	}
}

// checkDefault asserts that the mock holds Fleet's default for a fleet.
func (m *patchDeadlineMock) checkDefault(fleetID int64) func(*terraform.State) error {
	return func(*terraform.State) error {
		if got, want := m.current(fleetID), fleetdm.DefaultPatchDeadlineSettings(fleetID); !reflect.DeepEqual(got, want) {
			return fmt.Errorf("fleet %d settings = %+v, want default %+v", fleetID, got, want)
		}
		return nil
	}
}

func testAccPatchDeadlineMockConfig(serverURL string, fleetID int64, days []string, tm string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_patch_deadline_settings" "test" {
  fleet_id = %[2]d
  days     = [%[3]s]
  time     = %[4]q
}
`, serverURL, fleetID, quoteList(days), tm)
}

func quoteList(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = strconv.Quote(v)
	}
	return strings.Join(quoted, ", ")
}

// Create, import, update in place, then destroy resets the fleet to defaults.
func TestAccSoftwarePatchDeadlineSettingsResource_mockLifecycle(t *testing.T) {
	mock := newPatchDeadlineMock(3)
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             mock.checkDefault(3),
		Steps: []resource.TestStep{
			{
				Config: testAccPatchDeadlineMockConfig(server.URL, 3, []string{"friday", "monday"}, "08:30"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchDeadlineResourceName, "id", "3"),
					resource.TestCheckResourceAttr(patchDeadlineResourceName, "fleet_id", "3"),
					resource.TestCheckResourceAttr(patchDeadlineResourceName, "days.#", "2"),
					resource.TestCheckTypeSetElemAttr(patchDeadlineResourceName, "days.*", "monday"),
					resource.TestCheckTypeSetElemAttr(patchDeadlineResourceName, "days.*", "friday"),
					resource.TestCheckResourceAttr(patchDeadlineResourceName, "time", "08:30"),
					func(*terraform.State) error {
						want := fleetdm.PatchDeadlineSettings{FleetID: 3, Days: []string{"monday", "friday"}, Time: "08:30"}
						if got := mock.current(3); !reflect.DeepEqual(got, want) {
							return fmt.Errorf("mock holds %+v, want %+v", got, want)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      patchDeadlineResourceName,
				ImportState:       true,
				ImportStateId:     "3",
				ImportStateVerify: true,
			},
			{
				Config: testAccPatchDeadlineMockConfig(server.URL, 3, []string{"sunday"}, "23:59"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(patchDeadlineResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchDeadlineResourceName, "id", "3"),
					resource.TestCheckResourceAttr(patchDeadlineResourceName, "days.#", "1"),
					resource.TestCheckTypeSetElemAttr(patchDeadlineResourceName, "days.*", "sunday"),
					resource.TestCheckResourceAttr(patchDeadlineResourceName, "time", "23:59"),
				),
			},
		},
	})
}

// Fleet returns days in week order; listing them in another order must not
// plan a change or send a request.
func TestAccSoftwarePatchDeadlineSettingsResource_mockDaysReorderNoDrift(t *testing.T) {
	mock := newPatchDeadlineMock(3)
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	var patchesAfterCreate int
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPatchDeadlineMockConfig(server.URL, 3, []string{"sunday", "wednesday", "monday"}, "12:00"),
				Check: func(*terraform.State) error {
					mock.mu.Lock()
					defer mock.mu.Unlock()
					patchesAfterCreate = mock.patches
					return nil
				},
			},
			{
				Config: testAccPatchDeadlineMockConfig(server.URL, 3, []string{"monday", "sunday", "wednesday"}, "12:00"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: func(*terraform.State) error {
					mock.mu.Lock()
					defer mock.mu.Unlock()
					if mock.patches != patchesAfterCreate {
						return fmt.Errorf("reordering days sent %d extra PATCH requests", mock.patches-patchesAfterCreate)
					}
					return nil
				},
			},
		},
	})
}

// fleet_id 0 is Unassigned: it must be sent, imported as "0" and reset on destroy.
func TestAccSoftwarePatchDeadlineSettingsResource_mockUnassigned(t *testing.T) {
	mock := newPatchDeadlineMock()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             mock.checkDefault(0),
		Steps: []resource.TestStep{
			{
				Config: testAccPatchDeadlineMockConfig(server.URL, 0, []string{"thursday"}, "06:05"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchDeadlineResourceName, "id", "0"),
					resource.TestCheckResourceAttr(patchDeadlineResourceName, "fleet_id", "0"),
					resource.TestCheckTypeSetElemAttr(patchDeadlineResourceName, "days.*", "thursday"),
					resource.TestCheckResourceAttr(patchDeadlineResourceName, "time", "06:05"),
				),
			},
			{
				ResourceName:      patchDeadlineResourceName,
				ImportState:       true,
				ImportStateId:     "0",
				ImportStateVerify: true,
			},
		},
	})
}

// A fleet_id change targets another fleet, so it replaces the resource.
func TestAccSoftwarePatchDeadlineSettingsResource_mockFleetIDRequiresReplace(t *testing.T) {
	mock := newPatchDeadlineMock(3, 4)
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPatchDeadlineMockConfig(server.URL, 3, []string{"monday"}, "10:00"),
			},
			{
				Config: testAccPatchDeadlineMockConfig(server.URL, 4, []string{"monday"}, "10:00"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(patchDeadlineResourceName, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchDeadlineResourceName, "id", "4"),
					mock.checkDefault(3),
				),
			},
		},
	})
}

// A deleted fleet drops the resource from state on refresh, and a fleet
// deleted between refresh and destroy still destroys cleanly.
func TestAccSoftwarePatchDeadlineSettingsResource_mockFleetDeleted(t *testing.T) {
	mock := newPatchDeadlineMock(5, 6)
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	providerOnly := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %q
  api_key        = "test-token"
}
`, server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPatchDeadlineMockConfig(server.URL, 5, []string{"monday"}, "10:00"),
			},
			{
				// Read sees 404 and removes the resource, so the next plan recreates it.
				PreConfig: func() {
					mock.mu.Lock()
					defer mock.mu.Unlock()
					delete(mock.fleets, 5)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccPatchDeadlineMockConfig(server.URL, 6, []string{"monday"}, "10:00"),
			},
			{
				PreConfig: func() {
					mock.mu.Lock()
					defer mock.mu.Unlock()
					mock.patchNotFound = true
				},
				Config: providerOnly,
				Check: func(*terraform.State) error {
					mock.mu.Lock()
					defer mock.mu.Unlock()
					if mock.patchNotFoundHits == 0 {
						return fmt.Errorf("delete never reached the 404 path")
					}
					return nil
				},
			},
		},
	})
}

// On Fleet before 4.95 the endpoint itself answers 404. A refresh must fail
// with the version requirement and keep the resource in state instead of
// dropping it as a deleted fleet, and destroy still succeeds.
func TestAccSoftwarePatchDeadlineSettingsResource_mockOlderFleet(t *testing.T) {
	mock := newPatchDeadlineMock(5)
	mock.version = "4.95.0"
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()
	cfg := testAccPatchDeadlineMockConfig(server.URL, 5, []string{"monday"}, "10:00")
	wantErr := regexp.MustCompile(`requires\s+Fleet\s+Premium\s+4\.95\.0\s+or\s+later`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				PreConfig: func() {
					mock.mu.Lock()
					defer mock.mu.Unlock()
					mock.version, mock.noRoute = "4.94.1", true
				},
				RefreshState: true,
				ExpectError:  wantErr,
			},
			{
				// Still in state: the plan errors on refresh rather than planning a create.
				Config:      cfg,
				PlanOnly:    true,
				ExpectError: wantErr,
			},
		},
	})
}

func TestAccSoftwarePatchDeadlineSettingsResource_mockOlderFleetCreate(t *testing.T) {
	mock := newPatchDeadlineMock(5)
	mock.version, mock.noRoute = "4.94.1", true
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccPatchDeadlineMockConfig(server.URL, 5, []string{"monday"}, "10:00"),
				ExpectError: regexp.MustCompile(`requires\s+Fleet\s+Premium\s+4\.95\.0\s+or\s+later`),
			},
		},
	})
}

func TestAccSoftwarePatchDeadlineSettingsResource_mockImportIDInvalid(t *testing.T) {
	mock := newPatchDeadlineMock(3)
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPatchDeadlineMockConfig(server.URL, 3, []string{"monday"}, "10:00"),
			},
			{
				ResourceName:  patchDeadlineResourceName,
				ImportState:   true,
				ImportStateId: "abc",
				ExpectError:   regexp.MustCompile("Could not parse Patch Deadline Settings Fleet ID"),
			},
			{
				ResourceName:  patchDeadlineResourceName,
				ImportState:   true,
				ImportStateId: "-1",
				ExpectError:   regexp.MustCompile(`ID\s+must\s+not\s+be\s+negative`),
			},
		},
	})
}

// Invalid days and times fail at plan time. Duplicate days cannot be tested:
// a set collapses them before validation.
func TestAccSoftwarePatchDeadlineSettingsResource_mockValidators(t *testing.T) {
	cases := map[string]struct {
		days []string
		time string
		err  string
	}{
		"capitalised day":     {[]string{"Monday"}, "17:00", `value must be one of`},
		"unknown day":         {[]string{"funday"}, "17:00", `value must be one of`},
		"no days":             {[]string{}, "17:00", `set must contain at least 1 elements`},
		"single-digit hour":   {[]string{"monday"}, "9:00", `two-digit hour`},
		"hour 24":             {[]string{"monday"}, "24:00", `two-digit hour`},
		"minute 60":           {[]string{"monday"}, "17:60", `two-digit hour`},
		"no colon":            {[]string{"monday"}, "1700", `two-digit hour`},
		"seconds":             {[]string{"monday"}, "17:00:00", `two-digit hour`},
		"empty time":          {[]string{"monday"}, "", `two-digit hour`},
		"twelve hour clock":   {[]string{"monday"}, "5:00pm", `two-digit hour`},
		"surrounding spaces":  {[]string{"monday"}, " 17:00", `two-digit hour`},
		"one valid, one not":  {[]string{"monday", "mon"}, "17:00", `value must be one of`},
		"trailing whitespace": {[]string{"monday "}, "17:00", `value must be one of`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      testAccPatchDeadlineMockConfig("http://localhost:0", 1, tc.days, tc.time),
						ExpectError: regexp.MustCompile(tc.err),
					},
				},
			})
		})
	}
}

// Boundary values Fleet accepts must pass validation and apply unchanged.
func TestAccSoftwarePatchDeadlineSettingsResource_mockValidBoundaries(t *testing.T) {
	mock := newPatchDeadlineMock(3)
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	allDays := slices.Clone(fleetdm.PatchDeadlineDays)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPatchDeadlineMockConfig(server.URL, 3, allDays, "00:00"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchDeadlineResourceName, "days.#", "7"),
					resource.TestCheckResourceAttr(patchDeadlineResourceName, "time", "00:00"),
				),
			},
			{
				Config: testAccPatchDeadlineMockConfig(server.URL, 3, allDays, "23:59"),
				Check:  resource.TestCheckResourceAttr(patchDeadlineResourceName, "time", "23:59"),
			},
		},
	})
}

// skipBeforeFleet495PatchDeadlines skips a live test on servers older than
// Fleet 4.95, which have no patch deadline endpoints. An unparsable version
// runs the test.
func skipBeforeFleet495PatchDeadlines(t *testing.T) {
	t.Helper()
	info, err := testAccPatchDeadlineLiveClient(t).GetVersion(context.Background())
	if err != nil {
		t.Fatalf("failed to read the Fleet version: %v", err)
	}
	parts := strings.SplitN(strings.TrimPrefix(info.Version, "v"), ".", 3)
	if len(parts) < 2 {
		return
	}
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	if errMajor != nil || errMinor != nil {
		return
	}
	if major < 4 || (major == 4 && minor < 95) {
		t.Skipf("patch deadline settings need Fleet 4.95+, server reports %s", info.Version)
	}
}

func testAccPatchDeadlineLiveClient(t *testing.T) *fleetdm.Client {
	t.Helper()
	client, err := fleetdm.NewClient(fleetdm.ClientConfig{
		ServerAddress: os.Getenv("FLEETDM_URL"),
		APIKey:        os.Getenv("FLEETDM_API_TOKEN"),
		VerifyTLS:     os.Getenv("FLEETDM_VERIFY_TLS") != "false" && os.Getenv("FLEETDM_VERIFY_TLS") != "0",
	})
	if err != nil {
		t.Fatalf("failed to build fleet client: %v", err)
	}
	return client
}

// testAccCheckPatchDeadlineLive asserts what Fleet itself reports for the fleet
// that the named resource (or literal "0") points at.
func testAccCheckPatchDeadlineLive(t *testing.T, fleetIDFrom string, wantDays []string, wantTime string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		fleetID := int64(0)
		if fleetIDFrom != "0" {
			rs, ok := s.RootModule().Resources[fleetIDFrom]
			if !ok {
				return fmt.Errorf("resource %s not found in state", fleetIDFrom)
			}
			id, err := strconv.ParseInt(rs.Primary.ID, 10, 64)
			if err != nil {
				return err
			}
			fleetID = id
		}
		got, err := testAccPatchDeadlineLiveClient(t).GetPatchDeadlineSettings(context.Background(), fleetID)
		if err != nil {
			return err
		}
		if !slices.Equal(got.Days, wantDays) || got.Time != wantTime {
			return fmt.Errorf("fleet %d reports days=%v time=%q, want days=%v time=%q", fleetID, got.Days, got.Time, wantDays, wantTime)
		}
		return nil
	}
}

func testAccPatchDeadlineLiveConfig(fleetName string, withSettings bool, fleetDays, unassignedDays []string, fleetTime, unassignedTime string) string {
	cfg := providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q
}
`, fleetName)
	if !withSettings {
		return cfg
	}
	return cfg + fmt.Sprintf(`
resource "fleetdm_software_patch_deadline_settings" "test" {
  fleet_id = fleetdm_fleet.test.id
  days     = [%[1]s]
  time     = %[2]q
}

resource "fleetdm_software_patch_deadline_settings" "unassigned" {
  fleet_id = 0
  days     = [%[3]s]
  time     = %[4]q
}
`, quoteList(fleetDays), fleetTime, quoteList(unassignedDays), unassignedTime)
}

// TestAccSoftwarePatchDeadlineSettingsResource_live covers a new fleet and
// Unassigned against Fleet Premium 4.95+, and checks that removing the
// resources resets both to Fleet's defaults.
func TestAccSoftwarePatchDeadlineSettingsResource_live(t *testing.T) {
	testAccPreCheck(t)
	skipBeforeFleet495PatchDeadlines(t)

	fleetName := "tf-acc-patch-deadline-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	const unassigned = "fleetdm_software_patch_deadline_settings.unassigned"
	defaultDays := []string{fleetdm.DefaultPatchDeadlineDay}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchDeadlineLive(t, "0", defaultDays, fleetdm.DefaultPatchDeadlineTime),
		Steps: []resource.TestStep{
			{
				Config: testAccPatchDeadlineLiveConfig(fleetName, true, []string{"friday", "monday"}, []string{"wednesday"}, "08:30", "12:00"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(patchDeadlineResourceName, "fleet_id", "fleetdm_fleet.test", "id"),
					resource.TestCheckResourceAttrPair(patchDeadlineResourceName, "id", "fleetdm_fleet.test", "id"),
					resource.TestCheckResourceAttr(patchDeadlineResourceName, "days.#", "2"),
					resource.TestCheckTypeSetElemAttr(patchDeadlineResourceName, "days.*", "monday"),
					resource.TestCheckTypeSetElemAttr(patchDeadlineResourceName, "days.*", "friday"),
					resource.TestCheckResourceAttr(patchDeadlineResourceName, "time", "08:30"),
					resource.TestCheckResourceAttr(unassigned, "id", "0"),
					resource.TestCheckResourceAttr(unassigned, "fleet_id", "0"),
					resource.TestCheckTypeSetElemAttr(unassigned, "days.*", "wednesday"),
					resource.TestCheckResourceAttr(unassigned, "time", "12:00"),
					testAccCheckPatchDeadlineLive(t, "fleetdm_fleet.test", []string{"monday", "friday"}, "08:30"),
					testAccCheckPatchDeadlineLive(t, "0", []string{"wednesday"}, "12:00"),
				),
			},
			{
				ResourceName:      patchDeadlineResourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["fleetdm_fleet.test"].Primary.ID, nil
				},
			},
			{
				ResourceName:      unassigned,
				ImportState:       true,
				ImportStateId:     "0",
				ImportStateVerify: true,
			},
			// Reordering days alone is a no-op.
			{
				Config: testAccPatchDeadlineLiveConfig(fleetName, true, []string{"monday", "friday"}, []string{"wednesday"}, "08:30", "12:00"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: testAccPatchDeadlineLiveConfig(fleetName, true, []string{"sunday", "saturday", "tuesday"}, []string{"thursday", "monday"}, "23:59", "00:00"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(patchDeadlineResourceName, plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction(unassigned, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckPatchDeadlineLive(t, "fleetdm_fleet.test", []string{"tuesday", "saturday", "sunday"}, "23:59"),
					testAccCheckPatchDeadlineLive(t, "0", []string{"monday", "thursday"}, "00:00"),
				),
			},
			// Removing the resources keeps the fleet and resets both to defaults.
			{
				Config: testAccPatchDeadlineLiveConfig(fleetName, false, nil, nil, "", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckPatchDeadlineLive(t, "fleetdm_fleet.test", defaultDays, fleetdm.DefaultPatchDeadlineTime),
					testAccCheckPatchDeadlineLive(t, "0", defaultDays, fleetdm.DefaultPatchDeadlineTime),
				),
			},
		},
	})
}
