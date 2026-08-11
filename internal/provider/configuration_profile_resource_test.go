package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

const testMobileConfig = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>PayloadIdentifier</key>
  <string>com.example.test</string>
  <key>PayloadType</key>
  <string>Configuration</string>
  <key>PayloadVersion</key>
  <integer>1</integer>
  <key>PayloadDisplayName</key>
  <string>Test Profile</string>
</dict>
</plist>`

func TestAccConfigurationProfileResource_basic(t *testing.T) {
	const profileUUID = "uuid-test-profile-1234"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/configuration_profiles" && r.Method == "POST":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"profile_uuid": profileUUID,
			})
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+profileUUID && r.URL.Query().Get("alt") == "media" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/x-apple-aspen-config")
			w.WriteHeader(http.StatusOK)
			// The heredoc in the config appends a trailing newline; match it here so Read
			// produces no diff against the config value.
			w.Write([]byte(testMobileConfig + "\n"))
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+profileUUID && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"profile_uuid":       profileUUID,
				"team_id":            nil,
				"name":               "Test Profile",
				"platform":           "darwin",
				"identifier":         "com.example.test",
				"checksum":           "abc123checksum",
				"created_at":         "2024-01-15T10:00:00Z",
				"uploaded_at":        "2024-01-15T10:00:00Z",
				"labels_include_all": []interface{}{},
				"labels_include_any": []interface{}{},
				"labels_exclude_any": []interface{}{},
			})
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+profileUUID && r.Method == "DELETE":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccConfigurationProfileResourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "profile_uuid", profileUUID),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "name", "Test Profile"),
					resource.TestCheckNoResourceAttr("fleetdm_configuration_profile.test", "display_name"),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "platform", "darwin"),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "identifier", "com.example.test"),
				),
			},
		},
	})
}

const testWindowsXMLProfile = `<?xml version="1.0" encoding="utf-8"?>
<SyncML xmlns="SYNCML:SYNCML1.2">
  <SyncBody>
    <Replace>
      <CmdID>1</CmdID>
      <Item>
        <Target><LocURI>./Device/Vendor/MSFT/BitLocker/RequireDeviceEncryption</LocURI></Target>
        <Data>1</Data>
      </Item>
    </Replace>
  </SyncBody>
</SyncML>`

func TestAccConfigurationProfileResource_displayName(t *testing.T) {
	const profileUUID = "uuid-win-profile-5678"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/configuration_profiles" && r.Method == "POST":
			// Verify the uploaded filename includes the display name
			err := r.ParseMultipartForm(10 << 20)
			if err != nil {
				t.Errorf("failed to parse multipart form: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			file, header, err := r.FormFile("profile")
			if err != nil {
				t.Errorf("failed to get form file: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer file.Close()
			if header.Filename != "BitLocker Policy.xml" {
				t.Errorf("expected filename 'BitLocker Policy.xml', got %q", header.Filename)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"profile_uuid": profileUUID,
			})
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+profileUUID && r.URL.Query().Get("alt") == "media" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(testWindowsXMLProfile + "\n"))
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+profileUUID && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"profile_uuid":       profileUUID,
				"team_id":            nil,
				"name":               "BitLocker Policy",
				"platform":           "windows",
				"identifier":         "",
				"checksum":           "",
				"created_at":         "2024-01-15T10:00:00Z",
				"uploaded_at":        "2024-01-15T10:00:00Z",
				"labels_include_all": []interface{}{},
				"labels_include_any": []interface{}{},
				"labels_exclude_any": []interface{}{},
			})
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+profileUUID && r.Method == "DELETE":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccConfigurationProfileWindowsConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.win_test", "profile_uuid", profileUUID),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.win_test", "display_name", "BitLocker Policy"),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.win_test", "name", "BitLocker Policy"),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.win_test", "platform", "windows"),
				),
			},
		},
	})
}

func testAccConfigurationProfileWindowsConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

resource "fleetdm_configuration_profile" "win_test" {
  display_name    = "BitLocker Policy"
  profile_content = <<-EOT
` + testWindowsXMLProfile + `
  EOT
}
`
}

func TestAccConfigurationProfileResource_windowsRequiresDisplayName(t *testing.T) {
	windowsProfileConfig := func(displayNameLine string) string {
		return `
provider "fleetdm" {
  server_address = "http://localhost:0"
  api_key        = "test-token"
}

resource "fleetdm_configuration_profile" "t" {
  ` + displayNameLine + `
  profile_content = <<-EOT
` + testWindowsXMLProfile + `
  EOT
}
`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      windowsProfileConfig(""),
				ExpectError: regexp.MustCompile(`display_name is required for Windows profiles`),
			},
			{
				Config:      windowsProfileConfig(`display_name = ""`),
				ExpectError: regexp.MustCompile(`display_name is required for Windows profiles`),
			},
			{
				Config:      windowsProfileConfig(`display_name = "My/Policy"`),
				ExpectError: regexp.MustCompile(`must not contain path separators`),
			},
			{
				Config:      windowsProfileConfig(`display_name = "Policy.xml"`),
				ExpectError: regexp.MustCompile(`must not include a profile file extension`),
			},
		},
	})
}

func testAccConfigurationProfileResourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

resource "fleetdm_configuration_profile" "test" {
  profile_content = <<-EOT
` + testMobileConfig + `
  EOT
}
`
}

// profileMockState is shared mutable state for the stateful mock server used
// by the in-place update tests.
type profileMockState struct {
	content          string
	identifier       string
	labelsIncludeAll []string
	labelsIncludeAny []string
	labelsExcludeAny []string
	patchCount       int
}

// newProfileMockServer returns a stateful mock that supports POST (create),
// GET (metadata + alt=media content), PATCH (in-place update with
// full-replace label semantics, mirroring live Fleet 4.90), and DELETE.
func newProfileMockServer(t *testing.T, uuid, name, platform string, st *profileMockState) *httptest.Server {
	t.Helper()
	labelObjs := func(names []string) []map[string]interface{} {
		if len(names) == 0 {
			return nil
		}
		out := make([]map[string]interface{}, len(names))
		for i, n := range names {
			out[i] = map[string]interface{}{"name": n}
		}
		return out
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/configuration_profiles" && r.Method == "POST":
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Errorf("create: parse multipart: %v", err)
			}
			file, _, err := r.FormFile("profile")
			if err != nil {
				t.Errorf("create: missing profile file: %v", err)
			} else {
				buf := new(bytes.Buffer)
				buf.ReadFrom(file)
				file.Close()
				st.content = buf.String()
				// Real Fleet derives the identifier from the content.
				if id, ok := fleetdm.ProfileIdentifierFromContent(buf.Bytes()); ok {
					st.identifier = id
				}
			}
			st.labelsIncludeAll = r.MultipartForm.Value["labels_include_all"]
			st.labelsIncludeAny = r.MultipartForm.Value["labels_include_any"]
			st.labelsExcludeAny = r.MultipartForm.Value["labels_exclude_any"]
			json.NewEncoder(w).Encode(map[string]interface{}{"profile_uuid": uuid})
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+uuid && r.Method == "PATCH":
			st.patchCount++
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Errorf("patch: parse multipart: %v", err)
			}
			if file, _, err := r.FormFile("profile"); err == nil {
				buf := new(bytes.Buffer)
				buf.ReadFrom(file)
				file.Close()
				st.content = buf.String()
			}
			// Full-replace semantics: absent fields clear targeting.
			st.labelsIncludeAll = r.MultipartForm.Value["labels_include_all"]
			st.labelsIncludeAny = r.MultipartForm.Value["labels_include_any"]
			st.labelsExcludeAny = r.MultipartForm.Value["labels_exclude_any"]
			json.NewEncoder(w).Encode(map[string]interface{}{})
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+uuid && r.URL.Query().Get("alt") == "media" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte(st.content))
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+uuid && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"profile_uuid":       uuid,
				"name":               name,
				"platform":           platform,
				"identifier":         st.identifier,
				"checksum":           fmt.Sprintf("checksum-%d", st.patchCount),
				"created_at":         "2026-01-01T00:00:00Z",
				"uploaded_at":        fmt.Sprintf("2026-01-01T00:00:%02dZ", st.patchCount),
				"labels_include_all": labelObjs(st.labelsIncludeAll),
				"labels_include_any": labelObjs(st.labelsIncludeAny),
				"labels_exclude_any": labelObjs(st.labelsExcludeAny),
			})
		case r.URL.Path == "/api/v1/fleet/configuration_profiles/"+uuid && r.Method == "DELETE":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
}

// TestAccConfigurationProfileResource_labelsInPlaceUpdate verifies that label
// changes update in place (same UUID, no replacement), including clearing.
func TestAccConfigurationProfileResource_labelsInPlaceUpdate(t *testing.T) {
	const uuid = "uuid-labels-update"
	st := &profileMockState{}
	server := newProfileMockServer(t, uuid, "Test Profile", "darwin", st)
	defer server.Close()

	configWith := func(labels string) string {
		return `
provider "fleetdm" {
  server_address = "` + server.URL + `"
  api_key        = "test-token"
}

resource "fleetdm_configuration_profile" "test" {
  profile_content = <<-EOT
` + testMobileConfig + `
EOT
` + labels + `
}
`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: configWith(`  labels_include_all = ["lbl-1", "lbl-2"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "profile_uuid", uuid),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "labels_include_all.#", "2"),
				),
			},
			{
				Config: configWith(`  labels_exclude_any = ["lbl-3"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_configuration_profile.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "profile_uuid", uuid),
					resource.TestCheckNoResourceAttr("fleetdm_configuration_profile.test", "labels_include_all"),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "labels_exclude_any.0", "lbl-3"),
				),
			},
			{
				Config: configWith(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_configuration_profile.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "profile_uuid", uuid),
					resource.TestCheckNoResourceAttr("fleetdm_configuration_profile.test", "labels_exclude_any"),
				),
			},
		},
	})
}

// TestAccConfigurationProfileResource_contentInPlaceUpdate verifies that a
// content change with the same PayloadIdentifier updates in place.
func TestAccConfigurationProfileResource_contentInPlaceUpdate(t *testing.T) {
	const uuid = "uuid-content-update"
	st := &profileMockState{}
	server := newProfileMockServer(t, uuid, "Test Profile", "darwin", st)
	defer server.Close()

	configWith := func(displayName string) string {
		return `
provider "fleetdm" {
  server_address = "` + server.URL + `"
  api_key        = "test-token"
}

resource "fleetdm_configuration_profile" "test" {
  profile_content = <<-EOT
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>PayloadIdentifier</key>
  <string>com.example.test</string>
  <key>PayloadDisplayName</key>
  <string>` + displayName + `</string>
</dict>
</plist>
EOT
}
`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: configWith("Version One"),
				Check:  resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "profile_uuid", uuid),
			},
			{
				Config: configWith("Version Two"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_configuration_profile.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "profile_uuid", uuid),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "checksum", "checksum-1"),
				),
			},
		},
	})
}

// TestAccConfigurationProfileResource_identifierChangeReplaces verifies that
// changing the PayloadIdentifier forces replacement.
func TestAccConfigurationProfileResource_identifierChangeReplaces(t *testing.T) {
	const uuid = "uuid-ident-replace"
	st := &profileMockState{}
	server := newProfileMockServer(t, uuid, "Test Profile", "darwin", st)
	defer server.Close()

	configWith := func(identifier string) string {
		return `
provider "fleetdm" {
  server_address = "` + server.URL + `"
  api_key        = "test-token"
}

resource "fleetdm_configuration_profile" "test" {
  profile_content = <<-EOT
<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>PayloadIdentifier</key>
  <string>` + identifier + `</string>
</dict>
</plist>
EOT
}
`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: configWith("com.example.one"),
				Check:  resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "profile_uuid", uuid),
			},
			{
				Config: configWith("com.example.two"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_configuration_profile.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "identifier", "com.example.two"),
			},
		},
	})
}

// TestAccConfigurationProfileResource_updateOnOldFleet verifies the
// actionable error when Fleet lacks the PATCH route (pre-4.90).
func TestAccConfigurationProfileResource_updateOnOldFleet(t *testing.T) {
	const uuid = "uuid-old-fleet"
	st := &profileMockState{}
	inner := newProfileMockServer(t, uuid, "Test Profile", "darwin", st)
	defer inner.Close()

	// Wrap the stateful mock: 404 any PATCH, as a pre-4.90 Fleet would.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"message": "Resource Not Found"})
			return
		}
		inner.Config.Handler.ServeHTTP(w, r)
	}))
	defer server.Close()

	configWith := func(labels string) string {
		return `
provider "fleetdm" {
  server_address = "` + server.URL + `"
  api_key        = "test-token"
}

resource "fleetdm_configuration_profile" "test" {
  profile_content = <<-EOT
` + testMobileConfig + `
EOT
` + labels + `
}
`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: configWith(""),
			},
			{
				Config:      configWith(`  labels_include_any = ["lbl-1"]`),
				ExpectError: regexp.MustCompile(`require Fleet 4\.90\+`),
			},
		},
	})
}

// TestAccConfigurationProfileResource_liveWindowsInPlaceUpdate exercises the
// real Fleet 4.90 PATCH path end to end (requires the CI rig's Windows MDM,
// enabled by setup-fleet.sh): create a Windows profile with label targeting,
// then change content and labels in place, then clear labels — asserting the
// profile UUID never changes.
func TestAccConfigurationProfileResource_liveWindowsInPlaceUpdate(t *testing.T) {
	testAccPreCheck(t)

	suffix := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	// Two labels in non-alphabetical config order, to catch any server-side
	// reordering of label lists (which would surface as an inconsistent
	// apply result).
	labelName := "tf-acc-lbl-z-" + suffix
	labelName2 := "tf-acc-lbl-a-" + suffix

	winProfile := func(data string) string {
		return `<Replace>
  <Item>
    <Target><LocURI>./Device/Vendor/MSFT/Policy/Config/System/AllowTelemetry</LocURI></Target>
    <Data>` + data + `</Data>
  </Item>
</Replace>`
	}

	config := func(data, labelsLine string) string {
		return providerConfig() + `
resource "fleetdm_label" "test" {
  name  = "` + labelName + `"
  query = "SELECT 1;"
}

resource "fleetdm_label" "test2" {
  name  = "` + labelName2 + `"
  query = "SELECT 1;"
}

resource "fleetdm_configuration_profile" "test" {
  display_name    = "tf-acc-profile-` + suffix + `"
  profile_content = <<-EOT
` + winProfile(data) + `
EOT
` + labelsLine + `
}
`
	}

	var uuidStep1 string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("1", `  labels_include_any = [fleetdm_label.test.name, fleetdm_label.test2.name]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "platform", "windows"),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "labels_include_any.0", labelName),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "labels_include_any.1", labelName2),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["fleetdm_configuration_profile.test"]
						uuidStep1 = rs.Primary.Attributes["profile_uuid"]
						if uuidStep1 == "" {
							return fmt.Errorf("profile_uuid not set after create")
						}
						return nil
					},
				),
			},
			{
				// Content + label-mode change together: must be one in-place update.
				Config: config("3", `  labels_exclude_any = [fleetdm_label.test.name]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_configuration_profile.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "labels_exclude_any.0", labelName),
					resource.TestCheckNoResourceAttr("fleetdm_configuration_profile.test", "labels_include_any"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["fleetdm_configuration_profile.test"]
						got := rs.Primary.Attributes["profile_uuid"]
						if got != uuidStep1 {
							return fmt.Errorf("profile was replaced: uuid changed from %s to %s", uuidStep1, got)
						}
						return nil
					},
				),
			},
			{
				// Clear all labels: in-place update via empty PATCH.
				Config: config("3", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_configuration_profile.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("fleetdm_configuration_profile.test", "labels_exclude_any"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["fleetdm_configuration_profile.test"]
						got := rs.Primary.Attributes["profile_uuid"]
						if got != uuidStep1 {
							return fmt.Errorf("profile was replaced: uuid changed from %s to %s", uuidStep1, got)
						}
						return nil
					},
				),
			},
		},
	})
}
