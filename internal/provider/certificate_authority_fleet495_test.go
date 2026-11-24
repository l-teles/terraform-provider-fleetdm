package provider

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// Fake credentials only: base64 of "fake-p12" and a service account key
// without any key material.
const (
	fakeClientP12   = "ZmFrZS1wMTI="
	fakeP12Password = "fake-p12-password" // #nosec G101 -- test fixture, not a credential
	fakeGoogleKey   = `{"type":"service_account","project_id":"example-project","client_email":"fake@example-project.iam.gserviceaccount.com"}`
	fakeGoogleKey2  = `{"type":"service_account","project_id":"example-project","client_email":"fake2@example-project.iam.gserviceaccount.com"}`
)

// checkWriteRaw asserts the exact JSON body of write number n (0-based) of the
// given method.
func checkWriteRaw(t *testing.T, f *fakeCAServer, method string, n int, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		t.Helper()
		var matched []caWrite
		for _, w := range f.recordedWrites() {
			if w.method == method {
				matched = append(matched, w)
			}
		}
		if n >= len(matched) {
			return fmt.Errorf("want %s write #%d, only %d recorded", method, n, len(matched))
		}
		if got := matched[n].raw; got != want {
			return fmt.Errorf("%s body #%d mismatch:\n got: %s\nwant: %s", method, n, got, want)
		}
		return nil
	}
}

// TestCertificateAuthorityResource_ejbcaLifecycle covers the EJBCA wire shape
// (server/fleet/ejbca.go EJBCA and EJBCAUpdatePayload), no drift although Fleet
// never returns the PKCS#12 or its password, an in-place update that clears the
// optional fields, and import.
func TestCertificateAuthorityResource_ejbcaLifecycle(t *testing.T) {
	server, fake := newFakeCAServer()
	defer server.Close()
	const addr = "fleetdm_certificate_authority.test"

	full := testCAProviderBlock(server.URL) + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  ejbca = {
    name                             = "EJBCA_TEST"
    url                              = "https://ejbca.example.test"
    client_p12                       = %q
    password                         = %q
    trust_ca_bundle                  = "fake-ca-bundle"
    certificate_authority_name       = "FakeCA"
    certificate_profile_name         = "FakeProfile"
    end_entity_profile_name          = "FakeEndEntity"
    certificate_common_name          = "$FLEET_VAR_HOST_HARDWARE_SERIAL"
    username_template                = "fleet-$FLEET_VAR_HOST_HARDWARE_SERIAL"
    certificate_user_principal_names = ["upn@example.test"]
  }
}
`, fakeClientP12, fakeP12Password)

	minimal := testCAProviderBlock(server.URL) + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  ejbca = {
    name                       = "EJBCA_TEST"
    url                        = "https://ejbca2.example.test"
    client_p12                 = %q
    password                   = %q
    certificate_authority_name = "FakeCA"
    certificate_profile_name   = "FakeProfile"
    end_entity_profile_name    = "FakeEndEntity"
    certificate_common_name    = "$FLEET_VAR_HOST_HARDWARE_SERIAL"
  }
}
`, fakeClientP12, fakeP12Password)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: full,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkWriteRaw(t, fake, http.MethodPost, 0, `{"ejbca":{"name":"EJBCA_TEST","url":"https://ejbca.example.test",`+
						`"client_p12":"ZmFrZS1wMTI=","password":"fake-p12-password","trust_ca_bundle":"fake-ca-bundle",`+
						`"certificate_authority_name":"FakeCA","certificate_profile_name":"FakeProfile",`+
						`"end_entity_profile_name":"FakeEndEntity","certificate_common_name":"$FLEET_VAR_HOST_HARDWARE_SERIAL",`+
						`"username_template":"fleet-$FLEET_VAR_HOST_HARDWARE_SERIAL","certificate_user_principal_names":["upn@example.test"]}}`),
					resource.TestCheckResourceAttr(addr, "type", "ejbca"),
					resource.TestCheckResourceAttr(addr, "name", "EJBCA_TEST"),
					// Secrets survive a read that omits them.
					resource.TestCheckResourceAttr(addr, "ejbca.client_p12", fakeClientP12),
					resource.TestCheckResourceAttr(addr, "ejbca.password", fakeP12Password),
					resource.TestCheckResourceAttr(addr, "ejbca.trust_ca_bundle", "fake-ca-bundle"),
					resource.TestCheckResourceAttr(addr, "ejbca.certificate_user_principal_names.0", "upn@example.test"),
				),
			},
			{
				Config: full,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Dropping the optional fields clears them; the unchanged name is omitted.
				Config: minimal,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkWriteRaw(t, fake, http.MethodPatch, 0, `{"ejbca":{"url":"https://ejbca2.example.test",`+
						`"client_p12":"ZmFrZS1wMTI=","password":"fake-p12-password","trust_ca_bundle":"",`+
						`"certificate_authority_name":"FakeCA","certificate_profile_name":"FakeProfile",`+
						`"end_entity_profile_name":"FakeEndEntity","certificate_common_name":"$FLEET_VAR_HOST_HARDWARE_SERIAL",`+
						`"username_template":"","certificate_user_principal_names":[]}}`),
					resource.TestCheckResourceAttr(addr, "id", "1"),
					resource.TestCheckNoResourceAttr(addr, "ejbca.trust_ca_bundle"),
					resource.TestCheckNoResourceAttr(addr, "ejbca.username_template"),
				),
			},
			{
				Config: minimal,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:            addr,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"ejbca.client_p12", "ejbca.password"},
			},
		},
	})
}

// TestCertificateAuthorityResource_googleCASLifecycle covers the Google CAS
// wire shape (server/fleet/google_cas.go GoogleCASCA and
// GoogleCASCAUpdatePayload), no drift against the masked key, an in-place
// update, a rename, and import.
func TestCertificateAuthorityResource_googleCASLifecycle(t *testing.T) {
	server, fake := newFakeCAServer()
	defer server.Close()
	const addr = "fleetdm_certificate_authority.test"

	config := func(name, pool, key string) string {
		return testCAProviderBlock(server.URL) + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  google_cas = {
    name         = %q
    project      = "example-project"
    location     = "us-central1"
    ca_pool      = %q
    api_key_json = %q
  }
}
`, name, pool, key)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("GCAS_TEST", "fake-pool", fakeGoogleKey),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkWriteRaw(t, fake, http.MethodPost, 0, `{"google_cas":{"name":"GCAS_TEST","project":"example-project",`+
						`"location":"us-central1","ca_pool":"fake-pool","api_key_json":`+fmt.Sprintf("%q", fakeGoogleKey)+`}}`),
					resource.TestCheckResourceAttr(addr, "type", "google_cas"),
					resource.TestCheckResourceAttr(addr, "google_cas.project", "example-project"),
					// The masked read must not overwrite the configured key.
					resource.TestCheckResourceAttr(addr, "google_cas.api_key_json", fakeGoogleKey),
				),
			},
			{
				Config: config("GCAS_TEST", "fake-pool", fakeGoogleKey),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: config("GCAS_TEST", "fake-pool-2", fakeGoogleKey2),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkWriteRaw(t, fake, http.MethodPatch, 0, `{"google_cas":{"project":"example-project",`+
						`"location":"us-central1","ca_pool":"fake-pool-2","api_key_json":`+fmt.Sprintf("%q", fakeGoogleKey2)+`}}`),
					resource.TestCheckResourceAttr(addr, "id", "1"),
					resource.TestCheckResourceAttr(addr, "google_cas.api_key_json", fakeGoogleKey2),
				),
			},
			{
				Config: config("GCAS_RENAMED", "fake-pool-2", fakeGoogleKey2),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkWriteCarried(t, fake, http.MethodPatch, map[string]interface{}{"name": "GCAS_RENAMED"}),
					resource.TestCheckResourceAttr(addr, "name", "GCAS_RENAMED"),
				),
			},
			{
				ResourceName:            addr,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"google_cas.api_key_json"},
			},
		},
	})
}

// TestCertificateAuthorityResource_fleet495WriteOnly covers the write-only
// credentials of both new types: they reach Fleet, stay out of state, and a
// non-secret EJBCA update re-sends the PKCS#12 and password Fleet requires
// together.
func TestCertificateAuthorityResource_fleet495WriteOnly(t *testing.T) {
	server, fake := newFakeCAServer()
	defer server.Close()
	const addr = "fleetdm_certificate_authority.test"

	ejbca := func(profile string) string {
		return testCAProviderBlock(server.URL) + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  secrets_wo_version = 1
  ejbca = {
    name                       = "EJBCA_TEST"
    url                        = "https://ejbca.example.test"
    client_p12_wo              = %q
    password_wo                = %q
    certificate_authority_name = "FakeCA"
    certificate_profile_name   = %q
    end_entity_profile_name    = "FakeEndEntity"
    certificate_common_name    = "fake-cn"
  }
}
`, fakeClientP12, fakeP12Password, profile)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		Steps: []resource.TestStep{
			{
				Config: ejbca("FakeProfile"),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkWriteCarried(t, fake, http.MethodPost, map[string]interface{}{
						"client_p12": fakeClientP12, "password": fakeP12Password,
					}),
					resource.TestCheckNoResourceAttr(addr, "ejbca.client_p12"),
					resource.TestCheckNoResourceAttr(addr, "ejbca.client_p12_wo"),
					resource.TestCheckNoResourceAttr(addr, "ejbca.password"),
					resource.TestCheckNoResourceAttr(addr, "ejbca.password_wo"),
				),
			},
			{
				Config: ejbca("FakeProfile2"),
				Check: checkWriteCarried(t, fake, http.MethodPatch, map[string]interface{}{
					"certificate_profile_name": "FakeProfile2", "client_p12": fakeClientP12, "password": fakeP12Password,
				}),
			},
			{
				Config: testCAProviderBlock(server.URL) + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  secrets_wo_version = 1
  google_cas = {
    name            = "GCAS_TEST"
    project         = "example-project"
    location        = "us-central1"
    ca_pool         = "fake-pool"
    api_key_json_wo = %q
  }
}
`, fakeGoogleKey),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkWriteCarried(t, fake, http.MethodPost, map[string]interface{}{"api_key_json": fakeGoogleKey}),
					resource.TestCheckNoResourceAttr(addr, "google_cas.api_key_json"),
					resource.TestCheckNoResourceAttr(addr, "google_cas.api_key_json_wo"),
				),
			},
		},
	})
}

// TestCertificateAuthorityResource_fleet495Validation checks the plan-time
// mirrors of Fleet's EJBCA and Google CAS validation.
func TestCertificateAuthorityResource_fleet495Validation(t *testing.T) {
	ejbca := func(name, p12, usernameTemplate string) string {
		ut := ""
		if usernameTemplate != "" {
			ut = fmt.Sprintf("username_template = %q", usernameTemplate)
		}
		return fmt.Sprintf(`ejbca = {
    name                       = %q
    url                        = "https://ejbca.example.test"
    client_p12                 = %q
    password                   = "fake-p12-password"
    certificate_authority_name = "FakeCA"
    certificate_profile_name   = "FakeProfile"
    end_entity_profile_name    = "FakeEndEntity"
    certificate_common_name    = "fake-cn"
    %s
  }`, name, p12, ut)
	}
	gcas := func(project, location, pool, key string) string {
		return fmt.Sprintf(`google_cas = {
    name         = "GCAS_TEST"
    project      = %q
    location     = %q
    ca_pool      = %q
    api_key_json = %q
  }`, project, location, pool, key)
	}

	for _, tc := range []struct {
		name  string
		block string
		want  string
	}{
		{"ejbca name NDES", ejbca("ndes", fakeClientP12, ""), `value must be none of`},
		{"ejbca p12 not base64", ejbca("EJBCA_TEST", "not base64!", ""), `base64-encoded PKCS#12`},
		{"ejbca masked p12", ejbca("EJBCA_TEST", "********", ""), `Invalid Certificate Authority Secret`},
		{"ejbca blank username template", ejbca("EJBCA_TEST", fakeClientP12, "   "), `must not be only whitespace`},
		{"gcas project", gcas("Bad_Project", "us-central1", "fake-pool", fakeGoogleKey), `lowercase letters, numbers, hyphens, periods and colons`},
		{"gcas location", gcas("example-project", "US", "fake-pool", fakeGoogleKey), `lowercase letters, numbers and hyphens`},
		{"gcas pool", gcas("example-project", "us-central1", "fake/pool", fakeGoogleKey), `letters, numbers, hyphens and underscores`},
		{"gcas key not json", gcas("example-project", "us-central1", "fake-pool", "not-json"), `service account key file's JSON object`},
		{"gcas key null", gcas("example-project", "us-central1", "fake-pool", "null"), `service account key file's JSON object`},
		{"gcas masked key", gcas("example-project", "us-central1", "fake-pool", "********"), `Invalid Certificate Authority Secret`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, fake := newFakeCAServer()
			defer server.Close()

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: testCAProviderBlock(server.URL) + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  %s
}
`, tc.block),
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(strings.ReplaceAll(regexp.QuoteMeta(tc.want), " ", `\s+`)),
					},
				},
			})
			if writes := fake.recordedWrites(); len(writes) != 0 {
				t.Errorf("no write expected, got %d", len(writes))
			}
		})
	}
}

// TestJSONObjectValidatorDoesNotEchoKey guards the validator's promise that a
// malformed key never appears in the diagnostic.
func TestJSONObjectValidatorDoesNotEchoKey(t *testing.T) {
	const secretish = `{"private_key": "fake-key-material-0xdeadbeef"`
	for _, value := range []string{secretish, "null", "fake-key-material-0xdeadbeef"} {
		resp := &validator.StringResponse{}
		jsonObjectValidator{}.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("api_key_json"),
			ConfigValue: types.StringValue(value),
		}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected an error for %q", value)
		}
		for _, d := range resp.Diagnostics {
			if strings.Contains(d.Summary()+d.Detail(), "fake-key-material") {
				t.Errorf("diagnostic echoes the key: %s %s", d.Summary(), d.Detail())
			}
		}
	}
}
