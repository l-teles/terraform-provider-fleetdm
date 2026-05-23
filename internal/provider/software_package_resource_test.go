package provider

import (
	"bytes"
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
	"sync/atomic"
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
						"platform":    "darwin",
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
type fleetSWMockState struct {
	mu           sync.Mutex
	currentSHA   string
	uploadCount  int
	deleteCount  int
	titleID      int
	titleName    string
	displayName  string
	titleVersion string
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
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"software_package": map[string]interface{}{
					"title_id": state.titleID,
					"team_id":  0,
				},
			})
		case r.URL.Path == titleIDPath+"/package" && r.Method == "PATCH":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == titleIDPath && r.Method == "GET":
			state.mu.Lock()
			sha := state.currentSHA
			state.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"software_title": map[string]interface{}{
					"id":             state.titleID,
					"name":           state.titleName,
					"display_name":   state.displayName,
					"source":         "pkg",
					"hosts_count":    0,
					"versions_count": 1,
					"software_package": map[string]interface{}{
						"title_id":    state.titleID,
						"platform":    "darwin",
						"hash_sha256": sha,
					},
					"versions": []map[string]interface{}{
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

func snapshotFleet(state *fleetSWMockState) (uploads, deletes int) {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.uploadCount, state.deleteCount
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
					uploadsBefore, deletesBefore := snapshotFleet(fleet)
					t.Logf("Before no-op step: head=%d get=%d uploads=%d deletes=%d", headBefore, getBefore, uploadsBefore, deletesBefore)
					t.Setenv("__test_s3_head_before", fmt.Sprintf("%d", headBefore))
					t.Setenv("__test_s3_get_before", fmt.Sprintf("%d", getBefore))
					t.Setenv("__test_fleet_uploads_before", fmt.Sprintf("%d", uploadsBefore))
					t.Setenv("__test_fleet_deletes_before", fmt.Sprintf("%d", deletesBefore))
				},
				Config: testAccSoftwarePackageResourceConfig_s3(fleetServer.URL, s3Server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.s3_test", "package_sha256", shaV1),
					func(s *terraform.State) error {
						_, getNow := s3.snapshot()
						uploadsNow, deletesNow := snapshotFleet(fleet)
						getBefore, _ := strconv.Atoi(os.Getenv("__test_s3_get_before"))
						uploadsBefore, _ := strconv.Atoi(os.Getenv("__test_fleet_uploads_before"))
						deletesBefore, _ := strconv.Atoi(os.Getenv("__test_fleet_deletes_before"))
						if getNow != getBefore {
							return fmt.Errorf("no-op step downloaded the body: get count %d -> %d", getBefore, getNow)
						}
						if uploadsNow != uploadsBefore {
							return fmt.Errorf("no-op step uploaded: upload count %d -> %d", uploadsBefore, uploadsNow)
						}
						if deletesNow != deletesBefore {
							return fmt.Errorf("no-op step deleted: delete count %d -> %d", deletesBefore, deletesNow)
						}
						return nil
					},
				),
			},
			// Step 3: S3 content changes → update should detect SHA mismatch and re-upload.
			{
				PreConfig: func() {
					s3.setContent(contentV2)
				},
				Config: testAccSoftwarePackageResourceConfig_s3(fleetServer.URL, s3Server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.s3_test", "title_id", "55"),
					resource.TestCheckResourceAttr("fleetdm_software_package.s3_test", "package_sha256", shaV2),
					func(s *terraform.State) error {
						uploadsNow, deletesNow := snapshotFleet(fleet)
						// Step 1: create (1 upload). Step 2: no-op (0 uploads, 0 deletes). Step 3: re-upload (1 delete + 1 upload).
						if uploadsNow != 2 {
							return fmt.Errorf("expected 2 total uploads (create + re-upload), got %d", uploadsNow)
						}
						if deletesNow != 1 {
							return fmt.Errorf("expected 1 delete (before re-upload), got %d", deletesNow)
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
					uploadsBefore, _ := snapshotFleet(fleet)
					t.Setenv("__metaonly_get_before", fmt.Sprintf("%d", getBefore))
					t.Setenv("__metaonly_uploads_before", fmt.Sprintf("%d", uploadsBefore))
				},
				Config: testAccSoftwarePackageResourceConfig_s3(fleetServer.URL, s3Server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						_, getNow := s3.snapshot()
						uploadsNow, _ := snapshotFleet(fleet)
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
					uploadsBefore, deletesBefore := snapshotFleet(fleet)
					t.Setenv("__nocs_get_before", fmt.Sprintf("%d", getBefore))
					t.Setenv("__nocs_uploads_before", fmt.Sprintf("%d", uploadsBefore))
					t.Setenv("__nocs_deletes_before", fmt.Sprintf("%d", deletesBefore))
				},
				Config: testAccSoftwarePackageResourceConfig_s3(fleetServer.URL, s3Server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						_, getNow := s3.snapshot()
						uploadsNow, deletesNow := snapshotFleet(fleet)
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

// TestAccSoftwarePackageResource_replaceWithAttachedPolicy exercises the
// detach-before-delete / reattach-after-upload path that exists to work around
// Fleet's HTTP 409 "Couldn't delete. Policy automation uses this software."
// guard. It drives a create + update through the mock Fleet server with a
// single install_software policy initially pointing at the title, and asserts
// that during the update the provider issues:
//   - One PATCH to /global/policies/1 with software_title_id=null (detach)
//   - DELETE + POST /software/package (the existing replace path)
//   - One PATCH to /global/policies/1 with software_title_id=<new title id> (reattach)
//
// in that order, and that no extra detaches/reattaches happen on the no-op
// step that comes after.
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
			// Step 1: create. The provider has no reason to touch policies on
			// create (only Update goes through replaceSoftwarePackage).
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
			// Policy must be detached, then reattached to the (new) title id.
			{
				PreConfig: func() {
					if err := os.WriteFile(pkgPath, contentV2, 0o600); err != nil {
						t.Fatal(err)
					}
				},
				Config: testAccSoftwarePackageResourceConfig(fleetServer.URL, pkgPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "package_sha256", shaV2),
					func(s *terraform.State) error {
						ps.mu.Lock()
						defer ps.mu.Unlock()
						if ps.detachCount != 1 {
							return fmt.Errorf("expected exactly 1 detach, got %d", ps.detachCount)
						}
						if ps.reattachCount != 1 {
							return fmt.Errorf("expected exactly 1 reattach, got %d", ps.reattachCount)
						}
						if len(ps.patchOrder) != 2 || ps.patchOrder[0] != 0 || ps.patchOrder[1] != 60 {
							return fmt.Errorf("expected patch order [0, 60], got %v", ps.patchOrder)
						}
						if ps.installSoftwareTitleID == nil || *ps.installSoftwareTitleID != 60 {
							return fmt.Errorf("expected policy to end with software_title_id=60, got %v", ps.installSoftwareTitleID)
						}
						uploads, deletes := snapshotFleet(fleet)
						if uploads != 2 {
							return fmt.Errorf("expected 2 uploads (create + replace), got %d", uploads)
						}
						if deletes != 1 {
							return fmt.Errorf("expected 1 delete, got %d", deletes)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccSoftwarePackageResource_replaceWithAttachedPolicy_reattachFailureSavesState
// verifies the recovery path when the binary has already been replaced but
// re-attaching install_software automation to one of the previously-detached
// policies fails. The provider must:
//   - Persist the new title id and SHA into state before bailing (so a follow-up
//     apply does not re-create the package).
//   - Surface an error that names the affected policy so the operator knows
//     what needs follow-up.
func TestAccSoftwarePackageResource_replaceWithAttachedPolicy_reattachFailureSavesState(t *testing.T) {
	contentV1 := []byte("PKGV1FAIL")
	contentV2 := []byte("PKGV2FAIL")
	shaV1 := hex.EncodeToString(sumOf(contentV1))
	shaV2 := hex.EncodeToString(sumOf(contentV2))

	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "test-app.pkg")
	if err := os.WriteFile(pkgPath, contentV1, 0o600); err != nil {
		t.Fatal(err)
	}

	fleet := &fleetSWMockState{
		currentSHA:   shaV1,
		titleID:      62,
		titleName:    "test-app.pkg",
		displayName:  "Test App",
		titleVersion: "1.0.0",
	}

	initialTitleRef := 62
	ps := &fleetPolicyMockState{installSoftwareTitleID: &initialTitleRef}

	var failReattach atomic.Bool
	baseHandler := installSoftwarePolicyHandler(t, ps, fleet, "/api/v1/fleet/global/policies", "/api/v1/fleet/global/policies/1")
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/fleet/global/policies/1" && r.Method == http.MethodPatch && failReattach.Load() {
			raw, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(raw))
			var body struct {
				SoftwareTitleID *int `json:"software_title_id"`
			}
			_ = json.Unmarshal(raw, &body)
			if body.SoftwareTitleID != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"message":"reattach failed for test"}`))
				return
			}
		}
		baseHandler(w, r)
	})
	fleetServer := httptest.NewServer(handler)
	defer fleetServer.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: create cleanly.
			{
				Config: testAccSoftwarePackageResourceConfig(fleetServer.URL, pkgPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "title_id", "62"),
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "package_sha256", shaV1),
				),
			},
			// Step 2: rotate content. Detach + delete + upload succeed; the
			// reattach PATCH gets a 500. Apply must fail with a message that
			// names the affected policy.
			{
				PreConfig: func() {
					if err := os.WriteFile(pkgPath, contentV2, 0o600); err != nil {
						t.Fatal(err)
					}
					failReattach.Store(true)
				},
				Config:      testAccSoftwarePackageResourceConfig(fleetServer.URL, pkgPath),
				ExpectError: regexp.MustCompile(`(?s)Error re-attaching install_software automation.*1.*Install Test App`),
			},
			// Step 3: lift the injected failure and re-apply with the same
			// content. If step 2 saved state correctly, this is a no-op for
			// the package (state's package_sha256 already equals shaV2 and
			// Fleet's GET returns shaV2 too), and the assertion below proves
			// state survived the failed step.
			{
				PreConfig: func() {
					failReattach.Store(false)
				},
				Config: testAccSoftwarePackageResourceConfig(fleetServer.URL, pkgPath),
				Check: func(s *terraform.State) error {
					rs, ok := s.RootModule().Resources["fleetdm_software_package.test"]
					if !ok {
						return fmt.Errorf("resource fleetdm_software_package.test missing from state — state from the failed step 2 was lost")
					}
					if got := rs.Primary.Attributes["package_sha256"]; got != shaV2 {
						return fmt.Errorf("expected package_sha256 in state to be the new SHA %s (proof that state was saved before the reattach error bailed), got %s", shaV2, got)
					}
					uploads, deletes := snapshotFleet(fleet)
					if uploads != 2 {
						return fmt.Errorf("expected 2 total uploads (create + the failed step's successful upload), got %d", uploads)
					}
					if deletes != 1 {
						return fmt.Errorf("expected exactly 1 delete (from the failed step's successful pre-upload delete), got %d", deletes)
					}
					return nil
				},
			},
		},
	})
}

// TestAccSoftwarePackageResource_replaceWithAttachedPolicy_teamScope is the
// team-scoped sibling of TestAccSoftwarePackageResource_replaceWithAttachedPolicy.
// It exists to guard the scope dispatch in ListPoliciesByInstallSoftwareTitleID
// and SetPolicyInstallSoftwareTitleID — i.e. that a team-scoped software
// package hits /fleets/{teamID}/policies (not /global/policies) for both the
// detach scan and the per-policy PATCH.
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
					resource.TestCheckResourceAttr("fleetdm_software_package.test", "package_sha256", shaV2),
					func(s *terraform.State) error {
						ps.mu.Lock()
						defer ps.mu.Unlock()
						if ps.detachCount != 1 {
							return fmt.Errorf("expected exactly 1 detach on team-scoped policy, got %d", ps.detachCount)
						}
						if ps.reattachCount != 1 {
							return fmt.Errorf("expected exactly 1 reattach on team-scoped policy, got %d", ps.reattachCount)
						}
						if len(ps.patchOrder) != 2 || ps.patchOrder[0] != 0 || ps.patchOrder[1] != 61 {
							return fmt.Errorf("expected patch order [0, 61], got %v", ps.patchOrder)
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
