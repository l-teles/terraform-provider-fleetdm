package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const testMobileConfig = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>PayloadIdentifier</key>
  <string>com.example.test</string>
  <key>PayloadType</key>
  <string>Configuration</string>
  <key>PayloadVersion</key>
  <integer>1</integer>
  <key>PayloadDisplayName</key>
  <string>Test Profile</string>
</dict>
</plist>`

func TestAccConfigurationProfileResource_basic(t *testing.T) {
	const profileUUID = "uuid-test-profile-1234"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/configuration_profiles" && r.Method == "POST":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"profile_uuid": profileUUID,
			})
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+profileUUID && r.URL.Query().Get("alt") == "media" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/x-apple-aspen-config")
			w.WriteHeader(http.StatusOK)
			// The heredoc in the config appends a trailing newline; match it here so Read
			// produces no diff against the config value.
			w.Write([]byte(testMobileConfig + "\n"))
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+profileUUID && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"profile_uuid":       profileUUID,
				"team_id":            nil,
				"name":               "Test Profile",
				"platform":           "darwin",
				"identifier":         "com.example.test",
				"checksum":           "abc123checksum",
				"created_at":         "2024-01-15T10:00:00Z",
				"uploaded_at":        "2024-01-15T10:00:00Z",
				"labels_include_all": []interface{}{},
				"labels_include_any": []interface{}{},
				"labels_exclude_any": []interface{}{},
			})
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+profileUUID && r.Method == "DELETE":
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
				Config: testAccConfigurationProfileResourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "profile_uuid", profileUUID),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "name", "Test Profile"),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "display_name", "Test Profile"),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "platform", "darwin"),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "identifier", "com.example.test"),
				),
			},
		},
	})
}

const testWindowsXMLProfile = `<?xml version="1.0" encoding="utf-8"?>
<SyncML xmlns="SYNCML:SYNCML1.2">
  <SyncBody>
    <Replace>
      <CmdID>1</CmdID>
      <Item>
        <Target><LocURI>./Device/Vendor/MSFT/BitLocker/RequireDeviceEncryption</LocURI></Target>
        <Data>1</Data>
      </Item>
    </Replace>
  </SyncBody>
</SyncML>`

func TestAccConfigurationProfileResource_displayName(t *testing.T) {
	const profileUUID = "uuid-win-profile-5678"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/configuration_profiles" && r.Method == "POST":
			// Verify the uploaded filename includes the display name
			err := r.ParseMultipartForm(10 << 20)
			if err != nil {
				t.Errorf("failed to parse multipart form: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			file, header, err := r.FormFile("profile")
			if err != nil {
				t.Errorf("failed to get form file: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer file.Close()
			if header.Filename != "BitLocker Policy.xml" {
				t.Errorf("expected filename 'BitLocker Policy.xml', got %q", header.Filename)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"profile_uuid": profileUUID,
			})
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+profileUUID && r.URL.Query().Get("alt") == "media" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(testWindowsXMLProfile + "\n"))
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+profileUUID && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"profile_uuid":       profileUUID,
				"team_id":            nil,
				"name":               "BitLocker Policy",
				"platform":           "windows",
				"identifier":         "",
				"checksum":           "",
				"created_at":         "2024-01-15T10:00:00Z",
				"uploaded_at":        "2024-01-15T10:00:00Z",
				"labels_include_all": []interface{}{},
				"labels_include_any": []interface{}{},
				"labels_exclude_any": []interface{}{},
			})
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+profileUUID && r.Method == "DELETE":
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
				Config: testAccConfigurationProfileWindowsConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.win_test", "profile_uuid", profileUUID),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.win_test", "display_name", "BitLocker Policy"),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.win_test", "name", "BitLocker Policy"),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.win_test", "platform", "windows"),
				),
			},
		},
	})
}

func testAccConfigurationProfileWindowsConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

resource "fleetdm_configuration_profile" "win_test" {
  display_name    = "BitLocker Policy"
  profile_content = <<-EOT
` + testWindowsXMLProfile + `
  EOT
}
`
}

func TestAccConfigurationProfileResource_windowsRequiresDisplayName(t *testing.T) {
	windowsProfileConfig := func(displayNameLine string) string {
		return `
provider "fleetdm" {
  server_address = "http://localhost:0"
  api_key        = "test-token"
}

resource "fleetdm_configuration_profile" "t" {
  ` + displayNameLine + `
  profile_content = <<-EOT
` + testWindowsXMLProfile + `
  EOT
}
`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      windowsProfileConfig(""),
				ExpectError: regexp.MustCompile(`display_name is required for Windows profiles`),
			},
			{
				Config:      windowsProfileConfig(`display_name = ""`),
				ExpectError: regexp.MustCompile(`display_name is required for Windows profiles`),
			},
			{
				Config:      windowsProfileConfig(`display_name = "My/Policy"`),
				ExpectError: regexp.MustCompile(`must not contain path separators`),
			},
			{
				Config:      windowsProfileConfig(`display_name = "Policy.xml"`),
				ExpectError: regexp.MustCompile(`must not include a profile file extension`),
			},
		},
	})
}

func testAccConfigurationProfileResourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

resource "fleetdm_configuration_profile" "test" {
  profile_content = <<-EOT
` + testMobileConfig + `
  EOT
}
`
}
