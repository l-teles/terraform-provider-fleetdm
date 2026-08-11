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
					// An unscoped global policy reports no label selectors, and
					// Fleet only supports continuous automations on team
					// policies — it echoes false here.
					resource.TestCheckNoResourceAttr("data.fleetdm_policy.test", "labels_include_all.#"),
					resource.TestCheckNoResourceAttr("data.fleetdm_policy.test", "labels_exclude_all.#"),
					resource.TestCheckResourceAttr("data.fleetdm_policy.test", "continuous_automations_enabled", "false"),
				),
			},
		},
	})
}

// TestAccPolicyDataSource_labelScopingAndContinuousAutomations verifies the
// Fleet 4.90 fields surface through the data source. Uses a team policy
// because continuous_automations_enabled is team-only, and keeps the include
// and exclude labels disjoint because Fleet rejects a label named on both
// sides.
func TestAccPolicyDataSource_labelScopingAndContinuousAutomations(t *testing.T) {
	policyName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	teamName := "tf-acc-team-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	labelA := "tf-acc-label-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	labelB := "tf-acc-label-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPolicyDataSourceConfigLabelScoping(policyName, teamName, labelA, labelB),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_policy.test", "labels_include_all.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_policy.test", "labels_include_all.0", labelA),
					resource.TestCheckResourceAttr("data.fleetdm_policy.test", "labels_exclude_any.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_policy.test", "labels_exclude_any.0", labelB),
					resource.TestCheckNoResourceAttr("data.fleetdm_policy.test", "labels_include_any.#"),
					resource.TestCheckNoResourceAttr("data.fleetdm_policy.test", "labels_exclude_all.#"),
					resource.TestCheckResourceAttr("data.fleetdm_policy.test", "continuous_automations_enabled", "true"),
				),
			},
		},
	})
}

func testAccPolicyDataSourceConfigLabelScoping(policyName, teamName, labelA, labelB string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name        = %[2]q
  description = "team for policy data source label scoping test"
}

resource "fleetdm_label" "a" {
  name  = %[3]q
  query = "SELECT 1 FROM os_version WHERE platform = 'darwin';"
}

resource "fleetdm_label" "b" {
  name  = %[4]q
  query = "SELECT 1 FROM os_version WHERE platform = 'linux';"
}

resource "fleetdm_policy" "test" {
  name                           = %[1]q
  query                          = "SELECT 1 WHERE 1=1;"
  team_id                        = fleetdm_fleet.test.id
  labels_include_all             = [fleetdm_label.a.name]
  labels_exclude_any             = [fleetdm_label.b.name]
  continuous_automations_enabled = true
}

data "fleetdm_policy" "test" {
  id      = fleetdm_policy.test.id
  team_id = fleetdm_fleet.test.id
}
`, policyName, teamName, labelA, labelB)
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
