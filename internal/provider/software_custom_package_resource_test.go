package provider

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func testAccSoftwareCustomPackageConfig(serverURL, pkgPath string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_custom_package" "test" {
  package_path   = %[2]q
  filename       = "test-app.pkg"
  install_script = "echo install"
}
`, serverURL, pkgPath)
}

func testAccSoftwareCustomPackageConfigUpdated(serverURL, pkgPath string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_custom_package" "test" {
  package_path   = %[2]q
  filename       = "test-app.pkg"
  install_script = "echo updated"
  self_service   = true
}
`, serverURL, pkgPath)
}

// TestAccSoftwareCustomPackageResource_basic exercises the happy path:
// Create against a fake Fleet that accepts the multipart upload and
// returns a software_title GET shaped like a custom package.
func TestAccSoftwareCustomPackageResource_basic(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/software/package" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_package": map[string]any{"title_id": 42, "team_id": 0},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/42" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_title": map[string]any{
					"id":             42,
					"name":           "test-app.pkg",
					"source":         "pkg",
					"hosts_count":    0,
					"versions_count": 1,
					"software_package": map[string]any{
						"title_id":    42,
						"platform":    "darwin",
						"hash_sha256": hex.EncodeToString(sumOf([]byte("FAKEPKG"))),
					},
					"versions": []map[string]any{
						{"id": 1, "version": "1.0.0", "hosts_count": 0},
					},
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/42/available_for_install" && r.Method == http.MethodDelete:
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
				Config: testAccSoftwareCustomPackageConfig(server.URL, pkgPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "title_id", "42"),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "name", "test-app.pkg"),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "self_service", "false"),
					resource.TestCheckResourceAttrSet("fleetdm_software_custom_package.test", "package_sha256"),
				),
			},
		},
	})
}

// TestAccSoftwareCustomPackageResource_metadataUpdateUsesMultipart verifies
// that a metadata-only Update produces a PATCH that is multipart/form-data
// (Fleet's PATCH /software/titles/{id}/package endpoint rejects JSON).
// This guards against the bug class fixed in PR #50.
func TestAccSoftwareCustomPackageResource_metadataUpdateUsesMultipart(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}

	f := newFakeFleetSoftwareServer(t)
	f.titleID = 42 // align with the title ID this resource will return
	patchContentTypes := []string{}

	// Wrap the server's handler so we can capture PATCH Content-Type.
	wrapped := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/fleet/software/titles/42/package" && r.Method == http.MethodPatch {
			patchContentTypes = append(patchContentTypes, r.Header.Get("Content-Type"))
		}
		f.srv.Config.Handler.ServeHTTP(w, r)
	}))
	defer wrapped.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: testAccSoftwareCustomPackageConfig(wrapped.URL, pkgPath)},
			{
				Config: testAccSoftwareCustomPackageConfigUpdated(wrapped.URL, pkgPath),
				Check: func(_ *terraform.State) error {
					if len(patchContentTypes) == 0 {
						return fmt.Errorf("expected at least one PATCH, got none")
					}
					for i, ct := range patchContentTypes {
						if !strings.HasPrefix(ct, "multipart/form-data;") {
							return fmt.Errorf("patch #%d Content-Type must start with multipart/form-data;, got %q", i, ct)
						}
					}
					return nil
				},
			},
		},
	})
}

// TestAccSoftwareCustomPackageResource_wrongTypeOnImport confirms the
// Read-time wrong-type guard refuses to populate state when a user
// imports a VPP title (app_store_app shape) into this resource.
func TestAccSoftwareCustomPackageResource_wrongTypeOnImport(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/software/titles/888" && r.Method == http.MethodGet:
			// Title 888 exists but is a VPP app, NOT a custom package.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_title": map[string]any{
					"id":             888,
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
		case r.URL.Path == "/api/v1/fleet/software/titles/888/available_for_install" && r.Method == http.MethodDelete:
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

resource "fleetdm_software_custom_package" "imp" {
  package_path = %[2]q
  filename     = "test-app.pkg"
}
`, server.URL, pkgPath)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:            cfg,
				ResourceName:      "fleetdm_software_custom_package.imp",
				ImportState:       true,
				ImportStateId:     "888",
				ImportStateVerify: false,
				ExpectError:       regexp.MustCompile(`(?i)Wrong software type|use fleetdm_software_app_store_app`),
			},
		},
	})
}

// TestAccSoftwareCustomPackageResource_conflictingLabels exercises the
// three-way ConflictsWith matrix on the new resource's schema. Same shape
// as the legacy _conflictingLabels test.
func TestAccSoftwareCustomPackageResource_conflictingLabels(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}

	config := func(labels string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = "http://localhost:1"
  api_key        = "test-token"
}

resource "fleetdm_software_custom_package" "test" {
  package_path = %[1]q
  filename     = "test-app.pkg"
%[2]s
}
`, pkgPath, labels)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(`
  labels_include_any = ["A"]
  labels_exclude_any = ["B"]`),
				ExpectError: regexp.MustCompile(`(?i)Invalid Attribute Combination|labels_exclude_any|labels_include_any`),
			},
		},
	})
}
