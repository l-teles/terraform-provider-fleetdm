package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
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

// TestAccPolicyResource_labelsMutualExclusion verifies the ValidateConfig
// guard that rejects setting both labels_include_any and labels_exclude_any.
func TestAccPolicyResource_labelsMutualExclusion(t *testing.T) {
	policyName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "fleetdm_policy" "test" {
  name               = %[1]q
  query              = "SELECT 1;"
  labels_include_any = ["a"]
  labels_exclude_any = ["b"]
}
`, policyName),
				ExpectError: regexp.MustCompile("Conflicting label selectors"),
			},
		},
	})
}

// TestAccPolicyResource_labels verifies labels_include_any can be set on
// create, mutated on update, and cleared by setting to null.
func TestAccPolicyResource_labels(t *testing.T) {
	policyName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	labelA := "tf-acc-label-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	labelB := "tf-acc-label-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPolicyResourceConfigLabels(policyName, labelA, labelB, []string{labelA}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_policy.test", "labels_include_any.#", "1"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "labels_include_any.0", labelA),
				),
			},
			{
				Config: testAccPolicyResourceConfigLabels(policyName, labelA, labelB, []string{labelA, labelB}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_policy.test", "labels_include_any.#", "2"),
				),
			},
			{
				Config: testAccPolicyResourceConfigLabels(policyName, labelA, labelB, nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("fleetdm_policy.test", "labels_include_any.#"),
				),
			},
		},
	})
}

// TestAccPolicyResource_teamAutomationScriptID verifies that script_id can
// be attached to a team policy and cleared by setting it to null in HCL.
// This exercises the no-omitempty serialization on UpdatePolicyRequest —
// without it, "set then null" would silently leave the script attached.
func TestAccPolicyResource_teamAutomationScriptID(t *testing.T) {
	policyName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	teamName := "tf-acc-team-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	scriptName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum) + ".sh"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPolicyResourceConfigTeamScript(policyName, teamName, scriptName, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fleetdm_policy.test", "script_id"),
					resource.TestCheckResourceAttrSet("fleetdm_policy.test", "run_script.id"),
					resource.TestCheckResourceAttrPair("fleetdm_policy.test", "script_id", "fleetdm_script.test", "id"),
				),
			},
			{
				Config: testAccPolicyResourceConfigTeamScript(policyName, teamName, scriptName, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("fleetdm_policy.test", "script_id"),
				),
			},
		},
	})
}

// TestAccPolicyResource_calendarAndCA verifies the post-create follow-up
// PATCH path for calendar_events_enabled / conditional_access_enabled.
// These fields are not accepted by Fleet's Create endpoint, so the
// resource's Create method does Create-then-Update on team policies when
// they're set in the plan.
func TestAccPolicyResource_calendarAndCA(t *testing.T) {
	policyName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	teamName := "tf-acc-team-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPolicyResourceConfigCalendarCA(policyName, teamName, true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_policy.test", "calendar_events_enabled", "true"),
					resource.TestCheckResourceAttr("fleetdm_policy.test", "conditional_access_enabled", "false"),
				),
			},
			{
				Config: testAccPolicyResourceConfigCalendarCA(policyName, teamName, false, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_policy.test", "calendar_events_enabled", "false"),
				),
			},
		},
	})
}

// TestAccPolicyResource_outOfBandAutomationDrift is the upgrade-behavior
// guard. It mirrors the existing _outOfBandDeletion pattern: applies a
// minimal config, then mutates state via a direct API call (simulating a
// Fleet UI change), and verifies that the next plan correctly surfaces the
// drift rather than silently inheriting it. Re-applying the original config
// should restore Terraform's view.
func TestAccPolicyResource_outOfBandAutomationDrift(t *testing.T) {
	policyName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	teamName := "tf-acc-team-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	scriptName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum) + ".sh"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPolicyResourceConfigTeamNoAutomation(policyName, teamName, scriptName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("fleetdm_policy.test", "script_id"),
				),
			},
			{
				PreConfig:          func() { setPolicyScriptIDOutOfBand(t, policyName, scriptName) },
				Config:             testAccPolicyResourceConfigTeamNoAutomation(policyName, teamName, scriptName),
				ExpectNonEmptyPlan: true,
				PlanOnly:           true,
			},
			{
				Config: testAccPolicyResourceConfigTeamNoAutomation(policyName, teamName, scriptName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("fleetdm_policy.test", "script_id"),
				),
			},
		},
	})
}

func testAccPolicyResourceConfigLabels(policyName, labelA, labelB string, includes []string) string {
	includeBlock := ""
	if includes != nil {
		quoted := make([]string, 0, len(includes))
		for _, l := range includes {
			quoted = append(quoted, fmt.Sprintf("%q", l))
		}
		includeBlock = "  labels_include_any = [" + strings.Join(quoted, ", ") + "]\n"
	}

	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "a" {
  name  = %[2]q
  query = "SELECT 1 FROM os_version WHERE platform = 'darwin';"
}

resource "fleetdm_label" "b" {
  name  = %[3]q
  query = "SELECT 1 FROM os_version WHERE platform = 'linux';"
}

resource "fleetdm_policy" "test" {
  name  = %[1]q
  query = "SELECT 1;"
%[4]s
  depends_on = [fleetdm_label.a, fleetdm_label.b]
}
`, policyName, labelA, labelB, includeBlock)
}

func testAccPolicyResourceConfigTeamScript(policyName, teamName, scriptName string, withScript bool) string {
	scriptLine := ""
	if withScript {
		scriptLine = "  script_id = fleetdm_script.test.id\n"
	}
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name        = %[2]q
  description = "team for policy automation test"
}

resource "fleetdm_script" "test" {
  name    = %[3]q
  team_id = fleetdm_fleet.test.id
  content = "#!/bin/bash\necho hello"
}

resource "fleetdm_policy" "test" {
  name    = %[1]q
  query   = "SELECT 1;"
  team_id = fleetdm_fleet.test.id
%[4]s}
`, policyName, teamName, scriptName, scriptLine)
}

func testAccPolicyResourceConfigCalendarCA(policyName, teamName string, calendar, ca bool) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name        = %[2]q
  description = "team for calendar/ca test"
}

resource "fleetdm_policy" "test" {
  name                       = %[1]q
  query                      = "SELECT 1;"
  team_id                    = fleetdm_fleet.test.id
  calendar_events_enabled    = %[3]t
  conditional_access_enabled = %[4]t
}
`, policyName, teamName, calendar, ca)
}

func testAccPolicyResourceConfigTeamNoAutomation(policyName, teamName, scriptName string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name        = %[2]q
  description = "team for drift test"
}

resource "fleetdm_script" "test" {
  name    = %[3]q
  team_id = fleetdm_fleet.test.id
  content = "#!/bin/bash\necho hello"
}

resource "fleetdm_policy" "test" {
  name    = %[1]q
  query   = "SELECT 1;"
  team_id = fleetdm_fleet.test.id
}
`, policyName, teamName, scriptName)
}

// setPolicyScriptIDOutOfBand simulates a Fleet UI configuration: it looks
// up the team, the policy, and the script by name, then PATCHes the policy
// to attach the script — bypassing Terraform entirely. Used by
// TestAccPolicyResource_outOfBandAutomationDrift.
func setPolicyScriptIDOutOfBand(t *testing.T, policyName, scriptName string) {
	t.Helper()
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

	teams, err := client.ListTeams(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("failed to list teams: %v", err)
	}
	var teamID int
	for _, team := range teams {
		tID := int(team.ID)
		policies, err := client.ListTeamPolicies(context.Background(), tID)
		if err != nil {
			continue
		}
		for _, p := range policies {
			if p.Name == policyName {
				teamID = tID
				break
			}
		}
		if teamID != 0 {
			break
		}
	}
	if teamID == 0 {
		t.Fatalf("policy %q not found on any team", policyName)
	}

	scripts, err := client.ListScripts(context.Background(), &teamID)
	if err != nil {
		t.Fatalf("failed to list scripts for team %d: %v", teamID, err)
	}
	var scriptID int
	for _, s := range scripts {
		if s.Name == scriptName {
			scriptID = int(s.ID)
			break
		}
	}
	if scriptID == 0 {
		t.Fatalf("script %q not found on team %d", scriptName, teamID)
	}

	policies, err := client.ListTeamPolicies(context.Background(), teamID)
	if err != nil {
		t.Fatalf("failed to list policies for team %d: %v", teamID, err)
	}
	var policyID int
	for _, p := range policies {
		if p.Name == policyName {
			policyID = p.ID
			break
		}
	}
	if policyID == 0 {
		t.Fatalf("policy %q not found on team %d after second lookup", policyName, teamID)
	}

	if _, err := client.UpdatePolicy(context.Background(), policyID, &teamID, fleetdm.UpdatePolicyRequest{
		ScriptID: &scriptID,
	}); err != nil {
		t.Fatalf("failed to attach script %d to policy %d out-of-band: %v", scriptID, policyID, err)
	}
}
