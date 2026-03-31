package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccFleetDataSource_basic(t *testing.T) {
	fleetName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create a fleet first, then read it via data source
			{
				Config: testAccFleetDataSourceConfig(fleetName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_fleet.test", "name", fleetName),
					resource.TestCheckResourceAttr("data.fleetdm_fleet.test", "description", "Test fleet for data source"),
					resource.TestCheckResourceAttrSet("data.fleetdm_fleet.test", "id"),
					resource.TestCheckResourceAttrSet("data.fleetdm_fleet.test", "user_count"),
					resource.TestCheckResourceAttrSet("data.fleetdm_fleet.test", "host_count"),
				),
			},
		},
	})
}

func testAccFleetDataSourceConfig(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name        = %[1]q
  description = "Test fleet for data source"
}

data "fleetdm_fleet" "test" {
  id = fleetdm_fleet.test.id
}
`, name)
}
