package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTeamDataSource_basic(t *testing.T) {
	teamName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create a team first, then read it via data source
			{
				Config: testAccTeamDataSourceConfig(teamName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_team.test", "name", teamName),
					resource.TestCheckResourceAttr("data.fleetdm_team.test", "description", "Test team for data source"),
					resource.TestCheckResourceAttrSet("data.fleetdm_team.test", "id"),
					resource.TestCheckResourceAttrSet("data.fleetdm_team.test", "user_count"),
					resource.TestCheckResourceAttrSet("data.fleetdm_team.test", "host_count"),
				),
			},
		},
	})
}

func testAccTeamDataSourceConfig(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_team" "test" {
  name        = %[1]q
  description = "Test team for data source"
}

data "fleetdm_team" "test" {
  id = fleetdm_team.test.id
}
`, name)
}
