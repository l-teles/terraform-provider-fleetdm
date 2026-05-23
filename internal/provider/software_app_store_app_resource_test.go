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
