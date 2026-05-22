package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

func TestAccReportResource_basic(t *testing.T) {
	reportName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: testAccReportResourceConfig(reportName, "SELECT * FROM system_info;", "Initial description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_report.test", "name", reportName),
					resource.TestCheckResourceAttr("fleetdm_report.test", "description", "Initial description"),
					resource.TestCheckResourceAttr("fleetdm_report.test", "query", "SELECT * FROM system_info;"),
					resource.TestCheckResourceAttrSet("fleetdm_report.test", "id"),
					resource.TestCheckResourceAttr("fleetdm_report.test", "observer_can_run", "false"),
					resource.TestCheckResourceAttr("fleetdm_report.test", "automations_enabled", "false"),
				),
			},
			// ImportState
			{
				ResourceName:      "fleetdm_report.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update query and description
			{
				Config: testAccReportResourceConfig(reportName, "SELECT * FROM os_version;", "Updated description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_report.test", "description", "Updated description"),
					resource.TestCheckResourceAttr("fleetdm_report.test", "query", "SELECT * FROM os_version;"),
				),
			},
			// Update with observer_can_run, logging, and platform
			{
				Config: testAccReportResourceConfigFull(reportName, "SELECT * FROM os_version;", "Updated description", true, "snapshot", "darwin"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_report.test", "observer_can_run", "true"),
					resource.TestCheckResourceAttr("fleetdm_report.test", "logging", "snapshot"),
					resource.TestCheckResourceAttr("fleetdm_report.test", "platform.#", "1"),
					resource.TestCheckResourceAttr("fleetdm_report.test", "platform.0", "darwin"),
				),
			},
			// Update platform and logging to different values
			{
				Config: testAccReportResourceConfigFull(reportName, "SELECT * FROM os_version;", "Final description", true, "differential", "linux"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_report.test", "description", "Final description"),
					resource.TestCheckResourceAttr("fleetdm_report.test", "logging", "differential"),
					resource.TestCheckResourceAttr("fleetdm_report.test", "platform.#", "1"),
					resource.TestCheckResourceAttr("fleetdm_report.test", "platform.0", "linux"),
				),
			},
		},
	})
}

// TestAccReportResource_platformClearForcesReplace exercises the
// requireReplaceOnPlatformShrink plan modifier. Fleet's PATCH /reports/{id}
// endpoint silently drops empty `platform` (it gets stripped by `omitempty`),
// so the provider must turn a non-empty -> empty change into a destroy+recreate
// rather than letting Terraform produce an inconsistent post-apply state error.
// Subset shrinks and swaps stay in-place because Fleet honors any non-empty
// platform value sent.
func TestAccReportResource_platformClearForcesReplace(t *testing.T) {
	reportName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: create with platform = ["windows", "darwin"].
			{
				Config: testAccReportResourceConfigMultiPlatform(reportName, `["windows", "darwin"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_report.test", "platform.#", "2"),
				),
			},
			// Step 2: grow the list — in-place update.
			{
				Config: testAccReportResourceConfigMultiPlatform(reportName, `["windows", "darwin", "linux"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_report.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_report.test", "platform.#", "3"),
				),
			},
			// Step 3: subset shrink — also in-place, because a non-empty
			// platform string still goes on the wire and Fleet overwrites
			// the stored value.
			{
				Config: testAccReportResourceConfigMultiPlatform(reportName, `["darwin"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_report.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_report.test", "platform.#", "1"),
					resource.TestCheckResourceAttr("fleetdm_report.test", "platform.0", "darwin"),
				),
			},
			// Step 4: clear the list entirely — MUST replace, because the
			// PATCH body would otherwise drop platform via omitempty and
			// Fleet would silently keep the previous value.
			{
				Config: testAccReportResourceConfigMultiPlatform(reportName, `[]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_report.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_report.test", "platform.#", "0"),
				),
			},
		},
	})
}

func testAccReportResourceConfigMultiPlatform(name, platformsHCL string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_report" "test" {
  name     = %[1]q
  query    = "SELECT 1;"
  platform = %[2]s
}
`, name, platformsHCL)
}

func TestAccReportResource_withOptions(t *testing.T) {
	reportName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccReportResourceConfigWithOptions(reportName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_report.test", "name", reportName),
					resource.TestCheckResourceAttr("fleetdm_report.test", "observer_can_run", "true"),
					resource.TestCheckResourceAttr("fleetdm_report.test", "logging", "snapshot"),
					resource.TestCheckResourceAttr("fleetdm_report.test", "platform.#", "1"),
					resource.TestCheckResourceAttr("fleetdm_report.test", "platform.0", "darwin"),
				),
			},
		},
	})
}

func testAccReportResourceConfig(name, query, description string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_report" "test" {
  name        = %[1]q
  description = %[3]q
  query       = %[2]q
}
`, name, query, description)
}

func testAccReportResourceConfigFull(name, query, description string, observerCanRun bool, logging, platform string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_report" "test" {
  name             = %[1]q
  description      = %[3]q
  query            = %[2]q
  observer_can_run = %[4]t
  logging          = %[5]q
  platform         = [%[6]q]
}
`, name, query, description, observerCanRun, logging, platform)
}

func testAccReportResourceConfigWithOptions(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_report" "test" {
  name             = %[1]q
  description      = "Report with options"
  query            = "SELECT * FROM system_info;"
  platform         = ["darwin"]
  observer_can_run = true
  logging          = "snapshot"
}
`, name)
}

func TestAccReportResource_moveStateFromQuery(t *testing.T) {
	reportName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	fleetName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	sqlQuery := "SELECT * FROM system_info;"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		Steps: []resource.TestStep{
			// Step 1: Create a fleet and a fleet-scoped fleetdm_query (with team_id set).
			// This ensures the team_id → fleet_id field mapping is actually exercised.
			{
				Config: testAccQueryResourceConfigScoped(fleetName, reportName, sqlQuery),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_query.test", "name", reportName),
					resource.TestCheckResourceAttrSet("fleetdm_query.test", "id"),
					resource.TestCheckResourceAttrSet("fleetdm_query.test", "team_id"),
					resource.TestCheckResourceAttrPair("fleetdm_query.test", "team_id", "fleetdm_fleet.scoped", "id"),
				),
			},
			// Step 2: Move state to fleetdm_report via a moved block. Verify no destroy/
			// recreate (plan is a no-op), and that team_id was correctly mapped to fleet_id.
			{
				Config: testAccReportResourceConfigWithMovedFromQuery(fleetName, reportName, sqlQuery),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_report.test", "name", reportName),
					resource.TestCheckResourceAttr("fleetdm_report.test", "query", sqlQuery),
					resource.TestCheckResourceAttrSet("fleetdm_report.test", "id"),
					resource.TestCheckResourceAttrSet("fleetdm_report.test", "fleet_id"),
					resource.TestCheckResourceAttrPair("fleetdm_report.test", "fleet_id", "fleetdm_fleet.scoped", "id"),
				),
			},
		},
	})
}

func testAccQueryResourceConfigScoped(fleetName, queryName, sqlQuery string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "scoped" {
  name = %[1]q
}

resource "fleetdm_query" "test" {
  name    = %[2]q
  query   = %[3]q
  team_id = fleetdm_fleet.scoped.id
}
`, fleetName, queryName, sqlQuery)
}

func testAccReportResourceConfigWithMovedFromQuery(fleetName, reportName, sqlQuery string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "scoped" {
  name = %[1]q
}

moved {
  from = fleetdm_query.test
  to   = fleetdm_report.test
}

resource "fleetdm_report" "test" {
  name     = %[2]q
  query    = %[3]q
  fleet_id = fleetdm_fleet.scoped.id
}
`, fleetName, reportName, sqlQuery)
}
