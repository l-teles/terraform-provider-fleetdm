package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func testAccSoftwareAppStoreAppConfig(serverURL string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_app_store_app" "test" {
  app_store_id = "899247664"
  platform     = "darwin"
  self_service = true
}
`, serverURL)
}

// TestAccSoftwareAppStoreAppResource_wrongTypeOnImport confirms the
// Read-time wrong-type guard refuses to populate state when a user
// imports a non-VPP title (custom package or FMA) into this resource.
// The test sets ImportStateId; the post-import Read sees the wrong
// shape and surfaces the "Wrong software type" error.
func TestAccSoftwareAppStoreAppResource_wrongTypeOnImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/software/titles/777" && r.Method == http.MethodGet:
			// Title 777 exists but is a custom package, NOT a VPP app —
			// the response has software_package populated and app_store_app
			// absent. The Read-time guard must catch this.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_title": map[string]any{
					"id":             777,
					"name":           "wrong-shape.pkg",
					"source":         "pkg",
					"hosts_count":    0,
					"versions_count": 1,
					"software_package": map[string]any{
						"title_id": 777,
						"platform": "darwin",
					},
					"versions": []map[string]any{{"id": 1, "version": "1.0.0", "hosts_count": 0}},
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/777/available_for_install" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// HCL declares an app_store_id that won't be used (the import sets
	// title_id directly; the post-import Read uses that title_id). The
	// terraform import command flow is: parse ID -> Configure -> Read.
	// The Read sees the wrong shape and must error before state.Set.
	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_app_store_app" "imp" {
  app_store_id = "899247664"
}
`, server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:            cfg,
				ResourceName:      "fleetdm_software_app_store_app.imp",
				ImportState:       true,
				ImportStateId:     "777",
				ImportStateVerify: false,
				ExpectError:       regexp.MustCompile(`(?i)Wrong software type|use fleetdm_software_custom_package|use fleetdm_software_fleet_maintained_app`),
			},
		},
	})
}

// TestAccSoftwareAppStoreAppResource_labelLifecycle drives Create then
// several Updates that switch label types and toggle between populated /
// empty / unset. Verifies that the JSON wire encoding follows the
// nil/empty/populated convention documented on UpdateAppStoreAppRequest
// (nil = "no change", empty = "clear", populated = "set"). Uses the
// shared fake which records each PATCH's label slices.
func TestAccSoftwareAppStoreAppResource_labelLifecycle(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 100

	cfg := func(labels string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_app_store_app" "test" {
  app_store_id = "899247664"
  platform     = "darwin"
  self_service = true
%[2]s
}
`, f.srv.URL, labels)
	}

	patchCount := 0
	requirePatch := func(check func() error) func(*terraform.State) error {
		return func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.vppPatchCount == patchCount {
				return fmt.Errorf("expected a PATCH to fire on this step (count still %d)", patchCount)
			}
			patchCount = f.vppPatchCount
			return check()
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(`  labels_include_any = ["Engineering"]`),
			},
			{
				// Switch sides: include → exclude. PATCH JSON must carry
				// labels_exclude_any=["Contractors"] and labels_include_any=null
				// (which marshals to null since the slice is nil).
				Config: cfg(`  labels_exclude_any = ["Contractors"]`),
				Check: requirePatch(func() error {
					if got := f.vppPatchExcludeLabels; len(got) != 1 || got[0] != "Contractors" {
						return fmt.Errorf("PATCH labels_exclude_any=%v, want [Contractors]", got)
					}
					if len(f.vppPatchIncludeLabels) != 0 {
						return fmt.Errorf("PATCH must omit labels_include_any when HCL switched to labels_exclude_any, got %v", f.vppPatchIncludeLabels)
					}
					return nil
				}),
			},
			{
				// Explicit clear: labels_exclude_any=[]. PATCH JSON sends
				// "labels_exclude_any":[] which Fleet treats as "clear".
				Config: cfg(`  labels_exclude_any = []`),
				Check: requirePatch(func() error {
					if got := f.vppPatchExcludeLabels; got == nil {
						return fmt.Errorf("expected labels_exclude_any to be present (empty array) on the wire, got nil")
					}
					if len(f.vppPatchExcludeLabels) != 0 {
						return fmt.Errorf("expected labels_exclude_any=[] on the wire, got %v", f.vppPatchExcludeLabels)
					}
					return nil
				}),
			},
			{
				// Remove the attribute entirely. PATCH JSON should send
				// "labels_exclude_any":null (nil slice).
				Config: cfg(``),
				Check: requirePatch(func() error {
					if len(f.vppPatchIncludeLabels) != 0 || len(f.vppPatchExcludeLabels) != 0 {
						return fmt.Errorf("expected both label arrays empty/null in PATCH, got include=%v exclude=%v", f.vppPatchIncludeLabels, f.vppPatchExcludeLabels)
					}
					return nil
				}),
			},
		},
	})
}

// TestAccSoftwareAppStoreAppResource_basic exercises Create+Read against a
// fake Fleet that returns a software_title with an app_store_app block.
func TestAccSoftwareAppStoreAppResource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/software/app_store_apps" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"software_title_id": 100})
		case r.URL.Path == "/api/v1/fleet/software/titles/100" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_title": map[string]any{
					"id":             100,
					"name":           "TestFlight",
					"source":         "apps",
					"hosts_count":    0,
					"versions_count": 1,
					"app_store_app": map[string]any{
						"app_store_id":   "899247664",
						"platform":       "darwin",
						"name":           "TestFlight",
						"latest_version": "3.2.0",
						"self_service":   true,
					},
					"versions": []map[string]any{
						{"id": 1, "version": "3.2.0", "hosts_count": 0},
					},
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/100/available_for_install" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/v1/fleet/global/policies" && r.Method == http.MethodGet:
			// Delete handler enumerates policies to detach install_software
			// automation before issuing the DELETE.
			_ = json.NewEncoder(w).Encode(map[string]any{"policies": []map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSoftwareAppStoreAppConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "title_id", "100"),
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "name", "TestFlight"),
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "app_store_id", "899247664"),
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "self_service", "true"),
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "platform", "darwin"),
				),
			},
		},
	})
}

// TestAccSoftwareAppStoreAppResource_installDuringSetupLifecycle drives
// Create-true → Update-false → Update-true again and asserts a PUT
// /setup_experience/software fires on each transition with the right
// set membership. VPP-specific concern: Fleet has no install_during_setup
// field on Create/PATCH JSON — the only path that flips it is the
// out-of-band setup-experience PUT, so this test pins that contract.
func TestAccSoftwareAppStoreAppResource_installDuringSetupLifecycle(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 142
	f.titleSource = "app_store_app"

	cfg := func(flag bool) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_app_store_app" "test" {
  app_store_id         = "899247664"
  platform             = "darwin"
  self_service         = true
  install_during_setup = %[2]t
}
`, f.srv.URL, flag)
	}

	priorPuts := 0

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "install_during_setup", "true"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.setupExperiencePuts == priorPuts {
							return fmt.Errorf("expected a PUT /setup_experience/software on Create-true, got none")
						}
						priorPuts = f.setupExperiencePuts
						for _, id := range f.setupExperienceSet {
							if id == f.titleID {
								return nil
							}
						}
						return fmt.Errorf("expected title %d in setup-experience set after Create-true, got %v", f.titleID, f.setupExperienceSet)
					},
				),
			},
			{
				Config: cfg(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "install_during_setup", "false"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.setupExperiencePuts == priorPuts {
							return fmt.Errorf("expected a PUT /setup_experience/software on Update-false, got none")
						}
						priorPuts = f.setupExperiencePuts
						for _, id := range f.setupExperienceSet {
							if id == f.titleID {
								return fmt.Errorf("title %d must NOT be in setup-experience set after Update-false, got %v", f.titleID, f.setupExperienceSet)
							}
						}
						return nil
					},
				),
			},
			{
				Config: cfg(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "install_during_setup", "true"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.setupExperiencePuts == priorPuts {
							return fmt.Errorf("expected a PUT /setup_experience/software on Update-true again, got none")
						}
						for _, id := range f.setupExperienceSet {
							if id == f.titleID {
								return nil
							}
						}
						return fmt.Errorf("expected title %d in setup-experience set after Update-true-again, got %v", f.titleID, f.setupExperienceSet)
					},
				),
			},
		},
	})
}

// TestAccSoftwareAppStoreAppResource_displayNameLifecycle covers
// display_name across Create → Update. VPP has no categories attribute,
// so this test only validates the display_name wire-and-state round trip.
func TestAccSoftwareAppStoreAppResource_displayNameLifecycle(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 162
	f.titleSource = "app_store_app"

	cfg := func(displayName string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_app_store_app" "test" {
  app_store_id = "899247664"
  platform     = "darwin"
  self_service = true
  display_name = %[2]q
}
`, f.srv.URL, displayName)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("MyVPPApp"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "display_name", "MyVPPApp"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.vppDisplayName != "MyVPPApp" {
							return fmt.Errorf("VPP Create display_name=%q, want MyVPPApp", f.vppDisplayName)
						}
						return nil
					},
				),
			},
			{
				Config: cfg("MyVPPApp Renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "display_name", "MyVPPApp Renamed"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.vppPatchDisplayName != "MyVPPApp Renamed" {
							return fmt.Errorf("VPP PATCH display_name=%q, want MyVPPApp Renamed", f.vppPatchDisplayName)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccSoftwareAppStoreAppResource_labelsIncludeAllLifecycle exercises
// the new labels_include_all attribute on the VPP resource end-to-end:
// set on Create, switch to labels_include_any, drop entirely. The VPP
// path is JSON (Add + PATCH) so the fake captures the slice directly.
func TestAccSoftwareAppStoreAppResource_labelsIncludeAllLifecycle(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 172
	f.titleSource = "app_store_app"

	cfg := func(labels string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_app_store_app" "test" {
  app_store_id = "899247664"
  platform     = "darwin"
  self_service = true
%[2]s
}
`, f.srv.URL, labels)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Fleet's POST /software/app_store_apps doesn't accept labels.
				// The resource applies them via a follow-up PATCH after Add,
				// so the labels land on f.vppPatchIncludeAll, not on the POST.
				Config: cfg(`  labels_include_all = ["Engineering", "macOS"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "labels_include_all.#", "2"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if len(f.vppPatchIncludeAll) != 2 {
							return fmt.Errorf("VPP follow-up PATCH labels_include_all=%v, want 2 entries", f.vppPatchIncludeAll)
						}
						return nil
					},
				),
			},
			{
				Config: cfg(`  labels_include_any = ["Engineering"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "labels_include_any.#", "1"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if len(f.vppPatchIncludeLabels) != 1 || f.vppPatchIncludeLabels[0] != "Engineering" {
							return fmt.Errorf("VPP PATCH labels_include_any=%v, want [Engineering]", f.vppPatchIncludeLabels)
						}
						return nil
					},
				),
			},
			{
				Config: cfg(``),
				Check: func(_ *terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					if len(f.vppPatchIncludeAll) != 0 || len(f.vppPatchIncludeLabels) != 0 || len(f.vppPatchExcludeLabels) != 0 {
						return fmt.Errorf("VPP PATCH must clear/omit all label slices; got incAll=%v incAny=%v excAny=%v",
							f.vppPatchIncludeAll, f.vppPatchIncludeLabels, f.vppPatchExcludeLabels)
					}
					return nil
				},
			},
		},
	})
}

// testAccVPPConfig builds a VPP resource with an arbitrary extra attribute
// block, used by the Fleet 4.90 auto-update / configuration tests below.
func testAccVPPConfig(serverURL, extra string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_app_store_app" "test" {
  app_store_id = "497799835"
  %[2]s
}
`, serverURL, extra)
}

// TestAccSoftwareAppStoreAppResource_autoUpdateLifecycle drives the 4.90
// automatic-update settings through Create and Update. VPP needs a real Apple
// token, so a live Fleet can never reach this code path — the fake mirrors
// Fleet's window validation (enabling without both bounds is a 422) so the
// assertions still mean something.
//
// Create is the interesting half: Fleet's Add endpoint has no auto_update_*
// fields, so the values have to arrive via a follow-up PATCH.
func TestAccSoftwareAppStoreAppResource_autoUpdateLifecycle(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 300
	f.titleName = "Logic Pro"

	wantWindow := func(enabled bool, start, end string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.vppPatchAutoUpdateEnabled == nil || *f.vppPatchAutoUpdateEnabled != enabled {
				return fmt.Errorf("auto_update_enabled on the wire = %v, want %v", f.vppPatchAutoUpdateEnabled, enabled)
			}
			if start == "" {
				if f.vppPatchAutoUpdateStart != nil {
					return fmt.Errorf("expected auto_update_window_start omitted, got %q", *f.vppPatchAutoUpdateStart)
				}
			} else if f.vppPatchAutoUpdateStart == nil || *f.vppPatchAutoUpdateStart != start {
				return fmt.Errorf("auto_update_window_start on the wire = %v, want %q", f.vppPatchAutoUpdateStart, start)
			}
			if end == "" {
				if f.vppPatchAutoUpdateEnd != nil {
					return fmt.Errorf("expected auto_update_window_end omitted, got %q", *f.vppPatchAutoUpdateEnd)
				}
			} else if f.vppPatchAutoUpdateEnd == nil || *f.vppPatchAutoUpdateEnd != end {
				return fmt.Errorf("auto_update_window_end on the wire = %v, want %q", f.vppPatchAutoUpdateEnd, end)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccVPPConfig(f.srv.URL, `platform                 = "ios"
  auto_update_enabled      = true
  auto_update_window_start = "01:30"
  auto_update_window_end   = "04:00"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "auto_update_enabled", "true"),
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "auto_update_window_start", "01:30"),
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "auto_update_window_end", "04:00"),
					wantWindow(true, "01:30", "04:00"),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.vppPatchCount != 1 {
							return fmt.Errorf("expected exactly 1 follow-up PATCH on create, got %d", f.vppPatchCount)
						}
						return nil
					},
				),
			},
			{
				// Move the window.
				Config: testAccVPPConfig(f.srv.URL, `platform                 = "ios"
  auto_update_enabled      = true
  auto_update_window_start = "23:00"
  auto_update_window_end   = "02:00"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "auto_update_window_start", "23:00"),
					// An end before the start is legal — Fleet wraps to the next day.
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "auto_update_window_end", "02:00"),
					wantWindow(true, "23:00", "02:00"),
				),
			},
			{
				// Disabling needs no window, and Fleet accepts false alone.
				Config: testAccVPPConfig(f.srv.URL, `platform            = "ios"
  auto_update_enabled = false`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "auto_update_enabled", "false"),
					wantWindow(false, "", ""),
				),
			},
		},
	})
}

// TestAccSoftwareAppStoreAppResource_autoUpdateValidation covers the plan-time
// guards. Each case must fail during validate/plan, before any request reaches
// Fleet — the whole point is turning Fleet's 4xx into an early error.
func TestAccSoftwareAppStoreAppResource_autoUpdateValidation(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 301

	tests := []struct {
		name        string
		extra       string
		expectError *regexp.Regexp
	}{
		{
			name:        "enabled without any window",
			extra:       `auto_update_enabled = true`,
			expectError: regexp.MustCompile(`(?is)auto_update_window_start is required when auto_update_enabled is true`),
		},
		{
			name: "enabled with only a start",
			extra: `auto_update_enabled      = true
  auto_update_window_start = "01:00"`,
			expectError: regexp.MustCompile(`(?is)auto_update_window_end`),
		},
		{
			// AlsoRequires: a window is meaningless without the flag.
			name:        "window without the enable flag",
			extra:       `auto_update_window_start = "01:00"`,
			expectError: regexp.MustCompile(`(?is)auto_update_enabled|auto_update_window_end`),
		},
		{
			name: "single-digit hour is rejected",
			extra: `auto_update_enabled      = true
  auto_update_window_start = "1:00"
  auto_update_window_end   = "04:00"`,
			expectError: regexp.MustCompile(`(?is)24-hour time of day formatted`),
		},
		{
			name: "hour 24 is rejected",
			extra: `auto_update_enabled      = true
  auto_update_window_start = "24:00"
  auto_update_window_end   = "04:00"`,
			expectError: regexp.MustCompile(`(?is)24-hour time of day formatted`),
		},
		{
			name: "minute 60 is rejected",
			extra: `auto_update_enabled      = true
  auto_update_window_start = "01:60"
  auto_update_window_end   = "04:00"`,
			expectError: regexp.MustCompile(`(?is)24-hour time of day formatted`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      testAccVPPConfig(f.srv.URL, tt.extra),
						PlanOnly:    true,
						ExpectError: tt.expectError,
					},
				},
			})
		})
	}
}

// TestAccSoftwareAppStoreAppResource_androidConfiguration covers the Android
// half of the 4.90 work: `platform = "android"` must be accepted, and a JSON
// managed configuration must reach Fleet as a JSON *object* (Fleet validates it
// with ValidateAndroidAppConfiguration and rejects a quoted string).
//
// The configuration travels on the Add request, not a follow-up PATCH.
func TestAccSoftwareAppStoreAppResource_androidConfiguration(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 302
	f.titleName = "Zoom"

	const cfgJSON = `{"managedConfiguration":{"enableLogging":true}}`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccVPPConfig(f.srv.URL, fmt.Sprintf(`platform      = "android"
  self_service  = true
  configuration = %q`, cfgJSON)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "platform", "android"),
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "configuration", cfgJSON),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.vppPlatform != "android" {
							return fmt.Errorf("expected platform android on the wire, got %q", f.vppPlatform)
						}
						if len(f.vppCreateConfiguration) == 0 {
							return fmt.Errorf("expected configuration on the Add request, got none")
						}
						if f.vppCreateConfiguration[0] != '{' {
							return fmt.Errorf("an Android configuration must stay a JSON object, got: %s", f.vppCreateConfiguration)
						}
						if string(f.vppCreateConfiguration) != cfgJSON {
							return fmt.Errorf("configuration altered in transit\n got: %s\nwant: %s", f.vppCreateConfiguration, cfgJSON)
						}
						return nil
					},
				),
			},
			{
				// Fleet echoes the configuration back; state must settle.
				Config: testAccVPPConfig(f.srv.URL, fmt.Sprintf(`platform      = "android"
  self_service  = true
  configuration = %q`, cfgJSON)),
				PlanOnly: true,
			},
		},
	})
}

// TestAccSoftwareAppStoreAppResource_iosXMLConfiguration is the iOS counterpart:
// the raw string is XML, which Fleet requires as "a JSON string containing the
// XML". The provider must do that wrapping for the user.
func TestAccSoftwareAppStoreAppResource_iosXMLConfiguration(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 303
	f.titleName = "Logic Pro"

	const cfgXML = `<dict><key>ServerURL</key><string>https://example.test</string></dict>`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccVPPConfig(f.srv.URL, fmt.Sprintf(`platform      = "ios"
  configuration = %q`, cfgXML)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_app_store_app.test", "configuration", cfgXML),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if len(f.vppCreateConfiguration) == 0 {
							return fmt.Errorf("expected configuration on the Add request, got none")
						}
						if f.vppCreateConfiguration[0] != '"' {
							return fmt.Errorf(`XML must be sent as a JSON string ("expected configuration as a JSON string containing the XML"), got: %s`, f.vppCreateConfiguration)
						}
						// And it has to decode back to the original XML.
						var decoded string
						if err := json.Unmarshal(f.vppCreateConfiguration, &decoded); err != nil {
							return fmt.Errorf("configuration is not a JSON string: %w", err)
						}
						if decoded != cfgXML {
							return fmt.Errorf("XML did not survive encoding\n got: %q\nwant: %q", decoded, cfgXML)
						}
						return nil
					},
				),
			},
			{
				Config: testAccVPPConfig(f.srv.URL, fmt.Sprintf(`platform      = "ios"
  configuration = %q`, cfgXML)),
				PlanOnly: true,
			},
		},
	})
}

// TestAccSoftwareAppStoreAppResource_fleet490FieldsOptIn locks in the opt-in
// convention for the new attributes: with none of them in HCL, no follow-up
// PATCH is issued at all, and values Fleet already holds are not adopted into
// state (so they can't produce a diff on a field the user doesn't manage).
func TestAccSoftwareAppStoreAppResource_fleet490FieldsOptIn(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 304
	f.titleName = "Logic Pro"
	// Pretend an admin configured all of this in the Fleet UI.
	enabled := true
	start, end := "01:00", "05:00"
	f.titleAutoUpdateEnabled = &enabled
	f.titleAutoUpdateStart = &start
	f.titleAutoUpdateEnd = &end

	cfg := testAccVPPConfig(f.srv.URL, `platform = "ios"`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("fleetdm_software_app_store_app.test", "auto_update_enabled"),
					resource.TestCheckNoResourceAttr("fleetdm_software_app_store_app.test", "auto_update_window_start"),
					resource.TestCheckNoResourceAttr("fleetdm_software_app_store_app.test", "auto_update_window_end"),
					resource.TestCheckNoResourceAttr("fleetdm_software_app_store_app.test", "configuration"),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.vppPatchCount != 0 {
							return fmt.Errorf("expected no follow-up PATCH when no 4.90 attribute is set, got %d", f.vppPatchCount)
						}
						if f.titleAutoUpdateEnabled == nil || !*f.titleAutoUpdateEnabled {
							return fmt.Errorf("UI-set auto-update config must be left untouched, got %v", f.titleAutoUpdateEnabled)
						}
						return nil
					},
				),
			},
			{
				Config:   cfg,
				PlanOnly: true,
			},
		},
	})
}
