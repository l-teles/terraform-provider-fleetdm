package provider

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// mockCACallbackHostEnv names the host Fleet should use to reach an in-test mock
// certificate authority. Fleet validates a CA's URL when the CA is saved, so a
// live acceptance test has to stand up something that answers the protocol. In
// CI the Fleet container shares the runner's network namespace, so the default
// loopback address works; when Fleet runs inside a VM (for example Rancher
// Desktop on macOS) set this to a host address the VM can route to, such as
// host.docker.internal.
const mockCACallbackHostEnv = "FLEETDM_TEST_CALLBACK_HOST"

// newTestCACertificateDER returns a throwaway self-signed CA certificate in DER
// form, for the mock responders to hand to Fleet.
func newTestCACertificateDER(t *testing.T, commonName string) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate mock CA key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("Failed to create mock CA certificate: %v", err)
	}
	return der
}

// serveMockCA starts the given handler on an ephemeral port and returns the
// base URL Fleet should use to reach it.
func serveMockCA(t *testing.T, handler http.Handler) string {
	t.Helper()

	// Loopback by default. Fleet is the client here, not the test process, so
	// it sometimes has to reach the test host across a container or VM boundary
	// — but binding beyond loopback exposes the responder to the local network,
	// so it happens only when the operator asks for it by naming the address
	// Fleet should use.
	host := os.Getenv(mockCACallbackHostEnv)
	bindAddr := "127.0.0.1:0"
	if host == "" {
		host = "127.0.0.1"
	} else {
		bindAddr = ":0"
	}

	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		t.Fatalf("Failed to listen for the mock certificate authority: %v", err)
	}

	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("Unexpected listener address type %T", listener.Addr())
	}
	return fmt.Sprintf("http://%s:%d", host, addr.Port)
}

// startMockSCEPServer starts a minimal SCEP responder and returns the SCEP URL
// Fleet should use. Fleet's validation performs a single GetCACert operation and
// requires a parseable DER certificate in response.
func startMockSCEPServer(t *testing.T) string {
	t.Helper()

	der := newTestCACertificateDER(t, "terraform-provider-fleetdm-test-scep-ca")
	mux := http.NewServeMux()
	mux.HandleFunc("/scep", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("operation") {
		case "GetCACaps":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "POSTPKIOperation\nSHA-256\nAES\nRenewal\n")
		case "GetCACert":
			w.Header().Set("Content-Type", "application/x-x509-ca-cert")
			_, _ = w.Write(der)
		default:
			http.NotFound(w, r)
		}
	})

	return serveMockCA(t, mux) + "/scep"
}

// startMockESTServer starts a minimal EST responder and returns the EST URL
// Fleet should use. Fleet validates an EST URL by fetching <url>/cacerts.
func startMockESTServer(t *testing.T) string {
	t.Helper()

	der := newTestCACertificateDER(t, "terraform-provider-fleetdm-test-est-ca")
	mux := http.NewServeMux()
	mux.HandleFunc("/est/cacerts", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pkcs7-mime")
		_ = pem.Encode(w, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	})

	return serveMockCA(t, mux) + "/est"
}

// TestAccCertificateAuthorityResource_customSCEP exercises the resource against
// a live Fleet instance, pointing Fleet at the in-test mock SCEP responder so
// its save-time URL validation succeeds.
func TestAccCertificateAuthorityResource_customSCEP(t *testing.T) {
	scepURL := startMockSCEPServer(t)
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlpha)
	name := "TFTEST_SCEP_" + suffix
	renamed := name + "_RENAMED"

	config := func(caName string) string {
		return providerConfig() + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  custom_scep_proxy = {
    name      = %q
    url       = %q
    challenge = "fake-challenge-for-acceptance-test"
  }
}
`, caName, scepURL)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fleetdm_certificate_authority.test", "id"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "name", name),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "type", "custom_scep_proxy"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.url", scepURL),
					// Fleet masks the challenge on read; the configured value must survive.
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.challenge", "fake-challenge-for-acceptance-test"),
				),
			},
			{
				// In-place rename through Fleet's PATCH endpoint.
				Config: config(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "name", renamed),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.name", renamed),
				),
			},
			{
				ResourceName:      "fleetdm_certificate_authority.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Fleet never returns CA secrets, so the challenge imports as null.
				ImportStateVerifyIgnore: []string{"custom_scep_proxy.challenge"},
			},
		},
	})
}

// TestAccCertificateAuthorityResource_customEST covers the EST type against a
// live Fleet instance.
func TestAccCertificateAuthorityResource_customEST(t *testing.T) {
	estURL := startMockESTServer(t)
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlpha)
	name := "TFTEST_EST_" + suffix

	config := providerConfig() + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  custom_est_proxy = {
    name     = %q
    url      = %q
    username = "fake-user"
    password = "fake-password-for-acceptance-test"
  }
}
`, name, estURL)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fleetdm_certificate_authority.test", "id"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "name", name),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "type", "custom_est_proxy"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "custom_est_proxy.username", "fake-user"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "custom_est_proxy.password", "fake-password-for-acceptance-test"),
				),
			},
		},
	})
}

// TestAccCertificateAuthorityResource_customSCEPWriteOnly exercises the
// write-only credential path against a live Fleet instance: the challenge is
// never persisted, and a secrets_wo_version bump rotates it in place.
//
// Skipped below Terraform 1.11, the first release supporting write-only
// attributes — the CI matrix still includes 1.5.
func TestAccCertificateAuthorityResource_customSCEPWriteOnly(t *testing.T) {
	scepURL := startMockSCEPServer(t)
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlpha)
	name := "TFTEST_SCEPWO_" + suffix

	config := func(challenge string, version int) string {
		return providerConfig() + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  secrets_wo_version = %d

  custom_scep_proxy = {
    name         = %q
    url          = %q
    challenge_wo = %q
  }
}
`, version, name, scepURL, challenge)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: config("fake-challenge-wo-for-acceptance-test", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fleetdm_certificate_authority.test", "id"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "name", name),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "secrets_wo_version", "1"),
					// Neither form of the secret reaches state.
					resource.TestCheckNoResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.challenge_wo"),
					resource.TestCheckNoResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.challenge"),
				),
			},
			{
				// Editing the write-only value alone is invisible to Terraform.
				Config: config("fake-challenge-wo-changed", 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// A version bump rotates in place through Fleet's PATCH endpoint.
				Config: config("fake-challenge-wo-rotated", 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_certificate_authority.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "secrets_wo_version", "2"),
					resource.TestCheckResourceAttr("fleetdm_certificate_authority.test", "name", name),
					resource.TestCheckNoResourceAttr("fleetdm_certificate_authority.test", "custom_scep_proxy.challenge_wo"),
				),
			},
		},
	})
}

// TestAccCertificateAuthoritiesDataSource_live reads the CA list from a live
// Fleet instance and checks the CA created in the same configuration appears.
func TestAccCertificateAuthoritiesDataSource_live(t *testing.T) {
	scepURL := startMockSCEPServer(t)
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlpha)
	name := "TFTEST_DS_" + suffix

	config := providerConfig() + fmt.Sprintf(`
resource "fleetdm_certificate_authority" "test" {
  custom_scep_proxy = {
    name      = %q
    url       = %q
    challenge = "fake-challenge-for-acceptance-test"
  }
}

data "fleetdm_certificate_authorities" "all" {
  depends_on = [fleetdm_certificate_authority.test]
}
`, name, scepURL)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckTypeSetElemNestedAttrs("data.fleetdm_certificate_authorities.all", "certificate_authorities.*", map[string]string{
						"name": name,
						"type": "custom_scep_proxy",
					}),
				),
			},
		},
	})
}
