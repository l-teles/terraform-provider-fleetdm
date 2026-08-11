package fleetdm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_ListSoftwareTitles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/software/titles" {
			t.Errorf("expected path '/api/v1/fleet/software/titles', got: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got: %s", r.Method)
		}

		resp := listSoftwareTitlesResponse{
			SoftwareTitles: []SoftwareTitle{
				{ID: 1, Name: "Google Chrome", Source: "programs", HostsCount: 100, VersionsCount: 5},
				{ID: 2, Name: "Firefox", Source: "programs", HostsCount: 50, VersionsCount: 3},
			},
			Count: 2,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	titles, count, err := client.ListSoftwareTitles(context.Background(), SoftwareTitleListOptions{})
	if err != nil {
		t.Fatalf("failed to list software titles: %v", err)
	}

	if count != 2 {
		t.Errorf("expected count 2, got: %d", count)
	}
	if len(titles) != 2 {
		t.Errorf("expected 2 software titles, got: %d", len(titles))
	}
	if titles[0].Name != "Google Chrome" {
		t.Errorf("expected first title 'Google Chrome', got: %s", titles[0].Name)
	}
}

func TestClient_GetSoftwareTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/software/titles/1" {
			t.Errorf("expected path '/api/v1/fleet/software/titles/1', got: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got: %s", r.Method)
		}

		resp := getSoftwareTitleResponse{
			SoftwareTitle: &SoftwareTitle{
				ID:            1,
				Name:          "Google Chrome",
				Source:        "programs",
				HostsCount:    100,
				VersionsCount: 5,
				Versions: []SoftwareTitleVersion{
					{ID: 1, Version: "120.0.0", HostsCount: 80},
					{ID: 2, Version: "119.0.0", HostsCount: 20},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	title, err := client.GetSoftwareTitle(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("failed to get software title: %v", err)
	}

	if title.ID != 1 {
		t.Errorf("expected ID 1, got: %d", title.ID)
	}
	if title.Name != "Google Chrome" {
		t.Errorf("expected name 'Google Chrome', got: %s", title.Name)
	}
	if len(title.Versions) != 2 {
		t.Errorf("expected 2 versions, got: %d", len(title.Versions))
	}
}

func TestClient_ListSoftwareVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/software/versions" {
			t.Errorf("expected path '/api/v1/fleet/software/versions', got: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got: %s", r.Method)
		}

		resp := listSoftwareVersionsResponse{
			Software: []SoftwareVersion{
				{ID: 1, Name: "Google Chrome", Version: "120.0.0", Source: "programs", HostsCount: 80},
				{ID: 2, Name: "Google Chrome", Version: "119.0.0", Source: "programs", HostsCount: 20},
			},
			Count: 2,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	versions, count, err := client.ListSoftwareVersions(context.Background(), SoftwareVersionListOptions{})
	if err != nil {
		t.Fatalf("failed to list software versions: %v", err)
	}

	if count != 2 {
		t.Errorf("expected count 2, got: %d", count)
	}
	if len(versions) != 2 {
		t.Errorf("expected 2 software versions, got: %d", len(versions))
	}
	if versions[0].Version != "120.0.0" {
		t.Errorf("expected first version '120.0.0', got: %s", versions[0].Version)
	}
}

func TestClient_GetSoftwareVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/software/versions/1" {
			t.Errorf("expected path '/api/v1/fleet/software/versions/1', got: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got: %s", r.Method)
		}

		resp := getSoftwareVersionResponse{
			Software: &SoftwareVersion{
				ID:         1,
				Name:       "Google Chrome",
				Version:    "120.0.0",
				Source:     "programs",
				HostsCount: 80,
				Vulnerabilities: []SoftwareVulnerability{
					{CVE: "CVE-2024-1234", CISAKnownExploit: true},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	version, err := client.GetSoftwareVersion(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("failed to get software version: %v", err)
	}

	if version.ID != 1 {
		t.Errorf("expected ID 1, got: %d", version.ID)
	}
	if version.Version != "120.0.0" {
		t.Errorf("expected version '120.0.0', got: %s", version.Version)
	}
	if len(version.Vulnerabilities) != 1 {
		t.Errorf("expected 1 vulnerability, got: %d", len(version.Vulnerabilities))
	}
	if version.Vulnerabilities[0].CVE != "CVE-2024-1234" {
		t.Errorf("expected CVE 'CVE-2024-1234', got: %s", version.Vulnerabilities[0].CVE)
	}
}

func TestClient_ListSoftwareTitlesWithFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("team_id") != "5" {
			t.Errorf("expected team_id=5, got: %s", query.Get("team_id"))
		}
		if query.Get("query") != "Chrome" {
			t.Errorf("expected query=Chrome, got: %s", query.Get("query"))
		}
		if query.Get("vulnerable") != "true" {
			t.Errorf("expected vulnerable=true, got: %s", query.Get("vulnerable"))
		}

		resp := listSoftwareTitlesResponse{
			SoftwareTitles: []SoftwareTitle{
				{ID: 1, Name: "Google Chrome", Source: "programs", HostsCount: 100},
			},
			Count: 1,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-api-key",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	teamID := 5
	titles, count, err := client.ListSoftwareTitles(context.Background(), SoftwareTitleListOptions{
		TeamID:         &teamID,
		Query:          "Chrome",
		VulnerableOnly: true,
	})
	if err != nil {
		t.Fatalf("failed to list software titles: %v", err)
	}

	if count != 1 {
		t.Errorf("expected count 1, got: %d", count)
	}
	if len(titles) != 1 {
		t.Errorf("expected 1 software title, got: %d", len(titles))
	}
}

func TestClient_GetSoftwareInstaller(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/software/titles/42/package" {
			t.Errorf("expected path /api/v1/fleet/software/titles/42/package, got: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got: %s", r.Method)
		}
		if r.URL.Query().Get("team_id") != "5" {
			t.Errorf("expected team_id=5, got: %s", r.URL.Query().Get("team_id"))
		}

		resp := map[string]interface{}{
			"software_installer": map[string]interface{}{
				"software_title_id": 42,
				"team_id":           5,
				"name":              "Zoom",
				"version":           "5.0.0",
				"filename":          "zoom.pkg",
				"self_service":      true,
				"install_script":    "installer -pkg /tmp/zoom.pkg -target /",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	teamID := 5
	installer, err := client.GetSoftwareInstaller(context.Background(), 42, &teamID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if installer.TitleID != 42 {
		t.Errorf("expected title ID 42, got: %d", installer.TitleID)
	}
	if installer.Name != "Zoom" {
		t.Errorf("expected name 'Zoom', got: %s", installer.Name)
	}
	if !installer.SelfService {
		t.Error("expected self_service to be true")
	}
}

func TestClient_DeleteSoftwarePackage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE request, got: %s", r.Method)
		}
		expectedPath := "/api/v1/fleet/software/titles/42/available_for_install"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got: %s", expectedPath, r.URL.Path)
		}
		if r.URL.Query().Get("team_id") != "5" {
			t.Errorf("expected team_id=5, got: %s", r.URL.Query().Get("team_id"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	teamID := 5
	err := client.DeleteSoftwarePackage(context.Background(), 42, &teamID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_DeleteSoftwarePackage_NoTeam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("team_id") != "" {
			t.Errorf("expected no team_id, got: %s", r.URL.Query().Get("team_id"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	err := client.DeleteSoftwarePackage(context.Background(), 42, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_PatchSoftwarePackage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH request, got: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/software/titles/42/package" {
			t.Errorf("expected path /api/v1/fleet/software/titles/42/package, got: %s", r.URL.Path)
		}
		if r.URL.Query().Get("team_id") != "5" {
			t.Errorf("expected team_id=5, got: %s", r.URL.Query().Get("team_id"))
		}

		// Fleet's PATCH /software/titles/{id}/package endpoint rejects
		// application/json; the provider must send multipart/form-data.
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data;") {
			t.Errorf("expected Content-Type to start with multipart/form-data;, got: %s", ct)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		if got := r.FormValue("install_script"); got != "new install script" {
			t.Errorf("expected install_script 'new install script', got: %q", got)
		}
		if got := r.FormValue("self_service"); got != "true" {
			t.Errorf("expected self_service form field 'true', got: %q", got)
		}
		// install_during_setup is no longer sent on the package-PATCH
		// endpoint — that field is the setup-experience flag which Fleet
		// manages via a separate endpoint (PUT /setup_experience/software).
		// The resource layer calls the setup_experience helper directly
		// when install_during_setup changes.
		if _, ok := r.MultipartForm.Value["install_during_setup"]; ok {
			t.Errorf("install_during_setup must not appear on the package PATCH form (use setup_experience endpoint instead), got: %q", r.FormValue("install_during_setup"))
		}
		// When both label pointers are nil, neither field must appear
		// in the multipart form. Fleet's API rejects requests that set
		// both labels_include_any and labels_exclude_any (the original
		// HTTP 400 we're guarding against).
		if _, ok := r.MultipartForm.Value["labels_include_any"]; ok {
			t.Errorf("expected labels_include_any absent for nil pointer, got: %q", r.FormValue("labels_include_any"))
		}
		if _, ok := r.MultipartForm.Value["labels_exclude_any"]; ok {
			t.Errorf("expected labels_exclude_any absent for nil pointer, got: %q", r.FormValue("labels_exclude_any"))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	teamID := 5
	err := client.PatchSoftwarePackage(context.Background(), 42, &PatchSoftwarePackageRequest{
		TeamID:        &teamID,
		InstallScript: "new install script",
		SelfService:   true,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// TestClient_PatchSoftwarePackage_EncodesLabels verifies that a populated
// label slice is JSON-encoded into the multipart form field. Note: in
// real provider usage the schema validator rejects HCL that sets both
// labels_include_any and labels_exclude_any. This test exercises the
// API client layer directly to pin the encoding shape.
func TestClient_PatchSoftwarePackage_EncodesLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if got := r.FormValue("labels_include_any"); got != `["Macs on Sonoma","Engineering"]` {
			t.Errorf("expected labels_include_any to be JSON-encoded array, got: %q", got)
		}
		if got := r.FormValue("labels_exclude_any"); got != `["Exempt"]` {
			t.Errorf("expected labels_exclude_any to be JSON-encoded array, got: %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	include := []string{"Macs on Sonoma", "Engineering"}
	exclude := []string{"Exempt"}
	err := client.PatchSoftwarePackage(context.Background(), 42, &PatchSoftwarePackageRequest{
		LabelsIncludeAny: &include,
		LabelsExcludeAny: &exclude,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// TestClient_PatchSoftwarePackage_ClearsLabelsViaEmpty verifies that a
// pointer to an empty slice serializes as "[]" — the explicit-clear path
// used when the user sets labels_include_any = [] in HCL.
func TestClient_PatchSoftwarePackage_ClearsLabelsViaEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if got := r.FormValue("labels_include_any"); got != `[]` {
			t.Errorf("expected labels_include_any '[]' for pointer-to-empty, got: %q", got)
		}
		// labels_exclude_any was never set on the request → must be absent.
		if _, ok := r.MultipartForm.Value["labels_exclude_any"]; ok {
			t.Errorf("expected labels_exclude_any absent when only include is set, got: %q", r.FormValue("labels_exclude_any"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	empty := []string{}
	err := client.PatchSoftwarePackage(context.Background(), 42, &PatchSoftwarePackageRequest{
		LabelsIncludeAny: &empty,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// TestClient_PatchSoftwarePackage_SurfacesError confirms the multipart helper
// surfaces an HTTP 400 from Fleet correctly (this regression test guards
// against ever re-introducing the application/json shape: the error message
// would change to mention multipart parsing). It also pins the structured
// errors[] array decode so the sendMultipart refactor cannot silently drop
// it.
func TestClient_PatchSoftwarePackage_SurfacesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"validation failed","errors":[{"name":"install_script","reason":"too long"}]}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	err := client.PatchSoftwarePackage(context.Background(), 42, &PatchSoftwarePackageRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("expected error to surface Fleet message, got: %v", err)
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected error chain to contain *APIError, got: %T (%v)", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("expected APIError.StatusCode 400, got %d", apiErr.StatusCode)
	}
	if len(apiErr.Errors) != 1 {
		t.Fatalf("expected APIError.Errors to carry 1 entry, got %d", len(apiErr.Errors))
	}
	if apiErr.Errors[0].Name != "install_script" || apiErr.Errors[0].Reason != "too long" {
		t.Errorf("expected APIError.Errors[0] = {install_script, too long}, got %+v", apiErr.Errors[0])
	}
}

// TestClient_PatchSoftwarePackage_WithBinary verifies the binary-replacement
// mode of the PATCH endpoint: when req.Software is non-nil, the helper sends
// a multipart/form-data request with a "software" file part whose content
// and filename match the request, alongside ALL metadata fields supplied on
// the request. The title_id stays in the URL path — there's no DELETE and
// no policy detach.
//
// This shape replaces the legacy DELETE + UPLOAD + detach/reattach dance
// that fell over against Fleet's patch-policy guard. The metadata pin
// matters because the new resource-level replace flow passes the binary
// and metadata together in one request; a regression that drops a
// metadata field on the binary path would silently revert the user's
// configuration without a plan-time signal.
func TestClient_PatchSoftwarePackage_WithBinary(t *testing.T) {
	// Use bytes that exercise null + non-printable + non-UTF-8 ranges so a
	// regression that mishandles binary content (UTF-8 sanitization, null
	// truncation) would fail here, not just on a real-world installer.
	wantBinary := []byte("PK\x03\x04 fake installer \x00\x01\x02\xff\xfe")
	wantFilename := "firefox-installer.msi"
	wantCategories := []string{"Browsers", "Productivity"}
	wantLabelsIncludeAny := []string{"Workstations", "Engineering"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/software/titles/42/package" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("team_id") != "5" {
			t.Errorf("expected team_id=5, got %q", r.URL.Query().Get("team_id"))
		}
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data;") {
			t.Errorf("expected multipart/form-data, got %q", ct)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		// File part assertions.
		files := r.MultipartForm.File["software"]
		if len(files) != 1 {
			t.Fatalf("expected exactly one 'software' file part, got %d", len(files))
		}
		if files[0].Filename != wantFilename {
			t.Errorf("expected filename %q, got %q", wantFilename, files[0].Filename)
		}
		f, err := files[0].Open()
		if err != nil {
			t.Fatalf("open uploaded file: %v", err)
		}
		gotBytes, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			t.Fatalf("read uploaded file: %v", err)
		}
		if !bytes.Equal(gotBytes, wantBinary) {
			t.Errorf("uploaded bytes mismatch: got %q want %q", gotBytes, wantBinary)
		}
		// Every metadata field travels alongside the file. The check is
		// exhaustive on purpose — see the test comment.
		if got := r.FormValue("install_script"); got != "msiexec /i install.msi /quiet" {
			t.Errorf("install_script: got %q", got)
		}
		if got := r.FormValue("uninstall_script"); got != "msiexec /x {GUID}" {
			t.Errorf("uninstall_script: got %q", got)
		}
		if got := r.FormValue("pre_install_query"); got != "SELECT 1 FROM os_version" {
			t.Errorf("pre_install_query: got %q", got)
		}
		if got := r.FormValue("post_install_script"); got != "echo done" {
			t.Errorf("post_install_script: got %q", got)
		}
		if got := r.FormValue("self_service"); got != "true" {
			t.Errorf("self_service: got %q", got)
		}
		if got := r.FormValue("display_name"); got != "Mozilla Firefox" {
			t.Errorf("display_name: got %q", got)
		}
		if got := r.FormValue("categories"); got != `["Browsers","Productivity"]` {
			t.Errorf("categories: got %q", got)
		}
		if got := r.FormValue("labels_include_any"); got != `["Workstations","Engineering"]` {
			t.Errorf("labels_include_any: got %q", got)
		}
		// labels_exclude_any / labels_include_all were not set on the request → must be absent.
		if _, ok := r.MultipartForm.Value["labels_exclude_any"]; ok {
			t.Errorf("labels_exclude_any must be absent when nil, got %q", r.FormValue("labels_exclude_any"))
		}
		if _, ok := r.MultipartForm.Value["labels_include_all"]; ok {
			t.Errorf("labels_include_all must be absent when nil, got %q", r.FormValue("labels_include_all"))
		}
		w.WriteHeader(http.StatusOK)
		// Response shape mirrors what the docs document for this endpoint;
		// the helper does not unmarshal it, but returning something realistic
		// guards against future changes that start to.
		_, _ = w.Write([]byte(`{"software_installer":{"name":"firefox-installer.msi","version":"125.0"}}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	teamID := 5
	err := client.PatchSoftwarePackage(context.Background(), 42, &PatchSoftwarePackageRequest{
		TeamID:            &teamID,
		Software:          wantBinary,
		Filename:          wantFilename,
		DisplayName:       "Mozilla Firefox",
		InstallScript:     "msiexec /i install.msi /quiet",
		UninstallScript:   "msiexec /x {GUID}",
		PreInstallQuery:   "SELECT 1 FROM os_version",
		PostInstallScript: "echo done",
		SelfService:       true,
		Categories:        &wantCategories,
		LabelsIncludeAny:  &wantLabelsIncludeAny,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// TestClient_PatchSoftwarePackage_WithBinaryClearsCategoriesViaEmpty verifies
// the categories "clear" path (pointer-to-empty) works on the binary
// replacement endpoint too — not just on metadata-only PATCH. This pins the
// extractOptionalLabels swap in the resource-level replace flow.
func TestClient_PatchSoftwarePackage_WithBinaryClearsCategoriesViaEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if got := r.FormValue("categories"); got != `[]` {
			t.Errorf(`expected categories "[]" for pointer-to-empty, got %q`, got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	empty := []string{}
	err := client.PatchSoftwarePackage(context.Background(), 42, &PatchSoftwarePackageRequest{
		Software:   []byte("bytes"),
		Filename:   "x.msi",
		Categories: &empty,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// TestClient_PatchSoftwarePackage_BinaryRequiresFilename pins the input-guard
// against a misuse where the caller passes Software bytes but forgets the
// Filename — Fleet's multipart parser would otherwise accept a file part
// with an empty filename and either reject it later or persist a nameless
// installer, neither of which is a user-friendly failure.
func TestClient_PatchSoftwarePackage_BinaryRequiresFilename(t *testing.T) {
	client, _ := NewClient(ClientConfig{ServerAddress: "http://unused", APIKey: "test-api-key", VerifyTLS: false})
	err := client.PatchSoftwarePackage(context.Background(), 42, &PatchSoftwarePackageRequest{
		Software: []byte("bytes"),
		// Filename intentionally omitted.
	})
	if err == nil {
		t.Fatal("expected error when Filename is missing with Software set, got nil")
	}
	if !strings.Contains(err.Error(), "Filename is required") {
		t.Errorf("expected error to mention Filename requirement, got: %v", err)
	}
}

func TestClient_UploadSoftwarePackage(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/fleet/software/package" {
			// Verify multipart form
			if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "multipart/form-data") {
				t.Errorf("expected multipart/form-data content type, got: %s", ct)
			}

			err := r.ParseMultipartForm(10 << 20)
			if err != nil {
				t.Fatalf("failed to parse multipart form: %v", err)
			}

			if r.FormValue("team_id") != "5" {
				t.Errorf("expected team_id=5, got: %s", r.FormValue("team_id"))
			}
			if r.FormValue("install_script") != "installer -pkg /tmp/test.pkg -target /" {
				t.Errorf("unexpected install_script: %s", r.FormValue("install_script"))
			}
			if r.FormValue("self_service") != "true" {
				t.Errorf("expected self_service=true, got: %s", r.FormValue("self_service"))
			}

			file, header, err := r.FormFile("software")
			if err != nil {
				t.Fatalf("failed to get form file: %v", err)
			}
			defer file.Close()
			if header.Filename != "test.pkg" {
				t.Errorf("expected filename test.pkg, got: %s", header.Filename)
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"software_package": map[string]interface{}{
					"team_id":  5,
					"title_id": 42,
				},
			})
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/fleet/software/titles/42" {
			json.NewEncoder(w).Encode(getSoftwareTitleResponse{
				SoftwareTitle: &SoftwareTitle{
					ID:   42,
					Name: "test.pkg",
				},
			})
			return
		}

		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	teamID := 5
	title, err := client.UploadSoftwarePackage(context.Background(), &UploadSoftwarePackageRequest{
		TeamID:        &teamID,
		Software:      []byte("fake-pkg-content"),
		Filename:      "test.pkg",
		InstallScript: "installer -pkg /tmp/test.pkg -target /",
		SelfService:   true,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if title.ID != 42 {
		t.Errorf("expected title ID 42, got: %d", title.ID)
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls (upload + get title), got: %d", callCount)
	}
}

// TestClient_UploadSoftwarePackage_OmitsUnsetLabels verifies that nil
// label pointers stay out of the multipart body. Mirrors the
// PatchSoftwarePackage guarantee at the Create-path layer.
func TestClient_UploadSoftwarePackage_OmitsUnsetLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/fleet/software/package" {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("parse multipart: %v", err)
			}
			if _, ok := r.MultipartForm.Value["labels_include_any"]; ok {
				t.Errorf("expected labels_include_any absent for nil pointer, got: %q", r.FormValue("labels_include_any"))
			}
			if _, ok := r.MultipartForm.Value["labels_exclude_any"]; ok {
				t.Errorf("expected labels_exclude_any absent for nil pointer, got: %q", r.FormValue("labels_exclude_any"))
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"software_package": map[string]interface{}{"team_id": 1, "title_id": 7},
			})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/fleet/software/titles/7" {
			json.NewEncoder(w).Encode(getSoftwareTitleResponse{SoftwareTitle: &SoftwareTitle{ID: 7, Name: "test.pkg"}})
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	teamID := 1
	if _, err := client.UploadSoftwarePackage(context.Background(), &UploadSoftwarePackageRequest{
		TeamID:   &teamID,
		Software: []byte("x"),
		Filename: "test.pkg",
	}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// TestClient_UploadSoftwarePackage_ClearsLabelsViaEmpty verifies that a
// pointer to an empty slice serializes as "[]" so a future Read can
// faithfully reflect the explicit "no labels" intent.
func TestClient_UploadSoftwarePackage_ClearsLabelsViaEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/fleet/software/package" {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("parse multipart: %v", err)
			}
			if got := r.FormValue("labels_include_any"); got != `[]` {
				t.Errorf("expected labels_include_any '[]' for pointer-to-empty, got: %q", got)
			}
			if _, ok := r.MultipartForm.Value["labels_exclude_any"]; ok {
				t.Errorf("expected labels_exclude_any absent when only include is set, got: %q", r.FormValue("labels_exclude_any"))
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"software_package": map[string]interface{}{"team_id": 1, "title_id": 8},
			})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/fleet/software/titles/8" {
			json.NewEncoder(w).Encode(getSoftwareTitleResponse{SoftwareTitle: &SoftwareTitle{ID: 8, Name: "test.pkg"}})
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	teamID := 1
	empty := []string{}
	if _, err := client.UploadSoftwarePackage(context.Background(), &UploadSoftwarePackageRequest{
		TeamID:           &teamID,
		Software:         []byte("x"),
		Filename:         "test.pkg",
		LabelsIncludeAny: &empty,
	}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_AddAppStoreApp(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/fleet/software/app_store_apps" {
			var req AddAppStoreAppRequest
			json.NewDecoder(r.Body).Decode(&req)

			if req.AppStoreID != "361309726" {
				t.Errorf("expected app_store_id '361309726', got: %s", req.AppStoreID)
			}
			if req.TeamID != 5 {
				t.Errorf("expected team_id 5, got: %d", req.TeamID)
			}
			if req.Platform != "darwin" {
				t.Errorf("expected platform 'darwin', got: %s", req.Platform)
			}
			if !req.SelfService {
				t.Error("expected self_service to be true")
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"software_title_id": 100,
			})
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/fleet/software/titles/100" {
			json.NewEncoder(w).Encode(getSoftwareTitleResponse{
				SoftwareTitle: &SoftwareTitle{
					ID:     100,
					Name:   "TestFlight",
					Source: "apps",
					AppStoreApp: &AppStoreAppInfo{
						AdamID:        "361309726",
						Platform:      "darwin",
						Name:          "TestFlight",
						LatestVersion: "3.2.0",
						SelfService:   true,
					},
				},
			})
			return
		}

		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	title, err := client.AddAppStoreApp(context.Background(), &AddAppStoreAppRequest{
		AppStoreID:  "361309726",
		TeamID:      5,
		Platform:    "darwin",
		SelfService: true,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if title.ID != 100 {
		t.Errorf("expected title ID 100, got: %d", title.ID)
	}
	if title.Name != "TestFlight" {
		t.Errorf("expected name 'TestFlight', got: %s", title.Name)
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls (post + get title), got: %d", callCount)
	}
}

func TestClient_UpdateAppStoreApp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH request, got: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/software/titles/100/app_store_app" {
			t.Errorf("expected path /api/v1/fleet/software/titles/100/app_store_app, got: %s", r.URL.Path)
		}

		var req UpdateAppStoreAppRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.TeamID != 5 {
			t.Errorf("expected team_id 5, got: %d", req.TeamID)
		}
		if !req.SelfService {
			t.Error("expected self_service to be true")
		}
		if len(req.LabelsIncludeAny) != 2 || req.LabelsIncludeAny[0] != "MacOS" || req.LabelsIncludeAny[1] != "Developers" {
			t.Errorf("unexpected labels_include_any: %v", req.LabelsIncludeAny)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	err := client.UpdateAppStoreApp(context.Background(), 100, &UpdateAppStoreAppRequest{
		TeamID:           5,
		SelfService:      true,
		LabelsIncludeAny: []string{"MacOS", "Developers"},
		LabelsExcludeAny: []string{},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClient_ListFleetMaintainedApps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/software/fleet_maintained_apps" {
			t.Errorf("expected path '/api/v1/fleet/software/fleet_maintained_apps', got: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got: %s", r.Method)
		}
		if r.URL.Query().Get("team_id") != "5" {
			t.Errorf("expected team_id=5, got: %s", r.URL.Query().Get("team_id"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listFleetMaintainedAppsResponse{
			FleetMaintainedApps: []FleetMaintainedApp{
				{ID: 1, Name: "Firefox", Slug: "firefox", Platform: "darwin", Version: "125.0"},
				{ID: 2, Name: "Slack", Slug: "slack", Platform: "darwin", Version: "4.38.0"},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	teamID := 5
	apps, err := client.ListFleetMaintainedApps(context.Background(), &teamID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("expected 2 apps, got: %d", len(apps))
	}
	if apps[0].Name != "Firefox" {
		t.Errorf("expected first app 'Firefox', got: %s", apps[0].Name)
	}
	if apps[1].Name != "Slack" {
		t.Errorf("expected second app 'Slack', got: %s", apps[1].Name)
	}
}

func TestClient_GetFleetMaintainedApp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/software/fleet_maintained_apps/1" {
			t.Errorf("expected path '/api/v1/fleet/software/fleet_maintained_apps/1', got: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got: %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(getFleetMaintainedAppResponse{
			FleetMaintainedApp: &FleetMaintainedApp{
				ID:              1,
				Name:            "Firefox",
				Slug:            "firefox",
				Platform:        "darwin",
				Version:         "125.0",
				Filename:        "firefox-125.0.dmg",
				URL:             "https://download.mozilla.org/firefox-125.0.dmg",
				InstallScript:   "installer -pkg /tmp/firefox.pkg -target /",
				UninstallScript: "rm -rf /Applications/Firefox.app",
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	app, err := client.GetFleetMaintainedApp(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if app.ID != 1 {
		t.Errorf("expected ID 1, got: %d", app.ID)
	}
	if app.Name != "Firefox" {
		t.Errorf("expected name 'Firefox', got: %s", app.Name)
	}
	if app.Slug != "firefox" {
		t.Errorf("expected slug 'firefox', got: %s", app.Slug)
	}
	if app.Platform != "darwin" {
		t.Errorf("expected platform 'darwin', got: %s", app.Platform)
	}
	if app.Filename != "firefox-125.0.dmg" {
		t.Errorf("expected filename 'firefox-125.0.dmg', got: %s", app.Filename)
	}
	if app.InstallScript != "installer -pkg /tmp/firefox.pkg -target /" {
		t.Errorf("unexpected install_script: %s", app.InstallScript)
	}
}

func TestClient_AddFleetMaintainedApp(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/fleet/software/fleet_maintained_apps" {
			var req AddFleetMaintainedAppRequest
			json.NewDecoder(r.Body).Decode(&req)

			if req.FleetMaintainedAppID != 1 {
				t.Errorf("expected fleet_maintained_app_id 1, got: %d", req.FleetMaintainedAppID)
			}
			if req.TeamID != 5 {
				t.Errorf("expected team_id 5, got: %d", req.TeamID)
			}
			if !req.SelfService {
				t.Error("expected self_service to be true")
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"software_title_id": 200,
			})
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/fleet/software/titles/200" {
			json.NewEncoder(w).Encode(getSoftwareTitleResponse{
				SoftwareTitle: &SoftwareTitle{
					ID:     200,
					Name:   "Firefox",
					Source: "pkg_packages",
					Versions: []SoftwareTitleVersion{
						{ID: 1, Version: "125.0"},
					},
					SoftwarePackage: &SoftwarePackageInfo{
						Name:     "Firefox",
						Version:  "125.0",
						Platform: "darwin",
					},
				},
			})
			return
		}

		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	title, err := client.AddFleetMaintainedApp(context.Background(), &AddFleetMaintainedAppRequest{
		FleetMaintainedAppID: 1,
		TeamID:               5,
		SelfService:          true,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if title.ID != 200 {
		t.Errorf("expected title ID 200, got: %d", title.ID)
	}
	if title.Name != "Firefox" {
		t.Errorf("expected name 'Firefox', got: %s", title.Name)
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls (post + get title), got: %d", callCount)
	}
}

func TestClient_ListAppStoreApps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/software/app_store_apps" {
			t.Errorf("expected path '/api/v1/fleet/software/app_store_apps', got: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got: %s", r.Method)
		}
		if r.URL.Query().Get("team_id") != "5" {
			t.Errorf("expected team_id=5, got: %s", r.URL.Query().Get("team_id"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListAppStoreAppsResponse{
			AppStoreApps: []AppStoreAppListItem{
				{AppStoreID: "361309726", Name: "TestFlight", Platform: "darwin", IconURL: "https://example.com/testflight.png", LatestVersion: "3.2.0"},
				{AppStoreID: "497799835", Name: "Xcode", Platform: "darwin", IconURL: "https://example.com/xcode.png", LatestVersion: "15.2"},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	apps, err := client.ListAppStoreApps(context.Background(), 5)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("expected 2 apps, got: %d", len(apps))
	}
	if apps[0].AppStoreID != "361309726" {
		t.Errorf("expected first app store ID '361309726', got: %s", apps[0].AppStoreID)
	}
	if apps[0].Name != "TestFlight" {
		t.Errorf("expected first app name 'TestFlight', got: %s", apps[0].Name)
	}
	if apps[1].Name != "Xcode" {
		t.Errorf("expected second app name 'Xcode', got: %s", apps[1].Name)
	}
}

func TestClient_ListSoftwareVersionsWithFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("team_id") != "3" {
			t.Errorf("expected team_id=3, got: %s", query.Get("team_id"))
		}
		if query.Get("query") != "Chrome" {
			t.Errorf("expected query=Chrome, got: %s", query.Get("query"))
		}
		if query.Get("vulnerable") != "true" {
			t.Errorf("expected vulnerable=true, got: %s", query.Get("vulnerable"))
		}
		if query.Get("per_page") != "10" {
			t.Errorf("expected per_page=10, got: %s", query.Get("per_page"))
		}

		resp := listSoftwareVersionsResponse{
			Software: []SoftwareVersion{
				{ID: 1, Name: "Chrome", Version: "120.0", Source: "programs"},
			},
			Count: 1,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	teamID := 3
	versions, count, err := client.ListSoftwareVersions(context.Background(), SoftwareVersionListOptions{
		TeamID:         &teamID,
		Query:          "Chrome",
		VulnerableOnly: true,
		ListOptions:    ListOptions{PerPage: 10},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got: %d", count)
	}
	if len(versions) != 1 {
		t.Errorf("expected 1 version, got: %d", len(versions))
	}
}

// --- Fleet 4.90: Fleet-maintained app version pinning -----------------------

// TestClient_PatchSoftwarePackagePinnedVersion is the load-bearing assertion
// for the version pin: Fleet rejects `version` combined with ANY other field
// ("Couldn't update. \"version\" can't be changed at the same time as other
// fields.", verified against a live Fleet v4.90.0), so this method must send a
// form containing exactly one value — nothing borrowed from
// PatchSoftwarePackage's always-send-everything shape.
func TestClient_PatchSoftwarePackagePinnedVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH request, got: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/software/titles/42/package" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("team_id"); got != "7" {
			t.Errorf("expected team_id=7 query param, got: %q", got)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/form-data;") {
			t.Errorf("expected multipart/form-data Content-Type, got: %s", ct)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}

		if got := r.FormValue("version"); got != "2.5.1" {
			t.Errorf("expected version '2.5.1', got: %q", got)
		}
		// The whole point of the dedicated method: exactly one form value, and
		// no file part.
		if len(r.MultipartForm.Value) != 1 {
			t.Errorf("expected exactly 1 form value (version), got %d: %v", len(r.MultipartForm.Value), r.MultipartForm.Value)
		}
		if len(r.MultipartForm.File) != 0 {
			t.Errorf("expected no file parts, got: %v", r.MultipartForm.File)
		}
		// Spot-check the fields PatchSoftwarePackage would have sent, so a
		// future refactor that routes the pin through the shared helper fails
		// here with a readable message instead of only tripping the count.
		for _, forbidden := range []string{
			"install_script", "uninstall_script", "pre_install_query",
			"post_install_script", "self_service", "display_name",
			"categories", "labels_include_any", "labels_exclude_any", "labels_include_all",
		} {
			if _, present := r.MultipartForm.Value[forbidden]; present {
				t.Errorf("field %q must not be sent alongside version — Fleet rejects the request", forbidden)
			}
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	teamID := 7
	if err := client.PatchSoftwarePackagePinnedVersion(context.Background(), 42, &teamID, "2.5.1"); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// TestClient_PatchSoftwarePackagePinnedVersion_Unpin covers the documented
// "back to latest" path: an empty version is a meaningful request, so the field
// must still be present in the form (present-and-empty), not dropped.
func TestClient_PatchSoftwarePackagePinnedVersion_Unpin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		values, present := r.MultipartForm.Value["version"]
		if !present {
			t.Fatal("expected the version field to be present even when empty (empty = unpin)")
		}
		if len(values) != 1 || values[0] != "" {
			t.Errorf("expected a single empty version value, got: %v", values)
		}
		if len(r.MultipartForm.Value) != 1 {
			t.Errorf("expected exactly 1 form value, got %d: %v", len(r.MultipartForm.Value), r.MultipartForm.Value)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	if err := client.PatchSoftwarePackagePinnedVersion(context.Background(), 42, nil, ""); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// TestClient_PatchSoftwarePackagePinnedVersion_NoTeam checks the team_id query
// param is omitted for a nil team (the "No team" case), matching
// PatchSoftwarePackage's endpoint construction.
func TestClient_PatchSoftwarePackagePinnedVersion_NoTeam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query string for nil team, got: %q", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	if err := client.PatchSoftwarePackagePinnedVersion(context.Background(), 42, nil, "^147"); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// TestClient_PatchSoftwarePackagePinnedVersion_SurfacesError replays Fleet's
// real rejection message so the wrapped error stays actionable for users.
func TestClient_PatchSoftwarePackagePinnedVersion_SurfacesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"Bad request","errors":[{"name":"base","reason":"Couldn't update. \"version\" can't be changed at the same time as other fields."}]}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	err := client.PatchSoftwarePackagePinnedVersion(context.Background(), 42, nil, "2.5.1")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "pinned version") {
		t.Errorf("expected the error to name the operation, got: %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected the wrapped error to be an *APIError, got: %T", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got: %d", apiErr.StatusCode)
	}
}

// TestClient_GetSoftwareTitle_DecodesPinnedVersion pins the read-side mapping.
// Fleet echoes the active pin as software_package.pinned_version and omits the
// key entirely when the title tracks latest, so the decoded pointer is what
// tells the resource layer "pinned" from "not pinned".
func TestClient_GetSoftwareTitle_DecodesPinnedVersion(t *testing.T) {
	tests := []struct {
		name string
		body string
		want *string
	}{
		{
			name: "pinned",
			body: `{"software_title":{"id":1,"name":"App","source":"apps","software_package":{"name":"app.pkg","version":"2.5.1","pinned_version":"2.5.1"}}}`,
			want: strPtr("2.5.1"),
		},
		{
			name: "pinned to a major version",
			body: `{"software_title":{"id":1,"name":"App","source":"apps","software_package":{"name":"app.pkg","version":"147.2","pinned_version":"^147"}}}`,
			want: strPtr("^147"),
		},
		{
			name: "key absent means not pinned",
			body: `{"software_title":{"id":1,"name":"App","source":"apps","software_package":{"name":"app.pkg","version":"2.5.1"}}}`,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
			title, err := client.GetSoftwareTitle(context.Background(), 1, nil)
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			got := title.SoftwarePackage.PinnedVersion
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("expected pinned_version nil, got: %q", *got)
			case tt.want != nil && got == nil:
				t.Errorf("expected pinned_version %q, got nil", *tt.want)
			case tt.want != nil && *got != *tt.want:
				t.Errorf("expected pinned_version %q, got: %q", *tt.want, *got)
			}
		})
	}
}

// --- Fleet 4.90: VPP / App Store app auto-update + managed configuration ----

// TestClient_GetSoftwareTitle_DecodesAutoUpdateConfig pins where the
// automatic-update settings live on the wire: Fleet embeds them at the *title*
// level, not inside app_store_app, and omits them when unset.
func TestClient_GetSoftwareTitle_DecodesAutoUpdateConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"software_title":{"id":1,"name":"App","source":"apps",
			"auto_update_enabled":true,
			"auto_update_window_start":"01:30",
			"auto_update_window_end":"04:00",
			"app_store_app":{"app_store_id":"123","platform":"ios","configuration":"<dict><key>k</key><string>v</string></dict>"}}}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	title, err := client.GetSoftwareTitle(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if title.AutoUpdateEnabled == nil || !*title.AutoUpdateEnabled {
		t.Errorf("expected auto_update_enabled true, got: %v", title.AutoUpdateEnabled)
	}
	if title.AutoUpdateWindowStart == nil || *title.AutoUpdateWindowStart != "01:30" {
		t.Errorf("unexpected auto_update_window_start: %v", title.AutoUpdateWindowStart)
	}
	if title.AutoUpdateWindowEnd == nil || *title.AutoUpdateWindowEnd != "04:00" {
		t.Errorf("unexpected auto_update_window_end: %v", title.AutoUpdateWindowEnd)
	}
	if got := DecodeAppConfiguration(title.AppStoreApp.Configuration); got != "<dict><key>k</key><string>v</string></dict>" {
		t.Errorf("unexpected decoded configuration: %q", got)
	}
}

// TestClient_GetSoftwareTitle_OmittedAutoUpdateConfigStaysNil is the other half
// of the opt-in Read convention: a title Fleet has no auto-update settings for
// must decode to nil pointers, not to false/"".
func TestClient_GetSoftwareTitle_OmittedAutoUpdateConfigStaysNil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"software_title":{"id":1,"name":"App","source":"apps","app_store_app":{"app_store_id":"123","platform":"darwin"}}}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	title, err := client.GetSoftwareTitle(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if title.AutoUpdateEnabled != nil || title.AutoUpdateWindowStart != nil || title.AutoUpdateWindowEnd != nil {
		t.Errorf("expected all auto-update fields nil, got: %v %v %v",
			title.AutoUpdateEnabled, title.AutoUpdateWindowStart, title.AutoUpdateWindowEnd)
	}
	if len(title.AppStoreApp.Configuration) != 0 {
		t.Errorf("expected configuration absent, got: %s", title.AppStoreApp.Configuration)
	}
}

// TestClient_UpdateAppStoreApp_AutoUpdateAndConfiguration asserts the EXACT
// request body. VPP needs a real Apple token so this path can't be exercised
// against a live server — byte-level assertions are the only thing standing
// between a typo'd JSON key and a silently ignored setting.
func TestClient_UpdateAppStoreApp_AutoUpdateAndConfiguration(t *testing.T) {
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/software/titles/100/app_store_app" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	enabled := true
	start, end := "01:30", "04:00"
	cfg, err := EncodeAppConfiguration(`<dict><key>ServerURL</key><string>https://example.test</string></dict>`)
	if err != nil {
		t.Fatalf("encode configuration: %v", err)
	}
	err = client.UpdateAppStoreApp(context.Background(), 100, &UpdateAppStoreAppRequest{
		TeamID:                5,
		SelfService:           true,
		LabelsIncludeAny:      []string{"iPads"},
		Configuration:         cfg,
		AutoUpdateEnabled:     &enabled,
		AutoUpdateWindowStart: &start,
		AutoUpdateWindowEnd:   &end,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// The configuration value reaches Fleet as a JSON string containing the
	// XML. encoding/json escapes <, > and & when it writes that string, so the
	// expected fragment is built with the same encoder rather than hand-escaped
	// — the point of the assertion is the key names, ordering and the
	// null-vs-omitted split, not encoding/json's escape table.
	xml := `<dict><key>ServerURL</key><string>https://example.test</string></dict>`
	cfgOnTheWire, err := json.Marshal(xml)
	if err != nil {
		t.Fatalf("marshal expected configuration: %v", err)
	}
	want := `{"team_id":5,"self_service":true,"labels_include_any":["iPads"],"labels_exclude_any":null,"labels_include_all":null,` +
		`"configuration":` + string(cfgOnTheWire) + `,` +
		`"auto_update_enabled":true,"auto_update_window_start":"01:30","auto_update_window_end":"04:00"}`
	if got := strings.TrimSpace(string(body)); got != want {
		t.Errorf("unexpected request body\n got: %s\nwant: %s", got, want)
	}

	// And the escaping must be lossless: what Fleet decodes is the original XML.
	var roundTripped struct {
		Configuration json.RawMessage `json:"configuration"`
	}
	if err := json.Unmarshal(body, &roundTripped); err != nil {
		t.Fatalf("request body is not valid JSON: %v", err)
	}
	if got := DecodeAppConfiguration(roundTripped.Configuration); got != xml {
		t.Errorf("configuration did not survive the round trip\n got: %q\nwant: %q", got, xml)
	}
}

// TestClient_UpdateAppStoreApp_OmitsUnsetFleet490Fields guards backwards
// compatibility: a configuration that doesn't use the 4.90 attributes must
// produce the same body the provider sent before this change, so the resource
// keeps working against older Fleet servers.
func TestClient_UpdateAppStoreApp_OmitsUnsetFleet490Fields(t *testing.T) {
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	err := client.UpdateAppStoreApp(context.Background(), 100, &UpdateAppStoreAppRequest{
		TeamID:           5,
		SelfService:      true,
		LabelsIncludeAny: []string{"Macs"},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	want := `{"team_id":5,"self_service":true,"labels_include_any":["Macs"],"labels_exclude_any":null,"labels_include_all":null}`
	if got := strings.TrimSpace(string(body)); got != want {
		t.Errorf("unexpected request body\n got: %s\nwant: %s", got, want)
	}
}

// TestClient_AddAppStoreApp_AndroidWithConfiguration covers the Add endpoint's
// half of the feature: `platform: android` is a valid enum value on Fleet 4.90
// ("platform must be one of 'ios', 'ipados', 'darwin', or 'android'", verified
// live), and `configuration` is accepted at create time — unlike the
// auto_update_* fields, which exist only on the Update endpoint.
func TestClient_AddAppStoreApp_AndroidWithConfiguration(t *testing.T) {
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/fleet/software/app_store_apps" {
			var err error
			body, err = io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"software_title_id": 101})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/fleet/software/titles/101" {
			_ = json.NewEncoder(w).Encode(getSoftwareTitleResponse{
				SoftwareTitle: &SoftwareTitle{
					ID:     101,
					Name:   "Zoom",
					Source: "android_apps",
					AppStoreApp: &AppStoreAppInfo{
						AdamID:      "com.zoom.videomeetings",
						Platform:    "android",
						Name:        "Zoom",
						SelfService: true,
					},
				},
			})
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-api-key", VerifyTLS: false})
	cfg, err := EncodeAppConfiguration(`{"managedConfiguration":{"enableLogging":true}}`)
	if err != nil {
		t.Fatalf("encode configuration: %v", err)
	}
	title, err := client.AddAppStoreApp(context.Background(), &AddAppStoreAppRequest{
		AppStoreID:    "com.zoom.videomeetings",
		TeamID:        5,
		Platform:      "android",
		SelfService:   true,
		Configuration: cfg,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if title.AppStoreApp.Platform != "android" {
		t.Errorf("expected platform android, got: %s", title.AppStoreApp.Platform)
	}

	// An Android managed configuration must reach Fleet as a JSON *object*,
	// not as a quoted string — Fleet validates it with
	// ValidateAndroidAppConfiguration and would reject a string.
	want := `{"app_store_id":"com.zoom.videomeetings","team_id":5,"platform":"android","self_service":true,` +
		`"configuration":{"managedConfiguration":{"enableLogging":true}}}`
	if got := strings.TrimSpace(string(body)); got != want {
		t.Errorf("unexpected request body\n got: %s\nwant: %s", got, want)
	}
}

// TestEncodeDecodeAppConfiguration covers the dual wire shape Fleet uses for
// managed app configuration: a JSON object for Android, a JSON string
// containing XML for iOS/iPadOS. The provider takes one raw string attribute
// for both, so the encoder has to pick — and the choice must round-trip.
func TestEncodeDecodeAppConfiguration(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		// wantPassthrough is true for input that is already valid JSON (the
		// Android managed-configuration object), which must reach Fleet
		// untouched so it decodes as an object. False means the input is not
		// JSON (the iOS/iPadOS XML), which must be wrapped into a JSON string.
		wantPassthrough bool
	}{
		{
			name: "empty stays nil",
			raw:  "",
		},
		{
			name:            "android json object passes through",
			raw:             `{"managedConfiguration":{"a":1}}`,
			wantPassthrough: true,
		},
		{
			name: "ios xml is wrapped into a json string",
			raw:  `<dict><key>a</key><string>b</string></dict>`,
		},
		{
			name: "xml with quotes and newlines survives",
			raw:  "<dict>\n  <key>URL</key>\n  <string>x?a=\"b\"</string>\n</dict>",
		},
		{
			name: "xml with an ampersand survives",
			raw:  `<dict><key>Name</key><string>Bits &amp; Bytes</string></dict>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EncodeAppConfiguration(tt.raw)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if tt.raw == "" {
				if encoded != nil {
					t.Errorf("expected nil for empty input so the field is omitted, got: %s", encoded)
				}
				return
			}
			// The encoder must always emit valid JSON — Fleet decodes the field
			// as json.RawMessage inside the request body, so invalid JSON here
			// would break the whole request, not just this field.
			if !json.Valid(encoded) {
				t.Fatalf("encoded value is not valid JSON: %s", encoded)
			}
			if tt.wantPassthrough {
				if string(encoded) != tt.raw {
					t.Errorf("expected valid JSON to pass through untouched\n got: %s\nwant: %s", encoded, tt.raw)
				}
				if encoded[0] != '{' {
					t.Errorf("expected an Android configuration to stay a JSON object, got: %s", encoded)
				}
			} else if encoded[0] != '"' {
				t.Errorf("expected non-JSON input to be wrapped in a JSON string (Fleet: \"expected configuration as a JSON string containing the XML\"), got: %s", encoded)
			}
			// The contract that actually matters: whatever escaping the encoder
			// applies, Fleet gets the original bytes back.
			if got := DecodeAppConfiguration(encoded); got != tt.raw {
				t.Errorf("round trip failed\n got: %q\nwant: %q", got, tt.raw)
			}
		})
	}
}
