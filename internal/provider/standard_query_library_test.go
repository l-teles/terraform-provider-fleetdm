package provider

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccStandardQueryLibrary_globalLibraryLoaded verifies that the standard
// query library has been imported into the Fleet instance (by the CI setup
// step that runs import-standard-queries.sh before the test suite). It lists
// all global queries and asserts that at least 10 are present.
func TestAccStandardQueryLibrary_globalLibraryLoaded(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
data "fleetdm_queries" "all" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith(
						"data.fleetdm_queries.all", "queries.#",
						func(value string) error {
							count, err := strconv.Atoi(value)
							if err != nil {
								return err
							}
							if count < 10 {
								return fmt.Errorf(
									"expected at least 10 standard library queries, got %d — "+
										"did the import-standard-queries.sh step run?", count)
							}
							return nil
						},
					),
				),
			},
		},
	})
}

// TestAccStandardQueryLibrary_CRUD exercises the full create → read →
// import → update → delete lifecycle for a single query whose SQL is taken
// from Fleet's standard query library.
func TestAccStandardQueryLibrary_CRUD(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	queryName := "tf-acc-stdql-crud-" + suffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// 1. Create – import the "Get authorized SSH keys" standard query.
			{
				Config: testAccStdQlCRUDConfig_create(queryName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_query.test", "name", queryName),
					resource.TestCheckResourceAttr("fleetdm_query.test", "description",
						"Presence of authorized SSH keys may be unusual on laptops."),
					resource.TestCheckResourceAttr("fleetdm_query.test", "platform.#", "2"),
				resource.TestCheckResourceAttr("fleetdm_query.test", "platform.0", "darwin"),
				resource.TestCheckResourceAttr("fleetdm_query.test", "platform.1", "linux"),
					resource.TestCheckResourceAttr("fleetdm_query.test", "observer_can_run", "false"),
					resource.TestCheckResourceAttrSet("fleetdm_query.test", "id"),
				),
			},
			// 2. ImportState – verify round-trip import.
			{
				ResourceName:      "fleetdm_query.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// 3. Update – change description, SQL, platform, and enable observer_can_run.
			//    Uses "Get crashes" SQL from the standard library.
			{
				Config: testAccStdQlCRUDConfig_update(queryName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_query.test", "name", queryName),
					resource.TestCheckResourceAttr("fleetdm_query.test", "description",
						"Retrieve application, system, and mobile app crash logs."),
					resource.TestCheckResourceAttr("fleetdm_query.test", "platform.#", "1"),
				resource.TestCheckResourceAttr("fleetdm_query.test", "platform.0", "darwin"),
					resource.TestCheckResourceAttr("fleetdm_query.test", "observer_can_run", "true"),
					resource.TestCheckResourceAttr("fleetdm_query.test", "logging", "snapshot"),
				),
			},
			// 4. Delete is handled implicitly by the test framework's cleanup.
		},
	})
}

// TestAccStandardQueryLibrary_teamScopedQuery creates a team and two
// team-scoped queries (using SQL from the standard library), then reads them
// back via the data source filtered by team_id. This exercises the full
// create → read → destroy lifecycle for team-assigned queries.
func TestAccStandardQueryLibrary_teamScopedQuery(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	teamName := "tf-acc-stdql-team-" + suffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccStdQlTeamQueryConfig(teamName, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_team.test", "name", teamName),
					resource.TestCheckResourceAttrPair(
						"fleetdm_query.ssh_keys", "team_id",
						"fleetdm_team.test", "id",
					),
					resource.TestCheckResourceAttrPair(
						"fleetdm_query.os_version", "team_id",
						"fleetdm_team.test", "id",
					),
					resource.TestCheckResourceAttr("data.fleetdm_queries.team", "queries.#", "2"),
				),
			},
		},
	})
}

// TestAccStandardQueryLibrary_multipleTeams creates two teams each with their
// own set of standard-library queries and verifies that the data source
// correctly scopes results by team_id.
func TestAccStandardQueryLibrary_multipleTeams(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	teamA := "tf-acc-stdql-teamA-" + suffix
	teamB := "tf-acc-stdql-teamB-" + suffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccStdQlMultiTeamConfig(teamA, teamB, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_queries.team_a", "queries.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_queries.team_b", "queries.#", "2"),
					resource.TestCheckResourceAttr(
						"data.fleetdm_queries.team_a", "queries.0.name",
						"tf-acc-stdql-teamA-"+suffix+"-Get authorized SSH keys",
					),
				),
			},
		},
	})
}

// TestAccStandardQueryLibrary_bulkCRUD creates three team-scoped standard
// queries in one step, verifies all three exist, then removes one of them and
// verifies only two remain. This exercises multi-resource create and targeted
// delete within the same team scope.
func TestAccStandardQueryLibrary_bulkCRUD(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	teamName := "tf-acc-stdql-bulk-" + suffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// 1. Create three queries on a team.
			{
				Config: testAccStdQlBulkConfig_threeQueries(teamName, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_queries.team", "queries.#", "3"),
					resource.TestCheckResourceAttrSet("fleetdm_query.ssh_keys", "id"),
					resource.TestCheckResourceAttrSet("fleetdm_query.os_version", "id"),
					resource.TestCheckResourceAttrSet("fleetdm_query.chrome_ext", "id"),
				),
			},
			// 2. Update one query (change description and SQL) and delete another.
			//    Config now contains only ssh_keys (updated) and os_version; chrome_ext is removed.
			{
				Config: testAccStdQlBulkConfig_updateAndDelete(teamName, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Only 2 queries remain on the team.
					resource.TestCheckResourceAttr("data.fleetdm_queries.team", "queries.#", "2"),
					// ssh_keys was updated: description changed, observer_can_run enabled.
					resource.TestCheckResourceAttr("fleetdm_query.ssh_keys", "description",
						"Updated: auditing SSH keys across all users."),
					resource.TestCheckResourceAttr("fleetdm_query.ssh_keys", "observer_can_run", "true"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Config helpers
// ---------------------------------------------------------------------------

func testAccStdQlCRUDConfig_create(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_query" "test" {
  name            = %[1]q
  description     = "Presence of authorized SSH keys may be unusual on laptops."
  query           = "SELECT username, authorized_keys.* FROM users CROSS JOIN authorized_keys USING (uid);"
  platform        = ["darwin", "linux"]
  observer_can_run = false
}
`, name)
}

func testAccStdQlCRUDConfig_update(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_query" "test" {
  name             = %[1]q
  description      = "Retrieve application, system, and mobile app crash logs."
  query            = "SELECT uid, datetime, responsible, exception_type, identifier, version, crash_path FROM users CROSS JOIN crashes USING (uid);"
  platform         = ["darwin"]
  observer_can_run = true
  logging          = "snapshot"
}
`, name)
}

func testAccStdQlTeamQueryConfig(teamName, suffix string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_team" "test" {
  name = %[1]q
}

resource "fleetdm_query" "ssh_keys" {
  name        = %[2]q
  description = "Presence of authorized SSH keys may be unusual on laptops."
  query       = "SELECT username, authorized_keys.* FROM users CROSS JOIN authorized_keys USING (uid);"
  platform    = ["darwin", "linux"]
  team_id     = tonumber(fleetdm_team.test.id)
}

resource "fleetdm_query" "os_version" {
  name        = %[3]q
  description = "Retrieve OS version information."
  query       = "SELECT * FROM os_version;"
  team_id     = tonumber(fleetdm_team.test.id)
}

data "fleetdm_queries" "team" {
  team_id    = tonumber(fleetdm_team.test.id)
  depends_on = [fleetdm_query.ssh_keys, fleetdm_query.os_version]
}
`,
		teamName,
		teamName+"-Get authorized SSH keys",
		teamName+"-Get OS version",
	)
}

func testAccStdQlMultiTeamConfig(teamA, teamB, suffix string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_team" "team_a" {
  name = %[1]q
}

resource "fleetdm_team" "team_b" {
  name = %[2]q
}

resource "fleetdm_query" "a_ssh_keys" {
  name    = %[3]q
  query   = "SELECT username, authorized_keys.* FROM users CROSS JOIN authorized_keys USING (uid);"
  team_id = tonumber(fleetdm_team.team_a.id)
}

resource "fleetdm_query" "b_os_version" {
  name    = %[4]q
  query   = "SELECT * FROM os_version;"
  team_id = tonumber(fleetdm_team.team_b.id)
}

resource "fleetdm_query" "b_chrome_ext" {
  name    = %[5]q
  query   = "SELECT * FROM users CROSS JOIN chrome_extensions USING (uid);"
  team_id = tonumber(fleetdm_team.team_b.id)
}

data "fleetdm_queries" "team_a" {
  team_id    = tonumber(fleetdm_team.team_a.id)
  depends_on = [fleetdm_query.a_ssh_keys]
}

data "fleetdm_queries" "team_b" {
  team_id    = tonumber(fleetdm_team.team_b.id)
  depends_on = [fleetdm_query.b_os_version, fleetdm_query.b_chrome_ext]
}
`,
		teamA,
		teamB,
		"tf-acc-stdql-teamA-"+suffix+"-Get authorized SSH keys",
		"tf-acc-stdql-teamB-"+suffix+"-Get OS version",
		"tf-acc-stdql-teamB-"+suffix+"-Get installed Chrome Extensions",
	)
}

func testAccStdQlBulkConfig_threeQueries(teamName, suffix string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_team" "test" {
  name = %[1]q
}

resource "fleetdm_query" "ssh_keys" {
  name        = %[2]q
  description = "Presence of authorized SSH keys may be unusual on laptops."
  query       = "SELECT username, authorized_keys.* FROM users CROSS JOIN authorized_keys USING (uid);"
  platform    = ["darwin", "linux"]
  team_id     = tonumber(fleetdm_team.test.id)
}

resource "fleetdm_query" "os_version" {
  name    = %[3]q
  query   = "SELECT * FROM os_version;"
  team_id = tonumber(fleetdm_team.test.id)
}

resource "fleetdm_query" "chrome_ext" {
  name    = %[4]q
  query   = "SELECT * FROM users CROSS JOIN chrome_extensions USING (uid);"
  team_id = tonumber(fleetdm_team.test.id)
}

data "fleetdm_queries" "team" {
  team_id    = tonumber(fleetdm_team.test.id)
  depends_on = [fleetdm_query.ssh_keys, fleetdm_query.os_version, fleetdm_query.chrome_ext]
}
`,
		teamName,
		teamName+"-ssh-keys",
		teamName+"-os-version",
		teamName+"-chrome-ext",
	)
}

func testAccStdQlBulkConfig_updateAndDelete(teamName, suffix string) string {
	// chrome_ext is intentionally absent — Terraform will delete it.
	// ssh_keys gets a new description and observer_can_run=true.
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_team" "test" {
  name = %[1]q
}

resource "fleetdm_query" "ssh_keys" {
  name             = %[2]q
  description      = "Updated: auditing SSH keys across all users."
  query            = "SELECT username, authorized_keys.* FROM users CROSS JOIN authorized_keys USING (uid);"
  platform         = ["darwin", "linux"]
  observer_can_run = true
  team_id          = tonumber(fleetdm_team.test.id)
}

resource "fleetdm_query" "os_version" {
  name    = %[3]q
  query   = "SELECT * FROM os_version;"
  team_id = tonumber(fleetdm_team.test.id)
}

data "fleetdm_queries" "team" {
  team_id    = tonumber(fleetdm_team.test.id)
  depends_on = [fleetdm_query.ssh_keys, fleetdm_query.os_version]
}
`,
		teamName,
		teamName+"-ssh-keys",
		teamName+"-os-version",
	)
}
