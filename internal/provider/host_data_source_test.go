package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccHostDataSource_byID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/hosts/42" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"host": map[string]interface{}{
					"id":                           42,
					"uuid":                         "abc-def-123",
					"hostname":                     "my-mac.local",
					"display_name":                 "My Mac",
					"computer_name":                "My Mac",
					"platform":                     "darwin",
					"os_version":                   "macOS 14.1",
					"build":                        "23B74",
					"platform_like":                "darwin",
					"cpu_type":                     "x86_64",
					"cpu_brand":                    "Intel Core i9",
					"cpu_physical_cores":           8,
					"cpu_logical_cores":            16,
					"memory":                       17179869184,
					"hardware_vendor":              "Apple",
					"hardware_model":               "MacBookPro18,1",
					"hardware_serial":              "SN12345678",
					"primary_ip":                   "10.0.0.5",
					"primary_mac":                  "aa:bb:cc:dd:ee:ff",
					"public_ip":                    "1.2.3.4",
					"team_id":                      nil,
					"team_name":                    "",
					"status":                       "online",
					"gigs_disk_space_available":    100.5,
					"percent_disk_space_available": 45.3,
					"seen_time":                    "2024-01-15T10:00:00Z",
					"created_at":                   "2024-01-01T00:00:00Z",
					"updated_at":                   "2024-01-15T10:00:00Z",
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
				Config: testAccHostDataSourceConfig(server.URL, 42),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_host.test", "id", "42"),
					resource.TestCheckResourceAttr("data.fleetdm_host.test", "hostname", "my-mac.local"),
					resource.TestCheckResourceAttr("data.fleetdm_host.test", "platform", "darwin"),
					resource.TestCheckResourceAttr("data.fleetdm_host.test", "os_version", "macOS 14.1"),
					resource.TestCheckResourceAttr("data.fleetdm_host.test", "status", "online"),
					resource.TestCheckResourceAttr("data.fleetdm_host.test", "hardware_serial", "SN12345678"),
				),
			},
		},
	})
}

func TestAccHostDataSource_byIdentifier(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/hosts/identifier/SN12345678" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"host": map[string]interface{}{
					"id":                           42,
					"uuid":                         "abc-def-123",
					"hostname":                     "my-mac.local",
					"display_name":                 "My Mac",
					"computer_name":                "My Mac",
					"platform":                     "darwin",
					"os_version":                   "macOS 14.1",
					"build":                        "23B74",
					"platform_like":                "darwin",
					"cpu_type":                     "x86_64",
					"cpu_brand":                    "Intel Core i9",
					"cpu_physical_cores":           8,
					"cpu_logical_cores":            16,
					"memory":                       17179869184,
					"hardware_vendor":              "Apple",
					"hardware_model":               "MacBookPro18,1",
					"hardware_serial":              "SN12345678",
					"primary_ip":                   "10.0.0.5",
					"primary_mac":                  "aa:bb:cc:dd:ee:ff",
					"public_ip":                    "1.2.3.4",
					"team_id":                      nil,
					"team_name":                    "",
					"status":                       "online",
					"gigs_disk_space_available":    100.5,
					"percent_disk_space_available": 45.3,
					"seen_time":                    "2024-01-15T10:00:00Z",
					"created_at":                   "2024-01-01T00:00:00Z",
					"updated_at":                   "2024-01-15T10:00:00Z",
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
				Config: testAccHostDataSourceConfigByIdentifier(server.URL, "SN12345678"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_host.test", "id", "42"),
					resource.TestCheckResourceAttr("data.fleetdm_host.test", "identifier", "SN12345678"),
					resource.TestCheckResourceAttr("data.fleetdm_host.test", "hostname", "my-mac.local"),
					resource.TestCheckResourceAttr("data.fleetdm_host.test", "platform", "darwin"),
					resource.TestCheckResourceAttr("data.fleetdm_host.test", "hardware_serial", "SN12345678"),
				),
			},
		},
	})
}

func TestAccHostDataSource_validationBothIDAndIdentifier(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "fleetdm" {
  server_address = "http://localhost:9999"
  api_key        = "test-token"
}

data "fleetdm_host" "test" {
  id         = 42
  identifier = "SN12345678"
}
`,
				ExpectError: regexp.MustCompile(`Exactly one of "id" or "identifier" must be specified`),
			},
		},
	})
}

func TestAccHostDataSource_validationNeitherIDNorIdentifier(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "fleetdm" {
  server_address = "http://localhost:9999"
  api_key        = "test-token"
}

data "fleetdm_host" "test" {}
`,
				ExpectError: regexp.MustCompile(`Exactly one of "id" or "identifier" must be specified`),
			},
		},
	})
}

func testAccHostDataSourceConfig(serverURL string, id int) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

data "fleetdm_host" "test" {
  id = %[2]d
}
`, serverURL, id)
}

func testAccHostDataSourceConfigByIdentifier(serverURL, identifier string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

data "fleetdm_host" "test" {
  identifier = %[2]q
}
`, serverURL, identifier)
}
