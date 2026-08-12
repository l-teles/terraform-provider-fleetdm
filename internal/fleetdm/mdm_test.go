package fleetdm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_ListMDMConfigProfiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/configuration_profiles" {
			t.Errorf("expected path /api/v1/fleet/configuration_profiles, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got %s", r.Method)
		}

		response := map[string]interface{}{
			"profiles": []map[string]interface{}{
				{
					"profile_uuid": "p-1234",
					"name":         "Test Profile",
					"platform":     "darwin",
					"identifier":   "com.example.test",
					"created_at":   "2024-01-01T00:00:00Z",
					"uploaded_at":  "2024-01-01T00:00:00Z",
				},
				{
					"profile_uuid": "p-5678",
					"name":         "Windows Profile",
					"platform":     "windows",
					"created_at":   "2024-01-02T00:00:00Z",
					"uploaded_at":  "2024-01-02T00:00:00Z",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	profiles, err := client.ListMDMConfigProfiles(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListMDMConfigProfiles failed: %v", err)
	}

	if len(profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(profiles))
	}

	if profiles[0].Name != "Test Profile" {
		t.Errorf("expected name 'Test Profile', got %s", profiles[0].Name)
	}

	if profiles[0].Platform != "darwin" {
		t.Errorf("expected platform 'darwin', got %s", profiles[0].Platform)
	}

	if profiles[1].Platform != "windows" {
		t.Errorf("expected platform 'windows', got %s", profiles[1].Platform)
	}
}

func TestClient_ListMDMConfigProfiles_WithTeamID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("team_id") != "5" {
			t.Errorf("expected team_id=5, got %s", r.URL.Query().Get("team_id"))
		}

		response := listMDMConfigProfilesResponse{
			Profiles: []MDMConfigProfile{},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	teamID := 5
	_, err = client.ListMDMConfigProfiles(context.Background(), &ListMDMConfigProfilesOptions{
		TeamID: &teamID,
	})
	if err != nil {
		t.Fatalf("ListMDMConfigProfiles with team ID failed: %v", err)
	}
}

func TestClient_GetMDMConfigProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/configuration_profiles/p-1234" {
			t.Errorf("expected path /api/v1/fleet/configuration_profiles/p-1234, got %s", r.URL.Path)
		}

		response := MDMConfigProfile{
			ProfileUUID: "p-1234",
			Name:        "Test Profile",
			Platform:    "darwin",
			Identifier:  "com.example.test",
			CreatedAt:   "2024-01-01T00:00:00Z",
			UploadedAt:  "2024-01-01T00:00:00Z",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	profile, err := client.GetMDMConfigProfile(context.Background(), "p-1234")
	if err != nil {
		t.Fatalf("GetMDMConfigProfile failed: %v", err)
	}

	if profile.ProfileUUID != "p-1234" {
		t.Errorf("expected UUID 'p-1234', got %s", profile.ProfileUUID)
	}

	if profile.Name != "Test Profile" {
		t.Errorf("expected name 'Test Profile', got %s", profile.Name)
	}
}

func TestClient_GetMDMSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/hosts/summary/mdm" {
			t.Errorf("expected path /api/v1/fleet/hosts/summary/mdm, got %s", r.URL.Path)
		}

		response := MDMSummary{
			CountsUpdatedAt: "2024-01-01T00:00:00Z",
			EnrollmentStatus: MDMEnrollmentSummary{
				EnrolledManualHostsCount:    50,
				EnrolledAutomatedHostsCount: 100,
				EnrolledPersonalHostsCount:  10,
				UnenrolledHostsCount:        5,
				PendingHostsCount:           3,
				HostsCount:                  168,
			},
			MDMSolutions: []MDMSolution{
				{
					ID:         1,
					Name:       "Fleet",
					ServerURL:  "https://fleet.example.com",
					HostsCount: 160,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	summary, err := client.GetMDMSummary(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("GetMDMSummary failed: %v", err)
	}

	if summary.EnrollmentStatus.EnrolledManualHostsCount != 50 {
		t.Errorf("expected manual=50, got %d", summary.EnrollmentStatus.EnrolledManualHostsCount)
	}

	if summary.EnrollmentStatus.EnrolledAutomatedHostsCount != 100 {
		t.Errorf("expected automated=100, got %d", summary.EnrollmentStatus.EnrolledAutomatedHostsCount)
	}

	if summary.EnrollmentStatus.HostsCount != 168 {
		t.Errorf("expected total=168, got %d", summary.EnrollmentStatus.HostsCount)
	}

	if len(summary.MDMSolutions) != 1 {
		t.Errorf("expected 1 MDM solution, got %d", len(summary.MDMSolutions))
	}

	if summary.MDMSolutions[0].Name != "Fleet" {
		t.Errorf("expected MDM name 'Fleet', got %s", summary.MDMSolutions[0].Name)
	}
}

func TestClient_GetMDMSummary_WithPlatform(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("platform") != "darwin" {
			t.Errorf("expected platform=darwin, got %s", r.URL.Query().Get("platform"))
		}

		response := MDMSummary{
			CountsUpdatedAt: "2024-01-01T00:00:00Z",
			EnrollmentStatus: MDMEnrollmentSummary{
				HostsCount: 50,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-key",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.GetMDMSummary(context.Background(), "darwin", nil)
	if err != nil {
		t.Fatalf("GetMDMSummary with platform failed: %v", err)
	}
}

func TestClient_DeleteConfigProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/configuration_profiles/p-1234" {
			t.Errorf("expected path /api/v1/fleet/configuration_profiles/p-1234, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	err := client.DeleteConfigProfile(context.Background(), "p-1234")
	if err != nil {
		t.Fatalf("DeleteConfigProfile failed: %v", err)
	}
}

func TestClient_GetBootstrapPackageMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/bootstrap/1/metadata" {
			t.Errorf("expected path /api/v1/fleet/bootstrap/1/metadata, got %s", r.URL.Path)
		}

		response := BootstrapPackage{
			TeamID:    1,
			Name:      "bootstrap.pkg",
			CreatedAt: "2024-01-01T00:00:00Z",
			Sha256:    "abc123",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	pkg, err := client.GetBootstrapPackageMetadata(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetBootstrapPackageMetadata failed: %v", err)
	}
	if pkg.Name != "bootstrap.pkg" {
		t.Errorf("expected name 'bootstrap.pkg', got %s", pkg.Name)
	}
	if pkg.TeamID != 1 {
		t.Errorf("expected team ID 1, got %d", pkg.TeamID)
	}
}

func TestClient_DeleteBootstrapPackage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/bootstrap/1" {
			t.Errorf("expected path /api/v1/fleet/bootstrap/1, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	err := client.DeleteBootstrapPackage(context.Background(), 1)
	if err != nil {
		t.Fatalf("DeleteBootstrapPackage failed: %v", err)
	}
}

func TestClient_GetSetupExperience(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/fleets/1" {
			t.Errorf("expected path /api/v1/fleet/fleets/1, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"team":{"id":1,"mdm":{"macos_setup":{
			"enable_end_user_authentication": true,
			"enable_release_device_manually": false
		}}}}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	setup, err := client.GetSetupExperience(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetSetupExperience failed: %v", err)
	}
	if !setup.EnableEndUserAuth {
		t.Error("expected EnableEndUserAuth to be true")
	}
	if setup.EnableReleaseManually {
		t.Error("expected EnableReleaseManually to be false")
	}
}

// TestClient_GetSetupExperience_RenamedKeys covers the spelling Fleet 4.90
// introduced with the team-to-fleet rename, where the same object is served as
// fleet.mdm.setup_experience with several fields renamed.
func TestClient_GetSetupExperience_RenamedKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/fleets/2" {
			t.Errorf("expected path /api/v1/fleet/fleets/2, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fleet":{"id":2,"mdm":{"setup_experience":{
			"enable_end_user_authentication": true,
			"apple_enable_release_device_manually": true,
			"lock_end_user_info": true,
			"require_all_software_macos": false,
			"require_all_software_windows": true,
			"macos_manual_agent_install": true
		}}}}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	setup, err := client.GetSetupExperience(context.Background(), 2)
	if err != nil {
		t.Fatalf("GetSetupExperience failed: %v", err)
	}
	if !setup.EnableEndUserAuth || !setup.EnableReleaseManually {
		t.Errorf("expected both base settings to be true, got %+v", setup)
	}
	if setup.LockEndUserInfo == nil || !*setup.LockEndUserInfo {
		t.Errorf("expected LockEndUserInfo to be true, got %v", setup.LockEndUserInfo)
	}
	if setup.RequireAllSoftwareMacOS == nil || *setup.RequireAllSoftwareMacOS {
		t.Errorf("expected RequireAllSoftwareMacOS to be false, got %v", setup.RequireAllSoftwareMacOS)
	}
	if setup.RequireAllSoftwareWindows == nil || !*setup.RequireAllSoftwareWindows {
		t.Errorf("expected RequireAllSoftwareWindows to be true, got %v", setup.RequireAllSoftwareWindows)
	}
	if setup.ManualAgentInstall == nil || !*setup.ManualAgentInstall {
		t.Errorf("expected ManualAgentInstall to be true, got %v", setup.ManualAgentInstall)
	}
}

// TestClient_GetSetupExperience_NoTeam covers team 0, Fleet's "no team" scope,
// which lives on the app config rather than on a team.
func TestClient_GetSetupExperience_NoTeam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/config" {
			t.Errorf("expected path /api/v1/fleet/config, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// Shape as served by Fleet 4.90: script and software are a string and a
		// list here, which the settings struct must not try to decode.
		_, _ = w.Write([]byte(`{"mdm":{"macos_setup":{
			"bootstrap_package": "",
			"enable_end_user_authentication": false,
			"enable_release_device_manually": true,
			"macos_setup_assistant": "",
			"script": "install.sh",
			"software": [],
			"lock_end_user_info": false,
			"manual_agent_install": true,
			"require_all_software_macos": true,
			"require_all_software_windows": false
		}}}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	setup, err := client.GetSetupExperience(context.Background(), 0)
	if err != nil {
		t.Fatalf("GetSetupExperience failed: %v", err)
	}
	if !setup.EnableReleaseManually {
		t.Error("expected EnableReleaseManually to be true")
	}
	if setup.ManualAgentInstall == nil || !*setup.ManualAgentInstall {
		t.Errorf("expected ManualAgentInstall to be true, got %v", setup.ManualAgentInstall)
	}
	if setup.RequireAllSoftwareMacOS == nil || !*setup.RequireAllSoftwareMacOS {
		t.Errorf("expected RequireAllSoftwareMacOS to be true, got %v", setup.RequireAllSoftwareMacOS)
	}
}

func TestClient_GetSetupExperience_MissingSettings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"team":{"id":3,"mdm":{}}}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	if _, err := client.GetSetupExperience(context.Background(), 3); err == nil {
		t.Error("expected an error when the response carries no setup experience settings")
	}
}

func TestClient_GetSetupExperience_TeamNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource Not Found"}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	_, err := client.GetSetupExperience(context.Background(), 42)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected a wrapped 404 APIError, got %v", err)
	}
}

func TestClient_UpdateSetupExperience(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/setup_experience" {
			t.Errorf("expected path /api/v1/fleet/setup_experience, got %s", r.URL.Path)
		}

		var req UpdateSetupExperienceRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.TeamID != 1 {
			t.Errorf("expected team_id 1, got %d", req.TeamID)
		}
		if req.EnableEndUserAuth == nil || !*req.EnableEndUserAuth {
			t.Error("expected enable_end_user_authentication to be true")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	enable := true
	err := client.UpdateSetupExperience(context.Background(), &UpdateSetupExperienceRequest{
		TeamID:            1,
		EnableEndUserAuth: &enable,
	})
	if err != nil {
		t.Fatalf("UpdateSetupExperience failed: %v", err)
	}
}

// captureSetupExperiencePatch returns a server that records the raw PATCH body
// sent to /setup_experience, so tests can assert on key presence rather than
// on the decoded struct (nil pointers and false are indistinguishable there).
func captureSetupExperiencePatch(t *testing.T, body *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/setup_experience" {
			t.Errorf("expected path /api/v1/fleet/setup_experience, got %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func TestClient_UpdateSetupExperience_OptInFields(t *testing.T) {
	var body map[string]any
	server := captureSetupExperiencePatch(t, &body)
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	enabled, disabled := true, false
	err := client.UpdateSetupExperience(context.Background(), &UpdateSetupExperienceRequest{
		TeamID:                    7,
		LockEndUserInfo:           &enabled,
		RequireAllSoftwareMacOS:   &disabled,
		RequireAllSoftwareWindows: &enabled,
		ManualAgentInstall:        &disabled,
	})
	if err != nil {
		t.Fatalf("UpdateSetupExperience failed: %v", err)
	}

	want := map[string]any{
		"team_id":                      float64(7),
		"lock_end_user_info":           true,
		"require_all_software_macos":   false,
		"require_all_software_windows": true,
		"manual_agent_install":         false,
	}
	if len(body) != len(want) {
		t.Fatalf("unexpected request body: got %v, want %v", body, want)
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("expected %s=%v, got %v", k, v, body[k])
		}
	}
}

func TestClient_UpdateSetupExperience_OmitsUnsetOptInFields(t *testing.T) {
	var body map[string]any
	server := captureSetupExperiencePatch(t, &body)
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	enabled := true
	err := client.UpdateSetupExperience(context.Background(), &UpdateSetupExperienceRequest{
		TeamID:            1,
		EnableEndUserAuth: &enabled,
	})
	if err != nil {
		t.Fatalf("UpdateSetupExperience failed: %v", err)
	}

	for _, field := range []string{
		"lock_end_user_info",
		"require_all_software_macos",
		"require_all_software_windows",
		"manual_agent_install",
	} {
		if _, ok := body[field]; ok {
			t.Errorf("expected %s to be omitted from the request body, got %v", field, body[field])
		}
	}
	if body["enable_end_user_authentication"] != true {
		t.Errorf("expected enable_end_user_authentication=true, got %v", body["enable_end_user_authentication"])
	}
}

func TestClient_GetSetupExperience_OptInFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// manual_agent_install is deliberately absent: Fleet omits or nulls it
		// for a team that has never had it written.
		_, _ = w.Write([]byte(`{"team":{"id":1,"mdm":{"macos_setup":{
			"enable_end_user_authentication": true,
			"enable_release_device_manually": false,
			"lock_end_user_info": true,
			"require_all_software_macos": false,
			"require_all_software_windows": true
		}}}}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	setup, err := client.GetSetupExperience(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetSetupExperience failed: %v", err)
	}
	if setup.LockEndUserInfo == nil || !*setup.LockEndUserInfo {
		t.Errorf("expected LockEndUserInfo to be true, got %v", setup.LockEndUserInfo)
	}
	if setup.RequireAllSoftwareMacOS == nil || *setup.RequireAllSoftwareMacOS {
		t.Errorf("expected RequireAllSoftwareMacOS to be false, got %v", setup.RequireAllSoftwareMacOS)
	}
	if setup.RequireAllSoftwareWindows == nil || !*setup.RequireAllSoftwareWindows {
		t.Errorf("expected RequireAllSoftwareWindows to be true, got %v", setup.RequireAllSoftwareWindows)
	}
	if setup.ManualAgentInstall != nil {
		t.Errorf("expected ManualAgentInstall to stay nil when absent, got %v", *setup.ManualAgentInstall)
	}
}

func TestClient_GetConfigProfileContent(t *testing.T) {
	const wantContent = `<?xml version="1.0"?><plist version="1.0"><dict><key>PayloadType</key><string>Configuration</string></dict></plist>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/fleet/configuration_profiles/p-abc123" {
			t.Errorf("expected path /api/v1/fleet/configuration_profiles/p-abc123, got: %s", r.URL.Path)
		}
		if r.URL.Query().Get("alt") != "media" {
			t.Errorf("expected alt=media query param, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/x-apple-aspen-config")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(wantContent))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	content, err := client.GetConfigProfileContent(context.Background(), "p-abc123")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if content != wantContent {
		t.Errorf("expected content %q, got: %q", wantContent, content)
	}
}

func TestProfileExtensionFromContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "macOS mobileconfig with PayloadType",
			content: `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>PayloadType</key>
  <string>Configuration</string>
</dict>
</plist>`,
			want: ".mobileconfig",
		},
		{
			name: "macOS mobileconfig with plist tag",
			content: `<?xml version="1.0"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict></dict></plist>`,
			want: ".mobileconfig",
		},
		{
			name: "Windows XML profile",
			content: `<?xml version="1.0" encoding="utf-8"?>
<SyncML xmlns="SYNCML:SYNCML1.2">
  <SyncBody>
    <Replace>
      <CmdID>1</CmdID>
      <Item>
        <Target><LocURI>./Device/Vendor/MSFT/BitLocker</LocURI></Target>
      </Item>
    </Replace>
  </SyncBody>
</SyncML>`,
			want: ".xml",
		},
		{
			name:    "Windows XML without XML declaration",
			content: `<SyncML xmlns="SYNCML:SYNCML1.2"><SyncBody></SyncBody></SyncML>`,
			want:    ".xml",
		},
		{
			name:    "macOS plist without XML declaration",
			content: `<plist version="1.0"><dict><key>PayloadType</key><string>Configuration</string></dict></plist>`,
			want:    ".mobileconfig",
		},
		{
			name:    "Apple declaration JSON",
			content: `{"Type": "com.apple.configuration.management.test", "Payload": {}}`,
			want:    ".json",
		},
		{
			name:    "empty content defaults to mobileconfig",
			content: "",
			want:    ".mobileconfig",
		},
		{
			name:    "whitespace-only content defaults to mobileconfig",
			content: "   \n\t  ",
			want:    ".mobileconfig",
		},
		{
			name:    "Windows XML with UTF-8 BOM",
			content: "\xEF\xBB\xBF<?xml version=\"1.0\"?><SyncML><SyncBody></SyncBody></SyncML>",
			want:    ".xml",
		},
		{
			name:    "JSON with UTF-8 BOM",
			content: "\xEF\xBB\xBF{\"Type\": \"com.apple.configuration.test\"}",
			want:    ".json",
		},
		{
			name:    "Windows XML with BOM and leading whitespace",
			content: "\xEF\xBB\xBF\n  <SyncML><SyncBody></SyncBody></SyncML>",
			want:    ".xml",
		},
		{
			name:    "JSON with BOM and leading whitespace",
			content: "\xEF\xBB\xBF  \n{\"Type\": \"com.apple.configuration.test\"}",
			want:    ".json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProfileExtensionFromContent([]byte(tt.content))
			if got != tt.want {
				t.Errorf("ProfileExtensionFromContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClient_CreateConfigProfile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/fleet/configuration_profiles", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			err := r.ParseMultipartForm(10 << 20)
			if err != nil {
				t.Fatalf("failed to parse multipart form: %v", err)
			}
			file, header, err := r.FormFile("profile")
			if err != nil {
				t.Fatalf("failed to get form file: %v", err)
			}
			defer file.Close()

			if header.Filename != "BitLocker Policy.xml" {
				t.Errorf("expected filename 'BitLocker Policy.xml', got %q", header.Filename)
			}
			if r.FormValue("team_id") != "1" {
				t.Errorf("expected team_id '1', got %q", r.FormValue("team_id"))
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"profile_uuid": "p-win-1234"})
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/v1/fleet/configuration_profiles/p-win-1234", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(MDMConfigProfile{
			ProfileUUID: "p-win-1234",
			Name:        "BitLocker Policy",
			Platform:    "windows",
			CreatedAt:   "2024-01-01T00:00:00Z",
			UploadedAt:  "2024-01-01T00:00:00Z",
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	teamID := 1
	profile, err := client.CreateConfigProfile(context.Background(), &CreateConfigProfileRequest{
		TeamID:   &teamID,
		Filename: "BitLocker Policy.xml",
		Profile:  []byte(`<?xml version="1.0"?><SyncML><SyncBody></SyncBody></SyncML>`),
	})
	if err != nil {
		t.Fatalf("CreateConfigProfile failed: %v", err)
	}
	if profile.ProfileUUID != "p-win-1234" {
		t.Errorf("expected UUID 'p-win-1234', got %s", profile.ProfileUUID)
	}
	if profile.Name != "BitLocker Policy" {
		t.Errorf("expected name 'BitLocker Policy', got %s", profile.Name)
	}
	if profile.Platform != "windows" {
		t.Errorf("expected platform 'windows', got %s", profile.Platform)
	}
}

func TestClient_CreateConfigProfile_DefaultFilename(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/fleet/configuration_profiles", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			err := r.ParseMultipartForm(10 << 20)
			if err != nil {
				t.Fatalf("failed to parse multipart form: %v", err)
			}
			file, header, err := r.FormFile("profile")
			if err != nil {
				t.Fatalf("failed to get form file: %v", err)
			}
			defer file.Close()
			if !strings.HasPrefix(header.Filename, "tf_") || !strings.HasSuffix(header.Filename, ".mobileconfig") {
				t.Errorf("expected filename matching 'tf_<random>.mobileconfig', got %q", header.Filename)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"profile_uuid": "p-mac-5678"})
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/v1/fleet/configuration_profiles/p-mac-5678", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(MDMConfigProfile{
			ProfileUUID: "p-mac-5678",
			Name:        "Test Profile",
			Platform:    "darwin",
			Identifier:  "com.example.test",
			CreatedAt:   "2024-01-01T00:00:00Z",
			UploadedAt:  "2024-01-01T00:00:00Z",
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Empty Filename should generate a random "tf_<hex>.mobileconfig" name
	profile, err := client.CreateConfigProfile(context.Background(), &CreateConfigProfileRequest{
		Profile: []byte(`<?xml version="1.0"?><plist version="1.0"><dict></dict></plist>`),
	})
	if err != nil {
		t.Fatalf("CreateConfigProfile failed: %v", err)
	}
	if profile.ProfileUUID != "p-mac-5678" {
		t.Errorf("expected UUID 'p-mac-5678', got %s", profile.ProfileUUID)
	}
}

func TestClient_GetConfigProfileContent_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("profile not found"))
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})

	_, err := client.GetConfigProfileContent(context.Background(), "p-does-not-exist")
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

// TestClient_GetConfigProfileContent_RejectsOversizeResponse pins that the
// raw content path, which bypasses doRequest, still applies the response cap.
func TestClient_GetConfigProfileContent_RejectsOversizeResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		streamOversizeBody(w, maxResponseBytes+1)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})

	_, err := client.GetConfigProfileContent(context.Background(), "p-oversize")
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("expected a body-limit error, got: %v", err)
	}
}

func TestProfileIdentifierFromContent(t *testing.T) {
	mobileconfig := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>PayloadContent</key>
  <array>
    <dict>
      <key>PayloadIdentifier</key>
      <string>com.example.nested.payload</string>
      <key>PayloadType</key>
      <string>com.apple.dock</string>
    </dict>
  </array>
  <key>PayloadDisplayName</key>
  <string>Test Profile</string>
  <key>PayloadIdentifier</key>
  <string>com.example.toplevel</string>
</dict>
</plist>`

	cases := []struct {
		name    string
		content string
		wantID  string
		wantOK  bool
	}{
		{"mobileconfig top-level wins over nested", mobileconfig, "com.example.toplevel", true},
		{"mobileconfig with BOM", "\xef\xbb\xbf" + mobileconfig, "com.example.toplevel", true},
		{"ddm json declaration", `{"Type":"com.apple.configuration.management.test","Identifier":"com.example.ddm"}`, "com.example.ddm", true},
		{"ddm json without identifier", `{"Type":"com.apple.configuration.management.test"}`, "", false},
		{"windows xml has no identifier", `<Replace><Item><Target><LocURI>./Device/X</LocURI></Target></Item></Replace>`, "", false},
		{"garbage", "not a profile", "", false},
		{"malformed plist", `<plist><dict><key>PayloadIdentifier</key>`, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, ok := ProfileIdentifierFromContent([]byte(c.content))
			if id != c.wantID || ok != c.wantOK {
				t.Errorf("got (%q, %v), want (%q, %v)", id, ok, c.wantID, c.wantOK)
			}
		})
	}
}

// TestClient_CreateConfigProfile_RepeatedLabelFields is a regression test:
// Fleet rejects comma-joined label values as a single unknown label name, so
// each label must be its own form field.
func TestClient_CreateConfigProfile_RepeatedLabelFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/fleet/configuration_profiles", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		got := r.MultipartForm.Value["labels_include_all"]
		want := []string{"label one", "label two"}
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("expected repeated labels_include_all fields %v, got %v", want, got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"profile_uuid": "p-lbl-1"})
	})
	mux.HandleFunc("/api/v1/fleet/configuration_profiles/p-lbl-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(MDMConfigProfile{ProfileUUID: "p-lbl-1"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.CreateConfigProfile(context.Background(), &CreateConfigProfileRequest{
		Filename:         "test.xml",
		Profile:          []byte(`<Replace></Replace>`),
		LabelsIncludeAll: []string{"label one", "label two"},
	})
	if err != nil {
		t.Fatalf("CreateConfigProfile failed: %v", err)
	}
}

func TestClient_UpdateConfigProfile_LabelsOnly(t *testing.T) {
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/configuration_profiles/p-upd-1" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		gotMethod = r.Method
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		if _, _, err := r.FormFile("profile"); err == nil {
			t.Error("expected no file part on labels-only update")
		}
		got := r.MultipartForm.Value["labels_exclude_any"]
		if len(got) != 2 || got[0] != "lbl-a" || got[1] != "lbl-b" {
			t.Errorf("expected repeated labels_exclude_any [lbl-a lbl-b], got %v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.UpdateConfigProfile(context.Background(), "p-upd-1", &UpdateConfigProfileRequest{
		LabelsExcludeAny: []string{"lbl-a", "lbl-b"},
	})
	if err != nil {
		t.Fatalf("UpdateConfigProfile failed: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("expected PATCH, got %s", gotMethod)
	}
}

func TestClient_UpdateConfigProfile_WithContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		file, header, err := r.FormFile("profile")
		if err != nil {
			t.Fatalf("expected file part: %v", err)
		}
		defer file.Close()
		if header.Filename != "updated.xml" {
			t.Errorf("expected filename 'updated.xml', got %q", header.Filename)
		}
		if got := r.MultipartForm.Value["labels_include_any"]; len(got) != 1 || got[0] != "keep-me" {
			t.Errorf("expected labels_include_any [keep-me] alongside content, got %v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.UpdateConfigProfile(context.Background(), "p-upd-2", &UpdateConfigProfileRequest{
		Profile:          []byte(`<Replace><Item></Item></Replace>`),
		Filename:         "updated.xml",
		LabelsIncludeAny: []string{"keep-me"},
	})
	if err != nil {
		t.Fatalf("UpdateConfigProfile failed: %v", err)
	}
}

func TestClient_UpdateConfigProfile_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"message": "Resource Not Found"})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{ServerAddress: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.UpdateConfigProfile(context.Background(), "p-missing", &UpdateConfigProfileRequest{})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Errorf("expected APIError with 404, got %v", err)
	}
}
