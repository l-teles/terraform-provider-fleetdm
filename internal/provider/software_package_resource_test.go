package provider

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
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
			json.NewEncoder(w).Encode(map[string]any{
				"software_package": map[string]any{
					"title_id": 42,
					"team_id":  0,
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/42" && r.Method == "GET":
			// Called by UploadSoftwarePackage after upload and by Read.
			json.NewEncoder(w).Encode(map[string]any{
				"software_title": map[string]any{
					"id":             42,
					"name":           "test-app.pkg",
					"display_name":   "Test App",
					"source":         "pkg",
					"hosts_count":    0,
					"versions_count": 1,
					"software_package": map[string]any{
						"title_id":    42,
						"platform":    "darwin",
						"hash_sha256": "ac7f05f70feb6201886d8a27a004bc322e7ba578262c984a213f48089e162183",
					},
					"versions": []map[string]any{
						{"id": 1, "version": "1.0.0", "hosts_count": 0},
					},
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/42/available_for_install" && r.Method == "DELETE":
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
			json.NewEncoder(w).Encode(map[string]any{
				"software_title_id": 100,
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/100" && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]any{
				"software_title": map[string]any{
					"id":             100,
					"name":           "TestFlight",
					"display_name":   "TestFlight",
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
		case r.URL.Path == "/api/v1/fleet/software/titles/100/available_for_install" && r.Method == "DELETE":
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
			json.NewEncoder(w).Encode(map[string]any{
				"software_title_id": 200,
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/200" && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]any{
				"software_title": map[string]any{
					"id":             200,
					"name":           "Firefox",
					"display_name":   "Firefox",
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
		case r.URL.Path == "/api/v1/fleet/software/titles/200/available_for_install" && r.Method == "DELETE":
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

// s3MockMode controls what HEAD response the mock S3 server emits.
type s3MockMode int

const (
	// s3ModeChecksumFullObject — return server-managed full-object SHA256.
	// This is the "fast path" supported scenario.
	s3ModeChecksumFullObject s3MockMode = iota
	// s3ModeChecksumComposite — return composite multipart checksum, which
	// the provider must reject.
	s3ModeChecksumComposite
	// s3ModeMetadataOnly — return only x-amz-meta-sha256, no server checksum.
	s3ModeMetadataOnly
	// s3ModeNoChecksum — return neither, exercising the download fallback.
	s3ModeNoChecksum
)

// s3Mock is a reusable HTTP mock for an S3 bucket with one object. It tracks
// HEAD and GET counts so tests can assert exactly which network operations
// happened. Update `content` to simulate a changed installer; update `mode` to
// switch checksum behavior. All access is mutex-protected for the testing
// terraform-plugin harness which runs handlers from separate goroutines.
type s3Mock struct {
	mu        sync.Mutex
	content   []byte
	mode      s3MockMode
	headCount int
	getCount  int
}

func (m *s3Mock) handler(t *testing.T, bucketKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+bucketKey {
			http.NotFound(w, r)
			return
		}
		m.mu.Lock()
		content := m.content
		mode := m.mode
		switch r.Method {
		case http.MethodHead:
			m.headCount++
		case http.MethodGet:
			m.getCount++
		}
		m.mu.Unlock()

		switch r.Method {
		case http.MethodHead:
			switch mode {
			case s3ModeChecksumFullObject:
				sum := sha256.Sum256(content)
				w.Header().Set("x-amz-checksum-sha256", base64.StdEncoding.EncodeToString(sum[:]))
				w.Header().Set("x-amz-checksum-type", "FULL_OBJECT")
			case s3ModeChecksumComposite:
				w.Header().Set("x-amz-checksum-sha256", base64.StdEncoding.EncodeToString([]byte("hash-of-hashes-irrelevant-32-byt")))
				w.Header().Set("x-amz-checksum-type", "COMPOSITE")
			case s3ModeMetadataOnly:
				sum := sha256.Sum256(content)
				w.Header().Set("x-amz-meta-sha256", hex.EncodeToString(sum[:]))
			case s3ModeNoChecksum:
				// nothing
			}
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(content)
		default:
			t.Errorf("unexpected method %s on %s", r.Method, r.URL.Path)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (m *s3Mock) setContent(b []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.content = b
}

func (m *s3Mock) snapshot() (head, get int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.headCount, m.getCount
}

// fleetSWMockState tracks Fleet API calls during software-package tests.
//
// uploadCount counts POST /software/package (create-path uploads).
// patchBinaryCount counts PATCH /software/titles/{id}/package requests that
// carry a "software" file part (binary-replace path). PATCHes without a file
// part — i.e. metadata-only updates — are deliberately NOT counted here, so
// tests can distinguish "binary actually got replaced" from "scripts/labels
// got updated".
type fleetSWMockState struct {
	mu               sync.Mutex
	currentSHA       string
	uploadCount      int
	deleteCount      int
	patchBinaryCount int
	titleID          int
	titleName        string
	displayName      string
	titleVersion     string
}

// fleetSoftwareHandler returns an http.HandlerFunc that emulates the subset of
// the Fleet software API exercised by these tests (upload, title get, patch,
// delete). It also returns empty global/team policy lists so the
// replaceSoftwarePackage detach-policies scan has something benign to hit
// when a test does not stage any install_software automation.
func fleetSoftwareHandler(t *testing.T, state *fleetSWMockState) http.HandlerFunc {
	titleIDPath := fmt.Sprintf("/api/v1/fleet/software/titles/%d", state.titleID)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/global/policies" && r.Method == http.MethodGet,
			strings.HasPrefix(r.URL.Path, "/api/v1/fleet/fleets/") && strings.HasSuffix(r.URL.Path, "/policies") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"policies": []map[string]any{}})
		case r.URL.Path == "/api/v1/fleet/software/package" && r.Method == "POST":
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Errorf("failed to parse multipart form: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			file, _, err := r.FormFile("software")
			if err != nil {
				t.Errorf("failed to get form file 'software': %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			uploaded, err := io.ReadAll(file)
			file.Close()
			if err != nil {
				t.Errorf("failed to read uploaded file: %v", err)
				http.Error(w, "read error", http.StatusInternalServerError)
				return
			}
			h := sha256.Sum256(uploaded)
			state.mu.Lock()
			state.currentSHA = hex.EncodeToString(h[:])
			state.uploadCount++
			state.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_package": map[string]any{
					"title_id": state.titleID,
					"team_id":  0,
				},
			})
		case r.URL.Path == titleIDPath+"/package" && r.Method == "PATCH":
			// PATCH on this endpoint is dual-mode: metadata-only (no file
			// part) and binary-replace (with a "software" file part). The
			// binary-replace path is the one that updates currentSHA and
			// bumps patchBinaryCount — it's what replaceSoftwarePackage now
			// goes through instead of DELETE+UPLOAD.
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Errorf("failed to parse multipart form on PATCH: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if files := r.MultipartForm.File["software"]; len(files) == 1 {
				f, err := files[0].Open()
				if err != nil {
					t.Errorf("open uploaded file on PATCH: %v", err)
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				uploaded, err := io.ReadAll(f)
				_ = f.Close()
				if err != nil {
					t.Errorf("read uploaded file on PATCH: %v", err)
					http.Error(w, "read error", http.StatusInternalServerError)
					return
				}
				h := sha256.Sum256(uploaded)
				state.mu.Lock()
				state.currentSHA = hex.EncodeToString(h[:])
				state.patchBinaryCount++
				state.mu.Unlock()
			}
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == titleIDPath && r.Method == "GET":
			state.mu.Lock()
			sha := state.currentSHA
			state.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_title": map[string]any{
					"id":             state.titleID,
					"name":           state.titleName,
					"display_name":   state.displayName,
					"source":         "pkg",
					"hosts_count":    0,
					"versions_count": 1,
					"software_package": map[string]any{
						"title_id":    state.titleID,
						"platform":    "darwin",
						"hash_sha256": sha,
					},
					"versions": []map[string]any{
						{"id": 1, "version": state.titleVersion, "hosts_count": 0},
					},
				},
			})
		case r.URL.Path == titleIDPath+"/available_for_install" && r.Method == "DELETE":
			state.mu.Lock()
			state.deleteCount++
			state.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}
}

// snapshotFleet returns the current upload / delete / patch-with-binary counts.
// Tests use these to assert what the provider did on a given step:
//   - uploadCount  ⇢ POST /software/package (create path)
//   - deleteCount  ⇢ DELETE /software/titles/{id}/available_for_install (destroy or legacy replace)
//   - patchBinaryCount ⇢ PATCH /software/titles/{id}/package with a "software" file part (in-place binary replace)
func snapshotFleet(state *fleetSWMockState) (uploads, deletes, patchBinaries int) {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.uploadCount, state.deleteCount, state.patchBinaryCount
}

func TestAccSoftwarePackageResource_s3(t *testing.T) {
	fleetdm.ResetS3ClientCache()
	contentV1 := []byte("FAKES3PKG")
	contentV2 := []byte("FAKES3PKGv2")
	shaV1 := hex.EncodeToString(sumOf(contentV1))
	shaV2 := hex.EncodeToString(sumOf(contentV2))

	s3 := &s3Mock{content: contentV1, mode: s3ModeChecksumFullObject}
	fleet := &fleetSWMockState{currentSHA: shaV1, titleID: 55, titleName: "test.pkg", displayName: "Test S3 Package", titleVersion: "1.0.0"}

	s3Server := httptest.NewServer(s3.handler(t, "test-bucket/test.pkg"))
	defer s3Server.Close()
	fleetServer := httptest.NewServer(fleetSoftwareHandler(t, fleet))
	defer fleetServer.Close()

	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

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
			// Step 2: No-op apply — content unchanged, SHA matches Fleet. The
			// fast path must skip the body download and skip the re-upload.
			{
				PreConfig: func() {
					// Snapshot counts before the no-op step so we can assert
					// what happens *during* it.
					headBefore, getBefore := s3.snapshot()
					uploadsBefore, deletesBefore, patchesBefore := snapshotFleet(fleet)
					t.Logf("Before no-op step: head=%d get=%d uploads=%d deletes=%d patchBinaries=%d", headBefore, getBefore, uploadsBefore, deletesBefore, patchesBefore)
					t.Setenv("__test_s3_head_before", fmt.Sprintf("%d", headBefore))
					t.Setenv("__test_s3_get_before", fmt.Sprintf("%d", getBefore))
					t.Setenv("__test_fleet_uploads_before", fmt.Sprintf("%d", uploadsBefore))
					t.Setenv("__test_fleet_deletes_before", fmt.Sprintf("%d", deletesBefore))
					t.Setenv("__test_fleet_patches_before", fmt.Sprintf("%d", patchesBefore))
				},
				Config: testAccSoftwarePackageResourceConfig_s3(fleetServer.URL, s3Server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.s3_test", "package_sha256", shaV1),
					func(s *terraform.State) error {
						_, getNow := s3.snapshot()
						uploadsNow, deletesNow, patchesNow := snapshotFleet(fleet)
						getBefore, _ := strconv.Atoi(os.Getenv("__test_s3_get_before"))
						uploadsBefore, _ := strconv.Atoi(os.Getenv("__test_fleet_uploads_before"))
						deletesBefore, _ := strconv.Atoi(os.Getenv("__test_fleet_deletes_before"))
						patchesBefore, _ := strconv.Atoi(os.Getenv("__test_fleet_patches_before"))
						if getNow != getBefore {
							return fmt.Errorf("no-op step downloaded the body: get count %d -> %d", getBefore, getNow)
						}
						if uploadsNow != uploadsBefore {
							return fmt.Errorf("no-op step uploaded: upload count %d -> %d", uploadsBefore, uploadsNow)
						}
						if deletesNow != deletesBefore {
							return fmt.Errorf("no-op step deleted: delete count %d -> %d", deletesBefore, deletesNow)
						}
						if patchesNow != patchesBefore {
							return fmt.Errorf("no-op step issued a binary PATCH: patch count %d -> %d", patchesBefore, patchesNow)
						}
						return nil
					},
				),
			},
			// Step 3: S3 content changes → update should detect SHA mismatch and replace
			// the binary via PATCH /software/titles/{id}/package. title_id is preserved
			// (no DELETE, no POST), so the count shape is 1 create-upload + 1
			// binary-PATCH, not 2 uploads + 1 delete.
			{
				PreConfig: func() {
					s3.setContent(contentV2)
				},
				Config: testAccSoftwarePackageResourceConfig_s3(fleetServer.URL, s3Server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.s3_test", "title_id", "55"),
					resource.TestCheckResourceAttr("fleetdm_software_package.s3_test", "package_sha256", shaV2),
					func(s *terraform.State) error {
						uploadsNow, deletesNow, patchesNow := snapshotFleet(fleet)
						if uploadsNow != 1 {
							return fmt.Errorf("expected exactly 1 upload (from create), got %d — the replace path must NOT use the create-path POST", uploadsNow)
						}
						if deletesNow != 0 {
							return fmt.Errorf("expected 0 deletes (PATCH-in-place preserves the title), got %d", deletesNow)
						}
						if patchesNow != 1 {
							return fmt.Errorf("expected exactly 1 binary-PATCH (the replace), got %d", patchesNow)
						}
						return nil
					},
				),
			},
		},
	})
}

// sumOf returns the raw 32-byte SHA256 of b.
func sumOf(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
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

func testAccSoftwarePackageResourceConfig_s3WithExpected(fleetURL, s3URL, expectedSHA string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_package" "s3_test" {
  filename = "test.pkg"

  package_s3 = {
    bucket          = "test-bucket"
    key             = "test.pkg"
    region          = "us-east-1"
    endpoint_url    = %[2]q
    expected_sha256 = %[3]q
  }
}
`, fleetURL, s3URL, expectedSHA)
}

// TestAccSoftwarePackageResource_s3_compositeChecksum confirms apply fails
// with the documented error when S3 only exposes a multipart composite
// checksum (which doesn't equal sha256(content)).
func TestAccSoftwarePackageResource_s3_compositeChecksum(t *testing.T) {
	fleetdm.ResetS3ClientCache()
	content := []byte("doesnt-matter")
	s3 := &s3Mock{content: content, mode: s3ModeChecksumComposite}
	fleet := &fleetSWMockState{titleID: 56, titleName: "test.pkg", displayName: "Composite", titleVersion: "1.0.0"}

	s3Server := httptest.NewServer(s3.handler(t, "test-bucket/test.pkg"))
	defer s3Server.Close()
	fleetServer := httptest.NewServer(fleetSoftwareHandler(t, fleet))
	defer fleetServer.Close()

	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccSoftwarePackageResourceConfig_s3(fleetServer.URL, s3Server.URL),
				ExpectError: regexp.MustCompile(`composite \(multipart\) SHA256 checksum`),
			},
		},
	})
}

// TestAccSoftwarePackageResource_s3_metadataSHA confirms the fast path also
// works when the SHA256 only comes from the x-amz-meta-sha256 header.
func TestAccSoftwarePackageResource_s3_metadataSHA(t *testing.T) {
	fleetdm.ResetS3ClientCache()
	content := []byte("metadata-only-content")
	shaHex := hex.EncodeToString(sumOf(content))
	s3 := &s3Mock{content: content, mode: s3ModeMetadataOnly}
	fleet := &fleetSWMockState{currentSHA: shaHex, titleID: 57, titleName: "test.pkg", displayName: "MetadataOnly", titleVersion: "1.0.0"}

	s3Server := httptest.NewServer(s3.handler(t, "test-bucket/test.pkg"))
	defer s3Server.Close()
	fleetServer := httptest.NewServer(fleetSoftwareHandler(t, fleet))
	defer fleetServer.Close()

	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: Create.
			{
				Config: testAccSoftwarePackageResourceConfig_s3(fleetServer.URL, s3Server.URL),
				Check:  resource.TestCheckResourceAttr("fleetdm_software_package.s3_test", "package_sha256", shaHex),
			},
			// Step 2: No-op apply must not download the body or re-upload.
			{
				PreConfig: func() {
					_, getBefore := s3.snapshot()
					uploadsBefore, _, _ := snapshotFleet(fleet)
					t.Setenv("__metaonly_get_before", fmt.Sprintf("%d", getBefore))
					t.Setenv("__metaonly_uploads_before", fmt.Sprintf("%d", uploadsBefore))
				},
				Config: testAccSoftwarePackageResourceConfig_s3(fleetServer.URL, s3Server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						_, getNow := s3.snapshot()
						uploadsNow, _, _ := snapshotFleet(fleet)
						getBefore, _ := strconv.Atoi(os.Getenv("__metaonly_get_before"))
						uploadsBefore, _ := strconv.Atoi(os.Getenv("__metaonly_uploads_before"))
						if getNow != getBefore {
							return fmt.Errorf("metadata-SHA path triggered a body download: get %d -> %d", getBefore, getNow)
						}
						if uploadsNow != uploadsBefore {
							return fmt.Errorf("metadata-SHA path triggered an upload: %d -> %d", uploadsBefore, uploadsNow)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccSoftwarePackageResource_s3_noChecksum_fallsBackToDownload confirms the
// warn-and-download fallback for objects with no usable SHA256.
func TestAccSoftwarePackageResource_s3_noChecksum_fallsBackToDownload(t *testing.T) {
	fleetdm.ResetS3ClientCache()
	content := []byte("no-checksum-content")
	shaHex := hex.EncodeToString(sumOf(content))
	s3 := &s3Mock{content: content, mode: s3ModeNoChecksum}
	fleet := &fleetSWMockState{currentSHA: shaHex, titleID: 58, titleName: "test.pkg", displayName: "NoChecksum", titleVersion: "1.0.0"}

	s3Server := httptest.NewServer(s3.handler(t, "test-bucket/test.pkg"))
	defer s3Server.Close()
	fleetServer := httptest.NewServer(fleetSoftwareHandler(t, fleet))
	defer fleetServer.Close()

	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: Create succeeds even without a checksum (downloads body).
			{
				Config: testAccSoftwarePackageResourceConfig_s3(fleetServer.URL, s3Server.URL),
				Check:  resource.TestCheckResourceAttr("fleetdm_software_package.s3_test", "package_sha256", shaHex),
			},
			// Step 2: Re-apply with unchanged content. We expect the body to
			// be downloaded again (since we can't shortcut without a SHA),
			// but the SHA still matches so no delete/upload should happen.
			{
				PreConfig: func() {
					_, getBefore := s3.snapshot()
					uploadsBefore, deletesBefore, _ := snapshotFleet(fleet)
					t.Setenv("__nocs_get_before", fmt.Sprintf("%d", getBefore))
					t.Setenv("__nocs_uploads_before", fmt.Sprintf("%d", uploadsBefore))
					t.Setenv("__nocs_deletes_before", fmt.Sprintf("%d", deletesBefore))
				},
				Config: testAccSoftwarePackageResourceConfig_s3(fleetServer.URL, s3Server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						_, getNow := s3.snapshot()
						uploadsNow, deletesNow, _ := snapshotFleet(fleet)
						getBefore, _ := strconv.Atoi(os.Getenv("__nocs_get_before"))
						uploadsBefore, _ := strconv.Atoi(os.Getenv("__nocs_uploads_before"))
						deletesBefore, _ := strconv.Atoi(os.Getenv("__nocs_deletes_before"))
						if getNow == getBefore {
							return fmt.Errorf("fallback path should have downloaded the body, but get count didn't change: %d -> %d", getBefore, getNow)
						}
						if uploadsNow != uploadsBefore {
							return fmt.Errorf("fallback no-op should not re-upload: uploads %d -> %d", uploadsBefore, uploadsNow)
						}
						if deletesNow != deletesBefore {
							return fmt.Errorf("fallback no-op should not delete: deletes %d -> %d", deletesBefore, deletesNow)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccSoftwarePackageResource_s3_expectedSHA confirms expected_sha256 bypasses
// HeadObject and lets users opt-in even when their bucket has no checksum.
func TestAccSoftwarePackageResource_s3_expectedSHA(t *testing.T) {
	fleetdm.ResetS3ClientCache()
	content := []byte("expected-sha-content")
	shaHex := hex.EncodeToString(sumOf(content))
	// Use a mode that returns NOTHING — proves we don't even look at HeadObject.
	s3 := &s3Mock{content: content, mode: s3ModeNoChecksum}
	fleet := &fleetSWMockState{currentSHA: shaHex, titleID: 59, titleName: "test.pkg", displayName: "ExpectedSHA", titleVersion: "1.0.0"}

	s3Server := httptest.NewServer(s3.handler(t, "test-bucket/test.pkg"))
	defer s3Server.Close()
	fleetServer := httptest.NewServer(fleetSoftwareHandler(t, fleet))
	defer fleetServer.Close()

	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: Create — Create always downloads the body (it has to upload
			// it to Fleet), but the cheap-SHA path is still honored everywhere else.
			{
				Config: testAccSoftwarePackageResourceConfig_s3WithExpected(fleetServer.URL, s3Server.URL, shaHex),
				Check:  resource.TestCheckResourceAttr("fleetdm_software_package.s3_test", "package_sha256", shaHex),
			},
			// Step 2: No-op apply — neither HEAD nor GET should be touched
			// for the cheap path (we trust expected_sha256).
			{
				PreConfig: func() {
					headBefore, getBefore := s3.snapshot()
					t.Setenv("__exp_head_before", fmt.Sprintf("%d", headBefore))
					t.Setenv("__exp_get_before", fmt.Sprintf("%d", getBefore))
				},
				Config: testAccSoftwarePackageResourceConfig_s3WithExpected(fleetServer.URL, s3Server.URL, shaHex),
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						headNow, getNow := s3.snapshot()
						headBefore, _ := strconv.Atoi(os.Getenv("__exp_head_before"))
						getBefore, _ := strconv.Atoi(os.Getenv("__exp_get_before"))
						if headNow != headBefore {
							return fmt.Errorf("expected_sha256 path should NOT HEAD the object: head %d -> %d", headBefore, headNow)
						}
						if getNow != getBefore {
							return fmt.Errorf("expected_sha256 path should NOT GET the object: get %d -> %d", getBefore, getNow)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccSoftwarePackageResource_s3_expectedSHA_malformed confirms validation
// rejects a non-hex expected_sha256 at plan time, before any network call.
func TestAccSoftwarePackageResource_s3_expectedSHA_malformed(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "fleetdm" {
  server_address = "http://invalid.test"
  api_key        = "test-token"
}

resource "fleetdm_software_package" "s3_test" {
  filename = "test.pkg"
  package_s3 = {
    bucket          = "b"
    key             = "k"
    expected_sha256 = "nope-not-a-sha"
  }
}
`,
				ExpectError: regexp.MustCompile(`expected_sha256 must be 64 lowercase hexadecimal characters`),
			},
		},
	})
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

// fleetPolicyMockState captures the install_software automation state for a
// single policy plus a record of the PATCH sequence, so tests can assert that
// the provider detaches before delete and reattaches after upload.
type fleetPolicyMockState struct {
	mu                     sync.Mutex
	installSoftwareTitleID *int
	detachCount            int
	reattachCount          int
	patchOrder             []int // 0 = detach (nil), >0 = reattach to that title id
}

// installSoftwarePolicyHandler returns an http.HandlerFunc that emulates the
// minimum subset of /policies endpoints needed to drive
// replaceSoftwarePackage's detach/reattach flow for a single policy whose
// install_software automation references a software title.
//
// Caller supplies the list endpoint (e.g. /api/v1/fleet/global/policies or
// /api/v1/fleet/fleets/{teamID}/policies) and the single-policy endpoint
// (.../{policyID}). The returned handler also delegates anything it doesn't
// know about to fleetSoftwareHandler so the same test mock works end-to-end.
func installSoftwarePolicyHandler(t *testing.T, ps *fleetPolicyMockState, fleet *fleetSWMockState, policyListPath, policyOnePath string) http.HandlerFunc {
	t.Helper()
	softwareHandler := fleetSoftwareHandler(t, fleet)
	snapshot := func() map[string]any {
		ps.mu.Lock()
		defer ps.mu.Unlock()
		one := map[string]any{
			"id":       1,
			"name":     "Install Test App",
			"query":    "SELECT 1",
			"critical": false,
		}
		if ps.installSoftwareTitleID != nil {
			one["install_software"] = map[string]any{
				"name":              "Test App",
				"software_title_id": *ps.installSoftwareTitleID,
			}
		}
		return one
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == policyListPath && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"policies": []map[string]any{
					snapshot(),
					{"id": 2, "name": "Unrelated", "query": "SELECT 1", "critical": false},
				},
			})
		case r.URL.Path == policyOnePath && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"policy": snapshot()})
		case r.URL.Path == policyOnePath && r.Method == http.MethodPatch:
			var body struct {
				SoftwareTitleID *int `json:"software_title_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode patch body: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			ps.mu.Lock()
			ps.installSoftwareTitleID = body.SoftwareTitleID
			if body.SoftwareTitleID == nil {
				ps.detachCount++
				ps.patchOrder = append(ps.patchOrder, 0)
			} else {
				ps.reattachCount++
				ps.patchOrder = append(ps.patchOrder, *body.SoftwareTitleID)
			}
			ps.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"policy": snapshot()})
		default:
			softwareHandler(w, r)
		}
	}
}

func testAccSoftwarePackageResourceConfig_team(serverURL, pkgPath string, teamID int) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_package" "test" {
  package_path = %[2]q
  filename     = "test-app.pkg"
  team_id      = %[3]d
}
`, serverURL, pkgPath, teamID)
}

// TestAccSoftwarePackageResource_replaceWithAttachedPolicy verifies that
// rotating the installer binary while an install_software policy points at
// the title leaves the policy untouched. The new replace path is
// PATCH /software/titles/{id}/package with the binary — title_id is
// preserved (it's in the URL path), so the policy never needs to be
// detached or reattached. The mock counters anchor exactly that:
// 0 detaches, 0 reattaches, 0 DELETEs, 1 binary-PATCH.
func TestAccSoftwarePackageResource_replaceWithAttachedPolicy(t *testing.T) {
	contentV1 := []byte("PKGV1")
	contentV2 := []byte("PKGV2")
	shaV1 := hex.EncodeToString(sumOf(contentV1))
	shaV2 := hex.EncodeToString(sumOf(contentV2))

	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, contentV1, 0o600); err != nil {
		t.Fatal(err)
	}

	fleet := &fleetSWMockState{
		currentSHA:   shaV1,
		titleID:      60,
		titleName:    "test-app.pkg",
		displayName:  "Test App",
		titleVersion: "1.0.0",
	}

	initialTitleRef := 60
	ps := &fleetPolicyMockState{installSoftwareTitleID: &initialTitleRef}

	handler := installSoftwarePolicyHandler(t, ps, fleet, "/api/v1/fleet/global/policies", "/api/v1/fleet/global/policies/1")
	fleetServer := httptest.NewServer(handler)
	defer fleetServer.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: create. Provider must not touch the policy on create.
			{
				Config: testAccSoftwarePackageResourceConfig(fleetServer.URL, pkgPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "title_id", "60"),
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "package_sha256", shaV1),
					func(s *terraform.State) error {
						ps.mu.Lock()
						defer ps.mu.Unlock()
						if ps.detachCount != 0 || ps.reattachCount != 0 {
							return fmt.Errorf("create step must not touch policies, got detach=%d reattach=%d", ps.detachCount, ps.reattachCount)
						}
						return nil
					},
				),
			},
			// Step 2: rotate the installer. SHA changes → replaceSoftwarePackage.
			// The new flow MUST NOT touch the policy (no detach, no reattach,
			// no DELETE) and MUST issue a binary PATCH that updates Fleet's
			// SHA. title_id is preserved across the upgrade, so the policy
			// stays linked at the end.
			{
				PreConfig: func() {
					if err := os.WriteFile(pkgPath, contentV2, 0o600); err != nil {
						t.Fatal(err)
					}
				},
				Config: testAccSoftwarePackageResourceConfig(fleetServer.URL, pkgPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "title_id", "60"),
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "package_sha256", shaV2),
					func(s *terraform.State) error {
						ps.mu.Lock()
						defer ps.mu.Unlock()
						if ps.detachCount != 0 {
							return fmt.Errorf("policy must not be detached on binary replace (PATCH preserves title_id), got %d detaches", ps.detachCount)
						}
						if ps.reattachCount != 0 {
							return fmt.Errorf("policy must not be reattached on binary replace, got %d reattaches", ps.reattachCount)
						}
						if len(ps.patchOrder) != 0 {
							return fmt.Errorf("policy PATCH endpoint must not be hit during a binary replace, got patchOrder=%v", ps.patchOrder)
						}
						if ps.installSoftwareTitleID == nil || *ps.installSoftwareTitleID != 60 {
							return fmt.Errorf("expected policy to still reference software_title_id=60 after replace, got %v", ps.installSoftwareTitleID)
						}
						uploads, deletes, patchBinaries := snapshotFleet(fleet)
						if uploads != 1 {
							return fmt.Errorf("expected exactly 1 upload (from create), got %d — the replace path must NOT use the create-path POST", uploads)
						}
						if deletes != 0 {
							return fmt.Errorf("expected 0 deletes on binary replace (PATCH-in-place preserves the title), got %d", deletes)
						}
						if patchBinaries != 1 {
							return fmt.Errorf("expected exactly 1 binary-PATCH (the replace), got %d", patchBinaries)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccSoftwarePackageResource_replaceWithAttachedPolicy_teamScope is the
// team-scoped sibling of TestAccSoftwarePackageResource_replaceWithAttachedPolicy.
// It guards the scope dispatch — a team-scoped software package's binary
// replace must hit /api/v1/fleet/software/titles/{id}/package?team_id={n}
// and must NOT issue any policy PATCH to either /global/policies/... or
// /fleets/{teamID}/policies/...
func TestAccSoftwarePackageResource_replaceWithAttachedPolicy_teamScope(t *testing.T) {
	contentV1 := []byte("PKGV1TEAM")
	contentV2 := []byte("PKGV2TEAM")
	shaV1 := hex.EncodeToString(sumOf(contentV1))
	shaV2 := hex.EncodeToString(sumOf(contentV2))

	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, contentV1, 0o600); err != nil {
		t.Fatal(err)
	}

	const teamID = 1
	fleet := &fleetSWMockState{
		currentSHA:   shaV1,
		titleID:      61,
		titleName:    "test-app.pkg",
		displayName:  "Test App",
		titleVersion: "1.0.0",
	}

	initialTitleRef := 61
	ps := &fleetPolicyMockState{installSoftwareTitleID: &initialTitleRef}

	handler := installSoftwarePolicyHandler(t, ps, fleet,
		fmt.Sprintf("/api/v1/fleet/fleets/%d/policies", teamID),
		fmt.Sprintf("/api/v1/fleet/fleets/%d/policies/1", teamID),
	)
	fleetServer := httptest.NewServer(handler)
	defer fleetServer.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSoftwarePackageResourceConfig_team(fleetServer.URL, pkgPath, teamID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "title_id", "61"),
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "team_id", "1"),
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "package_sha256", shaV1),
				),
			},
			{
				PreConfig: func() {
					if err := os.WriteFile(pkgPath, contentV2, 0o600); err != nil {
						t.Fatal(err)
					}
				},
				Config: testAccSoftwarePackageResourceConfig_team(fleetServer.URL, pkgPath, teamID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "title_id", "61"),
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "package_sha256", shaV2),
					func(s *terraform.State) error {
						ps.mu.Lock()
						defer ps.mu.Unlock()
						if ps.detachCount != 0 || ps.reattachCount != 0 || len(ps.patchOrder) != 0 {
							return fmt.Errorf("team-scoped binary replace must not touch the team policy, got detach=%d reattach=%d patchOrder=%v", ps.detachCount, ps.reattachCount, ps.patchOrder)
						}
						if ps.installSoftwareTitleID == nil || *ps.installSoftwareTitleID != 61 {
							return fmt.Errorf("expected team policy to still reference software_title_id=61, got %v", ps.installSoftwareTitleID)
						}
						uploads, deletes, patchBinaries := snapshotFleet(fleet)
						if uploads != 1 || deletes != 0 || patchBinaries != 1 {
							return fmt.Errorf("expected (uploads, deletes, patchBinaries) = (1, 0, 1) on team-scoped replace, got (%d, %d, %d)", uploads, deletes, patchBinaries)
						}
						return nil
					},
				),
			},
		},
	})
}

func testAccSoftwarePackageResourceConfig_metadata(serverURL, pkgPath string, selfService bool, installScript string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_package" "test" {
  package_path   = %[2]q
  filename       = "test-app.pkg"
  self_service   = %[3]t
  install_script = %[4]q
}
`, serverURL, pkgPath, selfService, installScript)
}

// TestAccSoftwarePackageResource_metadataOnlyUpdateUsesMultipart drives a
// create + metadata-only update and asserts that the PATCH that goes to
// Fleet's /software/titles/{id}/package endpoint uses multipart/form-data —
// not application/json. Fleet 4.x rejects the JSON shape with HTTP 400
// ("failed to parse multipart form"), so this is the regression guard.
//
// It exercises two different metadata fields — `self_service` (boolean) and
// `install_script` (string) — to make sure both wire-format encodings land
// correctly, not just one.
func TestAccSoftwarePackageResource_metadataOnlyUpdateUsesMultipart(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}

	type patchRecord struct {
		contentType   string
		selfService   string
		installScript string
	}
	var (
		mu                 sync.Mutex
		patches            []patchRecord
		titleSelfService   bool
		titleInstallScript string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/global/policies" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"policies": []map[string]any{}})
		case r.URL.Path == "/api/v1/fleet/software/package" && r.Method == http.MethodPost:
			// On create the upload itself carries the initial metadata; the
			// multipart upload handler already validates the request shape.
			// Parse install_script and self_service so the subsequent GETs
			// reflect them and the test doesn't trip on a refresh-plan diff.
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("create-side ParseMultipartForm failed: %v", err)
			} else {
				mu.Lock()
				titleInstallScript = r.FormValue("install_script")
				titleSelfService = r.FormValue("self_service") == "true"
				mu.Unlock()
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_package": map[string]any{
					"title_id": 70,
					"team_id":  0,
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/70" && r.Method == http.MethodGet:
			mu.Lock()
			selfService := titleSelfService
			installScript := titleInstallScript
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_title": map[string]any{
					"id":             70,
					"name":           "test-app.pkg",
					"display_name":   "Test App",
					"source":         "pkg",
					"hosts_count":    0,
					"versions_count": 1,
					"software_package": map[string]any{
						"title_id":       70,
						"platform":       "darwin",
						"hash_sha256":    hex.EncodeToString(sumOf([]byte("FAKEPKG"))),
						"self_service":   selfService,
						"install_script": installScript,
					},
					"versions": []map[string]any{
						{"id": 1, "version": "1.0.0", "hosts_count": 0},
					},
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/70/package" && r.Method == http.MethodPatch:
			ct := r.Header.Get("Content-Type")
			var selfService, installScript string
			if strings.HasPrefix(ct, "multipart/form-data;") {
				if err := r.ParseMultipartForm(1 << 20); err == nil {
					selfService = r.FormValue("self_service")
					installScript = r.FormValue("install_script")
				}
			}
			mu.Lock()
			patches = append(patches, patchRecord{contentType: ct, selfService: selfService, installScript: installScript})
			titleSelfService = selfService == "true"
			titleInstallScript = installScript
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v1/fleet/software/titles/70/available_for_install" && r.Method == http.MethodDelete:
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
				Config: testAccSoftwarePackageResourceConfig_metadata(server.URL, pkgPath, false, "echo create"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "self_service", "false"),
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "install_script", "echo create"),
				),
			},
			{
				Config: testAccSoftwarePackageResourceConfig_metadata(server.URL, pkgPath, true, "echo updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "self_service", "true"),
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "install_script", "echo updated"),
					func(s *terraform.State) error {
						mu.Lock()
						defer mu.Unlock()
						if len(patches) == 0 {
							return fmt.Errorf("expected at least one PATCH on /software/titles/70/package, got none")
						}
						for i, p := range patches {
							if !strings.HasPrefix(p.contentType, "multipart/form-data;") {
								return fmt.Errorf("patch #%d Content-Type must start with multipart/form-data;, got %q", i, p.contentType)
							}
						}
						last := patches[len(patches)-1]
						if last.selfService != "true" {
							return fmt.Errorf("expected last PATCH to carry self_service=true, got %q", last.selfService)
						}
						if last.installScript != "echo updated" {
							return fmt.Errorf("expected last PATCH to carry install_script=%q, got %q", "echo updated", last.installScript)
						}
						return nil
					},
				),
			},
		},
	})
}

// packageS3AttrTypes mirrors the attribute types of the package_s3 nested
// block, used to construct types.Object values directly in tests.
var packageS3AttrTypes = map[string]attr.Type{
	"bucket":          types.StringType,
	"key":             types.StringType,
	"region":          types.StringType,
	"endpoint_url":    types.StringType,
	"expected_sha256": types.StringType,
}

// newPackageS3 returns a types.Object matching the package_s3 schema with
// the given inner values. Any field not in `values` is treated as null.
func newPackageS3(t *testing.T, values map[string]attr.Value) types.Object {
	t.Helper()
	complete := map[string]attr.Value{
		"bucket":          types.StringNull(),
		"key":             types.StringNull(),
		"region":          types.StringNull(),
		"endpoint_url":    types.StringNull(),
		"expected_sha256": types.StringNull(),
	}
	for k, v := range values {
		complete[k] = v
	}
	obj, diags := types.ObjectValue(packageS3AttrTypes, complete)
	if diags.HasError() {
		t.Fatalf("failed to construct package_s3 object: %v", diags)
	}
	return obj
}

// TestValidatePackageS3_unknownBucketAccepted is the headline regression test:
// `terraform validate` runs without state, so references to module outputs or
// other resources' attributes evaluate to Unknown. Treating Unknown as a
// validation error (as the provider did before this fix) produces false
// positives on configs that resolve correctly at plan time.
func TestValidatePackageS3_unknownBucketAccepted(t *testing.T) {
	s3 := packageS3Model{
		Bucket:         types.StringUnknown(),
		Key:            types.StringUnknown(),
		Region:         types.StringNull(),
		EndpointURL:    types.StringNull(),
		ExpectedSHA256: types.StringNull(),
	}
	diags := validatePackageS3(s3)
	if diags.HasError() {
		t.Errorf("expected no diagnostics for Unknown bucket/key, got: %v", diags)
	}
}

func TestValidatePackageS3_unknownKeyOnlyAccepted(t *testing.T) {
	s3 := packageS3Model{
		Bucket:         types.StringValue("my-bucket"),
		Key:            types.StringUnknown(),
		Region:         types.StringNull(),
		EndpointURL:    types.StringNull(),
		ExpectedSHA256: types.StringNull(),
	}
	diags := validatePackageS3(s3)
	if diags.HasError() {
		t.Errorf("expected no diagnostics for Unknown key alone, got: %v", diags)
	}
}

func TestValidatePackageS3_emptyBucketRejected(t *testing.T) {
	s3 := packageS3Model{
		Bucket:         types.StringValue(""),
		Key:            types.StringValue("k"),
		Region:         types.StringNull(),
		EndpointURL:    types.StringNull(),
		ExpectedSHA256: types.StringNull(),
	}
	diags := validatePackageS3(s3)
	if !diags.HasError() {
		t.Fatal("expected a diagnostic for empty bucket, got none")
	}
	found := false
	for _, d := range diags.Errors() {
		if strings.Contains(d.Detail(), "bucket must not be empty") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a 'bucket must not be empty' diagnostic, got: %v", diags)
	}
}

func TestValidatePackageS3_emptyKeyRejected(t *testing.T) {
	s3 := packageS3Model{
		Bucket:         types.StringValue("my-bucket"),
		Key:            types.StringValue(""),
		Region:         types.StringNull(),
		EndpointURL:    types.StringNull(),
		ExpectedSHA256: types.StringNull(),
	}
	diags := validatePackageS3(s3)
	if !diags.HasError() {
		t.Fatal("expected a diagnostic for empty key, got none")
	}
}

func TestValidatePackageS3_invalidExpectedSHA256Rejected(t *testing.T) {
	s3 := packageS3Model{
		Bucket:         types.StringValue("my-bucket"),
		Key:            types.StringValue("k"),
		Region:         types.StringNull(),
		EndpointURL:    types.StringNull(),
		ExpectedSHA256: types.StringValue("not-a-hash"),
	}
	diags := validatePackageS3(s3)
	if !diags.HasError() {
		t.Fatal("expected a diagnostic for malformed expected_sha256, got none")
	}
}

// TestValidatePackageS3_invalidExpectedSHA256WithUnknownBucketRejected covers
// the corner case where a malformed expected_sha256 is paired with an Unknown
// bucket. validatePackageS3 must catch the bad SHA at the config-validation
// gate so it never reaches the HEAD-bypass logic in resolveRemoteSHA, which
// would otherwise trust whatever string was provided.
func TestValidatePackageS3_invalidExpectedSHA256WithUnknownBucketRejected(t *testing.T) {
	s3 := packageS3Model{
		Bucket:         types.StringUnknown(),
		Key:            types.StringUnknown(),
		Region:         types.StringNull(),
		EndpointURL:    types.StringNull(),
		ExpectedSHA256: types.StringValue("not-a-hash"),
	}
	diags := validatePackageS3(s3)
	if !diags.HasError() {
		t.Fatal("expected a diagnostic for malformed expected_sha256 even with Unknown bucket, got none")
	}
	found := false
	for _, d := range diags.Errors() {
		if strings.Contains(d.Detail(), "64 lowercase hexadecimal characters") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected the expected_sha256-format diagnostic, got: %v", diags)
	}
}

func TestValidatePackageS3_validConfigAccepted(t *testing.T) {
	s3 := packageS3Model{
		Bucket:         types.StringValue("my-bucket"),
		Key:            types.StringValue("installers/app.pkg"),
		Region:         types.StringValue("us-east-1"),
		EndpointURL:    types.StringNull(),
		ExpectedSHA256: types.StringValue("aa7f05f70feb6201886d8a27a004bc322e7ba578262c984a213f48089e162183"),
	}
	diags := validatePackageS3(s3)
	if diags.HasError() {
		t.Errorf("expected no diagnostics for valid config, got: %v", diags)
	}
}

// TestResolveRemoteSHA_unknownS3SoftSkip confirms that when package_s3.bucket
// is Unknown at plan time, resolveRemoteSHA returns no SHA, no error, and
// requiresDownload=false — which makes ModifyPlan defer the SHA computation
// to apply time without erroring or surfacing a warning. This is the second
// half of the fix: ValidateConfig accepting Unknown is necessary but not
// sufficient on its own.
func TestResolveRemoteSHA_unknownS3SoftSkip(t *testing.T) {
	ctx := context.Background()
	s3Obj := newPackageS3(t, map[string]attr.Value{
		"bucket": types.StringUnknown(),
		"key":    types.StringValue("k"),
	})
	model := &softwarePackageResourceModel{
		PackageS3: s3Obj,
	}

	sha, source, requiresDownload, diags := resolveRemoteSHA(ctx, model, true)
	if diags.HasError() {
		t.Errorf("expected no diagnostics for Unknown bucket, got: %v", diags)
	}
	if len(diags.Warnings()) != 0 {
		t.Errorf("expected no warnings for Unknown bucket, got: %v", diags.Warnings())
	}
	if sha != "" {
		t.Errorf("expected empty sha, got %q", sha)
	}
	if source != "" {
		t.Errorf("expected empty source, got %q", source)
	}
	if requiresDownload {
		t.Error("expected requiresDownload=false (Terraform will resolve at apply)")
	}
}

// TestResolveRemoteSHA_unknownBucketWithExpectedSHA confirms that even when
// the bucket is Unknown, a user-supplied expected_sha256 still wins — the
// provider doesn't need to HEAD the object at all.
func TestResolveRemoteSHA_unknownBucketWithExpectedSHA(t *testing.T) {
	ctx := context.Background()
	pinnedSHA := "aa7f05f70feb6201886d8a27a004bc322e7ba578262c984a213f48089e162183"
	s3Obj := newPackageS3(t, map[string]attr.Value{
		"bucket":          types.StringUnknown(),
		"key":             types.StringValue("k"),
		"expected_sha256": types.StringValue(pinnedSHA),
	})
	model := &softwarePackageResourceModel{
		PackageS3: s3Obj,
	}

	sha, source, requiresDownload, diags := resolveRemoteSHA(ctx, model, true)
	if diags.HasError() {
		t.Errorf("expected no diagnostics, got: %v", diags)
	}
	if sha != pinnedSHA {
		t.Errorf("expected pinned sha %s, got %q", pinnedSHA, sha)
	}
	if source != "expected_sha256" {
		t.Errorf("expected source=expected_sha256, got %q", source)
	}
	if requiresDownload {
		t.Error("expected requiresDownload=false")
	}
}

// fakeFleetForLabels stands up the minimum surface (create, get-title,
// patch, delete) the software_package resource hits during a label-focused
// test, and records each PATCH so the test can assert on the wire shape.
type fakeFleetForLabels struct {
	srv *httptest.Server
	mu  sync.Mutex
	// state mirrored back to the provider on subsequent GETs
	titleSelfService   bool
	titleInstallScript string
	// label snapshots captured on the most recent PATCH (nil means
	// "absent in the multipart form").
	patchIncludeLabels    []string
	patchExcludeLabels    []string
	patchIncludeFieldSeen bool
	patchExcludeFieldSeen bool
	patchCount            int
	uploadIncludeFieldSet bool
	uploadExcludeFieldSet bool
}

func newFakeFleetForLabels(t *testing.T) *fakeFleetForLabels {
	t.Helper()
	f := &fakeFleetForLabels{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/global/policies" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"policies": []map[string]any{}})
		case r.URL.Path == "/api/v1/fleet/software/package" && r.Method == http.MethodPost:
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("ParseMultipartForm (upload): %v", err)
				http.Error(w, "bad multipart", http.StatusBadRequest)
				return
			}
			f.mu.Lock()
			f.titleInstallScript = r.FormValue("install_script")
			f.titleSelfService = r.FormValue("self_service") == "true"
			_, f.uploadIncludeFieldSet = r.MultipartForm.Value["labels_include_any"]
			_, f.uploadExcludeFieldSet = r.MultipartForm.Value["labels_exclude_any"]
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_package": map[string]any{"title_id": 71, "team_id": 0},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/71" && r.Method == http.MethodGet:
			f.mu.Lock()
			selfService := f.titleSelfService
			installScript := f.titleInstallScript
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_title": map[string]any{
					"id":             71,
					"name":           "test-app.pkg",
					"source":         "pkg",
					"hosts_count":    0,
					"versions_count": 1,
					"software_package": map[string]any{
						"title_id":       71,
						"platform":       "darwin",
						"hash_sha256":    hex.EncodeToString(sumOf([]byte("FAKEPKG"))),
						"self_service":   selfService,
						"install_script": installScript,
					},
					"versions": []map[string]any{{"id": 1, "version": "1.0.0", "hosts_count": 0}},
				},
			})
		case r.URL.Path == "/api/v1/fleet/software/titles/71/package" && r.Method == http.MethodPatch:
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("ParseMultipartForm (patch): %v", err)
				http.Error(w, "bad multipart", http.StatusBadRequest)
				return
			}
			f.mu.Lock()
			f.patchCount++
			vals, incSeen := r.MultipartForm.Value["labels_include_any"]
			f.patchIncludeFieldSeen = incSeen
			f.patchIncludeLabels = nil
			if incSeen && len(vals) > 0 {
				_ = json.Unmarshal([]byte(vals[0]), &f.patchIncludeLabels)
			}
			vals, excSeen := r.MultipartForm.Value["labels_exclude_any"]
			f.patchExcludeFieldSeen = excSeen
			f.patchExcludeLabels = nil
			if excSeen && len(vals) > 0 {
				_ = json.Unmarshal([]byte(vals[0]), &f.patchExcludeLabels)
			}
			f.titleInstallScript = r.FormValue("install_script")
			f.titleSelfService = r.FormValue("self_service") == "true"
			f.mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v1/fleet/software/titles/71/available_for_install" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func testAccSoftwarePackageResourceConfig_labels(serverURL, pkgPath, labelBlock string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_package" "test" {
  package_path   = %[2]q
  filename       = "test-app.pkg"
  install_script = "echo hi"
%[3]s
}
`, serverURL, pkgPath, labelBlock)
}

// requirePatchInStep returns a Check func that fails unless an additional
// PATCH was recorded by f between the previous step and this one. It
// updates the captured counter via the closure so callers can chain Checks
// without rebuilding the bookkeeping in each step. Without this guard, a
// Check that asserts "field X was absent from the PATCH" would silently
// pass if no PATCH ran at all in that step.
func requirePatchInStep(f *fakeFleetForLabels, prevCount *int, inner func() error) func(*terraform.State) error {
	return func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.patchCount == *prevCount {
			return fmt.Errorf("expected step to trigger a PATCH (count still %d)", *prevCount)
		}
		*prevCount = f.patchCount
		return inner()
	}
}

// TestAccSoftwarePackageResource_metadataUpdateWithoutLabels reproduces the
// v0.6.2 regression: a metadata-only update on a package with no labels in
// HCL must not include labels_include_any or labels_exclude_any in the
// PATCH multipart body. Fleet enforces "only one of …" and rejects both
// being present with HTTP 400.
func TestAccSoftwarePackageResource_metadataUpdateWithoutLabels(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFakeFleetForLabels(t)
	patchCount := 0

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSoftwarePackageResourceConfig_labels(f.srv.URL, pkgPath, ""),
				Check: func(_ *terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					if f.uploadIncludeFieldSet {
						return fmt.Errorf("upload form must omit labels_include_any when HCL has no labels")
					}
					if f.uploadExcludeFieldSet {
						return fmt.Errorf("upload form must omit labels_exclude_any when HCL has no labels")
					}
					return nil
				},
			},
			{
				Config: strings.Replace(
					testAccSoftwarePackageResourceConfig_labels(f.srv.URL, pkgPath, ""),
					`install_script = "echo hi"`,
					`install_script = "echo updated"`, 1,
				),
				Check: requirePatchInStep(f, &patchCount, func() error {
					if f.patchIncludeFieldSeen {
						return fmt.Errorf("PATCH multipart body must omit labels_include_any when HCL has no labels, but field was present")
					}
					if f.patchExcludeFieldSeen {
						return fmt.Errorf("PATCH multipart body must omit labels_exclude_any when HCL has no labels, but field was present")
					}
					return nil
				}),
			},
		},
	})
}

// TestAccSoftwarePackageResource_labelLifecycle drives the full lifecycle
// of the label attribute on the multipart endpoints: set, explicit-clear
// via empty list, switch to the other side, and remove. At each step we
// assert which label fields land in the wire payload AND that a PATCH
// actually ran during this step (otherwise stale state from a prior step
// could mask a regression).
func TestAccSoftwarePackageResource_labelLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFakeFleetForLabels(t)
	patchCount := 0

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Create with labels_include_any = ["Engineering"].
				Config: testAccSoftwarePackageResourceConfig_labels(f.srv.URL, pkgPath, `  labels_include_any = ["Engineering"]`),
				Check: func(_ *terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					if !f.uploadIncludeFieldSet {
						return fmt.Errorf("upload form must include labels_include_any when HCL set it")
					}
					if f.uploadExcludeFieldSet {
						return fmt.Errorf("upload form must omit labels_exclude_any when HCL didn't set it")
					}
					return nil
				},
			},
			{
				// Switch sides: include → exclude. Provider must send
				// labels_exclude_any=["Contractors"] and omit
				// labels_include_any entirely, even though the prior
				// state carried it. This is the most likely path to
				// trip Fleet's "only one of …" rule if the resource
				// ever leaked the previous side's value.
				Config: testAccSoftwarePackageResourceConfig_labels(f.srv.URL, pkgPath, `  labels_exclude_any = ["Contractors"]`),
				Check: requirePatchInStep(f, &patchCount, func() error {
					if f.patchIncludeFieldSeen {
						return fmt.Errorf("PATCH must omit labels_include_any when HCL switched to labels_exclude_any")
					}
					if !f.patchExcludeFieldSeen {
						return fmt.Errorf("PATCH must include labels_exclude_any when HCL set it")
					}
					if len(f.patchExcludeLabels) != 1 || f.patchExcludeLabels[0] != "Contractors" {
						return fmt.Errorf("expected labels_exclude_any=[\"Contractors\"], got %v", f.patchExcludeLabels)
					}
					return nil
				}),
			},
			{
				// Explicit clear via empty list: must send labels_exclude_any="[]"
				// and still omit labels_include_any.
				Config: testAccSoftwarePackageResourceConfig_labels(f.srv.URL, pkgPath, `  labels_exclude_any = []`),
				Check: requirePatchInStep(f, &patchCount, func() error {
					if !f.patchExcludeFieldSeen {
						return fmt.Errorf("PATCH must include labels_exclude_any when HCL set [] for explicit clear")
					}
					if len(f.patchExcludeLabels) != 0 {
						return fmt.Errorf("expected labels_exclude_any to decode as []string{}, got %v", f.patchExcludeLabels)
					}
					if f.patchIncludeFieldSeen {
						return fmt.Errorf("PATCH must omit labels_include_any when HCL didn't set it")
					}
					return nil
				}),
			},
			{
				// Removing the attribute entirely → field absent again.
				Config: testAccSoftwarePackageResourceConfig_labels(f.srv.URL, pkgPath, ""),
				Check: requirePatchInStep(f, &patchCount, func() error {
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

// fakeFleetForTypeRouting stands up a fake server that handles all three
// type=* paths plus the setup_experience endpoints, used by the
// type-aware automatic_install routing tests below.
type fakeFleetForTypeRouting struct {
	srv        *httptest.Server
	mu         sync.Mutex
	titleID    int
	source     string // "pkg" | "app_store_app" | "fma"
	platform   string
	appStoreID string

	uploadAutomaticInstall string
	vppCreates             int
	fmaAutomaticInstall    bool
	setupExperienceSet     []int
	setupExperiencePuts    int
}

// newFakeFleetForTypeRouting builds a fake Fleet API surface that covers
// all three legacy-resource types: POST /software/package (multipart),
// POST /software/app_store_apps (JSON), POST /software/fleet_maintained_apps
// (JSON), GET title, GET+PUT /setup_experience/software, DELETE title.
// The test asserts on the captured wire fields to verify routing.
func newFakeFleetForTypeRouting(t *testing.T, titleID int, source string) *fakeFleetForTypeRouting {
	t.Helper()
	f := &fakeFleetForTypeRouting{titleID: titleID, source: source, platform: "darwin"}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		idStr := strconv.Itoa(f.titleID)
		switch {
		case r.URL.Path == "/api/v1/fleet/global/policies" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"policies": []map[string]any{}})

		case r.URL.Path == "/api/v1/fleet/software/package" && r.Method == http.MethodPost:
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("ParseMultipartForm (upload): %v", err)
				http.Error(w, "bad multipart", http.StatusBadRequest)
				return
			}
			f.mu.Lock()
			f.uploadAutomaticInstall = r.FormValue("automatic_install")
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_package": map[string]any{"title_id": f.titleID, "team_id": 0},
			})

		case r.URL.Path == "/api/v1/fleet/software/app_store_apps" && r.Method == http.MethodPost:
			var body struct {
				AppStoreID string `json:"app_store_id"`
				Platform   string `json:"platform"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.vppCreates++
			f.appStoreID = body.AppStoreID
			if body.Platform != "" {
				f.platform = body.Platform
			}
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"software_title_id": f.titleID})

		case r.URL.Path == "/api/v1/fleet/software/fleet_maintained_apps" && r.Method == http.MethodPost:
			var body struct {
				FleetMaintainedAppID int  `json:"fleet_maintained_app_id"`
				AutomaticInstall     bool `json:"automatic_install"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.fmaAutomaticInstall = body.AutomaticInstall
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"software_title_id": f.titleID})

		case r.URL.Path == "/api/v1/fleet/software/titles/"+idStr && r.Method == http.MethodGet:
			f.mu.Lock()
			payload := map[string]any{
				"id":             f.titleID,
				"name":           "test-app",
				"source":         "pkg",
				"hosts_count":    0,
				"versions_count": 1,
				"versions":       []map[string]any{{"id": 1, "version": "1.0.0", "hosts_count": 0}},
			}
			pkgBody := map[string]any{
				"title_id":    f.titleID,
				"platform":    f.platform,
				"hash_sha256": hex.EncodeToString(sumOf([]byte("FAKEPKG"))),
			}
			for _, id := range f.setupExperienceSet {
				if id == f.titleID {
					pkgBody["install_during_setup"] = true
					break
				}
			}
			// FMA's Read derives automatic_install from the presence of
			// automatic_install_policies (policy-based auto-install). Mirror
			// that here when the fake's FMA Add captured automatic_install=true.
			if f.source == "fma" && f.fmaAutomaticInstall {
				pkgBody["automatic_install_policies"] = []map[string]any{
					{"id": 7, "name": "Auto-install test-app"},
				}
			}
			if f.source == "app_store_app" {
				vppBody := map[string]any{
					"app_store_id":   f.appStoreID,
					"platform":       f.platform,
					"name":           "test-app",
					"latest_version": "1.0.0",
				}
				for _, id := range f.setupExperienceSet {
					if id == f.titleID {
						vppBody["install_during_setup"] = true
						break
					}
				}
				payload["app_store_app"] = vppBody
			} else {
				payload["software_package"] = pkgBody
			}
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"software_title": payload})

		case r.URL.Path == "/api/v1/fleet/setup_experience/software" && r.Method == http.MethodGet:
			f.mu.Lock()
			arr := []map[string]any{}
			for _, id := range f.setupExperienceSet {
				arr = append(arr, map[string]any{"id": id})
			}
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"software_titles": arr})

		case r.URL.Path == "/api/v1/fleet/setup_experience/software" && r.Method == http.MethodPut:
			var body struct {
				SoftwareTitleIDs []int `json:"software_title_ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.setupExperienceSet = append([]int{}, body.SoftwareTitleIDs...)
			f.setupExperiencePuts++
			f.mu.Unlock()
			w.WriteHeader(http.StatusOK)

		case r.URL.Path == "/api/v1/fleet/software/titles/"+idStr+"/available_for_install" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// TestAccSoftwarePackageResource_typeAwareAutomaticInstall_package verifies
// that on the legacy resource, `type=package` + `automatic_install=true`
// routes through the setup_experience PUT endpoint — NOT through any
// JSON automatic_install body field (Fleet's upload endpoint historically
// expected `install_during_setup` here; the form key was renamed but the
// semantics on the legacy resource map to setup-experience).
func TestAccSoftwarePackageResource_typeAwareAutomaticInstall_package(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFakeFleetForTypeRouting(t, 71, "pkg")

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_package" "test" {
  type              = "package"
  package_path      = %[2]q
  filename          = "test-app.pkg"
  install_script    = "echo hi"
  automatic_install = true
}
`, f.srv.URL, pkgPath)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: func(_ *terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					if f.setupExperiencePuts == 0 {
						return fmt.Errorf("type=package + automatic_install=true must issue a PUT /setup_experience/software, got 0 PUTs")
					}
					for _, id := range f.setupExperienceSet {
						if id == f.titleID {
							return nil
						}
					}
					return fmt.Errorf("expected title %d in setup-experience set, got %v", f.titleID, f.setupExperienceSet)
				},
			},
		},
	})
}

// TestAccSoftwarePackageResource_typeAwareAutomaticInstall_vpp verifies
// that `type=vpp` + `automatic_install=true` on the legacy resource
// routes through the setup_experience PUT endpoint (matching Fleet's
// model where VPP install_during_setup is the only meaningful semantic
// for the legacy automatic_install field).
func TestAccSoftwarePackageResource_typeAwareAutomaticInstall_vpp(t *testing.T) {
	f := newFakeFleetForTypeRouting(t, 101, "app_store_app")

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_package" "test" {
  type              = "vpp"
  app_store_id      = "899247664"
  platform          = "darwin"
  automatic_install = true
}
`, f.srv.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: func(_ *terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					if f.setupExperiencePuts == 0 {
						return fmt.Errorf("type=vpp + automatic_install=true must issue a PUT /setup_experience/software, got 0 PUTs")
					}
					for _, id := range f.setupExperienceSet {
						if id == f.titleID {
							return nil
						}
					}
					return fmt.Errorf("expected title %d in setup-experience set, got %v", f.titleID, f.setupExperienceSet)
				},
			},
		},
	})
}

// TestAccSoftwarePackageResource_typeAwareAutomaticInstall_fleetMaintained
// verifies that `type=fleet_maintained` + `automatic_install=true` routes
// through Fleet's JSON `automatic_install` field on the FMA Add endpoint
// (creating a Fleet policy) — NOT through the setup_experience PUT.
// This is the divergent-routing branch: FMA's automatic_install is
// policy-based, while package/vpp's automatic_install means setup-experience.
func TestAccSoftwarePackageResource_typeAwareAutomaticInstall_fleetMaintained(t *testing.T) {
	f := newFakeFleetForTypeRouting(t, 201, "fma")

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_package" "test" {
  type                    = "fleet_maintained"
  fleet_maintained_app_id = 1
  automatic_install       = true
}
`, f.srv.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: func(_ *terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					if !f.fmaAutomaticInstall {
						return fmt.Errorf("type=fleet_maintained + automatic_install=true must send automatic_install=true on FMA Add, got %v", f.fmaAutomaticInstall)
					}
					if f.setupExperiencePuts != 0 {
						return fmt.Errorf("type=fleet_maintained must NOT call setup_experience PUT, got %d PUTs", f.setupExperiencePuts)
					}
					return nil
				},
			},
		},
	})
}

// TestAccSoftwarePackageResource_conflictingLabels verifies the schema
// validator rejects HCL that sets both labels_include_any and
// labels_exclude_any, surfacing Fleet's "only one of …" invariant at
// plan time instead of letting it fail at apply time.
func TestAccSoftwarePackageResource_conflictingLabels(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, []byte("FAKEPKG"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFakeFleetForLabels(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSoftwarePackageResourceConfig_labels(f.srv.URL, pkgPath, `
  labels_include_any = ["A"]
  labels_exclude_any = ["B"]`),
				ExpectError: regexp.MustCompile(`(?i)Invalid Attribute Combination|labels_exclude_any|labels_include_any`),
			},
		},
	})
}
