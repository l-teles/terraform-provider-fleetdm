package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestCertificatesDataSource_readsOneFleet checks the data source is scoped to a
// single fleet and reports the list route's fields. It reuses the fake Fleet
// server from certificate_resource_test.go.
func TestCertificatesDataSource_readsOneFleet(t *testing.T) {
	server, _ := newFakeCertificatesServer()
	defer server.Close()

	config := testCertificateProviderBlock(server.URL) + `
resource "fleetdm_certificate" "no_fleet" {
  name                     = "NoFleet"
  certificate_authority_id = 3
  subject_name             = "CN=nofleet"
}

resource "fleetdm_certificate" "in_fleet" {
  fleet_id                 = 7
  name                     = "InFleet"
  certificate_authority_id = 3
  subject_name             = "CN=infleet"
  subject_alternative_name = "DNS=infleet.example.com"
}

data "fleetdm_certificates" "default_fleet" {
  depends_on = [fleetdm_certificate.no_fleet, fleetdm_certificate.in_fleet]
}

data "fleetdm_certificates" "fleet_seven" {
  fleet_id   = 7
  depends_on = [fleetdm_certificate.no_fleet, fleetdm_certificate.in_fleet]
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Omitting fleet_id reads fleet 0 only, never every fleet.
					resource.TestCheckResourceAttr("data.fleetdm_certificates.default_fleet", "certificates.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_certificates.default_fleet", "certificates.0.name", "NoFleet"),
					resource.TestCheckNoResourceAttr("data.fleetdm_certificates.default_fleet", "certificates.0.subject_alternative_name"),
					resource.TestCheckResourceAttr("data.fleetdm_certificates.default_fleet", "certificates.0.certificate_authority_name", "SCEP_TEST"),

					resource.TestCheckResourceAttr("data.fleetdm_certificates.fleet_seven", "certificates.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_certificates.fleet_seven", "certificates.0.name", "InFleet"),
					resource.TestCheckResourceAttr("data.fleetdm_certificates.fleet_seven", "certificates.0.subject_alternative_name", "DNS=infleet.example.com"),
					resource.TestCheckResourceAttr("data.fleetdm_certificates.fleet_seven", "certificates.0.subject_name", "CN=infleet"),
				),
			},
		},
	})
}
