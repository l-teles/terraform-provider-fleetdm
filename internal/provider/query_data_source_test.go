package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccQueryDataSource_basic(t *testing.T) {
	queryName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccQueryDataSourceConfig(queryName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_query.test", "name", queryName),
					resource.TestCheckResourceAttr("data.fleetdm_query.test", "description", "Test query for data source"),
					resource.TestCheckResourceAttrSet("data.fleetdm_query.test", "id"),
				),
			},
		},
	})
}

func testAccQueryDataSourceConfig(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_query" "test" {
  name        = %[1]q
  description = "Test query for data source"
  query       = "SELECT * FROM system_info;"
}

data "fleetdm_query" "test" {
  id = fleetdm_query.test.id
}
`, name)
}
