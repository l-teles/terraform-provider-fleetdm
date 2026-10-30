package fleetdm

import "testing"

// TestEncodeAppConfiguration_AndroidApplicationPolicyKeys checks the provider
// passes any Android ApplicationPolicy key through untouched: Fleet 4.94
// accepts every field but packageName and installType, so key validation
// belongs to Fleet, not the client.
func TestEncodeAppConfiguration_AndroidApplicationPolicyKeys(t *testing.T) {
	const raw = `{"credentialProviderPolicy":"CREDENTIAL_PROVIDER_ALLOWED","autoUpdateMode":"AUTO_UPDATE_HIGH_PRIORITY","managedConfiguration":{"a":1}}`
	got, err := EncodeAppConfiguration(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != raw {
		t.Errorf("expected the payload unchanged\n got: %s\nwant: %s", got, raw)
	}
	if !SameAppConfiguration(raw, `{"managedConfiguration":{"a":1},"autoUpdateMode":"AUTO_UPDATE_HIGH_PRIORITY","credentialProviderPolicy":"CREDENTIAL_PROVIDER_ALLOWED"}`) {
		t.Error("expected a reordered echo to compare equal")
	}
}
