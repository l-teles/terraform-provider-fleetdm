package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccQueryResource_basic(t *testing.T) {
	queryName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: testAccQueryResourceConfig(queryName, "SELECT * FROM system_info;", "Initial description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_query.test", "name", queryName),
					resource.TestCheckResourceAttr("fleetdm_query.test", "description", "Initial description"),
					resource.TestCheckResourceAttr("fleetdm_query.test", "query", "SELECT * FROM system_info;"),
					resource.TestCheckResourceAttrSet("fleetdm_query.test", "id"),
					resource.TestCheckResourceAttr("fleetdm_query.test", "observer_can_run", "false"),
					resource.TestCheckResourceAttr("fleetdm_query.test", "automations_enabled", "false"),
				),
			},
			// ImportState
			{
				ResourceName:      "fleetdm_query.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update query and description
			{
				Config: testAccQueryResourceConfig(queryName, "SELECT * FROM os_version;", "Updated description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_query.test", "description", "Updated description"),
					resource.TestCheckResourceAttr("fleetdm_query.test", "query", "SELECT * FROM os_version;"),
				),
			},
			// Update with observer_can_run, logging, and platform
			{
				Config: testAccQueryResourceConfigFull(queryName, "SELECT * FROM os_version;", "Updated description", true, "snapshot", "darwin"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_query.test", "observer_can_run", "true"),
					resource.TestCheckResourceAttr("fleetdm_query.test", "logging", "snapshot"),
					resource.TestCheckResourceAttr("fleetdm_query.test", "platform", "darwin"),
				),
			},
			// Update platform and logging to different values
			{
				Config: testAccQueryResourceConfigFull(queryName, "SELECT * FROM os_version;", "Final description", true, "differential", "linux"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_query.test", "description", "Final description"),
					resource.TestCheckResourceAttr("fleetdm_query.test", "logging", "differential"),
					resource.TestCheckResourceAttr("fleetdm_query.test", "platform", "linux"),
				),
			},
		},
	})
}

func TestAccQueryResource_withOptions(t *testing.T) {
	queryName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccQueryResourceConfigWithOptions(queryName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_query.test", "name", queryName),
					resource.TestCheckResourceAttr("fleetdm_query.test", "observer_can_run", "true"),
					resource.TestCheckResourceAttr("fleetdm_query.test", "logging", "snapshot"),
					resource.TestCheckResourceAttr("fleetdm_query.test", "platform", "darwin"),
				),
			},
		},
	})
}

func testAccQueryResourceConfig(name, query, description string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_query" "test" {
  name        = %[1]q
  description = %[3]q
  query       = %[2]q
}
`, name, query, description)
}

func testAccQueryResourceConfigFull(name, query, description string, observerCanRun bool, logging, platform string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_query" "test" {
  name             = %[1]q
  description      = %[3]q
  query            = %[2]q
  observer_can_run = %[4]t
  logging          = %[5]q
  platform         = %[6]q
}
`, name, query, description, observerCanRun, logging, platform)
}

func testAccQueryResourceConfigWithOptions(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_query" "test" {
  name            = %[1]q
  description     = "Query with options"
  query           = "SELECT * FROM system_info;"
  platform        = "darwin"
  observer_can_run = true
  logging         = "snapshot"
}
`, name)
}
