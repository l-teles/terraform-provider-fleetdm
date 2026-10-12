package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// TestAccConfigurationResource_integrations covers the Fleet 4.93 certificate
// request controls: set, change, clear with [], and stop managing them, which
// must leave Fleet's values in place.
func TestAccConfigurationResource_integrations(t *testing.T) {
	// app config is a global singleton shared by the serial suite. Introspection
	// URLs left behind would make every certificate request need IdP
	// credentials, so reset them even when a step fails.
	if os.Getenv("TF_ACC") != "" {
		t.Cleanup(func() { resetCertificateIntegrationsOutOfBand(t) })
	}

	cfg := func(block string) string {
		return providerConfig() + fmt.Sprintf(`
resource "fleetdm_configuration" "test" {
  org_name = "Integrations Org"
  %s
}
`, block)
	}
	const attr = "fleetdm_configuration.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(`integrations = {
    certificates_idp_introspection_urls        = ["https://idp-a.example.test/introspect", "https://idp-b.example.test/introspect"]
    certificates_idp_client_ids                = ["tf-acc-client"]
    certificates_disable_host_end_user_binding = true
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(attr, "integrations.certificates_idp_introspection_urls.#", "2"),
					resource.TestCheckTypeSetElemAttr(attr, "integrations.certificates_idp_introspection_urls.*", "https://idp-b.example.test/introspect"),
					resource.TestCheckTypeSetElemAttr(attr, "integrations.certificates_idp_client_ids.*", "tf-acc-client"),
					resource.TestCheckResourceAttr(attr, "integrations.certificates_disable_host_end_user_binding", "true"),
				),
			},
			{
				Config: cfg(`integrations = {
    certificates_idp_introspection_urls        = ["https://idp-b.example.test/introspect", "https://idp-a.example.test/introspect"]
    certificates_idp_client_ids                = ["tf-acc-client"]
    certificates_disable_host_end_user_binding = true
  }`),
				PlanOnly: true,
			},
			{
				Config: cfg(`integrations = {
    certificates_idp_introspection_urls        = ["https://idp-a.example.test/introspect"]
    certificates_idp_client_ids                = []
    certificates_disable_host_end_user_binding = false
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(attr, "integrations.certificates_idp_introspection_urls.#", "1"),
					resource.TestCheckResourceAttr(attr, "integrations.certificates_idp_client_ids.#", "0"),
					resource.TestCheckResourceAttr(attr, "integrations.certificates_disable_host_end_user_binding", "false"),
				),
			},
			{
				// Dropping the block stops managing the settings without clearing them.
				Config: cfg(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(attr, "integrations"),
					checkCertificateIntrospectionURLsInFleet([]string{"https://idp-a.example.test/introspect"}),
				),
			},
		},
	})
}

func TestAccConfigurationResource_integrationsValidators(t *testing.T) {
	cfg := func(block string) string {
		return providerConfig() + fmt.Sprintf(`
resource "fleetdm_configuration" "test" {
  org_name     = "Integrations Org"
  integrations = {
    %s
  }
}
`, block)
	}
	invalidURL := regexp.MustCompile(`(?s)must\s+be\s+an\s+absolute\s+https\s+URL`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg(`certificates_idp_introspection_urls = ["http://idp.example.test/i"]`), ExpectError: invalidURL},
			{Config: cfg(`certificates_idp_introspection_urls = ["https://user:pw@idp.example.test/i"]`), ExpectError: invalidURL},
			{Config: cfg(`certificates_idp_introspection_urls = ["/relative"]`), ExpectError: invalidURL},
			{
				Config:      cfg(`certificates_idp_introspection_urls = [" https://idp.example.test/i"]`),
				ExpectError: regexp.MustCompile(`(?s)leading\s+or\s+trailing\s+whitespace`),
			},
			{
				Config:      cfg(`certificates_idp_client_ids = [""]`),
				ExpectError: regexp.MustCompile(`(?s)must\s+not\s+be\s+empty`),
			},
			{
				Config: cfg(`certificates_idp_introspection_urls = []
    certificates_idp_client_ids         = ["abc"]`),
				ExpectError: regexp.MustCompile(`(?s)requires\s+at\s+least\s+one\s+certificates_idp_introspection_urls`),
			},
		},
	})
}

// TestBuildConfigurationIntegrationsUpdate pins the wire format: only managed
// keys are sent, and an empty set goes out as [] so it clears the list.
func TestBuildConfigurationIntegrationsUpdate(t *testing.T) {
	ctx := context.Background()
	empty := types.SetValueMust(types.StringType, nil)
	tests := []struct {
		name  string
		model *configurationIntegrationsModel
		want  string // "" means no integrations object at all
	}{
		{name: "block not declared", model: nil, want: ""},
		{name: "nothing managed", model: &configurationIntegrationsModel{
			CertificatesIdPIntrospectionURLs: types.SetNull(types.StringType),
			CertificatesIdPClientIDs:         types.SetNull(types.StringType),
		}, want: ""},
		{name: "clear client ids only", model: &configurationIntegrationsModel{
			CertificatesIdPIntrospectionURLs: types.SetNull(types.StringType),
			CertificatesIdPClientIDs:         empty,
		}, want: `{"certificates_idp_client_ids":[]}`},
		{name: "binding false is sent", model: &configurationIntegrationsModel{
			CertificatesIdPIntrospectionURLs:      types.SetNull(types.StringType),
			CertificatesIdPClientIDs:              types.SetNull(types.StringType),
			CertificatesDisableHostEndUserBinding: types.BoolValue(false),
		}, want: `{"certificates_disable_host_end_user_binding":false}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diags diag.Diagnostics
			got := buildConfigurationIntegrationsUpdate(ctx, tt.model, &diags)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if tt.want == "" {
				if got != nil {
					t.Fatalf("expected no integrations object, got %+v", got)
				}
				return
			}
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != tt.want {
				t.Errorf("got %s, want %s", raw, tt.want)
			}
		})
	}
}

// TestRefreshConfigurationIntegrations checks that only managed attributes are
// refreshed and that a list Fleet reports as null becomes an empty set.
func TestRefreshConfigurationIntegrations(t *testing.T) {
	m := &configurationIntegrationsModel{
		CertificatesIdPIntrospectionURLs:      types.SetValueMust(types.StringType, nil),
		CertificatesIdPClientIDs:              types.SetNull(types.StringType),
		CertificatesDisableHostEndUserBinding: types.BoolValue(true),
	}
	var diags diag.Diagnostics
	refreshConfigurationIntegrations(m, &fleetdm.AppConfigIntegrations{
		CertificatesIdPIntrospectionURLs: nil,
		CertificatesIdPClientIDs:         []string{"set-in-the-ui"},
	}, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if m.CertificatesIdPIntrospectionURLs.IsNull() || len(m.CertificatesIdPIntrospectionURLs.Elements()) != 0 {
		t.Errorf("expected an empty set for a null list, got %v", m.CertificatesIdPIntrospectionURLs)
	}
	if !m.CertificatesIdPClientIDs.IsNull() {
		t.Errorf("unmanaged client ids must stay null, got %v", m.CertificatesIdPClientIDs)
	}
	if m.CertificatesDisableHostEndUserBinding.ValueBool() {
		t.Error("expected binding to read back false when Fleet reports none")
	}
}

func certificateIntegrationsTestClient(t *testing.T) *fleetdm.Client {
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
	return client
}

// checkCertificateIntrospectionURLsInFleet asserts Fleet's stored URLs
// directly, for steps where Terraform no longer manages them.
func checkCertificateIntrospectionURLsInFleet(want []string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		client, err := fleetdm.NewClient(fleetdm.ClientConfig{
			ServerAddress: os.Getenv("FLEETDM_URL"),
			APIKey:        os.Getenv("FLEETDM_API_TOKEN"),
			VerifyTLS:     os.Getenv("FLEETDM_VERIFY_TLS") != "false" && os.Getenv("FLEETDM_VERIFY_TLS") != "0",
		})
		if err != nil {
			return err
		}
		cfg, err := client.GetAppConfig(context.Background())
		if err != nil {
			return err
		}
		if cfg.Integrations == nil {
			return fmt.Errorf("Fleet returned no integrations object")
		}
		got := cfg.Integrations.CertificatesIdPIntrospectionURLs
		if fmt.Sprint(got) != fmt.Sprint(want) {
			return fmt.Errorf("Fleet stores introspection URLs %v, want %v", got, want)
		}
		return nil
	}
}

// resetCertificateIntegrationsOutOfBand restores Fleet's defaults for the
// certificate request controls.
func resetCertificateIntegrationsOutOfBand(t *testing.T) {
	t.Helper()
	empty := []string{}
	off := false
	_, err := certificateIntegrationsTestClient(t).UpdateAppConfig(context.Background(), &fleetdm.UpdateAppConfigRequest{
		Integrations: &fleetdm.IntegrationsUpdate{
			CertificatesIdPIntrospectionURLs:      &empty,
			CertificatesIdPClientIDs:              &empty,
			CertificatesDisableHostEndUserBinding: &off,
		},
	})
	if err != nil {
		t.Logf("failed to reset certificate integrations: %v", err)
	}
}
