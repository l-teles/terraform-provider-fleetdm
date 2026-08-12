package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// ---------------------------------------------------------------------------
// Mock Fleet
// ---------------------------------------------------------------------------

// fakeFleet is a stand-in for Fleet's fleets endpoints that reproduces the
// merge semantics of v4.90.0's PATCH handler. It exists because several of the
// new attributes cannot be exercised against a live Fleet in dev mode:
// enable_recovery_lock_password needs MDM turned on, the Google Calendar and
// conditional access toggles need matching global integrations, and Apple OS
// minimum_version is validated against Apple's live Software Lookup Service.
//
// Reproducing the merge rules rather than echoing the request back is the point:
// the provider's refresh logic has to survive a server that reports a concrete
// value for every field regardless of what was sent, which is what Fleet does.
type fakeFleet struct {
	mu     sync.Mutex
	state  map[string]any
	bodies []string
}

func newFakeFleet() *fakeFleet {
	return &fakeFleet{state: map[string]any{
		"id":          float64(1),
		"name":        "placeholder",
		"description": "",
		"user_count":  float64(0),
		"host_count":  float64(0),
		"host_expiry_settings": map[string]any{
			"host_expiry_enabled": false,
			"host_expiry_window":  float64(0),
		},
		// Fleet reports host_status_webhook as null until it is configured, and
		// always reports failing_policies_webhook as a fully-populated object.
		"webhook_settings": map[string]any{
			"host_status_webhook": nil,
			"failing_policies_webhook": map[string]any{
				"enable_failing_policies_webhook": false,
				"destination_url":                 "",
				"policy_ids":                      nil,
				"host_batch_size":                 float64(0),
			},
		},
		"integrations": map[string]any{
			"jira":                       nil,
			"zendesk":                    nil,
			"google_calendar":            nil,
			"conditional_access_enabled": nil,
		},
		"mdm": map[string]any{
			"enable_disk_encryption":        false,
			"enable_recovery_lock_password": false,
			"windows_require_bitlocker_pin": false,
			"name_template":                 "",
			"macos_updates":                 appleOSUpdatesZero(),
			"ios_updates":                   appleOSUpdatesZero(),
			"ipados_updates":                appleOSUpdatesZero(),
			"windows_updates": map[string]any{
				"deadline_days":     nil,
				"grace_period_days": nil,
			},
		},
		"features": map[string]any{
			"enable_host_users":         true,
			"enable_software_inventory": true,
			"historical_data": map[string]any{
				"uptime":          true,
				"vulnerabilities": true,
			},
		},
	}}
}

func appleOSUpdatesZero() map[string]any {
	return map[string]any{
		"update_new_hosts": nil,
		"minimum_version":  nil,
		"deadline":         nil,
	}
}

// patchBodies returns every PATCH body the fake received, so tests can assert
// on which keys made it onto the wire.
func (f *fakeFleet) patchBodies() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.bodies...)
}

func (f *fakeFleet) start(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/api/v1/fleet/fleets" && r.Method == http.MethodPost:
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode create body: %v", err)
			}
			if v, ok := req["name"].(string); ok {
				f.state["name"] = v
			}
			if v, ok := req["description"].(string); ok {
				f.state["description"] = v
			}
			f.respond(t, w)

		case r.URL.Path == "/api/v1/fleet/fleets/1" && r.Method == http.MethodGet:
			f.respond(t, w)

		case r.URL.Path == "/api/v1/fleet/fleets/1" && r.Method == http.MethodPatch:
			var raw json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				t.Errorf("failed to decode patch body: %v", err)
			}
			f.bodies = append(f.bodies, string(raw))

			var req map[string]any
			if err := json.Unmarshal(raw, &req); err != nil {
				t.Errorf("failed to unmarshal patch body: %v", err)
			}
			f.apply(req)
			f.respond(t, w)

		case r.URL.Path == "/api/v1/fleet/fleets/1" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(map[string]any{}); err != nil {
				t.Errorf("failed to encode delete response: %v", err)
			}

		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func (f *fakeFleet) respond(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]any{"fleet": f.state}); err != nil {
		t.Errorf("failed to encode fleet response: %v", err)
	}
}

// apply mirrors Fleet v4.90.0's ModifyTeam merge rules.
func (f *fakeFleet) apply(req map[string]any) {
	for _, key := range []string{"name", "description"} {
		if v, ok := req[key]; ok {
			f.state[key] = v
		}
	}

	// host_expiry_settings and webhook_settings are assigned wholesale:
	// `team.Config.WebhookSettings = *payload.WebhookSettings`. A sub-block the
	// request omits is therefore cleared, not preserved.
	if v, ok := req["host_expiry_settings"]; ok {
		f.state["host_expiry_settings"] = v
	}
	if v, ok := req["webhook_settings"].(map[string]any); ok {
		replacement := map[string]any{"host_status_webhook": nil, "failing_policies_webhook": nil}
		if hs, ok := v["host_status_webhook"].(map[string]any); ok {
			replacement["host_status_webhook"] = withDefaults(hs, map[string]any{
				"enable_host_status_webhook": false,
				"destination_url":            "",
				"host_percentage":            float64(0),
				"days_count":                 float64(0),
			})
		}
		if fp, ok := v["failing_policies_webhook"].(map[string]any); ok {
			replacement["failing_policies_webhook"] = withDefaults(fp, map[string]any{
				"enable_failing_policies_webhook": false,
				"destination_url":                 "",
				"policy_ids":                      nil,
				"host_batch_size":                 float64(0),
			})
		}
		f.state["webhook_settings"] = replacement
	}

	// mdm, integrations and features merge per sub-key via optjson.
	if v, ok := req["mdm"].(map[string]any); ok {
		mdm := f.state["mdm"].(map[string]any)
		for _, key := range []string{
			"enable_disk_encryption", "enable_recovery_lock_password",
			"windows_require_bitlocker_pin", "name_template",
		} {
			if val, ok := v[key]; ok {
				mdm[key] = val
			}
		}
		for _, key := range []string{"macos_updates", "ios_updates", "ipados_updates", "windows_updates"} {
			if sub, ok := v[key].(map[string]any); ok {
				target := mdm[key].(map[string]any)
				for k, val := range sub {
					target[k] = val
				}
			}
		}
	}

	if v, ok := req["integrations"].(map[string]any); ok {
		integrations := f.state["integrations"].(map[string]any)
		if gc, ok := v["google_calendar"]; ok {
			integrations["google_calendar"] = gc
		}
		if ca, ok := v["conditional_access_enabled"]; ok {
			integrations["conditional_access_enabled"] = ca
		}
	}

	if v, ok := req["features"].(map[string]any); ok {
		if hd, ok := v["historical_data"].(map[string]any); ok {
			target := f.state["features"].(map[string]any)["historical_data"].(map[string]any)
			for k, val := range hd {
				target[k] = val
			}
		}
	}
}

// withDefaults fills in the keys a Go value-struct would have zeroed.
func withDefaults(got, defaults map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range defaults {
		if val, ok := got[k]; ok {
			out[k] = val
		} else {
			out[k] = v
		}
	}
	return out
}

func fakeFleetProviderConfig(serverURL string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %q
  api_key        = "test-token"
}
`, serverURL)
}

// ---------------------------------------------------------------------------
// The opt-in convention
// ---------------------------------------------------------------------------

// TestAccFleetResource_optInBlocksStayNull is the load-bearing test for the
// opt-in convention. The mock reports a concrete value for every attribute in
// every block, exactly as Fleet does. A config that declares none of the blocks
// must still end up with all four null in state: anything else would either
// produce a permanent diff or have Terraform reject the apply for returning a
// value the config never asked for.
func TestAccFleetResource_optInBlocksStayNull(t *testing.T) {
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
					statecheck.ExpectKnownValue("fleetdm_fleet.test", tfjsonpath.New("webhook_settings"), knownvalue.Null()),
					statecheck.ExpectKnownValue("fleetdm_fleet.test", tfjsonpath.New("mdm"), knownvalue.Null()),
					statecheck.ExpectKnownValue("fleetdm_fleet.test", tfjsonpath.New("integrations"), knownvalue.Null()),
					statecheck.ExpectKnownValue("fleetdm_fleet.test", tfjsonpath.New("features"), knownvalue.Null()),
				},
			},
			{
				// No permanent diff.
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

// TestAccFleetResource_legacyDiskEncryptionDoesNotClobberSiblings is the
// provider-level counterpart to the wire pins in internal/fleetdm: a config that
// only touches the legacy top-level enable_disk_encryption must not put
// windows_require_bitlocker_pin on the wire. Before the request structs were
// pointerized this body always carried
// `"windows_require_bitlocker_pin":false`, which reset a BitLocker PIN
// requirement an operator had enabled in the Fleet UI.
func TestAccFleetResource_legacyDiskEncryptionDoesNotClobberSiblings(t *testing.T) {
	fake := newFakeFleet()
	server := fake.start(t)
	name := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fakeFleetProviderConfig(server.URL) + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name                   = %q
  enable_disk_encryption = true
}
`, name),
				Check: resource.TestCheckResourceAttr("fleetdm_fleet.test", "enable_disk_encryption", "true"),
			},
		},
	})

	bodies := fake.patchBodies()
	if len(bodies) == 0 {
		t.Fatal("expected at least one PATCH request")
	}
	for _, body := range bodies {
		if !strings.Contains(body, `"enable_disk_encryption":true`) {
			t.Errorf("PATCH body should set enable_disk_encryption: %s", body)
		}
		for _, forbidden := range []string{
			"windows_require_bitlocker_pin",
			"enable_recovery_lock_password",
			"name_template",
			"macos_updates",
			"ios_updates",
			"ipados_updates",
			"windows_updates",
			"webhook_settings",
			"integrations",
			"features",
		} {
			if strings.Contains(body, forbidden) {
				t.Errorf("PATCH body must not mention %q when it is not configured: %s", forbidden, body)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Per-block coverage against the mock
// ---------------------------------------------------------------------------

func TestAccFleetResource_webhookSettingsMock(t *testing.T) {
	fake := newFakeFleet()
	server := fake.start(t)
	name := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	provider := fakeFleetProviderConfig(server.URL)

	both := provider + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q

  webhook_settings = {
    failing_policies_webhook = {
      enable_failing_policies_webhook = true
      destination_url                 = "https://example.com/failing-policies"
      policy_ids                      = [7, 11]
      host_batch_size                 = 50
    }
    host_status_webhook = {
      enable_host_status_webhook = true
      destination_url            = "https://example.com/host-status"
      host_percentage            = 25.5
      days_count                 = 7
    }
  }
}
`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: both,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "webhook_settings.failing_policies_webhook.enable_failing_policies_webhook", "true"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "webhook_settings.failing_policies_webhook.destination_url", "https://example.com/failing-policies"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "webhook_settings.failing_policies_webhook.host_batch_size", "50"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "webhook_settings.failing_policies_webhook.policy_ids.#", "2"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "webhook_settings.host_status_webhook.enable_host_status_webhook", "true"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "webhook_settings.host_status_webhook.host_percentage", "25.5"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "webhook_settings.host_status_webhook.days_count", "7"),
				),
			},
			{
				Config:   both,
				PlanOnly: true,
			},
			{
				// Dropping host_status_webhook clears it server-side, which is
				// what declaring the parent block means.
				Config: provider + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q

  webhook_settings = {
    failing_policies_webhook = {
      enable_failing_policies_webhook = false
      destination_url                 = ""
    }
  }
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "webhook_settings.failing_policies_webhook.enable_failing_policies_webhook", "false"),
					resource.TestCheckNoResourceAttr("fleetdm_fleet.test", "webhook_settings.host_status_webhook.enable_host_status_webhook"),
				),
			},
			{
				// Removing the whole block leaves it null without touching Fleet.
				Config: provider + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q
}
`, name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("fleetdm_fleet.test", tfjsonpath.New("webhook_settings"), knownvalue.Null()),
				},
			},
		},
	})
}

func TestAccFleetResource_mdmBlockMock(t *testing.T) {
	fake := newFakeFleet()
	server := fake.start(t)
	name := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	provider := fakeFleetProviderConfig(server.URL)

	full := provider + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name                   = %q
  enable_disk_encryption = true

  mdm = {
    enable_recovery_lock_password = true
    windows_require_bitlocker_pin = true
    name_template                 = "$HOST_HW_SERIAL"

    macos_updates = {
      minimum_version  = "26.6.1"
      deadline         = "2026-12-01"
      update_new_hosts = true
    }
    ios_updates = {
      minimum_version = "26.6"
      deadline        = "2026-12-02"
    }
    ipados_updates = {
      minimum_version = "26.6"
      deadline        = "2026-12-03"
    }
    windows_updates = {
      deadline_days     = 7
      grace_period_days = 2
    }
  }
}
`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: full,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "enable_disk_encryption", "true"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.enable_recovery_lock_password", "true"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.windows_require_bitlocker_pin", "true"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.name_template", "$HOST_HW_SERIAL"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.macos_updates.minimum_version", "26.6.1"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.macos_updates.deadline", "2026-12-01"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.macos_updates.update_new_hosts", "true"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.ios_updates.minimum_version", "26.6"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.ipados_updates.deadline", "2026-12-03"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.windows_updates.deadline_days", "7"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.windows_updates.grace_period_days", "2"),
					// Never declared, so never read back.
					resource.TestCheckNoResourceAttr("fleetdm_fleet.test", "mdm.ios_updates.update_new_hosts"),
				),
			},
			{
				Config:   full,
				PlanOnly: true,
			},
			{
				// The clear path: empty strings for the Apple settings, zeros
				// for Windows, and an empty name template.
				Config: provider + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q

  mdm = {
    name_template = ""
    macos_updates = {
      minimum_version = ""
      deadline        = ""
    }
    windows_updates = {
      deadline_days     = 0
      grace_period_days = 0
    }
  }
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.name_template", ""),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.macos_updates.minimum_version", ""),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.macos_updates.deadline", ""),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.windows_updates.deadline_days", "0"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.windows_updates.grace_period_days", "0"),
					// Dropped from the config, so dropped from state.
					resource.TestCheckNoResourceAttr("fleetdm_fleet.test", "mdm.ios_updates.minimum_version"),
					resource.TestCheckNoResourceAttr("fleetdm_fleet.test", "mdm.enable_recovery_lock_password"),
				),
			},
		},
	})
}

func TestAccFleetResource_integrationsMock(t *testing.T) {
	fake := newFakeFleet()
	server := fake.start(t)
	name := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	provider := fakeFleetProviderConfig(server.URL)

	config := provider + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q

  integrations = {
    conditional_access_enabled = true
    google_calendar = {
      enable_calendar_events = true
      webhook_url            = "https://example.com/calendar"
    }
  }
}
`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "integrations.conditional_access_enabled", "true"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "integrations.google_calendar.enable_calendar_events", "true"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "integrations.google_calendar.webhook_url", "https://example.com/calendar"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
			{
				Config: provider + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q

  integrations = {
    conditional_access_enabled = false
  }
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "integrations.conditional_access_enabled", "false"),
					resource.TestCheckNoResourceAttr("fleetdm_fleet.test", "integrations.google_calendar.enable_calendar_events"),
				),
			},
		},
	})

	// Jira and Zendesk are deliberately unmodelled; the provider must never
	// mention them, since Fleet would otherwise reject or replace them.
	for _, body := range fake.patchBodies() {
		for _, forbidden := range []string{"jira", "zendesk"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("PATCH body must not mention %q: %s", forbidden, body)
			}
		}
	}
}

func TestAccFleetResource_featuresMock(t *testing.T) {
	fake := newFakeFleet()
	server := fake.start(t)
	name := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	provider := fakeFleetProviderConfig(server.URL)

	config := provider + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q

  features = {
    historical_data = {
      vulnerabilities = false
    }
  }
}
`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "features.historical_data.vulnerabilities", "false"),
					// uptime was never declared, so it stays out of state even
					// though Fleet reports it as true.
					resource.TestCheckNoResourceAttr("fleetdm_fleet.test", "features.historical_data.uptime"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
			{
				Config: provider + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q

  features = {
    historical_data = {
      uptime          = false
      vulnerabilities = true
    }
  }
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "features.historical_data.uptime", "false"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "features.historical_data.vulnerabilities", "true"),
				),
			},
		},
	})

	// enable_host_users and enable_software_inventory are read-only on this
	// endpoint; sending them would be silently ignored, which is worse than not
	// offering them at all.
	for _, body := range fake.patchBodies() {
		for _, forbidden := range []string{"enable_host_users", "enable_software_inventory", "additional_queries"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("PATCH body must not mention %q: %s", forbidden, body)
			}
		}
	}
}

// TestAccFleetResource_moveStateWithNewBlocks covers the fleetdm_team ->
// fleetdm_fleet move now that the shared schema carries four nested blocks and
// a set. The blocks are null in the source state, so the mover has to round-trip
// null objects without the framework complaining about unassigned values.
func TestAccFleetResource_moveStateWithNewBlocks(t *testing.T) {
	fake := newFakeFleet()
	server := fake.start(t)
	name := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	provider := fakeFleetProviderConfig(server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		Steps: []resource.TestStep{
			{
				Config: provider + fmt.Sprintf(`
resource "fleetdm_team" "test" {
  name = %q
}
`, name),
			},
			{
				Config: provider + fmt.Sprintf(`
moved {
  from = fleetdm_team.test
  to   = fleetdm_fleet.test
}

resource "fleetdm_fleet" "test" {
  name = %q
}
`, name),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("fleetdm_fleet.test", tfjsonpath.New("webhook_settings"), knownvalue.Null()),
					statecheck.ExpectKnownValue("fleetdm_fleet.test", tfjsonpath.New("mdm"), knownvalue.Null()),
					statecheck.ExpectKnownValue("fleetdm_fleet.test", tfjsonpath.New("integrations"), knownvalue.Null()),
					statecheck.ExpectKnownValue("fleetdm_fleet.test", tfjsonpath.New("features"), knownvalue.Null()),
				},
			},
		},
	})
}

// TestAccFleetResource_moveStateWithPopulatedBlocks is the harder move: the
// source state has every block populated, including the policy_ids set, so the
// mover must carry nested objects and collections across rather than just nulls.
func TestAccFleetResource_moveStateWithPopulatedBlocks(t *testing.T) {
	fake := newFakeFleet()
	server := fake.start(t)
	name := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	provider := fakeFleetProviderConfig(server.URL)

	blocks := `
  webhook_settings = {
    failing_policies_webhook = {
      enable_failing_policies_webhook = true
      destination_url                 = "https://example.com/failing-policies"
      policy_ids                      = [3, 9]
    }
  }

  mdm = {
    windows_require_bitlocker_pin = true
    windows_updates = {
      deadline_days     = 5
      grace_period_days = 1
    }
  }

  integrations = {
    conditional_access_enabled = false
  }

  features = {
    historical_data = {
      uptime = false
    }
  }
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		Steps: []resource.TestStep{
			{
				Config: provider + fmt.Sprintf(`
resource "fleetdm_team" "test" {
  name = %q
%s
}
`, name, blocks),
			},
			{
				Config: provider + fmt.Sprintf(`
moved {
  from = fleetdm_team.test
  to   = fleetdm_fleet.test
}

resource "fleetdm_fleet" "test" {
  name = %q
%s
}
`, name, blocks),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "webhook_settings.failing_policies_webhook.policy_ids.#", "2"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.windows_require_bitlocker_pin", "true"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.windows_updates.deadline_days", "5"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.windows_updates.grace_period_days", "1"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "integrations.conditional_access_enabled", "false"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "features.historical_data.uptime", "false"),
				),
			},
		},
	})
}

// TestAccFleetResource_settingsValidators checks that the constraints Fleet
// enforces at apply time are caught at plan time instead, so a practitioner sees
// the problem before a partially-applied change. Every message below was
// reproduced as a 422 from a live Fleet v4.90.0 first.
func TestAccFleetResource_settingsValidators(t *testing.T) {
	fake := newFakeFleet()
	server := fake.start(t)
	provider := fakeFleetProviderConfig(server.URL)

	tests := []struct {
		name        string
		block       string
		expectError string
	}{
		{
			name: "windows_updates deadline_days without grace_period_days",
			block: `
  mdm = {
    windows_updates = { deadline_days = 7 }
  }`,
			expectError: `Invalid Attribute Combination`,
		},
		{
			name: "windows_updates grace_period_days without deadline_days",
			block: `
  mdm = {
    windows_updates = { grace_period_days = 2 }
  }`,
			expectError: `Invalid Attribute Combination`,
		},
		{
			name: "windows_updates deadline_days above 30",
			block: `
  mdm = {
    windows_updates = { deadline_days = 31, grace_period_days = 2 }
  }`,
			expectError: `Invalid Attribute Value`,
		},
		{
			name: "windows_updates grace_period_days above 7",
			block: `
  mdm = {
    windows_updates = { deadline_days = 7, grace_period_days = 8 }
  }`,
			expectError: `Invalid Attribute Value`,
		},
		{
			name: "macos_updates minimum_version without deadline",
			block: `
  mdm = {
    macos_updates = { minimum_version = "26.6.1" }
  }`,
			expectError: `Invalid Attribute Combination`,
		},
		{
			name: "ios_updates deadline without minimum_version",
			block: `
  mdm = {
    ios_updates = { deadline = "2026-12-01" }
  }`,
			expectError: `Invalid Attribute Combination`,
		},
		{
			name: "macos_updates deadline in the wrong date format",
			block: `
  mdm = {
    macos_updates = { minimum_version = "26.6.1", deadline = "01/12/2026" }
  }`,
			expectError: `must\s+be\s+a\s+date\s+in\s+YYYY-MM-DD\s+form`,
		},
		{
			name: "ipados_updates deadline as a timestamp",
			block: `
  mdm = {
    ipados_updates = { minimum_version = "26.6", deadline = "2026-12-01T00:00:00Z" }
  }`,
			expectError: `must\s+be\s+a\s+date\s+in\s+YYYY-MM-DD\s+form`,
		},
		{
			name: "failing policies destination_url is relative",
			block: `
  webhook_settings = {
    failing_policies_webhook = {
      enable_failing_policies_webhook = true
      destination_url                 = "/fleet/failing-policies"
    }
  }`,
			expectError: `must\s+be\s+an\s+absolute`,
		},
		{
			name: "host status destination_url has a non-http scheme",
			block: `
  webhook_settings = {
    host_status_webhook = {
      enable_host_status_webhook = true
      destination_url            = "ftp://example.com/host-status"
    }
  }`,
			expectError: `must\s+be\s+an\s+absolute`,
		},
		{
			name: "google_calendar webhook_url is not a URL",
			block: `
  integrations = {
    google_calendar = {
      enable_calendar_events = true
      webhook_url            = "example.com/calendar"
    }
  }`,
			expectError: `must\s+be\s+an\s+absolute`,
		},
		{
			name: "google_calendar enable_calendar_events without webhook_url",
			block: `
  integrations = {
    google_calendar = { enable_calendar_events = false }
  }`,
			expectError: `Invalid Attribute Combination`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: provider + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q
%s
}
`, name, tt.block),
						ExpectError: regexp.MustCompile(tt.expectError),
					},
				},
			})
		})
	}
}

// ---------------------------------------------------------------------------
// Live Fleet
// ---------------------------------------------------------------------------

// TestAccFleetResource_settingsLive exercises the blocks a live Fleet accepts in
// dev mode. Deliberately excluded, because Fleet rejects them without
// infrastructure a test rig does not have:
//
//   - mdm.enable_recovery_lock_password: needs MDM turned on.
//   - integrations.google_calendar with events enabled, and
//     integrations.conditional_access_enabled = true: need matching global
//     integrations.
//   - mdm.macos_updates/ios_updates/ipados_updates minimum_version: validated
//     against Apple's live Software Lookup Service with an exact version match,
//     so any value hard-coded here would rot as Apple rotates its published
//     versions.
//
// All of those are covered against the mock above.
func TestAccFleetResource_settingsLive(t *testing.T) {
	name := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	withSettings := providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q

  webhook_settings = {
    failing_policies_webhook = {
      enable_failing_policies_webhook = true
      destination_url                 = "https://example.com/failing-policies"
      host_batch_size                 = 25
    }
    host_status_webhook = {
      enable_host_status_webhook = true
      destination_url            = "https://example.com/host-status"
      host_percentage            = 10
      days_count                 = 3
    }
  }

  mdm = {
    windows_require_bitlocker_pin = true
    name_template                 = "tf-acc-$HOST_HW_SERIAL"
    windows_updates = {
      deadline_days     = 7
      grace_period_days = 2
    }
  }

  integrations = {
    conditional_access_enabled = false
  }

  features = {
    historical_data = {
      vulnerabilities = false
    }
  }
}
`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: withSettings,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "webhook_settings.failing_policies_webhook.enable_failing_policies_webhook", "true"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "webhook_settings.failing_policies_webhook.host_batch_size", "25"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "webhook_settings.host_status_webhook.host_percentage", "10"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "webhook_settings.host_status_webhook.days_count", "3"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.windows_require_bitlocker_pin", "true"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.name_template", "tf-acc-$HOST_HW_SERIAL"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.windows_updates.deadline_days", "7"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.windows_updates.grace_period_days", "2"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "integrations.conditional_access_enabled", "false"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "features.historical_data.vulnerabilities", "false"),
				),
			},
			{
				Config:   withSettings,
				PlanOnly: true,
			},
			{
				// Update and clear paths.
				Config: providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q

  webhook_settings = {
    failing_policies_webhook = {
      enable_failing_policies_webhook = false
      destination_url                 = ""
    }
  }

  mdm = {
    windows_require_bitlocker_pin = false
    name_template                 = ""
    windows_updates = {
      deadline_days     = 0
      grace_period_days = 0
    }
  }

  features = {
    historical_data = {
      vulnerabilities = true
    }
  }
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "webhook_settings.failing_policies_webhook.enable_failing_policies_webhook", "false"),
					resource.TestCheckNoResourceAttr("fleetdm_fleet.test", "webhook_settings.host_status_webhook.enable_host_status_webhook"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.windows_require_bitlocker_pin", "false"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.name_template", ""),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.windows_updates.deadline_days", "0"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "features.historical_data.vulnerabilities", "true"),
					resource.TestCheckNoResourceAttr("fleetdm_fleet.test", "integrations.conditional_access_enabled"),
				),
			},
			{
				// Import cannot recover which opt-in blocks the practitioner
				// declared -- that information only exists in the config -- so
				// an imported fleet starts with all four null and the following
				// plan shows whatever the config asks Fleet to write. The
				// alternative would be importing every block fully populated,
				// which would hand the practitioner a diff against settings
				// they never intended to manage.
				ResourceName:            "fleetdm_fleet.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"webhook_settings", "mdm", "integrations", "features"},
			},
		},
	})
}

// TestAccFleetResource_upgradeFromPreBlockConfigLive proves no state migration
// is needed: a fleet created by a config that predates these attributes gains
// them in place, with no replacement and no migration step.
func TestAccFleetResource_upgradeFromPreBlockConfigLive(t *testing.T) {
	name := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// The pre-4.87 shape of this resource.
				Config: providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name                   = %q
  description            = "Created before the settings blocks existed"
  host_expiry_enabled    = true
  host_expiry_window     = 30
  enable_disk_encryption = false
}
`, name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("fleetdm_fleet.test", tfjsonpath.New("webhook_settings"), knownvalue.Null()),
					statecheck.ExpectKnownValue("fleetdm_fleet.test", tfjsonpath.New("mdm"), knownvalue.Null()),
					statecheck.ExpectKnownValue("fleetdm_fleet.test", tfjsonpath.New("integrations"), knownvalue.Null()),
					statecheck.ExpectKnownValue("fleetdm_fleet.test", tfjsonpath.New("features"), knownvalue.Null()),
				},
			},
			{
				// Adding a block must be an in-place update, not a replacement.
				Config: providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name                   = %q
  description            = "Created before the settings blocks existed"
  host_expiry_enabled    = true
  host_expiry_window     = 30
  enable_disk_encryption = false

  mdm = {
    windows_updates = {
      deadline_days     = 14
      grace_period_days = 3
    }
  }
}
`, name),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_fleet.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "host_expiry_window", "30"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.windows_updates.deadline_days", "14"),
					resource.TestCheckResourceAttr("fleetdm_fleet.test", "mdm.windows_updates.grace_period_days", "3"),
				),
			},
		},
	})
}
