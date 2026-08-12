package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func TestAccCustomHostVitalResource_basic(t *testing.T) {
	vitalName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	renamed := vitalName + "-renamed"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: testAccCustomHostVitalResourceConfig(vitalName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_custom_host_vital.test", "name", vitalName),
					resource.TestCheckResourceAttrSet("fleetdm_custom_host_vital.test", "id"),
					// Fleet's create response omits timestamps; the resource
					// reads them back so state is complete after apply.
					resource.TestCheckResourceAttrSet("fleetdm_custom_host_vital.test", "created_at"),
					resource.TestCheckResourceAttrSet("fleetdm_custom_host_vital.test", "updated_at"),
				),
			},
			// ImportState — nothing about a vital is write-only, so a full
			// verify must pass.
			{
				ResourceName:      "fleetdm_custom_host_vital.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Rename in place: Fleet's PATCH endpoint updates the name and the
			// id is stable, so this must not force a replacement.
			{
				Config: testAccCustomHostVitalResourceConfig(renamed),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_custom_host_vital.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_custom_host_vital.test", "name", renamed),
				),
			},
		},
	})
}

// TestAccCustomHostVitalResource_labelCriteria covers the reason the resource
// exists: driving a host-vitals label's membership from a custom vital.
func TestAccCustomHostVitalResource_labelCriteria(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCustomHostVitalResourceConfigWithLabel(suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fleetdm_custom_host_vital.test", "id"),
					resource.TestCheckResourceAttr("fleetdm_label.test", "criteria.vital", "custom_host_vital"),
					resource.TestCheckResourceAttr("fleetdm_label.test", "criteria.value", "1234"),
					resource.TestCheckResourceAttr("fleetdm_label.test", "criteria.operator", "="),
					resource.TestCheckResourceAttr("fleetdm_label.test", "label_membership_type", "host_vitals"),
					// The label's criteria must point at the managed vital.
					resource.TestCheckResourceAttrPair(
						"fleetdm_label.test", "criteria.custom_host_vital_id",
						"fleetdm_custom_host_vital.test", "id",
					),
				),
			},
		},
	})
}

// TestAccCustomHostVitalResource_nameValidation checks the plan-time name rules
// mirror Fleet's, so a bad name fails before an apply is attempted. The
// trailing character here is U+00A0 (no-break space): Fleet's TrimSpace-based
// check rejects it, while an ASCII-only `\S` regex would let it through.
func TestAccCustomHostVitalResource_nameValidation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccCustomHostVitalResourceConfig("tf-acc-trailing-space "),
				ExpectError: regexp.MustCompile(`leading or trailing whitespace`),
			},
			{
				Config:      testAccCustomHostVitalResourceConfig("tf-acc-nbsp "),
				ExpectError: regexp.MustCompile(`leading or trailing whitespace`),
			},
			{
				Config:      testAccCustomHostVitalResourceConfig(""),
				ExpectError: regexp.MustCompile(`(?s)string length must be between|Invalid Attribute Value`),
			},
		},
	})
}

func testAccCustomHostVitalResourceConfig(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_custom_host_vital" "test" {
  name = %[1]q
}
`, name)
}

func testAccCustomHostVitalResourceConfigWithLabel(suffix string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_custom_host_vital" "test" {
  name = "tf-acc-test-vital-%[1]s"
}

resource "fleetdm_label" "test" {
  name        = "tf-acc-test-label-%[1]s"
  description = "Hosts whose asset tag matches"

  criteria = {
    vital                = "custom_host_vital"
    operator             = "="
    value                = "1234"
    custom_host_vital_id = fleetdm_custom_host_vital.test.id
  }
}
`, suffix)
}
