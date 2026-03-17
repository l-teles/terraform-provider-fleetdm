package fleetdm

import (
	"context"
	"encoding/json"
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

		var req PatchSoftwarePackageRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.InstallScript != "new install script" {
			t.Errorf("expected install_script 'new install script', got: %s", req.InstallScript)
		}
		if !req.SelfService {
			t.Error("expected self_service to be true")
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
