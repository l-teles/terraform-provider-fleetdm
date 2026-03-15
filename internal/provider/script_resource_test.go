package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccScriptResource_basic(t *testing.T) {
	// Fleet appends .sh to the name automatically.
	scriptName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum) + ".sh"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: testAccScriptResourceConfig(scriptName, "#!/bin/bash\necho 'hello'"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_script.test", "name", scriptName),
					resource.TestCheckResourceAttrSet("fleetdm_script.test", "id"),
					resource.TestCheckResourceAttrSet("fleetdm_script.test", "created_at"),
				),
			},
			// ImportState (content is not returned by API so we ignore it)
			{
				ResourceName:            "fleetdm_script.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"content"},
			},
			// Update content (triggers replacement since name is ForceNew)
			{
				Config: testAccScriptResourceConfig(scriptName, "#!/bin/bash\necho 'updated'"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_script.test", "name", scriptName),
					resource.TestCheckResourceAttrSet("fleetdm_script.test", "id"),
				),
			},
		},
	})
}

func TestAccScriptResource_withTeam(t *testing.T) {
	scriptName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum) + ".sh"
	teamName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccScriptResourceConfigWithTeam(scriptName, teamName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_script.test", "name", scriptName),
					resource.TestCheckResourceAttrSet("fleetdm_script.test", "team_id"),
					resource.TestCheckResourceAttrSet("fleetdm_script.test", "id"),
				),
			},
		},
	})
}

func testAccScriptResourceConfig(name, content string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_script" "test" {
  name    = %[1]q
  content = %[2]q
}
`, name, content)
}

func testAccScriptResourceConfigWithTeam(scriptName, teamName string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_team" "test" {
  name        = %[2]q
  description = "Team for script test"
}

resource "fleetdm_script" "test" {
  name    = %[1]q
  team_id = fleetdm_team.test.id
  content = "#!/bin/bash\necho 'team script'"
}
`, scriptName, teamName)
}
