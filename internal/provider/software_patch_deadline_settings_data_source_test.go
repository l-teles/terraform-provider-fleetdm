package provider

import (
	"fmt"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

const patchDeadlineDataSourceName = "data.fleetdm_software_patch_deadline_settings.test"

func testAccPatchDeadlineDataSourceMockConfig(serverURL string, fleetID int64) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %q
  api_key        = "test-token"
}

data "fleetdm_software_patch_deadline_settings" "test" {
  fleet_id = %d
}
`, serverURL, fleetID)
}

// Days come back in Fleet's week order; an unset fleet reports the defaults.
func TestAccSoftwarePatchDeadlineSettingsDataSource_mock(t *testing.T) {
	mock := newPatchDeadlineMock(3)
	mock.settings[3] = fleetdm.PatchDeadlineSettings{FleetID: 3, Days: []string{"monday", "saturday"}, Time: "07:45"}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPatchDeadlineDataSourceMockConfig(server.URL, 3),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchDeadlineDataSourceName, "fleet_id", "3"),
					resource.TestCheckResourceAttr(patchDeadlineDataSourceName, "days.#", "2"),
					resource.TestCheckResourceAttr(patchDeadlineDataSourceName, "days.0", "monday"),
					resource.TestCheckResourceAttr(patchDeadlineDataSourceName, "days.1", "saturday"),
					resource.TestCheckResourceAttr(patchDeadlineDataSourceName, "time", "07:45"),
				),
			},
			{
				Config: testAccPatchDeadlineDataSourceMockConfig(server.URL, 0),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchDeadlineDataSourceName, "fleet_id", "0"),
					resource.TestCheckResourceAttr(patchDeadlineDataSourceName, "days.#", "1"),
					resource.TestCheckResourceAttr(patchDeadlineDataSourceName, "days.0", "tuesday"),
					resource.TestCheckResourceAttr(patchDeadlineDataSourceName, "time", "17:00"),
				),
			},
		},
	})
}

func TestAccSoftwarePatchDeadlineSettingsDataSource_mockFleetNotFound(t *testing.T) {
	mock := newPatchDeadlineMock()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccPatchDeadlineDataSourceMockConfig(server.URL, 42),
				ExpectError: regexp.MustCompile(`fleet 42 does not exist`),
			},
		},
	})
}

// TestAccSoftwarePatchDeadlineSettingsDataSource_live reads a new fleet's
// settings after the resource sets them, and Unassigned's without changing them.
func TestAccSoftwarePatchDeadlineSettingsDataSource_live(t *testing.T) {
	testAccPreCheck(t)
	skipBeforeFleet495PatchDeadlines(t)

	fleetName := "tf-acc-patch-deadline-ds-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %q
}

resource "fleetdm_software_patch_deadline_settings" "test" {
  fleet_id = fleetdm_fleet.test.id
  days     = ["sunday", "monday"]
  time     = "11:15"
}

data "fleetdm_software_patch_deadline_settings" "test" {
  fleet_id = fleetdm_software_patch_deadline_settings.test.fleet_id
}

data "fleetdm_software_patch_deadline_settings" "unassigned" {
  fleet_id = 0
}
`, fleetName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(patchDeadlineDataSourceName, "fleet_id", "fleetdm_fleet.test", "id"),
					resource.TestCheckResourceAttr(patchDeadlineDataSourceName, "days.#", "2"),
					resource.TestCheckResourceAttr(patchDeadlineDataSourceName, "days.0", "monday"),
					resource.TestCheckResourceAttr(patchDeadlineDataSourceName, "days.1", "sunday"),
					resource.TestCheckResourceAttr(patchDeadlineDataSourceName, "time", "11:15"),
					resource.TestCheckResourceAttr("data.fleetdm_software_patch_deadline_settings.unassigned", "fleet_id", "0"),
					resource.TestCheckResourceAttrSet("data.fleetdm_software_patch_deadline_settings.unassigned", "time"),
					resource.TestCheckResourceAttrSet("data.fleetdm_software_patch_deadline_settings.unassigned", "days.0"),
				),
			},
		},
	})
}
