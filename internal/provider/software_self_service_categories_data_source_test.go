package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSoftwareSelfServiceCategoriesDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/software/self_service_categories" && r.Method == http.MethodGet {
			if got := r.URL.Query().Get("fleet_id"); got != "3" {
				t.Errorf("Expected the data source to send fleet_id=3, got %q", got)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"self_service_categories": []map[string]interface{}{
					{"id": 12, "name": "🌎 Browsers", "fleet_id": 3, "team_id": 3},
					{"id": 13, "name": "🧰 Developer tools", "fleet_id": 3, "team_id": 3},
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
				Config: testAccSelfServiceCategoriesDataSourceConfig(server.URL, 3),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_software_self_service_categories.test", "fleet_id", "3"),
					resource.TestCheckResourceAttr("data.fleetdm_software_self_service_categories.test", "categories.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_software_self_service_categories.test", "categories.0.id", "12"),
					resource.TestCheckResourceAttr("data.fleetdm_software_self_service_categories.test", "categories.0.name", "🌎 Browsers"),
					resource.TestCheckResourceAttr("data.fleetdm_software_self_service_categories.test", "categories.1.id", "13"),
					resource.TestCheckResourceAttr("data.fleetdm_software_self_service_categories.test", "categories.1.name", "🧰 Developer tools"),
				),
			},
		},
	})
}

// fleet_id 0 selects the categories for hosts that are not assigned to a
// fleet; the parameter must still be sent rather than dropped as a zero value.
func TestAccSoftwareSelfServiceCategoriesDataSource_zeroFleetID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if got := r.URL.RawQuery; got != "fleet_id=0" {
			t.Errorf("Expected query 'fleet_id=0', got %q", got)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"self_service_categories": []map[string]interface{}{
				{"id": 1, "name": "🛟 Support", "fleet_id": 0, "team_id": 0},
			},
		})
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSelfServiceCategoriesDataSourceConfig(server.URL, 0),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_software_self_service_categories.test", "categories.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_software_self_service_categories.test", "categories.0.name", "🛟 Support"),
				),
			},
		},
	})
}

// A fleet with only the legacy "team_id" key in the response must still list.
func TestAccSoftwareSelfServiceCategoriesDataSource_legacyTeamIDKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"self_service_categories": []map[string]interface{}{
				{"id": 9, "name": "🔐 Security", "team_id": 5},
			},
		})
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSelfServiceCategoriesDataSourceConfig(server.URL, 5),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_software_self_service_categories.test", "categories.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_software_self_service_categories.test", "categories.0.id", "9"),
				),
			},
		},
	})
}

func TestAccSoftwareSelfServiceCategoriesDataSource_empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"self_service_categories": []map[string]interface{}{},
		})
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSelfServiceCategoriesDataSourceConfig(server.URL, 7),
				Check: resource.TestCheckResourceAttr(
					"data.fleetdm_software_self_service_categories.test", "categories.#", "0"),
			},
		},
	})
}

func TestAccSoftwareSelfServiceCategoriesDataSource_error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "forbidden"})
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccSelfServiceCategoriesDataSourceConfig(server.URL, 3),
				ExpectError: regexp.MustCompile(`Unable to Read FleetDM Self-Service Categories`),
			},
		},
	})
}

// TestAccSoftwareSelfServiceCategoriesDataSource_live lists the categories of a
// freshly created fleet against a live Fleet instance. Fleet seeds every new
// fleet with default categories, so the list is non-empty even before
// Terraform adds one.
func TestAccSoftwareSelfServiceCategoriesDataSource_live(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	fleetName := "tf-acc-test-" + suffix
	categoryName := "🧰 tf-acc-test-" + suffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSelfServiceCategoriesDataSourceLiveConfig(fleetName, categoryName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.fleetdm_software_self_service_categories.test", "fleet_id",
						"fleetdm_fleet.test", "id",
					),
					resource.TestCheckResourceAttrSet(
						"data.fleetdm_software_self_service_categories.test", "categories.#"),
				),
			},
		},
	})
}

func testAccSelfServiceCategoriesDataSourceConfig(serverURL string, fleetID int) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

data "fleetdm_software_self_service_categories" "test" {
  fleet_id = %[2]d
}
`, serverURL, fleetID)
}

func testAccSelfServiceCategoriesDataSourceLiveConfig(fleetName, categoryName string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %[1]q
}

resource "fleetdm_software_self_service_category" "test" {
  fleet_id = fleetdm_fleet.test.id
  name     = %[2]q
}

data "fleetdm_software_self_service_categories" "test" {
  fleet_id   = fleetdm_fleet.test.id
  depends_on = [fleetdm_software_self_service_category.test]
}
`, fleetName, categoryName)
}
