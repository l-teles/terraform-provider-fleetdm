package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
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
						"hash_sha256": "ac7f05f70feb6201886d8a27a004bc322e7ba578262c984a213f48089e162183",
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

func TestAccSoftwarePackageResource_s3(t *testing.T) {
	contentV1 := []byte("FAKES3PKG")
	contentV2 := []byte("FAKES3PKGv2")
	shaV1 := "156d5f3dc917f38e5bb9d9f9609ba2cc8f7147148a2247f5e83e57eab9209439"

	// Mutex-protected state shared between test goroutine and httptest handlers.
	var mu sync.Mutex
	currentS3Content := contentV1
	currentFleetSHA := shaV1

	// Mock S3 server that serves the package content via path-style requests.
	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test-bucket/test.pkg" {
			mu.Lock()
			data := currentS3Content
			mu.Unlock()
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(data)
			return
		}
		http.NotFound(w, r)
	}))
	defer s3Server.Close()

	// Set dummy AWS credentials so the S3 SDK does not fail.
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	// Mock Fleet server that accepts the upload and returns title metadata.
	fleetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/software/package" && r.Method == "POST":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"software_package": map[string]interface{}{
					"title_id": 55,
					"team_id":  0,
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/55/package" && r.Method == "PATCH":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v1/fleet/software/titles/55" && r.Method == "GET":
			mu.Lock()
			sha := currentFleetSHA
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{
				"software_title": map[string]interface{}{
					"id":             55,
					"name":           "test.pkg",
					"display_name":   "Test S3 Package",
					"source":         "pkg",
					"hosts_count":    0,
					"versions_count": 1,
					"software_package": map[string]interface{}{
						"title_id":    55,
						"hash_sha256": sha,
					},
					"versions": []map[string]interface{}{
						{"id": 1, "version": "1.0.0", "hosts_count": 0},
					},
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/55/available_for_install" && r.Method == "DELETE":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer fleetServer.Close()

	v2Hash := sha256.Sum256(contentV2)
	shaV2 := hex.EncodeToString(v2Hash[:])

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: Create with v1 content.
			{
				Config: testAccSoftwarePackageResourceConfig_s3(fleetServer.URL, s3Server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.s3_test", "title_id", "55"),
					resource.TestCheckResourceAttr("fleetdm_software_package.s3_test", "name", "test.pkg"),
					resource.TestCheckResourceAttr("fleetdm_software_package.s3_test", "package_sha256", shaV1),
				),
			},
			// Step 2: S3 content changes → update should detect SHA mismatch and re-upload.
			{
				PreConfig: func() {
					mu.Lock()
					currentS3Content = contentV2
					currentFleetSHA = shaV2
					mu.Unlock()
				},
				Config: testAccSoftwarePackageResourceConfig_s3(fleetServer.URL, s3Server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.s3_test", "title_id", "55"),
					resource.TestCheckResourceAttr("fleetdm_software_package.s3_test", "package_sha256", shaV2),
				),
			},
		},
	})
}

func testAccSoftwarePackageResourceConfig_s3(fleetURL, s3URL string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_package" "s3_test" {
  filename = "test.pkg"

  package_s3 = {
    bucket       = "test-bucket"
    key          = "test.pkg"
    region       = "us-east-1"
    endpoint_url = %[2]q
  }
}
`, fleetURL, s3URL)
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
