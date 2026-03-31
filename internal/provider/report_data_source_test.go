package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccReportDataSource_basic(t *testing.T) {
	reportName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccReportDataSourceConfig(reportName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_report.test", "name", reportName),
					resource.TestCheckResourceAttr("data.fleetdm_report.test", "description", "Test report for data source"),
					resource.TestCheckResourceAttrSet("data.fleetdm_report.test", "id"),
				),
			},
		},
	})
}

func testAccReportDataSourceConfig(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_report" "test" {
  name        = %[1]q
  description = "Test report for data source"
  query       = "SELECT * FROM system_info;"
}

data "fleetdm_report" "test" {
  id = fleetdm_report.test.id
}
`, name)
}
