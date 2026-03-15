package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccScriptDataSource_basic(t *testing.T) {
	scriptName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum) + ".sh"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccScriptDataSourceConfig(scriptName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_script.test", "name", scriptName),
					resource.TestCheckResourceAttrSet("data.fleetdm_script.test", "id"),
					resource.TestCheckResourceAttrSet("data.fleetdm_script.test", "created_at"),
				),
			},
		},
	})
}

func testAccScriptDataSourceConfig(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_script" "test" {
  name    = %[1]q
  content = "#!/bin/bash\necho 'hello'"
}

data "fleetdm_script" "test" {
  id = fleetdm_script.test.id
}
`, name)
}
