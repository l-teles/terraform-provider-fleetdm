package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccLabelResource_basic(t *testing.T) {
	labelName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: testAccLabelResourceConfig(labelName, "Initial label description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_label.test", "name", labelName),
					resource.TestCheckResourceAttr("fleetdm_label.test", "description", "Initial label description"),
					resource.TestCheckResourceAttr("fleetdm_label.test", "query", "SELECT 1 FROM os_version WHERE platform = 'darwin'"),
					resource.TestCheckResourceAttrSet("fleetdm_label.test", "id"),
					resource.TestCheckResourceAttrSet("fleetdm_label.test", "host_count"),
				),
			},
			// ImportState
			{
				ResourceName:      "fleetdm_label.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update description (query is ForceNew so we don't change it)
			{
				Config: testAccLabelResourceConfig(labelName, "Updated label description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_label.test", "description", "Updated label description"),
				),
			},
		},
	})
}

func TestAccLabelResource_withPlatform(t *testing.T) {
	labelName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccLabelResourceConfigWithPlatform(labelName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_label.test", "name", labelName),
					resource.TestCheckResourceAttr("fleetdm_label.test", "platform", "darwin"),
					resource.TestCheckResourceAttrSet("fleetdm_label.test", "id"),
				),
			},
		},
	})
}

func testAccLabelResourceConfig(name, description string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name        = %[1]q
  description = %[2]q
  query       = "SELECT 1 FROM os_version WHERE platform = 'darwin'"
}
`, name, description)
}

func testAccLabelResourceConfigWithPlatform(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name        = %[1]q
  description = "Label scoped to darwin"
  query       = "SELECT 1 FROM os_version WHERE platform = 'darwin'"
  platform    = "darwin"
}
`, name)
}
