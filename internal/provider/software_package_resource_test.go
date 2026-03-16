package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSoftwarePackageResource_basic(t *testing.T) {
	// Write a minimal fake .pkg file to a temp path.
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/software/package" && r.Method == "POST":
			// Return the upload response with title_id so the client can fetch the title.
			json.NewEncoder(w).Encode(map[string]interface{}{
				"software_package": map[string]interface{}{
					"title_id": 42,
					"team_id":  0,
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/42" && r.Method == "GET":
			// Called by UploadSoftwarePackage after upload and by Read.
			json.NewEncoder(w).Encode(map[string]interface{}{
				"software_title": map[string]interface{}{
					"id":             42,
					"name":           "test-app.pkg",
					"display_name":   "Test App",
					"source":         "pkg",
					"hosts_count":    0,
					"versions_count": 1,
					"software_package": map[string]interface{}{
						"title_id":    42,
						"hash_sha256": "4a15546a2e78673a30dbc0b45e2aef0e3fd0c1a28f5a2f42f22de476e1b70f89",
					},
					"versions": []map[string]interface{}{
						{"id": 1, "version": "1.0.0", "hosts_count": 0},
					},
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/42/available_for_install" && r.Method == "DELETE":
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
				Config: testAccSoftwarePackageResourceConfig(server.URL, pkgPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "title_id", "42"),
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "name", "test-app.pkg"),
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "self_service", "false"),
					resource.TestCheckResourceAttrSet("fleetdm_software_package.test", "package_sha256"),
				),
			},
		},
	})
}

func TestAccSoftwarePackageResource_vpp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/software/app_store_apps" && r.Method == "POST":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"software_title_id": 100,
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/100" && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"software_title": map[string]interface{}{
					"id":             100,
					"name":           "TestFlight",
					"display_name":   "TestFlight",
					"source":         "apps",
					"hosts_count":    0,
					"versions_count": 1,
					"app_store_app": map[string]interface{}{
						"app_store_id":   "899247664",
						"platform":       "darwin",
						"name":           "TestFlight",
						"latest_version": "3.2.0",
						"self_service":   true,
					},
					"versions": []map[string]interface{}{
						{"id": 1, "version": "3.2.0", "hosts_count": 0},
					},
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/100/available_for_install" && r.Method == "DELETE":
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
				Config: testAccSoftwarePackageResourceConfig_vpp(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.vpp_test", "title_id", "100"),
					resource.TestCheckResourceAttr("fleetdm_software_package.vpp_test", "name", "TestFlight"),
					resource.TestCheckResourceAttr("fleetdm_software_package.vpp_test", "type", "vpp"),
					resource.TestCheckResourceAttr("fleetdm_software_package.vpp_test", "app_store_id", "899247664"),
					resource.TestCheckResourceAttr("fleetdm_software_package.vpp_test", "self_service", "true"),
					resource.TestCheckResourceAttr("fleetdm_software_package.vpp_test", "platform", "darwin"),
				),
			},
		},
	})
}

func TestAccSoftwarePackageResource_fleet_maintained(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/software/fleet_maintained_apps" && r.Method == "POST":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"software_title_id": 200,
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/200" && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"software_title": map[string]interface{}{
					"id":             200,
					"name":           "Firefox",
					"display_name":   "Firefox",
					"source":         "pkg_packages",
					"hosts_count":    0,
					"versions_count": 1,
					"software_package": map[string]interface{}{
						"name":         "Firefox",
						"version":      "125.0",
						"platform":     "darwin",
						"self_service": true,
					},
					"versions": []map[string]interface{}{
						{"id": 1, "version": "125.0", "hosts_count": 0},
					},
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/200/available_for_install" && r.Method == "DELETE":
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
				Config: testAccSoftwarePackageResourceConfig_fleet_maintained(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.fma_test", "title_id", "200"),
					resource.TestCheckResourceAttr("fleetdm_software_package.fma_test", "name", "Firefox"),
					resource.TestCheckResourceAttr("fleetdm_software_package.fma_test", "type", "fleet_maintained"),
					resource.TestCheckResourceAttr("fleetdm_software_package.fma_test", "self_service", "true"),
				),
			},
		},
	})
}

func testAccSoftwarePackageResourceConfig_vpp(serverURL string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_package" "vpp_test" {
  type         = "vpp"
  app_store_id = "899247664"
  platform     = "darwin"
  self_service = true
}
`, serverURL)
}

func testAccSoftwarePackageResourceConfig_fleet_maintained(serverURL string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_package" "fma_test" {
  type                    = "fleet_maintained"
  fleet_maintained_app_id = 1
  self_service            = true
}
`, serverURL)
}

func testAccSoftwarePackageResourceConfig(serverURL, pkgPath string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_package" "test" {
  package_path = %[2]q
  filename     = "test-app.pkg"
}
`, serverURL, pkgPath)
}
