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

func testAccSoftwareFleetMaintainedAppConfig(serverURL string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "test" {
  fleet_maintained_app_id = 1
  self_service            = true
}
`, serverURL)
}

// TestAccSoftwareFleetMaintainedAppResource_wrongTypeOnImport confirms
// the Read-time wrong-type guard refuses to populate state when a user
// imports a VPP title into this resource. (The FMA resource can't
// distinguish FMA from custom_package on Fleet's GET — both expose a
// software_package block — but it CAN reject VPP titles.)
func TestAccSoftwareFleetMaintainedAppResource_wrongTypeOnImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/software/titles/999" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_title": map[string]any{
					"id":             999,
					"name":           "VPP app in wrong slot",
					"source":         "apps",
					"hosts_count":    0,
					"versions_count": 1,
					"app_store_app": map[string]any{
						"app_store_id": "12345",
						"platform":     "darwin",
						"name":         "VPP app in wrong slot",
					},
					"versions": []map[string]any{{"id": 1, "version": "1.0.0", "hosts_count": 0}},
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/999/available_for_install" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "imp" {
  fleet_maintained_app_id = 1
}
`, server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:            cfg,
				ResourceName:      "fleetdm_software_fleet_maintained_app.imp",
				ImportState:       true,
				ImportStateId:     "999",
				ImportStateVerify: false,
				ExpectError:       regexp.MustCompile(`(?i)Wrong software type|use fleetdm_software_app_store_app`),
			},
		},
	})
}

// TestAccSoftwareFleetMaintainedAppResource_labelLifecycle drives Create
// then several Updates that switch label types. FMA Updates go through
// the multipart PATCH /software/titles/{id}/package endpoint, so the
// wire convention is *[]string-based (nil = omit, empty = "[]", populated
// = JSON array). Per-step PATCH-count gating ensures the assertions
// reflect *this* step's wire data, not stale state from a prior step.
func TestAccSoftwareFleetMaintainedAppResource_labelLifecycle(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 200
	f.titleName = "Firefox"

	cfg := func(labels string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "test" {
  fleet_maintained_app_id = 1
  self_service            = true
%[2]s
}
`, f.srv.URL, labels)
	}

	patchCount := 0
	requirePatch := func(check func() error) func(*terraform.State) error {
		return func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.patchCount == patchCount {
				return fmt.Errorf("expected a PATCH to fire on this step (count still %d)", patchCount)
			}
			patchCount = f.patchCount
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
				// Switch sides: include → exclude. Multipart form must
				// carry labels_exclude_any populated and labels_include_any
				// absent (nil pointer in PatchSoftwarePackageRequest).
				Config: cfg(`  labels_exclude_any = ["Contractors"]`),
				Check: requirePatch(func() error {
					if !f.patchExcludeFieldSeen {
						return fmt.Errorf("PATCH must include labels_exclude_any when HCL set it")
					}
					if got := f.patchExcludeLabels; len(got) != 1 || got[0] != "Contractors" {
						return fmt.Errorf("PATCH labels_exclude_any=%v, want [Contractors]", got)
					}
					if f.patchIncludeFieldSeen {
						return fmt.Errorf("PATCH must omit labels_include_any when HCL switched to labels_exclude_any")
					}
					return nil
				}),
			},
			{
				// Explicit clear: labels_exclude_any=[]. Multipart form
				// carries labels_exclude_any="[]".
				Config: cfg(`  labels_exclude_any = []`),
				Check: requirePatch(func() error {
					if !f.patchExcludeFieldSeen {
						return fmt.Errorf("PATCH must include labels_exclude_any (as []) for explicit clear")
					}
					if len(f.patchExcludeLabels) != 0 {
						return fmt.Errorf("expected labels_exclude_any=[] on the wire, got %v", f.patchExcludeLabels)
					}
					return nil
				}),
			},
			{
				// Remove attribute. Multipart form must omit both label
				// fields entirely.
				Config: cfg(``),
				Check: requirePatch(func() error {
					if f.patchIncludeFieldSeen {
						return fmt.Errorf("PATCH must omit labels_include_any when HCL removed the attribute")
					}
					if f.patchExcludeFieldSeen {
						return fmt.Errorf("PATCH must omit labels_exclude_any when HCL removed the attribute")
					}
					return nil
				}),
			},
		},
	})
}

// TestAccSoftwareFleetMaintainedAppResource_basic exercises Create+Read.
// FMA responses come back shaped like a software_package, so this test
// uses the same body shape as the custom-package test, minus the SHA256
// (Fleet doesn't surface one for FMA-managed titles before first install).
func TestAccSoftwareFleetMaintainedAppResource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/software/fleet_maintained_apps" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"software_title_id": 200})
		case r.URL.Path == "/api/v1/fleet/software/titles/200" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_title": map[string]any{
					"id":             200,
					"name":           "Firefox",
					"source":         "pkg_packages",
					"hosts_count":    0,
					"versions_count": 1,
					"software_package": map[string]any{
						"name":         "Firefox",
						"version":      "125.0",
						"platform":     "darwin",
						"self_service": true,
					},
					"versions": []map[string]any{
						{"id": 1, "version": "125.0", "hosts_count": 0},
					},
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/200/available_for_install" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSoftwareFleetMaintainedAppConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "title_id", "200"),
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "name", "Firefox"),
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "self_service", "true"),
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "fleet_maintained_app_id", "1"),
				),
			},
		},
	})
}

// TestAccSoftwareFleetMaintainedAppResource_installDuringSetupLifecycle
// drives Create-true → Update-false → Update-true again and asserts the
// out-of-band PUT /setup_experience/software fires on each transition.
// FMA-specific concern: install_during_setup is NOT a field on the
// FMA Add endpoint, so the only path that flips it is the
// setup-experience PUT.
func TestAccSoftwareFleetMaintainedAppResource_installDuringSetupLifecycle(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 242
	f.titleName = "Firefox"
	f.titleSource = "fma"

	cfg := func(flag bool) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "test" {
  fleet_maintained_app_id = 1
  self_service            = true
  install_during_setup    = %[2]t
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
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "install_during_setup", "true"),
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
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "install_during_setup", "false"),
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
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "install_during_setup", "true"),
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

// TestAccSoftwareFleetMaintainedAppResource_automaticInstallPolicyOnCreate
// verifies that automatic_install_policy=true sends Fleet's
// `automatic_install=true` JSON field on the FMA Add request, and that
// the Computed automatic_install_policies list surfaces the policies
// Fleet reports.
func TestAccSoftwareFleetMaintainedAppResource_automaticInstallPolicyOnCreate(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 251
	f.titleName = "Firefox"
	f.titleSource = "fma"
	f.titleAutomaticInstallPolicies = []map[string]any{
		{"id": 17, "name": "Auto-install Firefox"},
	}

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "test" {
  fleet_maintained_app_id  = 1
  automatic_install_policy = true
}
`, f.srv.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "automatic_install_policy", "true"),
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "automatic_install_policies.#", "1"),
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "automatic_install_policies.0.id", "17"),
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "automatic_install_policies.0.name", "Auto-install Firefox"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if !f.fmaAutomaticInstall {
							return fmt.Errorf("FMA Add must carry automatic_install=true, got %v", f.fmaAutomaticInstall)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccSoftwareFleetMaintainedAppResource_displayNameAndCategoriesLifecycle
// exercises the new display_name + categories attributes across Create
// (follow-up PATCH after Add) and Update. FMA's Add endpoint doesn't
// accept display_name/categories, so the resource sends them via a
// follow-up PATCH /software/titles/{id}/package call right after Add.
func TestAccSoftwareFleetMaintainedAppResource_displayNameAndCategoriesLifecycle(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 261
	f.titleName = "Firefox"
	f.titleSource = "fma"

	cfg := func(displayName, categoriesHCL string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "test" {
  fleet_maintained_app_id = 1
  display_name            = %[2]q
%[3]s
}
`, f.srv.URL, displayName, categoriesHCL)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("MyFMA", `  categories = ["Productivity", "Security"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "display_name", "MyFMA"),
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "categories.#", "2"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						// FMA Add doesn't accept display_name/categories so the
						// follow-up PATCH after Add is where they land.
						if f.patchDisplayName != "MyFMA" {
							return fmt.Errorf("FMA follow-up PATCH display_name=%q, want MyFMA", f.patchDisplayName)
						}
						if f.patchCategories == "" {
							return fmt.Errorf("FMA follow-up PATCH must include categories")
						}
						return nil
					},
				),
			},
			{
				Config: cfg("MyFMA Renamed", `  categories = ["Productivity"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "display_name", "MyFMA Renamed"),
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "categories.#", "1"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.patchDisplayName != "MyFMA Renamed" {
							return fmt.Errorf("FMA Update PATCH display_name=%q, want MyFMA Renamed", f.patchDisplayName)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccSoftwareFleetMaintainedAppResource_labelsIncludeAllLifecycle
// covers the new labels_include_all attribute on the FMA resource: set on
// Create (the FMA Add endpoint accepts labels_include_all), switch to
// labels_include_any, drop entirely.
func TestAccSoftwareFleetMaintainedAppResource_labelsIncludeAllLifecycle(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 271
	f.titleName = "Firefox"
	f.titleSource = "fma"

	cfg := func(labels string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "test" {
  fleet_maintained_app_id = 1
%[2]s
}
`, f.srv.URL, labels)
	}

	priorPatchCount := 0
	requirePatchAt := func(check func(*fakeFleetSoftwareServer) error) func(*terraform.State) error {
		return func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.patchCount == priorPatchCount {
				return fmt.Errorf("expected a PATCH at this step (count still %d)", priorPatchCount)
			}
			priorPatchCount = f.patchCount
			return check(f)
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(`  labels_include_all = ["Engineering", "macOS"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "labels_include_all.#", "2"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						priorPatchCount = f.patchCount
						if len(f.fmaCreateIncludeAll) != 2 {
							return fmt.Errorf("FMA Add labels_include_all=%v, want 2 entries", f.fmaCreateIncludeAll)
						}
						return nil
					},
				),
			},
			{
				Config: cfg(`  labels_include_any = ["Engineering"]`),
				Check: requirePatchAt(func(f *fakeFleetSoftwareServer) error {
					if !f.patchIncludeFieldSeen {
						return fmt.Errorf("FMA PATCH must include labels_include_any when HCL switches to it")
					}
					return nil
				}),
			},
			{
				Config: cfg(``),
				Check: requirePatchAt(func(f *fakeFleetSoftwareServer) error {
					if f.patchIncludeFieldSeen || f.patchExcludeFieldSeen || f.patchIncludeAllFieldSeen {
						return fmt.Errorf("FMA PATCH must omit labels when HCL drops them; got include=%v exclude=%v include_all=%v",
							f.patchIncludeFieldSeen, f.patchExcludeFieldSeen, f.patchIncludeAllFieldSeen)
					}
					return nil
				}),
			},
		},
	})
}
