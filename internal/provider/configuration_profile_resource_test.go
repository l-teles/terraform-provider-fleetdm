package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "platform", "darwin"),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "identifier", "com.example.test"),
				),
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
