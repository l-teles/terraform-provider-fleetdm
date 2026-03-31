package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccFleetResource_basic(t *testing.T) {
	fleetName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccFleetResourceConfig(fleetName, "Test fleet description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "name", fleetName),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "description", "Test fleet description"),
					resource.TestCheckResourceAttrSet("fleetdm_fleet.test", "id"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "host_expiry_enabled", "false"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "enable_disk_encryption", "false"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "fleetdm_fleet.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccFleetResourceConfig(fleetName+"-updated", "Updated description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "name", fleetName+"-updated"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "description", "Updated description"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func TestAccFleetResource_withSettings(t *testing.T) {
	fleetName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with settings
			{
				Config: testAccFleetResourceConfigWithSettings(fleetName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "name", fleetName),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "host_expiry_enabled", "true"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "host_expiry_window", "30"),
				),
			},
			// Update settings
			{
				Config: testAccFleetResourceConfigWithUpdatedSettings(fleetName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "host_expiry_enabled", "false"),
				),
			},
		},
	})
}

func testAccFleetResourceConfig(name, description string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name        = %[1]q
  description = %[2]q
}
`, name, description)
}

func testAccFleetResourceConfigWithSettings(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name        = %[1]q
  description = "Fleet with settings"

  host_expiry_enabled = true
  host_expiry_window  = 30
}
`, name)
}

func testAccFleetResourceConfigWithUpdatedSettings(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name        = %[1]q
  description = "Fleet with updated settings"

  host_expiry_enabled = false
}
`, name)
}
