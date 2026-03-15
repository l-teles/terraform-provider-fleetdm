package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccUserResource_basic(t *testing.T) {
	userName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	userEmail := userName + "@example.com"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: testAccUserResourceConfig(userName, userEmail, "observer"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_user.test", "name", userName),
					resource.TestCheckResourceAttr("fleetdm_user.test", "email", userEmail),
					resource.TestCheckResourceAttr("fleetdm_user.test", "global_role", "observer"),
					resource.TestCheckResourceAttrSet("fleetdm_user.test", "id"),
					resource.TestCheckResourceAttr("fleetdm_user.test", "api_only", "false"),
					resource.TestCheckResourceAttr("fleetdm_user.test", "sso_enabled", "false"),
				),
			},
			// Update global role
			{
				Config: testAccUserResourceConfig(userName, userEmail, "maintainer"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_user.test", "global_role", "maintainer"),
				),
			},
			// Update name
			{
				Config: testAccUserResourceConfig(userName+"-updated", userEmail, "maintainer"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_user.test", "name", userName+"-updated"),
					resource.TestCheckResourceAttr("fleetdm_user.test", "global_role", "maintainer"),
				),
			},
		},
	})
}

func TestAccUserResource_apiOnly(t *testing.T) {
	userName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	userEmail := userName + "@example.com"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserResourceConfigAPIOnly(userName, userEmail),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_user.test", "name", userName),
					resource.TestCheckResourceAttr("fleetdm_user.test", "email", userEmail),
					resource.TestCheckResourceAttr("fleetdm_user.test", "api_only", "true"),
					resource.TestCheckResourceAttr("fleetdm_user.test", "global_role", "observer"),
				),
			},
		},
	})
}

func testAccUserResourceConfig(name, email, role string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_user" "test" {
  name        = %[1]q
  email       = %[2]q
  password    = "FleetTest@12345!"
  global_role = %[3]q
}
`, name, email, role)
}

func testAccUserResourceConfigAPIOnly(name, email string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_user" "test" {
  name        = %[1]q
  email       = %[2]q
  password    = "FleetTest@12345!"
  global_role = "observer"
  api_only    = true
}
`, name, email)
}
