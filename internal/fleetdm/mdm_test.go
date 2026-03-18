package fleetdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		if r.URL.Path != "/api/v1/fleet/setup_experience" {
			t.Errorf("expected path /api/v1/fleet/setup_experience, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("team_id") != "1" {
			t.Errorf("expected team_id=1, got %s", r.URL.Query().Get("team_id"))
		}

		response := SetupExperience{
			EnableEndUserAuth:     true,
			EnableReleaseManually: false,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
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
			_, header, _ := r.FormFile("profile")
			if header.Filename != "profile.mobileconfig" {
				t.Errorf("expected default filename 'profile.mobileconfig', got %q", header.Filename)
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

	// Empty Filename should fall back to "profile.mobileconfig"
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
