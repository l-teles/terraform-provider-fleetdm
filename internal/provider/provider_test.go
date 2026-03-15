package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"fleetdm": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheck validates the necessary test environment variables exist.
// Tests using PreCheck will be skipped (not failed) when the env vars are absent,
// allowing mock-server tests and live-Fleet tests to coexist in the same suite.
func testAccPreCheck(t *testing.T) {
	t.Helper()
	if v := os.Getenv("FLEETDM_URL"); v == "" {
		t.Skip("FLEETDM_URL must be set for live acceptance tests")
	}
	if v := os.Getenv("FLEETDM_API_TOKEN"); v == "" {
		t.Skip("FLEETDM_API_TOKEN must be set for live acceptance tests")
	}
}

// providerConfig returns a string containing the provider configuration
// for acceptance tests. Uses environment variables for configuration.
func providerConfig() string {
	return "provider \"fleetdm\" {}\n"
}
