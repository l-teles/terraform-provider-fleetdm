package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTeamResource_basic(t *testing.T) {
	teamName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccTeamResourceConfig(teamName, "Test team description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_team.test", "name", teamName),
					resource.TestCheckResourceAttr("fleetdm_team.test", "description", "Test team description"),
					resource.TestCheckResourceAttrSet("fleetdm_team.test", "id"),
					resource.TestCheckResourceAttr("fleetdm_team.test", "host_expiry_enabled", "false"),
					resource.TestCheckResourceAttr("fleetdm_team.test", "enable_disk_encryption", "false"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "fleetdm_team.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccTeamResourceConfig(teamName+"-updated", "Updated description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_team.test", "name", teamName+"-updated"),
					resource.TestCheckResourceAttr("fleetdm_team.test", "description", "Updated description"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func TestAccTeamResource_withSettings(t *testing.T) {
	teamName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with settings
			{
				Config: testAccTeamResourceConfigWithSettings(teamName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_team.test", "name", teamName),
					resource.TestCheckResourceAttr("fleetdm_team.test", "host_expiry_enabled", "true"),
					resource.TestCheckResourceAttr("fleetdm_team.test", "host_expiry_window", "30"),
				),
			},
			// Update settings
			{
				Config: testAccTeamResourceConfigWithUpdatedSettings(teamName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_team.test", "host_expiry_enabled", "false"),
				),
			},
		},
	})
}

func testAccTeamResourceConfig(name, description string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_team" "test" {
  name        = %[1]q
  description = %[2]q
}
`, name, description)
}

func testAccTeamResourceConfigWithSettings(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_team" "test" {
  name        = %[1]q
  description = "Team with settings"

  host_expiry_enabled = true
  host_expiry_window  = 30
}
`, name)
}

func testAccTeamResourceConfigWithUpdatedSettings(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_team" "test" {
  name        = %[1]q
  description = "Team with updated settings"

  host_expiry_enabled = false
}
`, name)
}
