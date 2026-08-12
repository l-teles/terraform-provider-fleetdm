package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// labelsListWireBody is a verbatim GET /labels body from Fleet 4.90.0 holding
// one label of each membership kind. Raw JSON rather than an encoded map so the
// fixture pins Fleet's real field names — in particular host count arrives as
// `count`, and the list route echoes `criteria` in full, which is why this data
// source needs no per-label lookup.
const labelsListWireBody = `{"labels":[
	{"created_at":"2026-08-12T10:36:35Z","updated_at":"2026-08-12T10:36:35Z",
	 "id":1,"author_id":1,"name":"manual-label","description":"a manual label",
	 "query":"","platform":"","label_type":"regular","label_membership_type":"manual",
	 "team_id":null,"display_text":"manual-label","count":5,"fleet_id":null},
	{"created_at":"2026-08-12T10:36:35Z","updated_at":"2026-08-12T10:36:35Z",
	 "id":2,"author_id":1,"name":"dynamic-label","description":"a dynamic label",
	 "query":"SELECT 1 FROM os_version WHERE platform = 'darwin'","platform":"darwin",
	 "label_type":"regular","label_membership_type":"dynamic",
	 "team_id":null,"display_text":"dynamic-label","count":3,"fleet_id":null},
	{"created_at":"2026-08-12T10:36:35Z","updated_at":"2026-08-12T10:36:35Z",
	 "id":3,"author_id":1,"name":"idp-label","description":"an idp group label",
	 "query":"","criteria":{"value":"engineering","vital":"end_user_idp_group"},
	 "platform":"","label_type":"regular","label_membership_type":"host_vitals",
	 "team_id":null,"display_text":"idp-label","count":4,"fleet_id":null},
	{"created_at":"2026-08-12T10:36:54Z","updated_at":"2026-08-12T10:36:54Z",
	 "id":4,"author_id":1,"name":"chv-label","description":"a custom vital label",
	 "query":"","criteria":{"value":"acme","vital":"custom_host_vital",
	 "operator":"=","custom_host_vital_id":42},
	 "platform":"","label_type":"regular","label_membership_type":"host_vitals",
	 "team_id":null,"display_text":"chv-label","count":6,"fleet_id":null}
]}`

func TestAccLabelsDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/fleet/labels" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(labelsListWireBody))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccLabelsDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.#", "4"),

					// Manual: membership type reported, criteria null.
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.0.name", "manual-label"),
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.0.label_membership_type", "manual"),
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.0.host_count", "5"),
					resource.TestCheckNoResourceAttr("data.fleetdm_labels.test", "labels.0.criteria.vital"),
					resource.TestCheckNoResourceAttr("data.fleetdm_labels.test", "labels.0.criteria.%"),

					// Dynamic: still no criteria.
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.1.name", "dynamic-label"),
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.1.platform", "darwin"),
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.1.label_membership_type", "dynamic"),
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.1.host_count", "3"),
					resource.TestCheckNoResourceAttr("data.fleetdm_labels.test", "labels.1.criteria.vital"),
					resource.TestCheckNoResourceAttr("data.fleetdm_labels.test", "labels.1.criteria.%"),

					// Host vitals via IdP group: the list route carries criteria.
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.2.label_membership_type", "host_vitals"),
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.2.criteria.vital", "end_user_idp_group"),
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.2.criteria.value", "engineering"),
					resource.TestCheckNoResourceAttr("data.fleetdm_labels.test", "labels.2.criteria.operator"),
					resource.TestCheckNoResourceAttr("data.fleetdm_labels.test", "labels.2.criteria.custom_host_vital_id"),

					// Host vitals via custom host vital: operator and id too.
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.3.label_membership_type", "host_vitals"),
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.3.criteria.vital", "custom_host_vital"),
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.3.criteria.value", "acme"),
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.3.criteria.operator", "="),
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.3.criteria.custom_host_vital_id", "42"),
					resource.TestCheckResourceAttr("data.fleetdm_labels.test", "labels.3.host_count", "6"),
				),
			},
		},
	})
}

func testAccLabelsDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_labels" "test" {}
`
}

// TestAccLabelsDataSource_live creates a label then verifies it appears in the list.
func TestAccLabelsDataSource_live(t *testing.T) {
	labelName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccLabelsDataSourceConfig_live(labelName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.fleetdm_labels.test", "labels.#"),
				),
			},
		},
	})
}

func testAccLabelsDataSourceConfig_live(labelName string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name  = %[1]q
  query = "SELECT 1"
}

data "fleetdm_labels" "test" {
  depends_on = [fleetdm_label.test]
}
`, labelName)
}

// TestAccLabelsDataSource_criteriaInList proves Fleet's list route echoes
// criteria and label_membership_type, so the plural data source resolves a host
// vitals label without a per-label lookup.
//
// The rig is shared, so this matches the managed label as a list element by
// name rather than asserting on a fixed index or the total count.
func TestAccLabelsDataSource_criteriaInList(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	labelName := "tf-acc-test-" + suffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccLabelsDataSourceConfig_criteria(suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckTypeSetElemNestedAttrs(
						"data.fleetdm_labels.test",
						"labels.*",
						map[string]string{
							"name":                  labelName,
							"label_membership_type": "host_vitals",
							"criteria.vital":        "end_user_idp_department",
							"criteria.value":        "finance",
						},
					),
				),
			},
		},
	})
}

func testAccLabelsDataSourceConfig_criteria(suffix string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_label" "test" {
  name        = "tf-acc-test-%[1]s"
  description = "Host vitals label for labels data source"

  criteria = {
    vital = "end_user_idp_department"
    value = "finance"
  }
}

data "fleetdm_labels" "test" {
  depends_on = [fleetdm_label.test]
}
`, suffix)
}
