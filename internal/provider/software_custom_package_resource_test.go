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
		case r.URL.Path == "/api/v1/fleet/global/policies" && r.Method == http.MethodGet:
			// Delete handler enumerates policies to detach install_software /
			// patch_software automation before issuing the DELETE. CI's
			// free-tier Fleet has no teams, so only the global endpoint is hit.
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

// TestAccSoftwareCustomPackageResource_omittedScriptsNoDiff guards the
// install_script / uninstall_script perpetual-diff regression on the custom
// package resource (shares softwareScriptAttributes() with FMA). When the
// config omits both scripts, Fleet generates defaults for the package type
// and returns them on the title GET. Because the attributes are
// Optional+Computed, the provider must adopt Fleet's values into state without
// a plan diff. Under the old Optional-only schema this apply failed with a
// "was null, but now ..." inconsistent-result error.
func TestAccSoftwareCustomPackageResource_omittedScriptsNoDiff(t *testing.T) {
	const defaultInstall = "#!/bin/sh\ninstaller -pkg \"$INSTALLER_PATH\" -target /\n"
	const defaultUninstall = "#!/bin/sh\n/usr/local/bin/uninstaller --quiet\n"

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
				"software_package": map[string]any{"title_id": 43, "team_id": 0},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/43" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_title": map[string]any{
					"id":             43,
					"name":           "test-app.pkg",
					"source":         "pkg",
					"hosts_count":    0,
					"versions_count": 1,
					"software_package": map[string]any{
						"title_id":         43,
						"platform":         "darwin",
						"hash_sha256":      hex.EncodeToString(sumOf([]byte("FAKEPKG"))),
						"install_script":   defaultInstall,
						"uninstall_script": defaultUninstall,
					},
					"versions": []map[string]any{
						{"id": 1, "version": "1.0.0", "hosts_count": 0},
					},
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/43/available_for_install" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/v1/fleet/global/policies" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"policies": []map[string]any{}})
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

resource "fleetdm_software_custom_package" "test" {
  package_path = %[2]q
  filename     = "test-app.pkg"
}
`, server.URL, pkgPath)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "title_id", "43"),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "install_script", defaultInstall),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "uninstall_script", defaultUninstall),
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

// TestAccSoftwareCustomPackageResource_installDuringSetupLifecycle drives
// install_during_setup through Create-true, Update-false, Update-true.
// Each transition must produce a PUT to /setup_experience/software with
// the right title-IDs payload. Reading state after each step confirms the
// resource correctly reflects Fleet's setup-experience set.
func TestAccSoftwareCustomPackageResource_installDuringSetupLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 42

	cfg := func(ids bool) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_custom_package" "test" {
  package_path         = %[2]q
  filename             = "test-app.pkg"
  install_script       = "echo install"
  install_during_setup = %[3]t
}
`, f.srv.URL, pkgPath, ids)
	}

	priorPuts := 0

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "install_during_setup", "true"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.setupExperiencePuts == priorPuts {
							return fmt.Errorf("expected a PUT /setup_experience/software on Create-true, got none")
						}
						priorPuts = f.setupExperiencePuts
						found := false
						for _, id := range f.setupExperienceSet {
							if id == f.titleID {
								found = true
							}
						}
						if !found {
							return fmt.Errorf("expected title %d in setup-experience set after Create-true, got %v", f.titleID, f.setupExperienceSet)
						}
						return nil
					},
				),
			},
			{
				Config: cfg(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "install_during_setup", "false"),
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
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "install_during_setup", "true"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.setupExperiencePuts == priorPuts {
							return fmt.Errorf("expected a PUT /setup_experience/software on Update-true again, got none")
						}
						found := false
						for _, id := range f.setupExperienceSet {
							if id == f.titleID {
								found = true
							}
						}
						if !found {
							return fmt.Errorf("expected title %d in setup-experience set after Update-true-again, got %v", f.titleID, f.setupExperienceSet)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccSoftwareCustomPackageResource_installDuringSetupOmitted verifies
// the opt-in semantics of `install_during_setup`: when the attribute is
// absent from HCL, the provider must NOT call Fleet's setup-experience
// endpoint and must NOT flip the title's install-during-setup state.
// Critical regression guard for the bug where a Default-false on the
// schema turned imported `install_during_setup=true` titles into a
// spurious true → false flip every apply.
func TestAccSoftwareCustomPackageResource_installDuringSetupOmitted(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 42

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_custom_package" "test" {
  package_path   = %[2]q
  filename       = "test-app.pkg"
  install_script = "echo install"
}
`, f.srv.URL, pkgPath)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "install_during_setup", "false"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.setupExperiencePuts != 0 {
							return fmt.Errorf("expected zero PUT /setup_experience/software calls when HCL omits install_during_setup, got %d", f.setupExperiencePuts)
						}
						return nil
					},
				),
			},
			{
				// Re-applying the same HCL must be a no-op for setup-experience.
				Config:   cfg,
				PlanOnly: true,
			},
		},
	})
}

// TestAccSoftwareCustomPackageResource_automaticInstallPolicyOnCreate
// verifies that automatic_install_policy=true on Create sends Fleet's
// `automatic_install=true` form field on the upload, and that the
// Computed automatic_install_policies list surfaces the policies Fleet
// reports.
func TestAccSoftwareCustomPackageResource_automaticInstallPolicyOnCreate(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 51
	// Simulate Fleet creating a policy in response to automatic_install=true.
	f.titleAutomaticInstallPolicies = []map[string]any{
		{"id": 7, "name": "Auto-install test-app.pkg"},
	}

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_custom_package" "test" {
  package_path             = %[2]q
  filename                 = "test-app.pkg"
  install_script           = "echo install"
  automatic_install_policy = true
}
`, f.srv.URL, pkgPath)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "automatic_install_policy", "true"),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "automatic_install_policies.#", "1"),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "automatic_install_policies.0.id", "7"),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "automatic_install_policies.0.name", "Auto-install test-app.pkg"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.uploadAutomaticInstall != "true" {
							return fmt.Errorf("upload form must carry automatic_install=true, got %q", f.uploadAutomaticInstall)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccSoftwareCustomPackageResource_displayNameAndCategoriesLifecycle
// exercises the new display_name + categories attributes across Create
// and Update. Verifies each transition lands on the wire and round-trips
// through state.
func TestAccSoftwareCustomPackageResource_displayNameAndCategoriesLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 61

	cfg := func(displayName, categoriesHCL string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_custom_package" "test" {
  package_path   = %[2]q
  filename       = "test-app.pkg"
  install_script = "echo install"
  display_name   = %[3]q
%[4]s
}
`, f.srv.URL, pkgPath, displayName, categoriesHCL)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("MyApp", `  categories = ["Productivity", "Security"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "display_name", "MyApp"),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "categories.#", "2"),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "categories.0", "Productivity"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.uploadDisplayName != "MyApp" {
							return fmt.Errorf("upload display_name=%q, want MyApp", f.uploadDisplayName)
						}
						if f.uploadCategories == "" {
							return fmt.Errorf("upload form must include categories")
						}
						return nil
					},
				),
			},
			{
				Config: cfg("MyApp Renamed", `  categories = ["Productivity"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "display_name", "MyApp Renamed"),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "categories.#", "1"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.patchDisplayName != "MyApp Renamed" {
							return fmt.Errorf("patch display_name=%q, want MyApp Renamed", f.patchDisplayName)
						}
						if f.patchCategories == "" {
							return fmt.Errorf("patch form must include categories")
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccSoftwareCustomPackageResource_labelsIncludeAllLifecycle covers
// the new labels_include_all attribute end-to-end: set, switch to
// labels_include_any (clearing include_all via empty list), drop entirely.
// Verifies each step's PATCH multipart body carries the right form keys.
func TestAccSoftwareCustomPackageResource_labelsIncludeAllLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 71

	cfg := func(labels string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_custom_package" "test" {
  package_path   = %[2]q
  filename       = "test-app.pkg"
  install_script = "echo install"
%[3]s
}
`, f.srv.URL, pkgPath, labels)
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
			},
			{
				Config: cfg(`  labels_include_any = ["Engineering"]`),
				Check: requirePatchAt(func(f *fakeFleetSoftwareServer) error {
					if !f.patchIncludeFieldSeen {
						return fmt.Errorf("PATCH must include labels_include_any when HCL switches to it")
					}
					return nil
				}),
			},
			{
				Config: cfg(``),
				Check: requirePatchAt(func(f *fakeFleetSoftwareServer) error {
					if f.patchIncludeFieldSeen || f.patchExcludeFieldSeen {
						return fmt.Errorf("PATCH must omit labels when HCL drops them; got include=%v exclude=%v", f.patchIncludeFieldSeen, f.patchExcludeFieldSeen)
					}
					return nil
				}),
			},
		},
	})
}
