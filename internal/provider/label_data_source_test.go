package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccLabelDataSource_basic(t *testing.T) {
	labelName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccLabelDataSourceConfig(labelName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "name", labelName),
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "description", "Test label for data source"),
					resource.TestCheckResourceAttrSet("data.fleetdm_label.test", "id"),
					resource.TestCheckResourceAttrSet("data.fleetdm_label.test", "host_count"),
				),
			},
		},
	})
}

func testAccLabelDataSourceConfig(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name        = %[1]q
  description = "Test label for data source"
  query       = "SELECT 1"
}

data "fleetdm_label" "test" {
  id = fleetdm_label.test.id
}
`, name)
}
