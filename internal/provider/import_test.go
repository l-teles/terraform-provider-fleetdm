package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccLabelResource_import tests importing a label resource.
func TestAccLabelResource_import(t *testing.T) {
	labelName := "tf-acc-import-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create
			{
				Config: providerConfig() + testAccLabelResourceConfig_forImport(labelName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_label.import_test", "name", labelName),
					resource.TestCheckResourceAttrSet("fleetdm_label.import_test", "id"),
				),
			},
			// Import
			{
				ResourceName:      "fleetdm_label.import_test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccLabelResourceConfig_forImport(name string) string {
	return `
resource "fleetdm_label" "import_test" {
  name        = "` + name + `"
  description = "Test label for import"
  query       = "SELECT 1"
  platform    = "darwin"
}
`
}

// TestAccQueryResource_import tests importing a query resource.
func TestAccQueryResource_import(t *testing.T) {
	queryName := "tf-acc-import-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create
			{
				Config: providerConfig() + testAccQueryResourceConfig_forImport(queryName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_query.import_test", "name", queryName),
					resource.TestCheckResourceAttrSet("fleetdm_query.import_test", "id"),
				),
			},
			// Import
			{
				ResourceName:      "fleetdm_query.import_test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccQueryResourceConfig_forImport(name string) string {
	return `
resource "fleetdm_query" "import_test" {
  name        = "` + name + `"
  description = "Test query for import"
  query       = "SELECT * FROM system_info;"
  platform    = "darwin"
}
`
}

// TestAccPolicyResource_import tests importing a policy resource.
func TestAccPolicyResource_import(t *testing.T) {
	policyName := "tf-acc-import-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create
			{
				Config: providerConfig() + testAccPolicyResourceConfig_forImport(policyName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_policy.import_test", "name", policyName),
					resource.TestCheckResourceAttrSet("fleetdm_policy.import_test", "id"),
				),
			},
			// Import
			{
				ResourceName:      "fleetdm_policy.import_test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccPolicyResourceConfig_forImport(name string) string {
	return `
resource "fleetdm_policy" "import_test" {
  name        = "` + name + `"
  description = "Test policy for import"
  query       = "SELECT 1 WHERE 1=1;"
  platform    = "darwin"
  resolution  = "This is a test policy"
}
`
}

// TestAccScriptResource_import tests importing a script resource.
func TestAccScriptResource_import(t *testing.T) {
	scriptName := "tf-acc-import-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	// The config appends ".sh"; Fleet stores and returns the name with that suffix.
	scriptNameWithExt := scriptName + ".sh"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create
			{
				Config: providerConfig() + testAccScriptResourceConfig_forImport(scriptName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_script.import_test", "name", scriptNameWithExt),
					resource.TestCheckResourceAttrSet("fleetdm_script.import_test", "id"),
				),
			},
			// Import
			{
				ResourceName:            "fleetdm_script.import_test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"content"}, // Content is not returned by API
			},
		},
	})
}

func testAccScriptResourceConfig_forImport(name string) string {
	return `
resource "fleetdm_script" "import_test" {
  name    = "` + name + `.sh"
  content = "#!/bin/bash\necho 'hello world'"
}
`
}

// TestAccEnrollSecretResource_import tests importing an enroll secret resource.
func TestAccEnrollSecretResource_import(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create
			{
				Config: providerConfig() + testAccEnrollSecretResourceConfig_forImport(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.import_test", "id", "global"),
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.import_test", "secrets.#", "1"),
				),
			},
			// Import
			{
				ResourceName:            "fleetdm_enroll_secret.import_test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"secrets"}, // Secrets are managed differently
			},
		},
	})
}

func testAccEnrollSecretResourceConfig_forImport() string {
	return `
resource "fleetdm_enroll_secret" "import_test" {
  secrets = [
    { secret = "test-import-secret-12345" },
  ]
}
`
}

// TestAccConfigurationResource_import tests importing a configuration resource.
func TestAccConfigurationResource_import(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read (configuration is a singleton, so we just need to read it)
			{
				Config: providerConfig() + testAccConfigurationResourceConfig_forImport(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration.import_test", "id", "configuration"),
				),
			},
			// Import
			{
				ResourceName:      "fleetdm_configuration.import_test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccConfigurationResourceConfig_forImport() string {
	return `
resource "fleetdm_configuration" "import_test" {
  org_name = "Fleet Import Test"
}
`
}
