package provider

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
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

// TestAccSoftwareCustomPackageResource_attachedPolicyDoesNotForceReplacement
// is a drift regression test: a package created with automatic_install_policy
// unset (default false) must not flip the attribute to true — and, since the
// attribute is ForceNew, plan a destroy/recreate on every run — when an
// install-software policy is attached to the title out-of-band (a
// first-class fleetdm_policy resource or the Fleet UI). The attached policy
// must still surface in the Computed automatic_install_policies list.
func TestAccSoftwareCustomPackageResource_attachedPolicyDoesNotForceReplacement(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFakeFleetSoftwareServer(t)
	f.titleID = 52

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
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "automatic_install_policy", "false"),
					func(_ *terraform.State) error {
						// Attach an install policy out-of-band, as a
						// fleetdm_policy resource or a UI admin would.
						f.mu.Lock()
						defer f.mu.Unlock()
						f.titleAutomaticInstallPolicies = []map[string]any{
							{"id": 9, "name": "[Install software] test-app"},
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
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "automatic_install_policy", "false"),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "automatic_install_policies.#", "1"),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "automatic_install_policies.0.id", "9"),
				),
			},
			{
				// An EXPLICIT config change of the ForceNew attribute must
				// still plan a replacement — the drift fix only stops
				// refresh-side flapping, not deliberate reconfiguration
				// (Fleet only honors the flag at create).
				Config: strings.Replace(cfg, "  install_script = \"echo install\"\n",
					"  install_script           = \"echo install\"\n  automatic_install_policy = true\n", 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_software_custom_package.test", plancheck.ResourceActionReplace),
					},
				},
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
						if len(f.uploadCategories) == 0 {
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
						if len(f.patchCategories) == 0 {
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

// TestAccSoftwareCustomPackageResource_pythonScriptInstaller covers Fleet
// 4.90's `.py` script installers. Support is entirely extension-driven on
// Fleet's side — the provider just uploads the bytes and the filename — so
// there is no client change to test; what needs guarding is that the resource
// round-trips a `.py` upload without the provider getting in the way.
//
// Two Fleet behaviors make script installers different from regular packages,
// both confirmed in Fleet's 4.90 source:
//
//   - The file contents ARE the install script, so Fleet ignores a
//     user-supplied install_script on both add and edit rather than erroring.
//     That matters because the provider's metadata PATCH always sends
//     install_script — for a `.py` title it sends Fleet's own echoed script
//     back, and Fleet drops it. Hence no perpetual diff, which the PlanOnly
//     step below asserts.
//   - automatic_install is rejected for script packages ("Fleet can't create a
//     policy to detect existing installations for .py packages"), so this
//     config leaves automatic_install_policy unset.
func TestAccSoftwareCustomPackageResource_pythonScriptInstaller(t *testing.T) {
	const pyContents = "#!/usr/bin/env python3\nprint(\"installing\")\n"

	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "tf-acc-installer.py")
	if err := os.WriteFile(pkgPath, []byte(pyContents), 0o600); err != nil {
		t.Fatal(err)
	}

	f := newFakeFleetSoftwareServer(t)
	f.titleID = 910
	f.titleName = "tf-acc-installer.py"
	f.titleHashSHA256 = hex.EncodeToString(sumOf([]byte(pyContents)))
	// Fleet derives the install script from the uploaded file for script
	// packages; mirror that so Read sees what a real server would return.
	f.titleInstallScript = pyContents

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_custom_package" "py" {
  package_path = %[2]q
  filename     = "tf-acc-installer.py"
  self_service = true
}
`, f.srv.URL, pkgPath)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.py", "title_id", "910"),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.py", "filename", "tf-acc-installer.py"),
					resource.TestCheckResourceAttrSet("fleetdm_software_custom_package.py", "package_sha256"),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						// The provider must not invent an install_script for a
						// script installer — Fleet supplies it from the file.
						if f.uploadInstallScript != "" {
							return fmt.Errorf("expected no install_script on the upload for a .py installer, got %q", f.uploadInstallScript)
						}
						if f.uploadAutomaticInstall == "true" {
							return fmt.Errorf("automatic_install must not be sent for a script installer (Fleet rejects it)")
						}
						return nil
					},
				),
			},
			{
				// Fleet echoes the file contents as install_script; the
				// Optional+Computed attribute must adopt it without a diff.
				Config:   cfg,
				PlanOnly: true,
			},
		},
	})
}

// TestAccSoftwareCustomPackageResource_pythonScriptInstallerLive is the live
// counterpart, run against a real Fleet (skipped without FLEETDM_URL /
// FLEETDM_API_TOKEN). It is the only assertion that Fleet actually accepts the
// extension, which is the whole claim being made — the mock test above can only
// prove the provider doesn't interfere.
//
// Requires Fleet 4.90 or later; earlier versions reject `.py` with
// "File type not supported".
func TestAccSoftwareCustomPackageResource_pythonScriptInstallerLive(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	filename := fmt.Sprintf("tf-acc-%s.py", suffix)

	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, filename)
	// A unique docstring keeps each run's installer bytes distinct, so Fleet
	// doesn't dedupe against a previous run's package.
	contents := fmt.Sprintf("#!/usr/bin/env python3\n\"\"\"tf-acc %s\"\"\"\nprint(\"installing\")\n", suffix)
	if err := os.WriteFile(pkgPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := fmt.Sprintf(`
%[1]s

resource "fleetdm_software_custom_package" "py" {
  package_path = %[2]q
  filename     = %[3]q
  self_service = true
}
`, providerConfig(), pkgPath, filename)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fleetdm_software_custom_package.py", "title_id"),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.py", "filename", filename),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.py", "self_service", "true"),
					// Fleet turns the uploaded file into the install script.
					resource.TestCheckResourceAttrSet("fleetdm_software_custom_package.py", "install_script"),
				),
			},
			{
				// No perpetual diff once Fleet's derived script is in state.
				Config:   cfg,
				PlanOnly: true,
			},
			{
				ResourceName:      "fleetdm_software_custom_package.py",
				ImportState:       true,
				ImportStateVerify: false,
			},
		},
	})
}

// TestAccSoftwareCustomPackageResource_rejectsEmptyLabelName pins the
// plan-time guard on an empty name inside a label or category list. Fleet
// reads these lists as repeated form fields, so a lone empty name is
// indistinguishable on the wire from an explicit clear — accepting it would
// silently drop the software's targeting and make it available to every host.
// The plan must fail before any request is made.
func TestAccSoftwareCustomPackageResource_rejectsEmptyLabelName(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFakeFleetSoftwareServer(t)

	cfg := func(attrLine string) string {
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
`, f.srv.URL, pkgPath, attrLine)
	}

	for _, attr := range []string{
		"labels_include_any",
		"labels_exclude_any",
		"labels_include_all",
		"categories",
	} {
		t.Run(attr, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      cfg(fmt.Sprintf("  %s = [\"\"]", attr)),
						ExpectError: regexp.MustCompile(`(?i)Invalid Attribute Value|at least 1|string length`),
					},
				},
			})
		})
	}
}

// TestAccSoftwareCustomPackageResource_categoriesRoundTrip covers categories
// through the same three states the label attributes get: set on create,
// changed on update, and cleared with an explicit empty list. Categories ride
// the identical repeated-field encoding, and Fleet drops names it does not
// recognise without erroring — so a wrong encoding here is silent, and only a
// round-trip through the fake's mirrored state catches it.
func TestAccSoftwareCustomPackageResource_categoriesRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}

	f := newFakeFleetSoftwareServer(t)
	f.titleID = 92

	cfg := func(categories string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_custom_package" "test" {
  package_path   = %[2]q
  filename       = "test-app.pkg"
  install_script = "echo install"
  self_service   = true
%[3]s
}
`, f.srv.URL, pkgPath, categories)
	}

	const res = "fleetdm_software_custom_package.test"

	wireCategories := func(want ...string) func(*terraform.State) error {
		return func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			got := f.titleCategories
			if len(want) == 0 {
				if len(got) != 0 {
					return fmt.Errorf("Fleet-side categories = %v, want none", got)
				}
				return nil
			}
			if !slices.Equal(got, want) {
				return fmt.Errorf("Fleet-side categories = %v, want %v", got, want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Two names, so a single joined or JSON-encoded field would
				// not survive: it would arrive as one bogus category.
				Config: cfg(`  categories = ["Browsers", "Productivity"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "categories.#", "2"),
					wireCategories("Browsers", "Productivity"),
				),
			},
			{
				Config:   cfg(`  categories = ["Browsers", "Productivity"]`),
				PlanOnly: true,
			},
			{
				Config: cfg(`  categories = ["Developer tools"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "categories.#", "1"),
					wireCategories("Developer tools"),
				),
			},
			{
				// Explicit clear: one empty occurrence, which Fleet reads as
				// "no categories" — distinct from omitting the attribute.
				Config: cfg(`  categories = []`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "categories.#", "0"),
					wireCategories(),
				),
			},
		},
	})
}

// TestAccSoftwareCustomPackageResource_labelOrderIsNotADiff reproduces the
// failure that only showed up against a real Fleet: Fleet returns label names
// in its own order (by label id), which need not match the order written in
// HCL. With a plain list that is a permanent diff — every plan wants to
// rewrite the same set of labels.
//
// The fake echoes the labels back reversed to stand in for Fleet's ordering,
// so the second step's empty-plan check is what proves semantic equality is
// doing its job.
func TestAccSoftwareCustomPackageResource_labelOrderIsNotADiff(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}

	f := newFakeFleetSoftwareServer(t)
	f.titleID = 91
	f.reverseLabelsOnRead = true

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_custom_package" "test" {
  package_path       = %[2]q
  filename           = "test-app.pkg"
  install_script     = "echo install"
  labels_include_any = ["Engineering", "Workstations", "Contractors"]
}
`, f.srv.URL, pkgPath)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					// State keeps the configured order, not Fleet's.
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "labels_include_any.0", "Engineering"),
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "labels_include_any.2", "Contractors"),
				),
			},
			{
				// The regression: a reversed read must not plan a change.
				Config:   cfg,
				PlanOnly: true,
			},
		},
	})
}

// assertFleetLabels asserts Fleet's OWN view of a package's label targeting —
// all three scopes — by reading the installer back over the API. Checking all
// three is what catches a switch between attributes that accumulates instead
// of replacing.
//
// Terraform state is not a usable oracle for the clear step: after a clear,
// Fleet's GET returns no labels, labelsToStringListValue maps that to null,
// and the Read gate declines to refresh a null attribute — so state simply
// keeps the empty list the plan put there. A state-only check therefore
// passes even if Fleet ignored the clear entirely, which is exactly the
// zero-occurrence regression the encoding has to avoid.
func assertFleetLabels(t *testing.T, titleIDAttr string, wantIncludeAny, wantExcludeAny, wantIncludeAll []string) resource.TestCheckFunc {
	t.Helper()
	return func(st *terraform.State) error {
		rs, ok := st.RootModule().Resources[titleIDAttr]
		if !ok {
			return fmt.Errorf("resource %s not found in state", titleIDAttr)
		}
		titleID, err := strconv.Atoi(rs.Primary.Attributes["title_id"])
		if err != nil {
			return fmt.Errorf("title_id: %w", err)
		}

		verifyTLS := true
		if v := os.Getenv("FLEETDM_VERIFY_TLS"); v == "false" || v == "0" {
			verifyTLS = false
		}
		client, err := fleetdm.NewClient(fleetdm.ClientConfig{
			ServerAddress: os.Getenv("FLEETDM_URL"),
			APIKey:        os.Getenv("FLEETDM_API_TOKEN"),
			VerifyTLS:     verifyTLS,
		})
		if err != nil {
			return fmt.Errorf("build fleet client: %w", err)
		}

		// GET /software/titles/{id} is the metadata read the resource itself
		// uses. The sibling /package endpoint is download-only — without
		// alt=media it answers 422 — so it cannot serve as an oracle here.
		title, err := client.GetSoftwareTitle(context.Background(), titleID, nil)
		if err != nil {
			return fmt.Errorf("get software title %d: %w", titleID, err)
		}
		pkg := title.SoftwarePackage
		if pkg == nil {
			return fmt.Errorf("software title %d has no software_package", titleID)
		}

		names := func(labels []fleetdm.SoftwareLabel) []string {
			out := make([]string, 0, len(labels))
			for _, l := range labels {
				out = append(out, l.Name)
			}
			slices.Sort(out)
			return out
		}
		sorted := func(want []string) []string {
			out := append([]string(nil), want...)
			slices.Sort(out)
			return out
		}

		for _, scope := range []struct {
			key  string
			got  []string
			want []string
		}{
			{"labels_include_any", names(pkg.LabelsIncludeAny), sorted(wantIncludeAny)},
			{"labels_exclude_any", names(pkg.LabelsExcludeAny), sorted(wantExcludeAny)},
			{"labels_include_all", names(pkg.LabelsIncludeAll), sorted(wantIncludeAll)},
		} {
			if !slices.Equal(scope.got, scope.want) {
				return fmt.Errorf("Fleet's %s = %v, want %v", scope.key, scope.got, scope.want)
			}
		}
		return nil
	}
}

// TestAccSoftwareCustomPackageResource_labelTargetingLive is the live
// counterpart to the mock label tests, and the only assertion that Fleet
// actually accepts the label names the provider puts on the wire.
//
// It exists because every other software label test drives a fake server:
// the client used to send the names as one JSON-encoded form field, which
// each mock happily accepted while a real Fleet rejected the whole request
// with `Label "[...]" doesn't exist`. Fleet reads these multipart keys as
// repeated fields, so only a real server can catch that class of mistake.
//
// The lifecycle covers all three targeting attributes plus the explicit
// clear (`= []`), which is a distinct wire shape from omitting the
// attribute and has its own way of failing silently.
//
// Requires Fleet 4.90 or later: the package is a `.py` script installer so
// its bytes can be arbitrary, and earlier versions reject that extension
// with "File type not supported".
func TestAccSoftwareCustomPackageResource_labelTargetingLive(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	filename := fmt.Sprintf("tf-acc-%s.py", suffix)

	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, filename)
	contents := fmt.Sprintf("#!/usr/bin/env python3\n\"\"\"tf-acc %s\"\"\"\nprint(\"installing\")\n", suffix)
	if err := os.WriteFile(pkgPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	// Two labels so the multi-name case is exercised: a single name would
	// pass even if the client joined the names into one field.
	cfg := func(labelLine string) string {
		return fmt.Sprintf(`
%[1]s

resource "fleetdm_label" "one" {
  name  = "tf-acc-label-one-%[2]s"
  query = "SELECT 1;"
}

resource "fleetdm_label" "two" {
  name  = "tf-acc-label-two-%[2]s"
  query = "SELECT 1;"
}

resource "fleetdm_software_custom_package" "labelled" {
  package_path = %[3]q
  filename     = %[4]q
%[5]s
}
`, providerConfig(), suffix, pkgPath, filename, labelLine)
	}

	const res = "fleetdm_software_custom_package.labelled"
	one := fmt.Sprintf("tf-acc-label-one-%s", suffix)
	two := fmt.Sprintf("tf-acc-label-two-%s", suffix)
	both := []string{one, two}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Create with two include-any labels: the shape that fails
				// against a real Fleet when the names are JSON-encoded.
				Config: cfg("  labels_include_any = [fleetdm_label.one.name, fleetdm_label.two.name]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(res, "title_id"),
					resource.TestCheckResourceAttr(res, "labels_include_any.#", "2"),
					resource.TestCheckResourceAttr(res, "labels_include_any.0", fmt.Sprintf("tf-acc-label-one-%s", suffix)),
					resource.TestCheckResourceAttr(res, "labels_include_any.1", fmt.Sprintf("tf-acc-label-two-%s", suffix)),
					assertFleetLabels(t, res, both, nil, nil),
				),
			},
			{
				Config:   cfg("  labels_include_any = [fleetdm_label.one.name, fleetdm_label.two.name]"),
				PlanOnly: true,
			},
			{
				// Update to a single name, via PATCH rather than the upload.
				Config: cfg("  labels_include_any = [fleetdm_label.two.name]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "labels_include_any.#", "1"),
					resource.TestCheckResourceAttr(res, "labels_include_any.0", fmt.Sprintf("tf-acc-label-two-%s", suffix)),
					assertFleetLabels(t, res, []string{two}, nil, nil),
				),
			},
			{
				// Explicit clear. Fleet only reads this as "clear" when the
				// field arrives once with an empty value; zero occurrences
				// would leave the label attached and report success, and
				// Terraform state cannot tell the difference — so this step
				// has to ask Fleet.
				Config: cfg("  labels_include_any = []"),
				Check:  assertFleetLabels(t, res, nil, nil, nil),
			},
			{
				// Switch to exclude-any, then include-all: same encoding,
				// different keys, and Fleet rejects more than one at a time.
				Config: cfg("  labels_exclude_any = [fleetdm_label.one.name, fleetdm_label.two.name]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "labels_exclude_any.#", "2"),
					assertFleetLabels(t, res, nil, both, nil),
				),
			},
			{
				// Switching attributes without clearing the old one: Fleet
				// stores a single scope, so the incoming include-all replaces
				// the stored exclude-any wholesale rather than accumulating.
				// Asserting all three scopes is what proves that — and the
				// schema's ConflictsWith rules mean setting the old attribute
				// to [] in the same config is not even expressible.
				Config: cfg("  labels_include_all = [fleetdm_label.one.name, fleetdm_label.two.name]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "labels_include_all.#", "2"),
					assertFleetLabels(t, res, nil, nil, both),
				),
			},
		},
	})
}

// TestAccSoftwareCustomPackageResource_preInstallQueryOwnership covers the same
// ownership rule on this resource's two PATCH paths — the metadata Update and
// the package replacement — which the fleet-maintained-app tests do not reach.
// Fleet writes a managed pre_install_query onto an installer when a patch
// policy targeting it sets patch_when_closed (4.91+), so an omitted attribute
// must leave the field off the wire, while an explicit "" must still be sent to
// clear a query this resource owns.
func TestAccSoftwareCustomPackageResource_preInstallQueryOwnership(t *testing.T) {
	const fleetOwned = "SELECT 1 WHERE NOT EXISTS (SELECT 1 FROM apps WHERE bundle_identifier = 'com.example.app');"
	const declaredQuery = "SELECT 1 FROM os_version;"

	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}

	f := newFakeFleetSoftwareServer(t)
	f.titleID = 43

	cfg := func(selfService bool, queryLine string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_custom_package" "test" {
  package_path   = %[2]q
  filename       = "test-app.pkg"
  install_script = "echo install"
  self_service   = %[3]t
%[4]s
}
`, f.srv.URL, pkgPath, selfService, queryLine)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(true, ""),
				Check:  resource.TestCheckNoResourceAttr("fleetdm_software_custom_package.test", "pre_install_query"),
			},
			{
				// Fleet now owns a query this config never declared, and an
				// unrelated attribute changes: the Update PATCH must not carry
				// pre_install_query at all.
				PreConfig: func() {
					f.mu.Lock()
					f.titlePreInstallQuery = fleetOwned
					f.patchPreInstallQuerySeen = false
					f.mu.Unlock()
				},
				Config: cfg(false, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Asserted here, not just in the first step: this is the
					// only point at which Fleet actually holds a query, so it
					// is the only place a Read that absorbs a Fleet-owned value
					// into state would show up.
					resource.TestCheckNoResourceAttr("fleetdm_software_custom_package.test", "pre_install_query"),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if f.patchSelfService != "false" {
							return fmt.Errorf("expected the self_service change to be sent, got %q", f.patchSelfService)
						}
						if f.patchPreInstallQuerySeen {
							return fmt.Errorf("pre_install_query must be omitted when Fleet owns it, got %q", f.patchPreInstallQuery)
						}
						if f.titlePreInstallQuery != fleetOwned {
							return fmt.Errorf("Fleet's managed query was overwritten: %q", f.titlePreInstallQuery)
						}
						return nil
					},
				),
			},
			{
				// Declaring it takes ownership; the value goes on the wire.
				Config: cfg(false, fmt.Sprintf("  pre_install_query = %q", declaredQuery)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "pre_install_query", declaredQuery),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if !f.patchPreInstallQuerySeen {
							return errors.New("pre_install_query must be sent once Terraform owns it")
						}
						if f.patchPreInstallQuery != declaredQuery {
							return fmt.Errorf("wrong pre_install_query on the wire: got %q, want %q", f.patchPreInstallQuery, declaredQuery)
						}
						return nil
					},
				),
			},
			{
				// An explicit "" clears it, so the field must be present and empty.
				PreConfig: func() {
					f.mu.Lock()
					f.patchPreInstallQuerySeen = false
					f.patchPreInstallQuery = "unset"
					f.mu.Unlock()
				},
				Config: cfg(false, `  pre_install_query = ""`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_custom_package.test", "pre_install_query", ""),
					func(_ *terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if !f.patchPreInstallQuerySeen {
							return errors.New(`an explicit "" must be sent so Fleet clears the query`)
						}
						if f.patchPreInstallQuery != "" {
							return fmt.Errorf("expected an empty pre_install_query on the wire, got %q", f.patchPreInstallQuery)
						}
						if f.titlePreInstallQuery != "" {
							return fmt.Errorf("Fleet should have stored an empty query, got %q", f.titlePreInstallQuery)
						}
						return nil
					},
				),
			},
		},
	})
}
