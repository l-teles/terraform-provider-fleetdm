package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccConfigurationProfilesDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/configuration_profiles" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"profiles": []map[string]interface{}{
					{
						"profile_uuid":       "uuid-aaaa-bbbb-cccc",
						"team_id":            nil,
						"name":               "macOS Security Baseline",
						"platform":           "darwin",
						"identifier":         "com.example.security",
						"checksum":           "abc123",
						"created_at":         "2024-01-15T10:00:00Z",
						"uploaded_at":        "2024-01-15T10:00:00Z",
						"labels_include_all": []interface{}{},
						"labels_include_any": []interface{}{},
						"labels_exclude_any": []interface{}{},
					},
					{
						"profile_uuid":       "uuid-dddd-eeee-ffff",
						"team_id":            nil,
						"name":               "Windows Compliance",
						"platform":           "windows",
						"identifier":         "",
						"checksum":           "def456",
						"created_at":         "2024-01-16T10:00:00Z",
						"uploaded_at":        "2024-01-16T10:00:00Z",
						"labels_include_all": []interface{}{},
						"labels_include_any": []interface{}{},
						"labels_exclude_any": []interface{}{},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccConfigurationProfilesDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_configuration_profiles.test", "profiles.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_configuration_profiles.test", "profiles.0.name", "macOS Security Baseline"),
					resource.TestCheckResourceAttr("data.fleetdm_configuration_profiles.test", "profiles.0.platform", "darwin"),
					resource.TestCheckResourceAttr("data.fleetdm_configuration_profiles.test", "profiles.0.profile_uuid", "uuid-aaaa-bbbb-cccc"),
					resource.TestCheckResourceAttr("data.fleetdm_configuration_profiles.test", "profiles.1.name", "Windows Compliance"),
					resource.TestCheckResourceAttr("data.fleetdm_configuration_profiles.test", "profiles.1.platform", "windows"),
				),
			},
		},
	})
}

func testAccConfigurationProfilesDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_configuration_profiles" "test" {}
`
}
