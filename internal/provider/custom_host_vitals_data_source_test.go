package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCustomHostVitalsDataSource_basic(t *testing.T) {
	vitalName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCustomHostVitalsDataSourceConfig(vitalName),
				Check: resource.ComposeAggregateTestCheckFunc(
					// The rig is shared, so assert on the managed vital being
					// present rather than on the total count.
					resource.TestCheckTypeSetElemNestedAttrs(
						"data.fleetdm_custom_host_vitals.test",
						"custom_host_vitals.*",
						map[string]string{"name": vitalName},
					),
				),
			},
		},
	})
}

func testAccCustomHostVitalsDataSourceConfig(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_custom_host_vital" "test" {
  name = %[1]q
}

data "fleetdm_custom_host_vitals" "test" {
  depends_on = [fleetdm_custom_host_vital.test]
}
`, name)
}
