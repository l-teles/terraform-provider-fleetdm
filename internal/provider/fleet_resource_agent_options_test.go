package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// fleetAgentOptionsConfig declares agent_options as a heredoc rather than with
// jsonencode(), so the keys reach Fleet out of order and with whitespace. The
// fake sends them back sorted and compact, which is what the semantic
// comparison has to absorb.
func fleetAgentOptionsConfig(serverURL, name string, timeout int) string {
	return fakeFleetProviderConfig(serverURL) + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name          = %q
  agent_options = <<-EOT
    {
      "script_execution_timeout": %d,
      "config": { "options": { "distributed_interval": 10 } }
    }
  EOT
}
`, name, timeout)
}

// TestAccFleetResource_agentOptionsOptIn checks that a fleet without
// agent_options neither writes nor reads them, even though Fleet always holds
// a copy of the global ones.
func TestAccFleetResource_agentOptionsOptIn(t *testing.T) {
	fake := newFakeFleet()
	server := fake.start(t)
	name := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	config := fakeFleetProviderConfig(server.URL) + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q
}
`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("fleetdm_fleet.test", tfjsonpath.New("agent_options"), knownvalue.Null()),
				},
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})

	if got := fake.agentOptionsRequests(); len(got) != 0 {
		t.Errorf("expected no agent options requests, got %v", got)
	}
}

// TestAccFleetResource_agentOptions covers the managed lifecycle: create,
// no drift despite Fleet re-ordering the keys, update, letting go of the
// attribute, and import.
func TestAccFleetResource_agentOptions(t *testing.T) {
	fake := newFakeFleet()
	server := fake.start(t)
	name := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	unmanaged := fakeFleetProviderConfig(server.URL) + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q
}
`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fleetAgentOptionsConfig(server.URL, name, 600),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAgentOptionsTimeout("fleetdm_fleet.test", 600),
					func(*terraform.State) error { return expectAgentOptionsRequests(fake, 1) },
				),
			},
			{
				// Fleet returns the same options with sorted keys: no diff.
				Config:   fleetAgentOptionsConfig(server.URL, name, 600),
				PlanOnly: true,
			},
			{
				Config: fleetAgentOptionsConfig(server.URL, name, 1200),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAgentOptionsTimeout("fleetdm_fleet.test", 1200),
					func(*terraform.State) error { return expectAgentOptionsRequests(fake, 2) },
				),
			},
			{
				// Dropping the attribute leaves Fleet's value alone.
				Config: unmanaged,
				Check: func(*terraform.State) error {
					if err := expectAgentOptionsRequests(fake, 2); err != nil {
						return err
					}
					return expectFakeTimeout(fake, 1200)
				},
			},
			{
				Config:   unmanaged,
				PlanOnly: true,
			},
			{
				ResourceName:  "fleetdm_fleet.test",
				ImportState:   true,
				ImportStateId: "1",
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 imported state, got %d", len(states))
					}
					if v, ok := states[0].Attributes["agent_options"]; ok && v != "" {
						return fmt.Errorf("expected agent_options to be imported as null, got %q", v)
					}
					return nil
				},
			},
		},
	})
}

// TestAccFleetResource_agentOptionsCreateFailure checks that a rejected write
// after the fleet was created is reported against agent options.
func TestAccFleetResource_agentOptionsCreateFailure(t *testing.T) {
	fake := newFakeFleet()
	fake.agentOptionsStatus = http.StatusBadRequest
	server := fake.start(t)
	name := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fleetAgentOptionsConfig(server.URL, name, 600),
				ExpectError: regexp.MustCompile(`agent options could not be applied \(fleet ID saved to\s+state\)`),
			},
		},
	})
}

func checkAgentOptionsTimeout(resourceName string, want int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceName)
		}
		var options map[string]any
		if err := json.Unmarshal([]byte(rs.Primary.Attributes["agent_options"]), &options); err != nil {
			return fmt.Errorf("agent_options in state is not JSON: %w", err)
		}
		if options["script_execution_timeout"] != float64(want) {
			return fmt.Errorf("expected script_execution_timeout %d in state, got %v", want, options["script_execution_timeout"])
		}
		return nil
	}
}

func expectAgentOptionsRequests(fake *fakeFleet, want int) error {
	if got := len(fake.agentOptionsRequests()); got != want {
		return fmt.Errorf("expected %d agent options requests, got %d", want, got)
	}
	return nil
}

func expectFakeTimeout(fake *fakeFleet, want int) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	options, _ := fake.state["agent_options"].(map[string]any)
	if options["script_execution_timeout"] != float64(want) {
		return fmt.Errorf("expected Fleet to keep script_execution_timeout %d, got %v", want, options["script_execution_timeout"])
	}
	return nil
}
