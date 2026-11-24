package provider

import (
	"testing"

	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

func TestInstallerVersion(t *testing.T) {
	hosts := []fleetdm.SoftwareTitleVersion{{Version: "1.0"}}
	for _, tc := range []struct {
		name  string
		title fleetdm.SoftwareTitle
		want  string
	}{
		{"installer version wins over lagging hosts", fleetdm.SoftwareTitle{
			SoftwarePackage: &fleetdm.SoftwarePackageInfo{Version: "2.0"}, Versions: hosts}, "2.0"},
		{"falls back to host inventory", fleetdm.SoftwareTitle{
			SoftwarePackage: &fleetdm.SoftwarePackageInfo{}, Versions: hosts}, "1.0"},
		{"no package", fleetdm.SoftwareTitle{Versions: hosts}, "1.0"},
		{"nothing reported", fleetdm.SoftwareTitle{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := installerVersion(&tc.title); got != tc.want {
				t.Errorf("installerVersion = %q, want %q", got, tc.want)
			}
		})
	}
}
