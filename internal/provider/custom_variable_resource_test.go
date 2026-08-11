package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// customVariableTestCharSet is the character set for random test names. Fleet
// only accepts uppercase letters, digits and underscores in custom variable
// names, so acctest.CharSetAlphaNum (which includes lowercase) cannot be used.
const customVariableTestCharSet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// ---------------------------------------------------------------------------
// Unit tests — no Terraform CLI, no Fleet server, no sockets.
// ---------------------------------------------------------------------------

// TestCustomVariableNameFormat pins the provider's name regex to the rule Fleet
// enforces server-side on v4.90.0 (`^[A-Z0-9_]+$`). Divergence here means either
// a valid name rejected at plan time or a 422 surfacing mid-apply.
func TestCustomVariableNameFormat(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"MY_VAR", true},
		{"MYVAR", true},
		{"MY_VAR_9", true},
		{"_LEADING_UNDERSCORE", true},
		{"1LEADING_DIGIT", true},
		{"my_var", false},
		{"My_Var", false},
		{"MY-VAR", false},
		{"MY.VAR", false},
		{"MY VAR", false},
		{"MY$VAR", false},
		{"MY_VARÉ", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := customVariableNameFormat.MatchString(tt.name); got != tt.valid {
			t.Errorf("customVariableNameFormat.MatchString(%q) = %v, want %v", tt.name, got, tt.valid)
		}
	}
}

// TestCustomVariableNameNotReserved covers the provider-side guard on Fleet's
// reserved reference prefix. Fleet's create endpoint accepts such a name (it
// stores it verbatim) while its spec endpoint strips the prefix, so the provider
// refuses it rather than manage a name whose meaning depends on which endpoint
// wrote it.
func TestCustomVariableNameNotReserved(t *testing.T) {
	tests := []struct {
		name      string
		value     types.String
		wantError bool
	}{
		{"plain name", types.StringValue("MY_VAR"), false},
		{"name merely containing the prefix", types.StringValue("APP_FLEET_SECRET_X"), false},
		{"reserved prefix", types.StringValue("FLEET_SECRET_MY_VAR"), true},
		{"reserved prefix alone", types.StringValue("FLEET_SECRET_"), true},
		{"null", types.StringNull(), false},
		{"unknown", types.StringUnknown(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validator.StringRequest{
				Path:        path.Root("name"),
				ConfigValue: tt.value,
			}
			resp := &validator.StringResponse{}

			customVariableNameNotReserved{}.ValidateString(context.Background(), req, resp)

			if got := resp.Diagnostics.HasError(); got != tt.wantError {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, tt.wantError, resp.Diagnostics)
			}
			if tt.wantError {
				// The message must tell the practitioner what to do, not just
				// that the name is wrong.
				detail := resp.Diagnostics.Errors()[0].Detail()
				if !regexp.MustCompile(`Drop the prefix`).MatchString(detail) {
					t.Errorf("expected actionable guidance in the error detail, got: %s", detail)
				}
			}
		})
	}
}

func TestCustomVariableNameNotReserved_Descriptions(t *testing.T) {
	v := customVariableNameNotReserved{}
	if got := v.Description(context.Background()); got == "" {
		t.Error("expected a non-empty Description")
	}
	if got := v.MarkdownDescription(context.Background()); got == "" {
		t.Error("expected a non-empty MarkdownDescription")
	}
}

func TestTimestampOrNull(t *testing.T) {
	if got := timestampOrNull(""); !got.IsNull() {
		t.Errorf("expected an empty timestamp to map to null, got: %v", got)
	}
	got := timestampOrNull("2026-08-11T09:00:00Z")
	if got.IsNull() || got.ValueString() != "2026-08-11T09:00:00Z" {
		t.Errorf("expected the timestamp to be preserved, got: %v", got)
	}
}

// ---------------------------------------------------------------------------
// Acceptance tests — require a live Fleet with FLEET_SERVER_PRIVATE_KEY set.
// ---------------------------------------------------------------------------

// TestAccCustomVariableResource_basic covers create, read, import and delete
// using the state-stored `value` attribute.
func TestAccCustomVariableResource_basic(t *testing.T) {
	name := "TF_ACC_" + acctest.RandStringFromCharSet(10, customVariableTestCharSet)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: testAccCustomVariableResourceConfig(name, "tf-acc-fake-value"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_custom_variable.test", "name", name),
					resource.TestCheckResourceAttr("fleetdm_custom_variable.test", "value", "tf-acc-fake-value"),
					resource.TestCheckResourceAttrSet("fleetdm_custom_variable.test", "id"),
					resource.TestCheckResourceAttrSet("fleetdm_custom_variable.test", "created_at"),
					resource.TestCheckResourceAttrSet("fleetdm_custom_variable.test", "updated_at"),
					// Fleet never returns the value, so the write-only companion
					// must be absent from state whether or not it was used.
					resource.TestCheckNoResourceAttr("fleetdm_custom_variable.test", "value_wo"),
				),
			},
			// A second plan against the same config must be empty: the read is
			// state-preserving, so it must not surface the unreadable value as
			// drift.
			{
				Config: testAccCustomVariableResourceConfig(name, "tf-acc-fake-value"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			// ImportState by name. The value cannot be imported — Fleet has no
			// endpoint that returns it — so the value attributes are ignored.
			{
				ResourceName:            "fleetdm_custom_variable.test",
				ImportState:             true,
				ImportStateId:           name,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"value", "value_wo", "value_wo_version"},
			},
		},
	})
}

// TestAccCustomVariableResource_valueRotation checks that changing `value`
// rotates in place rather than replacing. Replacement would be unusable for a
// variable referenced by a script or profile, because Fleet refuses to delete
// one that is still in use.
func TestAccCustomVariableResource_valueRotation(t *testing.T) {
	name := "TF_ACC_" + acctest.RandStringFromCharSet(10, customVariableTestCharSet)

	// Captured in step 1 and compared in step 2. Fleet advances updated_at on
	// every value write, so this is the only server-side evidence available that
	// the rotation actually reached Fleet rather than Terraform merely
	// re-recording the configured value in state.
	var createdAt, updatedAtBefore string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCustomVariableResourceConfig(name, "tf-acc-fake-value"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_custom_variable.test", "value", "tf-acc-fake-value"),
					captureCustomVariableAttr("fleetdm_custom_variable.test", "created_at", &createdAt),
					captureCustomVariableAttr("fleetdm_custom_variable.test", "updated_at", &updatedAtBefore),
				),
			},
			{
				Config: testAccCustomVariableResourceConfig(name, "tf-acc-fake-value-rotated"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_custom_variable.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_custom_variable.test", "name", name),
					resource.TestCheckResourceAttr("fleetdm_custom_variable.test", "value", "tf-acc-fake-value-rotated"),
					// An in-place rotation, not a replacement: same id, same
					// created_at, later updated_at.
					resource.TestCheckResourceAttrWith("fleetdm_custom_variable.test", "created_at", func(v string) error {
						if v != createdAt {
							return fmt.Errorf("created_at changed from %q to %q — the variable was replaced, not rotated", createdAt, v)
						}
						return nil
					}),
					resource.TestCheckResourceAttrWith("fleetdm_custom_variable.test", "updated_at", func(v string) error {
						if v == updatedAtBefore {
							return fmt.Errorf("updated_at is still %q — Fleet did not record a new value write", v)
						}
						return nil
					}),
				),
			},
		},
	})
}

// captureCustomVariableAttr stores an attribute value from the current state so
// a later step can compare against it.
func captureCustomVariableAttr(resourceName, attr string, dest *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceName)
		}
		value, ok := rs.Primary.Attributes[attr]
		if !ok {
			return fmt.Errorf("attribute %s not set on %s", attr, resourceName)
		}
		*dest = value
		return nil
	}
}

// TestAccCustomVariableResource_nameRequiresReplace confirms a rename replaces
// the resource — Fleet has no rename endpoint.
func TestAccCustomVariableResource_nameRequiresReplace(t *testing.T) {
	firstName := "TF_ACC_" + acctest.RandStringFromCharSet(10, customVariableTestCharSet)
	secondName := "TF_ACC_" + acctest.RandStringFromCharSet(10, customVariableTestCharSet)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCustomVariableResourceConfig(firstName, "tf-acc-fake-value"),
			},
			{
				Config: testAccCustomVariableResourceConfig(secondName, "tf-acc-fake-value"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_custom_variable.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_custom_variable.test", "name", secondName),
				),
			},
		},
	})
}

// TestAccCustomVariableResource_writeOnly exercises the write-only path. Skipped
// below Terraform 1.11, which is the first release that supports write-only
// attributes — the CI matrix still includes 1.5.
func TestAccCustomVariableResource_writeOnly(t *testing.T) {
	name := "TF_ACC_" + acctest.RandStringFromCharSet(10, customVariableTestCharSet)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			// Create with a write-only value.
			{
				Config: testAccCustomVariableResourceConfigWriteOnly(name, "tf-acc-fake-wo-value", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_custom_variable.test", "name", name),
					resource.TestCheckResourceAttr("fleetdm_custom_variable.test", "value_wo_version", "1"),
					resource.TestCheckResourceAttrSet("fleetdm_custom_variable.test", "id"),
					// The whole point of the write-only path: the secret is in
					// neither the plan nor the state.
					resource.TestCheckNoResourceAttr("fleetdm_custom_variable.test", "value_wo"),
					resource.TestCheckNoResourceAttr("fleetdm_custom_variable.test", "value"),
				),
			},
			// Changing value_wo alone must not produce a diff: Terraform cannot
			// see a write-only value, so there is nothing to detect.
			{
				Config: testAccCustomVariableResourceConfigWriteOnly(name, "tf-acc-fake-wo-value-changed", 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			// Bumping the version rotates the value in place.
			{
				Config: testAccCustomVariableResourceConfigWriteOnly(name, "tf-acc-fake-wo-value-rotated", 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_custom_variable.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_custom_variable.test", "value_wo_version", "2"),
					resource.TestCheckNoResourceAttr("fleetdm_custom_variable.test", "value_wo"),
				),
			},
			// Import: nothing value-shaped can come back.
			{
				ResourceName:            "fleetdm_custom_variable.test",
				ImportState:             true,
				ImportStateId:           name,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"value", "value_wo", "value_wo_version"},
			},
		},
	})
}

// TestAccCustomVariableResource_reservedPrefixRejected checks the provider
// rejects Fleet's reserved reference prefix at plan time. Fleet itself accepts
// such a name, so this guard exists only in the provider.
func TestAccCustomVariableResource_reservedPrefixRejected(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccCustomVariableResourceConfig("FLEET_SECRET_TF_ACC", "tf-acc-fake-value"),
				ExpectError: regexp.MustCompile(`Reserved Custom Variable Name Prefix`),
			},
		},
	})
}

func TestAccCustomVariableResource_invalidNameRejected(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccCustomVariableResourceConfig("tf-acc-lowercase", "tf-acc-fake-value"),
				ExpectError: regexp.MustCompile(`must contain only uppercase letters`),
			},
		},
	})
}

// TestAccCustomVariableResource_valueRequired covers the ExactlyOneOf validator
// when neither value source is configured.
func TestAccCustomVariableResource_valueRequired(t *testing.T) {
	name := "TF_ACC_" + acctest.RandStringFromCharSet(10, customVariableTestCharSet)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "fleetdm_custom_variable" "test" {
  name = %[1]q
}
`, name),
				ExpectError: regexp.MustCompile(`Exactly one of these attributes must be configured`),
			},
		},
	})
}

// TestAccCustomVariableResource_valueAndValueWOConflict covers the ExactlyOneOf
// validator when both value sources are configured.
func TestAccCustomVariableResource_valueAndValueWOConflict(t *testing.T) {
	name := "TF_ACC_" + acctest.RandStringFromCharSet(10, customVariableTestCharSet)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "fleetdm_custom_variable" "test" {
  name     = %[1]q
  value    = "tf-acc-fake-value"
  value_wo = "tf-acc-fake-wo-value"
}
`, name),
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
		},
	})
}

// TestAccCustomVariableResource_valueWithVersionConflict checks that pairing the
// state-stored value with the write-only version counter is rejected rather than
// silently ignored.
func TestAccCustomVariableResource_valueWithVersionConflict(t *testing.T) {
	name := "TF_ACC_" + acctest.RandStringFromCharSet(10, customVariableTestCharSet)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "fleetdm_custom_variable" "test" {
  name             = %[1]q
  value            = "tf-acc-fake-value"
  value_wo_version = 1
}
`, name),
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
		},
	})
}

// TestAccCustomVariableResource_importNotFound checks the import error path for
// a name that does not exist in Fleet.
func TestAccCustomVariableResource_importNotFound(t *testing.T) {
	name := "TF_ACC_" + acctest.RandStringFromCharSet(10, customVariableTestCharSet)
	missing := "TF_ACC_MISSING_" + acctest.RandStringFromCharSet(10, customVariableTestCharSet)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCustomVariableResourceConfig(name, "tf-acc-fake-value"),
			},
			{
				ResourceName:  "fleetdm_custom_variable.test",
				ImportState:   true,
				ImportStateId: missing,
				ExpectError:   regexp.MustCompile(`No custom variable named`),
			},
			{
				ResourceName:  "fleetdm_custom_variable.test",
				ImportState:   true,
				ImportStateId: "not-a-valid-name",
				ExpectError:   regexp.MustCompile(`Invalid Import ID`),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Mock-server tests — no live Fleet, but they do need the Terraform CLI, so
// they run under the same TF_ACC gate as the acceptance tests.
// ---------------------------------------------------------------------------

// TestAccCustomVariableResource_createReadBackFailurePersistsState covers the
// orphaned-secret path: the create succeeds, then the read-back that fetches the
// timestamps fails.
//
// The variable exists in Fleet at that point, so Create must still write state.
// Returning an error instead would leave a secret in Fleet that Terraform does
// not track and cannot recreate later, because the name is already taken.
func TestAccCustomVariableResource_createReadBackFailurePersistsState(t *testing.T) {
	const name = "TF_MOCK_ORPHAN_GUARD"

	var listCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/fleet/custom_variables" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"id": 11, "name": name})

		case r.URL.Path == "/api/v1/fleet/custom_variables" && r.Method == http.MethodGet:
			listCalls++
			// Fail the post-create read-back, then recover so the test's own
			// refresh and destroy can proceed.
			if listCalls == 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, `{"message":"transient backend failure"}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"custom_variables": []map[string]interface{}{
					{"id": 11, "name": name, "created_at": "2026-08-11T09:00:00Z", "updated_at": "2026-08-11T09:00:00Z"},
				},
				"meta":  map[string]interface{}{"has_next_results": false, "has_previous_results": false},
				"count": 1,
			})

		case r.URL.Path == "/api/v1/fleet/custom_variables/11" && r.Method == http.MethodDelete:
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{}`)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCustomVariableResourceMockConfig(server.URL, name, "tf-mock-fake-value"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// State was written despite the failed read-back — this is
					// the assertion that proves the secret is not orphaned.
					resource.TestCheckResourceAttr("fleetdm_custom_variable.test", "id", "11"),
					resource.TestCheckResourceAttr("fleetdm_custom_variable.test", "name", name),
					resource.TestCheckResourceAttr("fleetdm_custom_variable.test", "value", "tf-mock-fake-value"),
					// Timestamps degrade to null rather than failing the apply.
					resource.TestCheckNoResourceAttr("fleetdm_custom_variable.test", "created_at"),
					resource.TestCheckNoResourceAttr("fleetdm_custom_variable.test", "updated_at"),
				),
			},
		},
	})

	if listCalls == 0 {
		t.Error("expected the provider to attempt a post-create read-back")
	}
}

// TestAccCustomVariableResource_readRouteMissingErrors covers a 404 from the
// collection endpoint during refresh.
//
// Reads resolve the variable by listing, so a 404 is the route missing (Fleet
// older than 4.90), never the variable missing. Silently removing the resource
// from state would turn an unsupported-server problem into a spurious recreate
// plan.
func TestAccCustomVariableResource_readRouteMissingErrors(t *testing.T) {
	const name = "TF_MOCK_ROUTE_MISSING"

	var created bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/fleet/custom_variables" && r.Method == http.MethodPost:
			created = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"id": 12, "name": name})

		case r.URL.Path == "/api/v1/fleet/custom_variables" && r.Method == http.MethodGet:
			// Pre-4.90 Fleet: the route does not exist.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"Not Found"}`)

		case r.URL.Path == "/api/v1/fleet/custom_variables/12" && r.Method == http.MethodDelete:
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{}`)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccCustomVariableResourceMockConfig(server.URL, name, "tf-mock-fake-value"),
				ExpectError: regexp.MustCompile(`Custom Variables Not Supported By This Fleet Server`),
			},
		},
	})

	if !created {
		t.Error("expected the provider to reach the create call before the failing read")
	}
}

func testAccCustomVariableResourceMockConfig(serverURL, name, value string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_custom_variable" "test" {
  name  = %[2]q
  value = %[3]q
}
`, serverURL, name, value)
}

func testAccCustomVariableResourceConfig(name, value string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_custom_variable" "test" {
  name  = %[1]q
  value = %[2]q
}
`, name, value)
}

func testAccCustomVariableResourceConfigWriteOnly(name, value string, version int) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_custom_variable" "test" {
  name             = %[1]q
  value_wo         = %[2]q
  value_wo_version = %[3]d
}
`, name, value, version)
}
