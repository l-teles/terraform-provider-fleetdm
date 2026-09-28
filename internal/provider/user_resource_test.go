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
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccUserResource_basic(t *testing.T) {
	userName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	userEmail := userName + "@example.com"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: testAccUserResourceConfig(userName, userEmail, "observer"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_user.test", "name", userName),
					resource.TestCheckResourceAttr("fleetdm_user.test", "email", userEmail),
					resource.TestCheckResourceAttr("fleetdm_user.test", "global_role", "observer"),
					resource.TestCheckResourceAttrSet("fleetdm_user.test", "id"),
					resource.TestCheckResourceAttr("fleetdm_user.test", "api_only", "false"),
					resource.TestCheckResourceAttr("fleetdm_user.test", "sso_enabled", "false"),
					// A never-logged-in user with a role is active: Fleet falls
					// back to created_at for the 30-day window.
					resource.TestCheckResourceAttr("fleetdm_user.test", "status", "active"),
					resource.TestCheckNoResourceAttr("fleetdm_user.test", "last_login_at"),
				),
			},
			// Update global role
			{
				Config: testAccUserResourceConfig(userName, userEmail, "maintainer"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_user.test", "global_role", "maintainer"),
				),
			},
			// Update name
			{
				Config: testAccUserResourceConfig(userName+"-updated", userEmail, "maintainer"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_user.test", "name", userName+"-updated"),
					resource.TestCheckResourceAttr("fleetdm_user.test", "global_role", "maintainer"),
				),
			},
		},
	})
}

func TestAccUserResource_apiOnly(t *testing.T) {
	userName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	userEmail := userName + "@example.com"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserResourceConfigAPIOnly(userName, userEmail),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_user.test", "name", userName),
					resource.TestCheckResourceAttr("fleetdm_user.test", "email", userEmail),
					resource.TestCheckResourceAttr("fleetdm_user.test", "api_only", "true"),
					resource.TestCheckResourceAttr("fleetdm_user.test", "global_role", "observer"),
				),
			},
		},
	})
}

// TestAccUserResource_apiEndpoints exercises the endpoint scope end to end
// against a live Fleet: created unrestricted, narrowed, widened, then cleared.
// The scope cannot be sent with the create call (POST /users/admin rejects
// api_endpoints), so this also covers the follow-up PATCH the resource issues.
//
// Fleet Premium only; on Fleet Free the apply fails with a missing-license
// error rather than silently ignoring the field.
func TestAccUserResource_apiEndpoints(t *testing.T) {
	userName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	userEmail := userName + "@example.com"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with a scope applied at creation time.
			{
				Config: testAccUserResourceConfigAPIEndpoints(userName, userEmail, `
  api_endpoints = [
    {
      method = "GET"
      path   = "/api/v1/fleet/hosts"
    },
  ]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_user.test", "api_only", "true"),
					resource.TestCheckResourceAttr("fleetdm_user.test", "api_endpoints.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs("fleetdm_user.test", "api_endpoints.*", map[string]string{
						"method": "GET",
						"path":   "/api/v1/fleet/hosts",
					}),
					// Fleet mints a token for API-only users at creation.
					resource.TestCheckResourceAttrSet("fleetdm_user.test", "token"),
				),
			},
			// Widen the scope.
			{
				Config: testAccUserResourceConfigAPIEndpoints(userName, userEmail, `
  api_endpoints = [
    {
      method = "GET"
      path   = "/api/v1/fleet/hosts"
    },
    {
      method = "GET"
      path   = "/api/v1/fleet/labels"
    },
  ]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_user.test", "api_endpoints.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("fleetdm_user.test", "api_endpoints.*", map[string]string{
						"method": "GET",
						"path":   "/api/v1/fleet/labels",
					}),
				),
			},
			// Narrow it again, to a different single endpoint.
			{
				Config: testAccUserResourceConfigAPIEndpoints(userName, userEmail, `
  api_endpoints = [
    {
      method = "GET"
      path   = "/api/v1/fleet/labels"
    },
  ]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_user.test", "api_endpoints.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs("fleetdm_user.test", "api_endpoints.*", map[string]string{
						"method": "GET",
						"path":   "/api/v1/fleet/labels",
					}),
				),
			},
			// An update that does not touch the scope must leave it intact.
			// This is the path where the resource never calls
			// PATCH /users/api_only/{id}, so the scope in state comes from the
			// plan rather than from the PATCH /users/{id} response — a rename
			// must neither clear the scope in Fleet nor produce an
			// "inconsistent result after apply".
			{
				Config: testAccUserResourceConfigAPIEndpoints(userName+"-renamed", userEmail, `
  api_endpoints = [
    {
      method = "GET"
      path   = "/api/v1/fleet/labels"
    },
  ]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_user.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_user.test", "name", userName+"-renamed"),
					resource.TestCheckResourceAttr("fleetdm_user.test", "api_endpoints.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs("fleetdm_user.test", "api_endpoints.*", map[string]string{
						"method": "GET",
						"path":   "/api/v1/fleet/labels",
					}),
				),
			},
			// Re-read from Fleet to prove the rename really did leave the
			// scope in place server-side, not just in state.
			{
				Config: testAccUserResourceConfigAPIEndpoints(userName+"-renamed", userEmail, `
  api_endpoints = [
    {
      method = "GET"
      path   = "/api/v1/fleet/labels"
    },
  ]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			// Removing the attribute clears the scope (Fleet takes a null),
			// restoring unrestricted access rather than leaving the last set
			// in place.
			{
				Config: testAccUserResourceConfigAPIEndpoints(userName+"-renamed", userEmail, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("fleetdm_user.test", "api_endpoints.#"),
				),
			},
		},
	})
}

// TestAccUserResource_apiEndpointsRequiresAPIOnly checks the plan-time
// validator that mirrors Fleet's "API endpoints can only be specified for API
// only users" rule, so the misconfiguration surfaces as a config error instead
// of an apply-time 422.
func TestAccUserResource_apiEndpointsRequiresAPIOnly(t *testing.T) {
	userName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	userEmail := userName + "@example.com"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "fleetdm_user" "test" {
  name        = %[1]q
  email       = %[2]q
  password    = "FleetTest@12345!"
  global_role = "observer"

  api_endpoints = [
    {
      method = "GET"
      path   = "/api/v1/fleet/hosts"
    },
  ]
}
`, userName, userEmail),
				ExpectError: regexp.MustCompile(`can only be set when .api_only. is`),
			},
		},
	})
}

// TestAccUserResource_apiEndpointsRejectsEmptySet checks the SizeAtLeast(1)
// validator. Fleet treats an empty array as invalid ("at least one API
// endpoint must be specified") and expects the attribute to be omitted to
// grant full access instead.
func TestAccUserResource_apiEndpointsRejectsEmptySet(t *testing.T) {
	userName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	userEmail := userName + "@example.com"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "fleetdm_user" "test" {
  name          = %[1]q
  email         = %[2]q
  password      = "FleetTest@12345!"
  global_role   = "observer"
  api_only      = true
  api_endpoints = []
}
`, userName, userEmail),
				ExpectError: regexp.MustCompile(`(?i)at least 1 element`),
			},
		},
	})
}

// TestAccUserResource_apiEndpointsRejectsBadMethod checks the method
// validator, which keeps a typo from reaching Fleet as an unknown-endpoint
// 422.
func TestAccUserResource_apiEndpointsRejectsBadMethod(t *testing.T) {
	userName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	userEmail := userName + "@example.com"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "fleetdm_user" "test" {
  name        = %[1]q
  email       = %[2]q
  password    = "FleetTest@12345!"
  global_role = "observer"
  api_only    = true

  api_endpoints = [
    {
      method = "FETCH"
      path   = "/api/v1/fleet/hosts"
    },
  ]
}
`, userName, userEmail),
				ExpectError: regexp.MustCompile(`(?i)value must be one of`),
			},
		},
	})
}

// TestAccUserResource_activityFields runs against a fake Fleet that, like Fleet
// 4.92, returns `status` only from GET and moves `last_activity_at` on its own.
// It proves status is known after create and that an update carries the
// activity fields from state instead of producing an inconsistent result.
func TestAccUserResource_activityFields(t *testing.T) {
	const (
		lastLogin    = "2026-09-01T10:00:00Z"
		lastActivity = "2026-09-28T12:00:00Z"
	)

	var (
		mu       sync.Mutex
		name     string
		email    string
		activity *string
		getCalls int
	)

	userJSON := func(withStatus bool, login, act *string) map[string]interface{} {
		u := map[string]interface{}{
			"id":                   7,
			"name":                 name,
			"email":                email,
			"global_role":          "observer",
			"force_password_reset": true,
			"last_login_at":        login,
			"last_activity_at":     act,
		}
		if withStatus {
			u["status"] = "active"
		}
		return u
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/fleet/users/admin":
			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)
			name, _ = req["name"].(string)
			email, _ = req["email"].(string)
			json.NewEncoder(w).Encode(map[string]interface{}{"user": userJSON(false, nil, nil)})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/fleet/users/7":
			getCalls++
			login := lastLogin
			json.NewEncoder(w).Encode(map[string]interface{}{"user": userJSON(true, &login, activity)})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/fleet/users/7":
			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)
			name, _ = req["name"].(string)
			// Session activity moves between refresh and apply.
			act := lastActivity
			activity = &act
			login := lastLogin
			json.NewEncoder(w).Encode(map[string]interface{}{"user": userJSON(false, &login, activity)})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/fleet/users/7":
			w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	config := func(n string) string {
		return fakeFleetProviderConfig(server.URL) + fmt.Sprintf(`
resource "fleetdm_user" "test" {
  name        = %q
  email       = "activity@example.com"
  password    = "FleetTest@12345!"
  global_role = "observer"
}
`, n)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// The create echo carries no status; the follow-up GET fills it.
			{
				Config: config("activity-user"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_user.test", "status", "active"),
					resource.TestCheckResourceAttr("fleetdm_user.test", "last_login_at", lastLogin),
					resource.TestCheckNoResourceAttr("fleetdm_user.test", "last_activity_at"),
				),
			},
			// The PATCH echo reports a newer last_activity_at and no status;
			// both must come from state.
			{
				Config: config("activity-user-renamed"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_user.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue("fleetdm_user.test", tfjsonpath.New("status"), knownvalue.StringExact("active")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_user.test", "name", "activity-user-renamed"),
					resource.TestCheckResourceAttr("fleetdm_user.test", "status", "active"),
					resource.TestCheckNoResourceAttr("fleetdm_user.test", "last_activity_at"),
				),
			},
			// A refresh picks up the new activity without planning a change.
			{
				Config: config("activity-user-renamed"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_user.test", "last_activity_at", lastActivity),
					resource.TestCheckResourceAttr("fleetdm_user.test", "status", "active"),
				),
			},
			{
				ResourceName:            "fleetdm_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
	})

	mu.Lock()
	defer mu.Unlock()
	if getCalls == 0 {
		t.Error("expected the resource to read the user back via GET")
	}
}

func testAccUserResourceConfig(name, email, role string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_user" "test" {
  name        = %[1]q
  email       = %[2]q
  password    = "FleetTest@12345!"
  global_role = %[3]q
}
`, name, email, role)
}

func testAccUserResourceConfigAPIOnly(name, email string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_user" "test" {
  name        = %[1]q
  email       = %[2]q
  password    = "FleetTest@12345!"
  global_role = "observer"
  api_only    = true
}
`, name, email)
}

func testAccUserResourceConfigAPIEndpoints(name, email, endpoints string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_user" "test" {
  name        = %[1]q
  email       = %[2]q
  password    = "FleetTest@12345!"
  global_role = "observer"
  api_only    = true
%[3]s
}
`, name, email, endpoints)
}

// TestAccUserResource_nameLength pins the 255-character cap on a user's name.
// Fleet's API returns a raw MySQL "Data too long" error past that, so the
// limit is enforced at plan time to produce an actionable message.
func TestAccUserResource_nameLength(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fakeFleetProviderConfig("http://127.0.0.1:1") + fmt.Sprintf(`
resource "fleetdm_user" "test" {
  name        = %[1]q
  email       = "len-test@example.com"
  password    = "TestPassword1234!"
  global_role = "observer"
}
`, strings.Repeat("n", 256)),
				ExpectError: regexp.MustCompile(`(?s)at\s+most\s+255`),
			},
		},
	})
}
