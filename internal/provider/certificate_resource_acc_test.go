package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// testAccCertificateImportID builds the "fleet_id:id" composite import ID from
// the resource's current state.
func testAccCertificateImportID(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", resourceName)
		}
		return fmt.Sprintf("%s:%s", rs.Primary.Attributes["fleet_id"], rs.Primary.Attributes["id"]), nil
	}
}

// TestAccCertificateResource_basic exercises a certificate template against a
// live Fleet instance, on a custom SCEP certificate authority created in the
// same configuration and pointed at the in-test mock SCEP responder so Fleet's
// save-time URL validation succeeds.
//
// The template depends on the authority through certificate_authority_id, so
// Terraform destroys it first — which is required: Fleet refuses to delete an
// authority while templates still reference it.
func TestAccCertificateResource_basic(t *testing.T) {
	scepURL := startMockSCEPServer(t)
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlpha)
	caName := "TFTEST_CERTCA_" + suffix
	name := "TFTEST_CERT_" + suffix
	renamed := name + "_RENAMED"

	config := func(templateName, subject string) string {
		return providerConfig() + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  custom_scep_proxy = {
    name      = %q
    url       = %q
    challenge = "fake-challenge-for-acceptance-test"
  }
}

resource "fleetdm_certificate" "test" {
  name                     = %q
  certificate_authority_id = fleetdm_certificate_authority.test.id
  subject_name             = %q
  subject_alternative_name = "DNS=host.example.com,UPN=$FLEET_VAR_HOST_END_USER_IDP_USERNAME"
}
`, caName, scepURL, templateName, subject)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(name, "CN=$FLEET_VAR_HOST_HARDWARE_SERIAL,O=Example"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fleetdm_certificate.test", "id"),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "fleet_id", "0"),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "name", name),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "subject_name", "CN=$FLEET_VAR_HOST_HARDWARE_SERIAL,O=Example"),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "subject_alternative_name", "DNS=host.example.com,UPN=$FLEET_VAR_HOST_END_USER_IDP_USERNAME"),
					// Only Fleet's read route reports these, so their presence
					// proves Create read the template back.
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "certificate_authority_name", caName),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "certificate_authority_type", "custom_scep_proxy"),
					resource.TestCheckResourceAttrSet("fleetdm_certificate.test", "created_at"),
					resource.TestCheckResourceAttrPair(
						"fleetdm_certificate.test", "certificate_authority_id",
						"fleetdm_certificate_authority.test", "id",
					),
				),
			},
			{
				// Nothing may drift on refresh, including fleet_id — which no
				// Fleet response carries.
				Config: config(name, "CN=$FLEET_VAR_HOST_HARDWARE_SERIAL,O=Example"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Fleet has no update endpoint, so a rename is a replacement.
				Config: config(renamed, "CN=$FLEET_VAR_HOST_UUID"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_certificate.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "name", renamed),
					resource.TestCheckResourceAttr("fleetdm_certificate.test", "subject_name", "CN=$FLEET_VAR_HOST_UUID"),
				),
			},
			{
				ResourceName:      "fleetdm_certificate.test",
				ImportState:       true,
				ImportStateIdFunc: testAccCertificateImportID("fleetdm_certificate.test"),
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccCertificateResource_inFleet covers a template scoped to a real fleet.
// Fleet never returns the template's fleet, so this is what proves fleet_id
// round-trips through create, refresh and a composite import.
func TestAccCertificateResource_inFleet(t *testing.T) {
	scepURL := startMockSCEPServer(t)
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlpha)
	caName := "TFTEST_CERTCAF_" + suffix
	fleetName := "TFTEST_CERTFLEET_" + suffix
	name := "TFTEST_CERTF_" + suffix

	config := providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q
}

resource "fleetdm_certificate_authority" "test" {
  custom_scep_proxy = {
    name      = %q
    url       = %q
    challenge = "fake-challenge-for-acceptance-test"
  }
}

resource "fleetdm_certificate" "test" {
  fleet_id                 = fleetdm_fleet.test.id
  name                     = %q
  certificate_authority_id = fleetdm_certificate_authority.test.id
  subject_name             = "CN=$FLEET_VAR_HOST_HARDWARE_SERIAL"
}

data "fleetdm_certificates" "in_fleet" {
  fleet_id   = fleetdm_fleet.test.id
  depends_on = [fleetdm_certificate.test]
}
`, fleetName, caName, scepURL, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"fleetdm_certificate.test", "fleet_id",
						"fleetdm_fleet.test", "id",
					),
					// No subject alternative name configured must read back as
					// absent, not as an empty string.
					resource.TestCheckNoResourceAttr("fleetdm_certificate.test", "subject_alternative_name"),
					// The fleet-scoped listing sees it.
					resource.TestCheckResourceAttr("data.fleetdm_certificates.in_fleet", "certificates.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_certificates.in_fleet", "certificates.0.name", name),
					resource.TestCheckResourceAttr("data.fleetdm_certificates.in_fleet", "certificates.0.certificate_authority_name", caName),
				),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "fleetdm_certificate.test",
				ImportState:       true,
				ImportStateIdFunc: testAccCertificateImportID("fleetdm_certificate.test"),
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccCertificateResource_rejectsNonSCEPAuthority checks Fleet's save-time
// authority type check against a live server: only custom_scep_proxy is
// supported on 4.90, and an EST authority is refused.
func TestAccCertificateResource_rejectsNonSCEPAuthority(t *testing.T) {
	estURL := startMockESTServer(t)
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlpha)
	caName := "TFTEST_CERTEST_" + suffix

	config := providerConfig() + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "est" {
  custom_est_proxy = {
    name     = %q
    url      = %q
    username = "fake-user"
    password = "fake-password-for-acceptance-test"
  }
}

resource "fleetdm_certificate" "test" {
  name                     = "TFTEST_CERT_%s"
  certificate_authority_id = fleetdm_certificate_authority.est.id
  subject_name             = "CN=x"
}
`, caName, estURL, suffix)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`(?s)Currently,\s+only\s+the\s+custom_scep_proxy\s+certificate\s+authority\s+is\s+supported`),
			},
		},
	})
}
