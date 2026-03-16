package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPolicyResource_basic(t *testing.T) {
	policyName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: testAccPolicyResourceConfig(policyName, "Initial description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_policy.test", "name", policyName),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "description", "Initial description"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "query", "SELECT 1 WHERE 1=1;"),
					resource.TestCheckResourceAttrSet("fleetdm_policy.test", "id"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "critical", "false"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "passing_host_count", "0"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "failing_host_count", "0"),
				),
			},
			// ImportState
			{
				ResourceName:      "fleetdm_policy.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update description only
			{
				Config: testAccPolicyResourceConfig(policyName, "Updated description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_policy.test", "description", "Updated description"),
				),
			},
			// Update with critical, platform, and resolution
			{
				Config: testAccPolicyResourceConfigFull(policyName, "Updated description", true, "darwin", "Restart the service."),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_policy.test", "critical", "true"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "platform.#", "1"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "platform.0", "darwin"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "resolution", "Restart the service."),
				),
			},
			// Update platform and resolution to different values
			{
				Config: testAccPolicyResourceConfigFull(policyName, "Final description", false, "linux", "Check system logs."),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_policy.test", "description", "Final description"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "critical", "false"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "platform.#", "1"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "platform.0", "linux"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "resolution", "Check system logs."),
				),
			},
		},
	})
}

func TestAccPolicyResource_critical(t *testing.T) {
	policyName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPolicyResourceConfigCritical(policyName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_policy.test", "name", policyName),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "critical", "true"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "platform.#", "1"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "platform.0", "darwin"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "resolution", "Check disk encryption settings."),
				),
			},
		},
	})
}

func testAccPolicyResourceConfig(name, description string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_policy" "test" {
  name        = %[1]q
  description = %[2]q
  query       = "SELECT 1 WHERE 1=1;"
}
`, name, description)
}

func testAccPolicyResourceConfigFull(name, description string, critical bool, platform, resolution string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_policy" "test" {
  name        = %[1]q
  description = %[2]q
  query       = "SELECT 1 WHERE 1=1;"
  critical    = %[3]t
  platform    = [%[4]q]
  resolution  = %[5]q
}
`, name, description, critical, platform, resolution)
}

func testAccPolicyResourceConfigCritical(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_policy" "test" {
  name        = %[1]q
  description = "Critical policy"
  query       = "SELECT 1 FROM disk_encryption WHERE encrypted = 1;"
  critical    = true
  platform    = ["darwin"]
  resolution  = "Check disk encryption settings."
}
`, name)
}
