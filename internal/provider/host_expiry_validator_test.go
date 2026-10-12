package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccHostExpiryValidator checks the plan-time host expiry rule on both
// resources that carry it. Every step fails or stops at plan, so no Fleet
// server is contacted.
func TestAccHostExpiryValidator(t *testing.T) {
	name := "tf-acc-test-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	invalid := regexp.MustCompile(`(?s)host_expiry_window\s+must\s+be\s+at\s+least\s+1`)

	for _, tc := range []struct {
		resourceType string
		extra        string
	}{
		{"fleetdm_configuration", `org_name = "tf-acc"`},
		{"fleetdm_fleet", fmt.Sprintf("name = %q", name)},
	} {
		t.Run(tc.resourceType, func(t *testing.T) {
			cfg := func(expiry string) string {
				return providerConfig() + fmt.Sprintf(`
resource %q "test" {
  %s
  %s
}
`, tc.resourceType, tc.extra, expiry)
			}
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: cfg("host_expiry_enabled = true"), ExpectError: invalid},
					{Config: cfg("host_expiry_enabled = true\n  host_expiry_window = 0"), ExpectError: invalid},
					{Config: cfg("host_expiry_enabled = true\n  host_expiry_window = 30"), PlanOnly: true, ExpectNonEmptyPlan: true},
					{Config: cfg("host_expiry_enabled = false\n  host_expiry_window = 0"), PlanOnly: true, ExpectNonEmptyPlan: true},
				},
			})
		})
	}
}
