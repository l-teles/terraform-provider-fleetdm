package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
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
			// Update description (query is ForceNew so we don't change it).
			//
			// The plan check is the regression guard: `platform` is
			// Optional+Computed and, without UseStateForUnknown, planned as
			// unknown whenever the config omitted it — which never equals the
			// stored value, so its RequiresReplace turned every edit into a
			// destroy/create. That churned the label id and dropped manual
			// membership while still satisfying attribute-only assertions.
			{
				Config: testAccLabelResourceConfig(labelName, "Updated label description"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_label.test", plancheck.ResourceActionUpdate),
					},
				},
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
				Config: testAccLabelResourceConfigWithPlatform(labelName, "darwin"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_label.test", "name", labelName),
					resource.TestCheckResourceAttr("fleetdm_label.test", "platform", "darwin"),
					resource.TestCheckResourceAttrSet("fleetdm_label.test", "id"),
				),
			},
			// A platform the user actually changed must still replace: Fleet's
			// modify-label endpoint cannot apply it. This is the other half of
			// the UseStateForUnknown fix — suppressing the spurious replace
			// must not suppress the legitimate one.
			{
				Config: testAccLabelResourceConfigWithPlatform(labelName, "windows"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_label.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_label.test", "platform", "windows"),
				),
			},
		},
	})
}

// TestAccLabelResource_criteria covers a host-vitals label driven by an IdP
// vital: the membership type flips to host_vitals and `query` stays absent.
func TestAccLabelResource_criteria(t *testing.T) {
	labelName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccLabelResourceConfigWithCriteria(labelName, "Engineering"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_label.test", "name", labelName),
					resource.TestCheckResourceAttr("fleetdm_label.test", "criteria.vital", "end_user_idp_group"),
					resource.TestCheckResourceAttr("fleetdm_label.test", "criteria.value", "Engineering"),
					resource.TestCheckResourceAttr("fleetdm_label.test", "label_membership_type", "host_vitals"),
					// `query` is Optional and not Computed, so a criteria label
					// must leave it null rather than storing Fleet's "".
					resource.TestCheckNoResourceAttr("fleetdm_label.test", "query"),
					// Omitted operator round-trips as null, since Fleet leaves
					// it out of the echo.
					resource.TestCheckNoResourceAttr("fleetdm_label.test", "criteria.operator"),
				),
			},
			{
				ResourceName:      "fleetdm_label.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Rename with criteria untouched: this must be an in-place update
			// that carries the criteria through. The PATCH response is what
			// repopulates criteria in state, so a regression there would abort
			// the apply with "inconsistent result after apply".
			{
				Config: testAccLabelResourceConfigWithCriteria(labelName+"-renamed", "Engineering"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_label.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_label.test", "name", labelName+"-renamed"),
					resource.TestCheckResourceAttr("fleetdm_label.test", "criteria.vital", "end_user_idp_group"),
					resource.TestCheckResourceAttr("fleetdm_label.test", "criteria.value", "Engineering"),
					resource.TestCheckResourceAttr("fleetdm_label.test", "label_membership_type", "host_vitals"),
				),
			},
			// Criteria is immutable in Fleet — its modify-label endpoint
			// ignores the field and answers 200 with the original criteria — so
			// a changed value must replace the label rather than be applied in
			// place and silently drift.
			{
				Config: testAccLabelResourceConfigWithCriteria(labelName+"-renamed", "Security"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_label.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_label.test", "criteria.value", "Security"),
					resource.TestCheckResourceAttr("fleetdm_label.test", "label_membership_type", "host_vitals"),
				),
			},
		},
	})
}

// TestAccLabelResource_criteriaValidation pins the plan-time rules so Fleet's
// apply-time 422s are caught earlier.
func TestAccLabelResource_criteriaValidation(t *testing.T) {
	labelName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccLabelResourceConfigCriteriaAndQuery(labelName),
				ExpectError: regexp.MustCompile(`(?s)Invalid Attribute Combination|cannot be specified when`),
			},
			{
				Config:      testAccLabelResourceConfigCriteriaMissingVitalID(labelName),
				ExpectError: regexp.MustCompile(`Missing Custom Host Vital ID`),
			},
			{
				Config:      testAccLabelResourceConfigCriteriaUnexpectedVitalID(labelName),
				ExpectError: regexp.MustCompile(`Unexpected Custom Host Vital ID`),
			},
			{
				Config:      testAccLabelResourceConfigCriteriaBadVital(labelName),
				ExpectError: regexp.MustCompile(`(?s)Invalid Attribute Value Match|value must be one of`),
			},
			// Fleet rejects a platform on a host vitals label with a 422;
			// catch it at plan time instead.
			{
				Config:      testAccLabelResourceConfigCriteriaAndPlatform(labelName),
				ExpectError: regexp.MustCompile(`(?s)Invalid Attribute Combination|cannot be specified when`),
			},
		},
	})
}

// TestAccLabelResource_criteriaUnknownAtPlan covers a criteria block whose
// value isn't known until apply.
//
// ValidateConfig has to read `criteria` as an opaque object before decoding it:
// reading it straight into the model raises a "Value Conversion Error" when the
// whole block is unknown, which would break plan for any config that builds
// criteria conditionally. The conditional below is unknown at plan time because
// it keys off another resource's computed id.
func TestAccLabelResource_criteriaUnknownAtPlan(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "fleetdm_custom_host_vital" "dep" {
  name = "tf-acc-test-vital-%[1]s"
}

resource "fleetdm_label" "test" {
  name = "tf-acc-test-label-%[1]s"

  criteria = fleetdm_custom_host_vital.dep.id > 0 ? {
    vital                = "custom_host_vital"
    operator             = "="
    value                = "1234"
    custom_host_vital_id = fleetdm_custom_host_vital.dep.id
  } : null
}
`, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_label.test", "criteria.vital", "custom_host_vital"),
					resource.TestCheckResourceAttr("fleetdm_label.test", "label_membership_type", "host_vitals"),
					resource.TestCheckResourceAttrPair(
						"fleetdm_label.test", "criteria.custom_host_vital_id",
						"fleetdm_custom_host_vital.dep", "id",
					),
				),
			},
		},
	})
}

// TestAccLabelResource_manual confirms a label with neither query nor criteria
// stays valid — `query` used to be Required, and Fleet accepts all three empty
// as a manual label.
func TestAccLabelResource_manual(t *testing.T) {
	labelName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name        = %[1]q
  description = "Manually managed membership"
}
`, labelName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_label.test", "label_membership_type", "manual"),
					resource.TestCheckNoResourceAttr("fleetdm_label.test", "query"),
					resource.TestCheckNoResourceAttr("fleetdm_label.test", "criteria"),
				),
			},
		},
	})
}

func testAccLabelResourceConfigWithCriteria(name, value string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name        = %[1]q
  description = "Hosts in an IdP group"

  criteria = {
    vital = "end_user_idp_group"
    value = %[2]q
  }
}
`, name, value)
}

func testAccLabelResourceConfigCriteriaAndQuery(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name  = %[1]q
  query = "SELECT 1"

  criteria = {
    vital = "end_user_idp_group"
    value = "Engineering"
  }
}
`, name)
}

func testAccLabelResourceConfigCriteriaMissingVitalID(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name = %[1]q

  criteria = {
    vital = "custom_host_vital"
    value = "1234"
  }
}
`, name)
}

func testAccLabelResourceConfigCriteriaUnexpectedVitalID(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name = %[1]q

  criteria = {
    vital                = "end_user_idp_group"
    value                = "Engineering"
    custom_host_vital_id = 1
  }
}
`, name)
}

func testAccLabelResourceConfigCriteriaAndPlatform(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name     = %[1]q
  platform = "darwin"

  criteria = {
    vital = "end_user_idp_group"
    value = "Engineering"
  }
}
`, name)
}

func testAccLabelResourceConfigCriteriaBadVital(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name = %[1]q

  criteria = {
    vital = "not_a_vital"
    value = "x"
  }
}
`, name)
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

func testAccLabelResourceConfigWithPlatform(name, platform string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name        = %[1]q
  description = "Label scoped to a platform"
  query       = "SELECT 1 FROM os_version WHERE platform = 'darwin'"
  platform    = %[2]q
}
`, name, platform)
}
