package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeHiddenPolicyServer is a stateful stand-in for Fleet 4.94's global and
// fleet policy endpoints, tracking `hidden` and `calendar_events_enabled` and
// recording every create/update body.
type fakeHiddenPolicyServer struct {
	srv *httptest.Server

	mu          sync.Mutex
	bodies      []recordedPolicyBody
	teamID      *int64
	description string
	hidden      bool
	calendar    bool
}

type recordedPolicyBody struct {
	method string
	body   map[string]json.RawMessage
}

func newFakeHiddenPolicyServer(t *testing.T, teamID *int64) *fakeHiddenPolicyServer {
	t.Helper()
	f := &fakeHiddenPolicyServer{teamID: teamID}
	base := "/api/v1/fleet/global/policies"
	if teamID != nil {
		base = fmt.Sprintf("/api/v1/fleet/fleets/%d/policies", *teamID)
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == base && r.Method == http.MethodPost:
			f.apply(t, r)
		case r.URL.Path == base+"/42" && r.Method == http.MethodPatch:
			f.apply(t, r)
		case r.URL.Path == base+"/42" && r.Method == http.MethodGet:
		case r.URL.Path == base+"/delete" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"deleted":[42]}`))
			return
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		p := map[string]any{
			"id":                      42,
			"name":                    "tf-mock-hidden",
			"query":                   "SELECT 1;",
			"description":             f.description,
			"type":                    "dynamic",
			"hidden":                  f.hidden,
			"calendar_events_enabled": f.calendar,
		}
		if f.teamID != nil {
			p["team_id"] = *f.teamID
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"policy": p})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// apply mirrors Fleet: a key left out of the body keeps its stored value.
func (f *fakeHiddenPolicyServer) apply(t *testing.T, r *http.Request) {
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("decode request body: %v", err)
		return
	}
	f.bodies = append(f.bodies, recordedPolicyBody{method: r.Method, body: body})
	if raw, ok := body["description"]; ok {
		_ = json.Unmarshal(raw, &f.description)
	}
	if raw, ok := body["hidden"]; ok {
		_ = json.Unmarshal(raw, &f.hidden)
	}
	if raw, ok := body["calendar_events_enabled"]; ok {
		_ = json.Unmarshal(raw, &f.calendar)
	}
}

// expectHiddenInBodies asserts that the bodies recorded since the previous
// call carry `hidden` with the given raw values, one per request, where ""
// means the key must be absent.
func (f *fakeHiddenPolicyServer) expectHiddenInBodies(seen *int, want ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		got := f.bodies[*seen:]
		*seen = len(f.bodies)
		if len(got) != len(want) {
			return fmt.Errorf("expected %d create/update requests, got %d", len(want), len(got))
		}
		for i, rec := range got {
			raw, ok := rec.body["hidden"]
			switch {
			case want[i] == "" && ok:
				return fmt.Errorf("request %d (%s): expected hidden to be absent, got %s", i, rec.method, raw)
			case want[i] != "" && string(raw) != want[i]:
				return fmt.Errorf("request %d (%s): expected hidden=%s, got %q", i, rec.method, want[i], raw)
			}
		}
		return nil
	}
}

func testAccPolicyHiddenMockConfig(serverURL, extra string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_policy" "test" {
  name  = "tf-mock-hidden"
  query = "SELECT 1;"
%[2]s}
`, serverURL, extra)
}

// TestAccPolicyResource_hiddenMock walks a global policy through the
// `hidden` lifecycle and checks the wire: Fleet Free rejects a present
// premium field even as false, so a policy that never sets `hidden` must
// never send it, and turning it off must send an explicit false.
func TestAccPolicyResource_hiddenMock(t *testing.T) {
	f := newFakeHiddenPolicyServer(t, nil)
	seen := 0
	const res = "fleetdm_policy.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Never set: absent on create.
				Config: testAccPolicyHiddenMockConfig(f.srv.URL, "  description = \"one\"\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "hidden", "false"),
					f.expectHiddenInBodies(&seen, ""),
				),
			},
			{
				// Still never set: absent on update.
				Config: testAccPolicyHiddenMockConfig(f.srv.URL, "  description = \"two\"\n"),
				Check:  f.expectHiddenInBodies(&seen, ""),
			},
			{
				Config: testAccPolicyHiddenMockConfig(f.srv.URL, "  description = \"two\"\n  hidden = true\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "hidden", "true"),
					f.expectHiddenInBodies(&seen, "true"),
				),
			},
			{
				// Back to false: Fleet keeps the stored value when the key is
				// left out, so the clear must be explicit.
				Config: testAccPolicyHiddenMockConfig(f.srv.URL, "  description = \"two\"\n  hidden = false\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "hidden", "false"),
					f.expectHiddenInBodies(&seen, "false"),
				),
			},
			{
				// Off and was off: absent again.
				Config: testAccPolicyHiddenMockConfig(f.srv.URL, "  description = \"three\"\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "hidden", "false"),
					f.expectHiddenInBodies(&seen, ""),
				),
			},
		},
	})
}

// TestAccPolicyResource_hiddenMockTeam covers the fleet-policy create path:
// `hidden = true` goes on the create itself, and the follow-up PATCH that
// calendar_events_enabled forces must not carry an unset `hidden`.
func TestAccPolicyResource_hiddenMockTeam(t *testing.T) {
	teamID := int64(7)
	cfg := func(serverURL, extra string) string {
		return testAccPolicyHiddenMockConfig(serverURL, "  team_id = 7\n  calendar_events_enabled = true\n"+extra)
	}

	t.Run("unset", func(t *testing.T) {
		f := newFakeHiddenPolicyServer(t, &teamID)
		seen := 0
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{
					Config: cfg(f.srv.URL, ""),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("fleetdm_policy.test", "hidden", "false"),
						resource.TestCheckResourceAttr("fleetdm_policy.test", "calendar_events_enabled", "true"),
						// The create, then the calendar follow-up PATCH.
						f.expectHiddenInBodies(&seen, "", ""),
					),
				},
			},
		})
	})

	t.Run("set", func(t *testing.T) {
		f := newFakeHiddenPolicyServer(t, &teamID)
		seen := 0
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{
					Config: cfg(f.srv.URL, "  hidden = true\n"),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("fleetdm_policy.test", "hidden", "true"),
						f.expectHiddenInBodies(&seen, "true", "true"),
					),
				},
			},
		})
	})
}

// TestAccPolicyResource_hiddenConflictsWithConditionalAccess pins the
// plan-time copy of Fleet's `"hidden" and "conditional_access_enabled"
// cannot both be set` check. No server is involved: the plan must fail first.
func TestAccPolicyResource_hiddenConflictsWithConditionalAccess(t *testing.T) {
	cfg := func(hidden, ca bool) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = "http://127.0.0.1:1"
  api_key        = "test-token"
}

resource "fleetdm_policy" "test" {
  name                       = "tf-mock-hidden"
  query                      = "SELECT 1;"
  team_id                    = 7
  hidden                     = %t
  conditional_access_enabled = %t
}
`, hidden, ca)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      cfg(true, true),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Conflicting\s+policy\s+options.*hidden\s+and\s+conditional_access_enabled\s+cannot\s+both\s+be\s+true`),
			},
		},
	})
}

// TestAccPolicyResource_hidden runs the `hidden` lifecycle against a live
// Fleet Premium 4.94+ on a global and a fleet policy: create hidden, import
// and compare, then turn it off.
func TestAccPolicyResource_hidden(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	cfg := func(hidden bool) string {
		return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = "tf-acc-team-hidden-%[1]s"
}

resource "fleetdm_policy" "global" {
  name   = "tf-acc-test-hidden-global-%[1]s"
  query  = "SELECT 1;"
  hidden = %[2]t
}

resource "fleetdm_policy" "team" {
  name    = "tf-acc-test-hidden-team-%[1]s"
  query   = "SELECT 1;"
  team_id = fleetdm_fleet.test.id
  hidden  = %[2]t
}
`, suffix, hidden)
	}

	flags := func(want string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("fleetdm_policy.global", "hidden", want),
			resource.TestCheckResourceAttr("fleetdm_policy.team", "hidden", want),
		)
	}
	ignore := []string{"conditional_access_bypass_enabled", "updated_at"}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg(true), Check: flags("true")},
			{Config: cfg(true), PlanOnly: true},
			{
				ResourceName:            "fleetdm_policy.global",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: ignore,
			},
			{
				ResourceName:            "fleetdm_policy.team",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: ignore,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["fleetdm_policy.team"]
					if !ok {
						return "", fmt.Errorf("fleetdm_policy.team not found in state")
					}
					return rs.Primary.Attributes["team_id"] + ":" + rs.Primary.Attributes["id"], nil
				},
			},
			{Config: cfg(false), Check: flags("false")},
			// The refresh reads hidden back from Fleet, so this also checks the
			// false was stored rather than left out of the PATCH.
			{Config: cfg(false), PlanOnly: true},
		},
	})
}

// TestCheckPolicyAutomationsEchoed_hidden checks the named error when Fleet
// (before 4.94) accepts a policy but drops hidden.
func TestCheckPolicyAutomationsEchoed_hidden(t *testing.T) {
	for _, tc := range []struct {
		name            string
		planned, echoed bool
		wantErr         bool
	}{
		{name: "dropped", planned: true, echoed: false, wantErr: true},
		{name: "applied", planned: true, echoed: true},
		{name: "not planned", planned: false, echoed: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			checkPolicyAutomationsEchoed(
				policyPriorAutomations{SoftwarePackageID: types.Int64Null(), ProfileUUID: types.StringNull(), Hidden: types.BoolValue(tc.planned)},
				PolicyResourceModel{SoftwarePackageID: types.Int64Null(), ProfileUUID: types.StringNull(), Hidden: types.BoolValue(tc.echoed)},
				&diags,
			)
			if diags.HasError() != tc.wantErr {
				t.Fatalf("HasError = %v, want %v: %v", diags.HasError(), tc.wantErr, diags)
			}
		})
	}
}
