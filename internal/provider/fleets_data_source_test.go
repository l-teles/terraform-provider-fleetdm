package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccFleetsDataSource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: testAccFleetsDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Just verify the data source can be read without error
					// The actual number of fleets will vary
					resource.TestCheckResourceAttrSet("data.fleetdm_fleets.all", "fleets.#"),
				),
			},
		},
	})
}

func testAccFleetsDataSourceConfig() string {
	return providerConfig() + `
data "fleetdm_fleets" "all" {}
`
}
