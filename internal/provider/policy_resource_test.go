package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
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

// TestAccPolicyResource_outOfBandDeletion verifies the provider handles
// out-of-band deletion by recreating the resource on the next apply, rather
// than failing with a 404 during Read. Exercises the isNotFound path in
// resource_helpers.go end-to-end against a real Fleet instance.
func TestAccPolicyResource_outOfBandDeletion(t *testing.T) {
	policyName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPolicyResourceConfig(policyName, "Initial description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_policy.test", "name", policyName),
					resource.TestCheckResourceAttrSet("fleetdm_policy.test", "id"),
				),
			},
			{
				// Delete the policy directly via the Fleet API, then re-apply
				// the same config. Terraform should notice the 404 in Read,
				// drop the resource from state, and recreate it.
				PreConfig: func() { deletePolicyOutOfBand(t, policyName) },
				Config:    testAccPolicyResourceConfig(policyName, "Initial description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_policy.test", "name", policyName),
					resource.TestCheckResourceAttrSet("fleetdm_policy.test", "id"),
				),
			},
		},
	})
}

// deletePolicyOutOfBand simulates an external deletion by looking up the
// policy by name and calling DeletePolicy directly. Fails the test if the
// policy can't be found — that would mean the previous step didn't actually
// create it, which invalidates the test.
func deletePolicyOutOfBand(t *testing.T, name string) {
	t.Helper()
	// Match provider.go's parsing: both "false" and "0" disable verification.
	verifyTLS := true
	if v := os.Getenv("FLEETDM_VERIFY_TLS"); v == "false" || v == "0" {
		verifyTLS = false
	}
	client, err := fleetdm.NewClient(fleetdm.ClientConfig{
		ServerAddress: os.Getenv("FLEETDM_URL"),
		APIKey:        os.Getenv("FLEETDM_API_TOKEN"),
		VerifyTLS:     verifyTLS,
	})
	if err != nil {
		t.Fatalf("failed to build fleet client: %v", err)
	}
	policies, err := client.ListGlobalPolicies(context.Background())
	if err != nil {
		t.Fatalf("failed to list policies: %v", err)
	}
	for _, p := range policies {
		if p.Name == name {
			if err := client.DeletePolicy(context.Background(), p.ID, nil); err != nil {
				t.Fatalf("failed to delete policy %d out-of-band: %v", p.ID, err)
			}
			return
		}
	}
	t.Fatalf("policy %q not found — cannot simulate out-of-band deletion", name)
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
