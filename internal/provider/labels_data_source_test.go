package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccLabelsDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/labels" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"labels": []map[string]interface{}{
					{
						"id":          1,
						"name":        "macOS",
						"description": "All macOS hosts",
						"query":       "SELECT 1 FROM os_version WHERE platform = 'darwin'",
						"platform":    "darwin",
						"label_type":  "regular",
						"host_count":  5,
					},
					{
						"id":          2,
						"name":        "Linux",
						"description": "All Linux hosts",
						"query":       "SELECT 1 FROM os_version WHERE platform = 'linux'",
						"platform":    "linux",
						"label_type":  "regular",
						"host_count":  3,
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
				Config: testAccLabelsDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.0.name", "macOS"),
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.0.platform", "darwin"),
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.1.name", "Linux"),
				),
			},
		},
	})
}

func testAccLabelsDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_labels" "test" {}
`
}

// TestAccLabelsDataSource_live creates a label then verifies it appears in the list.
func TestAccLabelsDataSource_live(t *testing.T) {
	labelName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccLabelsDataSourceConfig_live(labelName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.fleetdm_labels.test", "labels.#"),
				),
			},
		},
	})
}

func testAccLabelsDataSourceConfig_live(labelName string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name  = %[1]q
  query = "SELECT 1"
}

data "fleetdm_labels" "test" {
  depends_on = [fleetdm_label.test]
}
`, labelName)
}
