package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccActivitiesDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/activities" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"activities": []map[string]interface{}{
					{
						"id":              1,
						"created_at":      "2024-01-15T10:00:00Z",
						"actor_full_name": "Alice Admin",
						"actor_id":        1,
						"actor_gravatar":  "",
						"actor_email":     "alice@example.com",
						"type":            "created_team",
						"fleet_initiated": false,
					},
					{
						"id":              2,
						"created_at":      "2024-01-15T11:00:00Z",
						"actor_full_name": "",
						"actor_gravatar":  "",
						"actor_email":     "",
						"type":            "applied_spec_labels",
						"fleet_initiated": true,
					},
				},
				"meta": map[string]interface{}{
					"has_previous_results": false,
					"has_next_results":     false,
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
				Config: testAccActivitiesDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_activities.test", "activities.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_activities.test", "activities.0.actor_full_name", "Alice Admin"),
					resource.TestCheckResourceAttr("data.fleetdm_activities.test", "activities.0.type", "created_team"),
					resource.TestCheckResourceAttr("data.fleetdm_activities.test", "activities.0.fleet_initiated", "false"),
					resource.TestCheckResourceAttr("data.fleetdm_activities.test", "activities.1.fleet_initiated", "true"),
				),
			},
		},
	})
}

func testAccActivitiesDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_activities" "test" {}
`
}

// TestAccActivitiesDataSource_live tests the activities data source against a real Fleet instance.
func TestAccActivitiesDataSource_live(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Activities exist from the test setup itself; just verify the data source works.
				Config: providerConfig() + `data "fleetdm_activities" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// The activities list may be empty on a fresh instance, so we only check it is set.
					resource.TestCheckResourceAttrSet("data.fleetdm_activities.test", "activities.#"),
				),
			},
		},
	})
}
