package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTeamsDataSource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: testAccTeamsDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Just verify the data source can be read without error
					// The actual number of teams will vary
					resource.TestCheckResourceAttrSet("data.fleetdm_teams.all", "teams.#"),
				),
			},
		},
	})
}

func testAccTeamsDataSourceConfig() string {
	return providerConfig() + `
data "fleetdm_teams" "all" {}
`
}
