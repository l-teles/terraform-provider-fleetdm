package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// labelWireBodies are verbatim GET /labels/{id} bodies from Fleet 4.90.0, one
// per membership kind, keyed by request path. Raw strings rather than encoded
// structs so the fixtures pin Fleet's real field names (notably `count`, not
// `host_count`).
var labelWireBodies = map[string]string{
	// Manual: no query, no criteria.
	"/api/v1/fleet/labels/1": `{"label":{
		"created_at":"2026-08-12T10:36:35Z","updated_at":"2026-08-12T10:36:35Z",
		"id":1,"author_id":1,"name":"manual-label","description":"a manual label",
		"query":"","platform":"","label_type":"regular","label_membership_type":"manual",
		"team_id":null,"team_name":null,"display_text":"manual-label","count":2,
		"fleet_id":null,"fleet_name":null}}`,
	// Dynamic: query-driven.
	"/api/v1/fleet/labels/2": `{"label":{
		"created_at":"2026-08-12T10:36:35Z","updated_at":"2026-08-12T10:36:35Z",
		"id":2,"author_id":1,"name":"dynamic-label","description":"a dynamic label",
		"query":"SELECT 1 FROM osquery_info WHERE start_time > 0;","platform":"darwin",
		"label_type":"regular","label_membership_type":"dynamic",
		"team_id":null,"team_name":null,"display_text":"dynamic-label","count":3,
		"fleet_id":null,"fleet_name":null}}`,
	// Host vitals on an IdP group: criteria present, operator omitted.
	"/api/v1/fleet/labels/3": `{"label":{
		"created_at":"2026-08-12T10:36:35Z","updated_at":"2026-08-12T10:36:35Z",
		"id":3,"author_id":1,"name":"idp-label","description":"an idp group label",
		"query":"","criteria":{"value":"engineering","vital":"end_user_idp_group"},
		"platform":"","label_type":"regular","label_membership_type":"host_vitals",
		"team_id":null,"team_name":null,"display_text":"idp-label","count":4,
		"fleet_id":null,"fleet_name":null}}`,
	// Host vitals on a custom host vital: criteria carries operator and id.
	"/api/v1/fleet/labels/4": `{"label":{
		"created_at":"2026-08-12T10:36:54Z","updated_at":"2026-08-12T10:36:54Z",
		"id":4,"author_id":1,"name":"chv-label","description":"a custom vital label",
		"query":"","criteria":{"value":"acme","vital":"custom_host_vital",
		"operator":"=","custom_host_vital_id":42},
		"platform":"","label_type":"regular","label_membership_type":"host_vitals",
		"team_id":null,"team_name":null,"display_text":"chv-label","count":5,
		"fleet_id":null,"fleet_name":null}}`,
}

// newLabelWireServer serves the canned per-kind label detail bodies.
func newLabelWireServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := labelWireBodies[r.URL.Path]
		if !ok || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

// TestAccLabelDataSource_membershipKinds checks the data source surfaces
// label_membership_type and criteria for every kind of label: criteria is null
// for manual and dynamic labels, and fully populated for host vitals labels
// (with operator/custom_host_vital_id null when Fleet omits them).
func TestAccLabelDataSource_membershipKinds(t *testing.T) {
	server := newLabelWireServer(t)
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccLabelDataSourceConfig_kinds(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Manual label: no query, no criteria.
					resource.TestCheckResourceAttr("data.fleetdm_label.manual", "label_membership_type", "manual"),
					resource.TestCheckResourceAttr("data.fleetdm_label.manual", "host_count", "2"),
					resource.TestCheckNoResourceAttr("data.fleetdm_label.manual", "criteria.vital"),
					resource.TestCheckNoResourceAttr("data.fleetdm_label.manual", "criteria.%"),

					// Dynamic label: query drives it, still no criteria.
					resource.TestCheckResourceAttr("data.fleetdm_label.dynamic", "label_membership_type", "dynamic"),
					resource.TestCheckResourceAttr("data.fleetdm_label.dynamic", "platform", "darwin"),
					resource.TestCheckResourceAttr("data.fleetdm_label.dynamic", "host_count", "3"),
					resource.TestCheckNoResourceAttr("data.fleetdm_label.dynamic", "criteria.vital"),
					resource.TestCheckNoResourceAttr("data.fleetdm_label.dynamic", "criteria.%"),

					// Host vitals on an IdP group: operator and id stay null.
					resource.TestCheckResourceAttr("data.fleetdm_label.idp", "label_membership_type", "host_vitals"),
					resource.TestCheckResourceAttr("data.fleetdm_label.idp", "criteria.vital", "end_user_idp_group"),
					resource.TestCheckResourceAttr("data.fleetdm_label.idp", "criteria.value", "engineering"),
					resource.TestCheckNoResourceAttr("data.fleetdm_label.idp", "criteria.operator"),
					resource.TestCheckNoResourceAttr("data.fleetdm_label.idp", "criteria.custom_host_vital_id"),
					resource.TestCheckResourceAttr("data.fleetdm_label.idp", "host_count", "4"),

					// Host vitals on a custom host vital: full criteria echo.
					resource.TestCheckResourceAttr("data.fleetdm_label.chv", "label_membership_type", "host_vitals"),
					resource.TestCheckResourceAttr("data.fleetdm_label.chv", "criteria.vital", "custom_host_vital"),
					resource.TestCheckResourceAttr("data.fleetdm_label.chv", "criteria.value", "acme"),
					resource.TestCheckResourceAttr("data.fleetdm_label.chv", "criteria.operator", "="),
					resource.TestCheckResourceAttr("data.fleetdm_label.chv", "criteria.custom_host_vital_id", "42"),
					resource.TestCheckResourceAttr("data.fleetdm_label.chv", "host_count", "5"),
				),
			},
		},
	})
}

func testAccLabelDataSourceConfig_kinds(serverURL string) string {
	var b strings.Builder
	b.WriteString(`
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}
`)
	for name, id := range map[string]int{"manual": 1, "dynamic": 2, "idp": 3, "chv": 4} {
		fmt.Fprintf(&b, `
data "fleetdm_label" %[1]q {
  id = %[2]d
}
`, name, id)
	}
	return b.String()
}

func TestAccLabelDataSource_basic(t *testing.T) {
	labelName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccLabelDataSourceConfig(labelName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "name", labelName),
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "description", "Test label for data source"),
					resource.TestCheckResourceAttrSet("data.fleetdm_label.test", "id"),
					resource.TestCheckResourceAttrSet("data.fleetdm_label.test", "host_count"),
					// A query-backed label is dynamic and carries no criteria.
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "label_membership_type", "dynamic"),
					resource.TestCheckNoResourceAttr("data.fleetdm_label.test", "criteria.vital"),
				),
			},
		},
	})
}

func testAccLabelDataSourceConfig(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name        = %[1]q
  description = "Test label for data source"
  query       = "SELECT 1"
}

data "fleetdm_label" "test" {
  id = fleetdm_label.test.id
}
`, name)
}

// TestAccLabelDataSource_manual reads a manual label — neither query nor
// criteria — and checks Fleet reports it as "manual" with criteria null.
func TestAccLabelDataSource_manual(t *testing.T) {
	labelName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccLabelDataSourceConfig_manual(labelName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "name", labelName),
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "label_membership_type", "manual"),
					resource.TestCheckNoResourceAttr("data.fleetdm_label.test", "criteria.vital"),
				),
			},
		},
	})
}

func testAccLabelDataSourceConfig_manual(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name        = %[1]q
  description = "Manual label for data source"
}

data "fleetdm_label" "test" {
  id = fleetdm_label.test.id
}
`, name)
}

// TestAccLabelDataSource_criteriaIdPGroup reads a host vitals label keyed on an
// IdP group. Fleet omits `operator` when the label didn't set one, so the data
// source must report it as null rather than an empty string.
func TestAccLabelDataSource_criteriaIdPGroup(t *testing.T) {
	labelName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccLabelDataSourceConfig_criteriaIdPGroup(labelName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "name", labelName),
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "label_membership_type", "host_vitals"),
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "criteria.vital", "end_user_idp_group"),
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "criteria.value", "engineering"),
					resource.TestCheckNoResourceAttr("data.fleetdm_label.test", "criteria.operator"),
					resource.TestCheckNoResourceAttr("data.fleetdm_label.test", "criteria.custom_host_vital_id"),
					// A host vitals label has no query.
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "query", ""),
				),
			},
		},
	})
}

func testAccLabelDataSourceConfig_criteriaIdPGroup(name string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name        = %[1]q
  description = "IdP group label for data source"

  criteria = {
    vital = "end_user_idp_group"
    value = "engineering"
  }
}

data "fleetdm_label" "test" {
  id = fleetdm_label.test.id
}
`, name)
}

// TestAccLabelDataSource_criteriaCustomHostVital reads a host vitals label keyed
// on a custom host vital, the one kind whose criteria carries every field —
// operator and custom_host_vital_id included.
func TestAccLabelDataSource_criteriaCustomHostVital(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccLabelDataSourceConfig_criteriaCustomHostVital(suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "label_membership_type", "host_vitals"),
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "criteria.vital", "custom_host_vital"),
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "criteria.value", "acme"),
					resource.TestCheckResourceAttr("data.fleetdm_label.test", "criteria.operator", "="),
					// The id must match the vital the label was pointed at.
					resource.TestCheckResourceAttrPair(
						"data.fleetdm_label.test", "criteria.custom_host_vital_id",
						"fleetdm_custom_host_vital.test", "id",
					),
				),
			},
		},
	})
}

func testAccLabelDataSourceConfig_criteriaCustomHostVital(suffix string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_custom_host_vital" "test" {
  name = "tf-acc-vital-%[1]s"
}

resource "fleetdm_label" "test" {
  name        = "tf-acc-test-%[1]s"
  description = "Custom host vital label for data source"

  criteria = {
    vital                = "custom_host_vital"
    operator             = "="
    value                = "acme"
    custom_host_vital_id = fleetdm_custom_host_vital.test.id
  }
}

data "fleetdm_label" "test" {
  id = fleetdm_label.test.id
}
`, suffix)
}
