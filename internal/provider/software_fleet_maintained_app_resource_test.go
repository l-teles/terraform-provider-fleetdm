package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
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

// TestAccSoftwareFleetMaintainedAppResource_importBackfillsCatalogID
// verifies the post-import Read resolves fleet_maintained_app_id by
// listing the team's FMA catalog. Without this backfill, the imported
// state has fleet_maintained_app_id=null while the HCL config provides
// a real value, and the RequiresReplace plan modifier on that field
// schedules a destroy/recreate of the freshly imported title.
func TestAccSoftwareFleetMaintainedAppResource_importBackfillsCatalogID(t *testing.T) {
	const catalogID = 7777
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 555
	f.titleName = "test-app"
	f.titleSource = "fma"
	// Simulate a previously-created FMA title: the catalog ID would have
	// been set on Create; we set it directly so the list endpoint reports
	// the title-to-catalog mapping at import time.
	f.fmaCreateAppID = catalogID

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "imp" {
  fleet_maintained_app_id = %[2]d
}
`, f.srv.URL, catalogID)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             cfg,
				ResourceName:       "fleetdm_software_fleet_maintained_app.imp",
				ImportState:        true,
				ImportStateId:      "555",
				ImportStatePersist: true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 imported state, got %d", len(states))
					}
					got := states[0].Attributes["fleet_maintained_app_id"]
					want := fmt.Sprintf("%d", catalogID)
					if got != want {
						return fmt.Errorf("expected fleet_maintained_app_id=%s after import backfill, got %q", want, got)
					}
					return nil
				},
			},
			{
				// Locks in the actual fix: against the imported state, the HCL
				// must produce a no-op plan. Without the Read-time backfill,
				// fleet_maintained_app_id would still be null in state and the
				// RequiresReplace plan modifier would schedule a destroy/recreate
				// here, failing the step.
				Config:   cfg,
				PlanOnly: true,
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
		case r.URL.Path == "/api/v1/fleet/global/policies" && r.Method == http.MethodGet:
			// Delete handler enumerates policies to detach install_software /
			// patch_software automation before issuing the DELETE.
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

// TestAccSoftwareFleetMaintainedAppResource_omittedScriptsStayWithFleet pins the
// ownership rule for a Fleet Maintained App's scripts: when the HCL omits them,
// Fleet owns them. Fleet maintains these scripts upstream and regenerates them
// as it publishes new versions of the app, so Terraform must neither store a
// copy nor plan a change when Fleet's value moves.
//
// The second step changes the script server-side — standing in for both an
// upstream Fleet update and an edit made through the Fleet UI, which are
// indistinguishable on the wire — and requires the plan to stay empty.
func TestAccSoftwareFleetMaintainedAppResource_omittedScriptsStayWithFleet(t *testing.T) {
	const defaultInstall = "#!/bin/sh\ninstaller -pkg \"$INSTALLER_PATH\" -target /\n"
	const defaultUninstall = "#!/bin/sh\n/usr/local/bin/uninstaller --quiet\n"
	const upstreamInstall = "#!/bin/sh\n# regenerated by Fleet for a new version\ninstaller -pkg \"$INSTALLER_PATH\" -target / --verbose\n"

	var mu sync.Mutex
	installScript := defaultInstall
	setInstallScript := func(s string) {
		mu.Lock()
		installScript = s
		mu.Unlock()
	}
	currentInstallScript := func() string {
		mu.Lock()
		defer mu.Unlock()
		return installScript
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/software/fleet_maintained_apps" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"software_title_id": 201})
		case r.URL.Path == "/api/v1/fleet/software/titles/201" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_title": map[string]any{
					"id":             201,
					"name":           "Firefox",
					"source":         "pkg_packages",
					"hosts_count":    0,
					"versions_count": 1,
					"software_package": map[string]any{
						"name":             "Firefox",
						"version":          "125.0",
						"platform":         "darwin",
						"self_service":     true,
						"install_script":   currentInstallScript(),
						"uninstall_script": defaultUninstall,
					},
					"versions": []map[string]any{
						{"id": 1, "version": "125.0", "hosts_count": 0},
					},
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/201/available_for_install" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/v1/fleet/global/policies" && r.Method == http.MethodGet:
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
				Config: testAccSoftwareFleetMaintainedAppConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "title_id", "201"),
					// Fleet's generated scripts must not be mirrored into state:
					// Terraform does not own what it was not asked to manage.
					resource.TestCheckNoResourceAttr("fleetdm_software_fleet_maintained_app.test", "install_script"),
					resource.TestCheckNoResourceAttr("fleetdm_software_fleet_maintained_app.test", "uninstall_script"),
				),
			},
			{
				// Fleet regenerates the install script upstream. Refreshing must
				// not turn that into a diff, and nothing may be planned to undo it.
				PreConfig:          func() { setInstallScript(upstreamInstall) },
				Config:             testAccSoftwareFleetMaintainedAppConfig(server.URL),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				// The upstream value is still absent from state afterwards.
				Config: testAccSoftwareFleetMaintainedAppConfig(server.URL),
				Check:  resource.TestCheckNoResourceAttr("fleetdm_software_fleet_maintained_app.test", "install_script"),
			},
		},
	})
}

// TestAccSoftwareFleetMaintainedAppResource_omittedScriptSurvivesUnrelatedUpdate
// covers the failure mode that made mirroring the script dangerous rather than
// merely noisy: the metadata PATCH carries every script field, so an update
// triggered by an unrelated attribute used to rewrite the install script with
// whatever Terraform had stored. An undeclared script must be omitted from that
// request entirely, leaving Fleet's script in place.
func TestAccSoftwareFleetMaintainedAppResource_omittedScriptSurvivesUnrelatedUpdate(t *testing.T) {
	const fleetOwned = "#!/bin/sh\n# maintained by Fleet\ninstall\n"

	f := newFakeFleetSoftwareServer(t)
	f.titleID = 512
	f.titleName = "Firefox"

	cfg := func(selfService bool) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "test" {
  fleet_maintained_app_id = 1
  self_service            = %[2]t
}
`, f.srv.URL, selfService)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(true),
				Check:  resource.TestCheckNoResourceAttr("fleetdm_software_fleet_maintained_app.test", "install_script"),
			},
			{
				// Fleet now has a script Terraform never saw, and the config
				// changes an unrelated attribute.
				PreConfig: func() {
					f.mu.Lock()
					f.titleInstallScript = fleetOwned
					f.patchInstallScriptSeen = false
					f.patchUninstallScriptSeen = false
					f.mu.Unlock()
				},
				Config: cfg(false),
				Check: func(_ *terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					if f.patchSelfService != "false" {
						return fmt.Errorf("expected the self_service change to be sent, got %q", f.patchSelfService)
					}
					if f.patchInstallScriptSeen {
						return fmt.Errorf("install_script must be omitted from the PATCH when Fleet owns it, got %q", f.patchInstallScript)
					}
					if f.patchUninstallScriptSeen {
						return errors.New("uninstall_script must be omitted from the PATCH when Fleet owns it")
					}
					if f.titleInstallScript != fleetOwned {
						return fmt.Errorf("Fleet's script was overwritten: %q", f.titleInstallScript)
					}
					// This resource declares no team_id, so the scope must be
					// Fleet's "no team" value rather than merely present.
					if f.patchFleetID != "0" {
						return fmt.Errorf("expected fleet_id=0 for a title with no team_id, got %q", f.patchFleetID)
					}
					return nil
				},
			},
		},
	})
}

// TestAccSoftwareFleetMaintainedAppResource_declaredScriptEmptiedIsDetected
// covers the edge the ownership refresh must not miss: a Fleet-side edit that
// blanks an owned script. Refreshing only non-empty values would leave state
// holding the configured script, so the plan would be empty and hosts would keep
// running with no install script — the drift hardest to notice, silently
// undetected. Fleet rejects an empty install script for the package types that
// require one (verified: HTTP 400 "Install script is required for .zip
// packages"), but the resource must not depend on that server-side invariant
// holding for every package type and version.
func TestAccSoftwareFleetMaintainedAppResource_declaredScriptEmptiedIsDetected(t *testing.T) {
	const declared = "#!/bin/sh\n# owned by Terraform\ninstall --managed\n"

	f := newFakeFleetSoftwareServer(t)
	f.titleID = 514
	f.titleName = "Firefox"

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "test" {
  fleet_maintained_app_id = 1
  install_script          = %[2]q
}
`, f.srv.URL, declared)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check:  resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "install_script", declared),
			},
			{
				PreConfig: func() {
					f.mu.Lock()
					f.titleInstallScript = ""
					f.mu.Unlock()
				},
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: cfg,
				Check: func(_ *terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					if f.titleInstallScript != declared {
						return fmt.Errorf("expected the emptied script to be restored, got %q", f.titleInstallScript)
					}
					return nil
				},
			},
		},
	})
}

// TestAccSoftwareFleetMaintainedAppResource_declaredScriptIsManaged is the other
// half of the ownership rule: declaring install_script hands ownership to
// Terraform, so an out-of-band edit is detected on refresh and corrected on
// apply.
func TestAccSoftwareFleetMaintainedAppResource_declaredScriptIsManaged(t *testing.T) {
	const declared = "#!/bin/sh\n# owned by Terraform\ninstall --managed\n"
	const editedInFleet = "#!/bin/sh\n# edited in the Fleet UI\ninstall --tampered\n"

	f := newFakeFleetSoftwareServer(t)
	f.titleID = 513
	f.titleName = "Firefox"

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "test" {
  fleet_maintained_app_id = 1
  install_script          = %[2]q
}
`, f.srv.URL, declared)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check:  resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "install_script", declared),
			},
			{
				// Out-of-band edit must show up as a diff.
				PreConfig: func() {
					f.mu.Lock()
					f.titleInstallScript = editedInFleet
					f.mu.Unlock()
				},
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				// …and applying puts the configured script back.
				Config: cfg,
				Check: func(_ *terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					if !f.patchInstallScriptSeen {
						return errors.New("expected install_script to be sent when Terraform owns it")
					}
					if f.titleInstallScript != declared {
						return fmt.Errorf("expected Fleet's script reverted to the configured value, got %q", f.titleInstallScript)
					}
					return nil
				},
			},
		},
	})
}

// TestAccSoftwareFleetMaintainedAppResource_scriptOwnershipHandedBack completes
// the ownership lifecycle: removing a previously declared script returns it to
// Fleet rather than clearing it. The update must drop the attribute from state
// and omit the field from the PATCH, leaving the last applied script running on
// hosts until Fleet next regenerates it. Sending "" instead would be a very
// quiet outage — Fleet stores an empty script for the package types that permit
// one, and hosts then install nothing.
func TestAccSoftwareFleetMaintainedAppResource_scriptOwnershipHandedBack(t *testing.T) {
	const declaredInstall = "#!/bin/sh\n# owned by Terraform\ninstall --managed\n"
	const declaredUninstall = "#!/bin/sh\n# owned by Terraform\nuninstall --managed\n"

	f := newFakeFleetSoftwareServer(t)
	f.titleID = 515
	f.titleName = "Firefox"

	const addr = "fleetdm_software_fleet_maintained_app.test"

	cfg := func(scripts string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "test" {
  fleet_maintained_app_id = 1
%[2]s
}
`, f.srv.URL, scripts)
	}

	withScripts := cfg(fmt.Sprintf("  install_script   = %q\n  uninstall_script = %q", declaredInstall, declaredUninstall))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: withScripts,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "install_script", declaredInstall),
					resource.TestCheckResourceAttr(addr, "uninstall_script", declaredUninstall),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.titleInstallScript != declaredInstall {
							return fmt.Errorf("expected Fleet to hold the declared script, got %q", f.titleInstallScript)
						}
						return nil
					},
				),
			},
			{
				// Both attributes removed from the configuration.
				PreConfig: func() {
					f.mu.Lock()
					f.patchInstallScriptSeen = false
					f.patchUninstallScriptSeen = false
					f.mu.Unlock()
				},
				Config: cfg(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(addr, "install_script"),
					resource.TestCheckNoResourceAttr(addr, "uninstall_script"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.patchInstallScriptSeen {
							return fmt.Errorf("install_script must be omitted from the hand-back PATCH, got %q", f.patchInstallScript)
						}
						if f.patchUninstallScriptSeen {
							return errors.New("uninstall_script must be omitted from the hand-back PATCH")
						}
						if f.titleInstallScript != declaredInstall {
							return fmt.Errorf("Fleet must keep the last applied script after hand-back, got %q", f.titleInstallScript)
						}
						if f.titleUninstallScript != declaredUninstall {
							return fmt.Errorf("Fleet must keep the last applied uninstall script after hand-back, got %q", f.titleUninstallScript)
						}
						return nil
					},
				),
			},
			{
				// Ownership really is back with Fleet: an upstream change to
				// either script no longer produces a diff.
				PreConfig: func() {
					f.mu.Lock()
					f.titleInstallScript = "#!/bin/sh\n# regenerated by Fleet\ninstall\n"
					f.titleUninstallScript = "#!/bin/sh\n# regenerated by Fleet\nuninstall\n"
					f.mu.Unlock()
				},
				Config:             cfg(""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
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

// TestAccSoftwareFleetMaintainedAppResource_attachedPolicyDoesNotForceReplacement
// is a drift regression test mirroring the custom-package one: an FMA
// created with automatic_install_policy unset (default false) must not flip
// the attribute to true — and thereby plan a forced replacement — when an
// install-software policy is attached to the title out-of-band.
func TestAccSoftwareFleetMaintainedAppResource_attachedPolicyDoesNotForceReplacement(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 252
	f.titleName = "Firefox"
	f.titleSource = "fma"

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "test" {
  fleet_maintained_app_id = 1
}
`, f.srv.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "automatic_install_policy", "false"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						f.titleAutomaticInstallPolicies = []map[string]any{
							{"id": 19, "name": "[Install software] Firefox"},
						}
						return nil
					},
				),
			},
			{
				// Refresh + plan must be a no-op: the attached policy must
				// not flip automatic_install_policy into a replacement.
				Config:   cfg,
				PlanOnly: true,
			},
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "automatic_install_policy", "false"),
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "automatic_install_policies.#", "1"),
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "automatic_install_policies.0.id", "19"),
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

// testAccFMAPinnedVersionConfig builds an FMA resource with an optional extra
// attribute block, used by the version-pin tests below.
func testAccFMAPinnedVersionConfig(serverURL, extra string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "test" {
  fleet_maintained_app_id = 641
  %[2]s
}
`, serverURL, extra)
}

// TestAccSoftwareFleetMaintainedAppResource_pinnedVersionLifecycle is the
// central test for version pinning. The fake Fleet enforces the real 4.90
// constraint — a PATCH carrying `version` alongside anything else is rejected
// with Fleet's own message — so this exercises the thing that actually breaks
// if the pin is ever folded into the metadata PATCH.
//
// It walks: create-with-pin (Add can't carry a version, so it must be
// create-then-patch), pin change, a step that changes metadata AND the pin
// together (two sequential requests), and unpin via "".
func TestAccSoftwareFleetMaintainedAppResource_pinnedVersionLifecycle(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 641
	f.titleName = "Itsycal"

	// Each step asserts against the wire state left by that step's requests.
	checkPin := func(wantPin string, wantVersionPatches int) resource.TestCheckFunc {
		return func(*terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.patchVersionValue != wantPin {
				return fmt.Errorf("expected the last version-only PATCH to send %q, got %q", wantPin, f.patchVersionValue)
			}
			if f.patchVersionOnlyCount != wantVersionPatches {
				return fmt.Errorf("expected %d version-only PATCHes so far, got %d", wantVersionPatches, f.patchVersionOnlyCount)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Create with a pin. Fleet's Add-FMA endpoint has no version
				// field, so the pin has to arrive as a follow-up request.
				Config: testAccFMAPinnedVersionConfig(f.srv.URL, `pinned_version = "0.15.12"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "pinned_version", "0.15.12"),
					checkPin("0.15.12", 1),
				),
			},
			{
				// Pin-only change: exactly one more version-only PATCH.
				Config: testAccFMAPinnedVersionConfig(f.srv.URL, `pinned_version = "^0"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "pinned_version", "^0"),
					checkPin("^0", 2),
				),
			},
			{
				// Metadata AND pin in one apply. The metadata PATCH must not
				// carry the version (the fake would 400), and the pin still has
				// to land — so this step proves the sequential-PATCH behavior.
				Config: testAccFMAPinnedVersionConfig(f.srv.URL, "pinned_version = \"0.15.12\"\n  self_service   = true"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "pinned_version", "0.15.12"),
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "self_service", "true"),
					checkPin("0.15.12", 3),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						// A metadata PATCH happened too, so the total count is
						// strictly greater than the version-only count.
						if f.patchCount <= f.patchVersionOnlyCount {
							return fmt.Errorf("expected a separate metadata PATCH alongside the pin; total=%d version-only=%d",
								f.patchCount, f.patchVersionOnlyCount)
						}
						return nil
					},
				),
			},
			{
				// Unpin: an empty string is a real request, not a no-op, and
				// Fleet then stops echoing pinned_version at all.
				Config: testAccFMAPinnedVersionConfig(f.srv.URL, `pinned_version = ""`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "pinned_version", ""),
					checkPin("", 4),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.pinnedVersion != nil {
							return fmt.Errorf("expected the pin to be cleared server-side, got %q", *f.pinnedVersion)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccSoftwareFleetMaintainedAppResource_pinnedVersionOptIn locks in the
// opt-in convention: with the attribute omitted from HCL the provider must
// never send a version, and a pin set out-of-band (Fleet UI, GitOps) must not
// be adopted into state — otherwise Terraform would start reporting drift on a
// field the user never asked it to manage.
func TestAccSoftwareFleetMaintainedAppResource_pinnedVersionOptIn(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 642
	f.titleName = "Itsycal"
	// Someone pinned this title in the Fleet UI.
	uiPin := "0.15.12"
	f.pinnedVersion = &uiPin

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "test" {
  fleet_maintained_app_id = 642
}
`, f.srv.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("fleetdm_software_fleet_maintained_app.test", "pinned_version"),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.patchVersionOnlyCount != 0 {
							return fmt.Errorf("expected no version PATCH when pinned_version is omitted, got %d", f.patchVersionOnlyCount)
						}
						if f.pinnedVersion == nil || *f.pinnedVersion != uiPin {
							return fmt.Errorf("the UI-set pin must be left untouched, got %v", f.pinnedVersion)
						}
						return nil
					},
				),
			},
			{
				// And the unmanaged pin must not create a perpetual diff.
				Config:   cfg,
				PlanOnly: true,
			},
		},
	})
}

// TestAccSoftwareFleetMaintainedAppResource_pinnedVersionDriftDetected is the
// payoff of Fleet echoing pinned_version on read: once the attribute IS
// managed, an out-of-band change to the pin shows up as a plan diff instead of
// silently persisting.
func TestAccSoftwareFleetMaintainedAppResource_pinnedVersionDriftDetected(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 643
	f.titleName = "Itsycal"

	cfg := testAccFMAPinnedVersionConfigFor(f.srv.URL, 643, `pinned_version = "0.15.12"`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check:  resource.TestCheckResourceAttr("fleetdm_software_fleet_maintained_app.test", "pinned_version", "0.15.12"),
			},
			{
				// Simulate someone unpinning the title in the Fleet UI, then
				// refresh: the managed attribute must pick the change up.
				PreConfig: func() {
					f.mu.Lock()
					f.pinnedVersion = nil
					f.mu.Unlock()
				},
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// testAccFMAPinnedVersionConfigFor is testAccFMAPinnedVersionConfig with an
// explicit catalog ID, needed because the fake keys its title GET off titleID.
func testAccFMAPinnedVersionConfigFor(serverURL string, appID int, extra string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_fleet_maintained_app" "test" {
  fleet_maintained_app_id = %[2]d
  %[3]s
}
`, serverURL, appID, extra)
}

// TestAccSoftwareFleetMaintainedAppResource_pinnedVersionCaretValidation pins
// the plan-time guard for the one caret mistake Fleet rejects: a caret
// constraint naming more than the major version ("only the major version can
// be specified with a caret (^), without including minor and patch versions",
// verified against a live Fleet v4.90.0). Valid shapes — exact versions,
// bare-major carets, and the empty unpin — must pass validation untouched.
func TestAccSoftwareFleetMaintainedAppResource_pinnedVersionCaretValidation(t *testing.T) {
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 307

	invalid := []struct {
		name string
		pin  string
	}{
		{name: "caret with minor", pin: "^147.2"},
		{name: "caret with minor and patch", pin: "^147.2.1"},
		{name: "bare caret", pin: "^"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      testAccFMAPinnedVersionConfig(f.srv.URL, fmt.Sprintf("pinned_version = %q", tt.pin)),
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(`(?is)caret constraint may only specify the major\s+version`),
					},
				},
			})
		})
	}
}
