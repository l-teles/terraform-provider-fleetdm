package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccUserDataSource_basic(t *testing.T) {
	userName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	userEmail := userName + "@example.com"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserDataSourceConfig(userName, userEmail),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_user.test", "name", userName),
					resource.TestCheckResourceAttr("data.fleetdm_user.test", "email", userEmail),
					resource.TestCheckResourceAttr("data.fleetdm_user.test", "global_role", "observer"),
					resource.TestCheckResourceAttrSet("data.fleetdm_user.test", "id"),
				),
			},
		},
	})
}

func testAccUserDataSourceConfig(name, email string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_user" "test" {
  name        = %[1]q
  email       = %[2]q
  password    = "FleetTest@12345!"
  global_role = "observer"
}

data "fleetdm_user" "test" {
  id = fleetdm_user.test.id
}
`, name, email)
}
