package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestCertificateAuthoritiesDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/fleet/certificate_authorities" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"certificate_authorities": []map[string]interface{}{
				{"id": 4, "name": "SCEP_TEST", "type": "custom_scep_proxy"},
				{"id": 9, "name": "NDES", "type": "ndes_scep_proxy"},
			},
		})
	}))
	defer server.Close()

	config := testCAProviderBlock(server.URL) + `
data "fleetdm_certificate_authorities" "all" {}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_certificate_authorities.all", "certificate_authorities.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_certificate_authorities.all", "certificate_authorities.0.id", "4"),
					resource.TestCheckResourceAttr("data.fleetdm_certificate_authorities.all", "certificate_authorities.0.name", "SCEP_TEST"),
					resource.TestCheckResourceAttr("data.fleetdm_certificate_authorities.all", "certificate_authorities.0.type", "custom_scep_proxy"),
					resource.TestCheckResourceAttr("data.fleetdm_certificate_authorities.all", "certificate_authorities.1.name", "NDES"),
					resource.TestCheckResourceAttr("data.fleetdm_certificate_authorities.all", "certificate_authorities.1.type", "ndes_scep_proxy"),
				),
			},
		},
	})
}

func TestCertificateAuthoritiesDataSource_empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"certificate_authorities": []interface{}{}})
	}))
	defer server.Close()

	config := testCAProviderBlock(server.URL) + `
data "fleetdm_certificate_authorities" "all" {}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("data.fleetdm_certificate_authorities.all", "certificate_authorities.#", "0"),
			},
		},
	})
}

func TestCertificateAuthoritiesDataSource_error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Requires Fleet Premium license",
		})
	}))
	defer server.Close()

	config := testCAProviderBlock(server.URL) + `
data "fleetdm_certificate_authorities" "all" {}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`Unable to Read FleetDM Certificate Authorities`),
			},
		},
	})
}
