package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPolicyDataSource_basic(t *testing.T) {
	policyName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPolicyDataSourceConfig(policyName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_policy.test", "name", policyName),
					resource.TestCheckResourceAttr("data.fleetdm_policy.test", "description", "Test policy for data source"),
					resource.TestCheckResourceAttrSet("data.fleetdm_policy.test", "id"),
					resource.TestCheckResourceAttr("data.fleetdm_policy.test", "critical", "false"),
				),
			},
		},
	})
}

func testAccPolicyDataSourceConfig(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_policy" "test" {
  name        = %[1]q
  description = "Test policy for data source"
  query       = "SELECT 1 WHERE 1=1;"
}

data "fleetdm_policy" "test" {
  id = fleetdm_policy.test.id
}
`, name)
}
