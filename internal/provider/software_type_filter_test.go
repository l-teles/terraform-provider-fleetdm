package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccSoftwareDataSources_typeFilterMock checks that `source` and
// `extension_for` on both software list data sources reach Fleet as one
// sorted, comma-separated parameter each.
func TestAccSoftwareDataSources_typeFilterMock(t *testing.T) {
	var mu sync.Mutex
	queries := map[string]url.Values{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries[r.URL.Path] = r.URL.Query()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/fleet/software/titles":
			_, _ = w.Write([]byte(`{"software_titles":[{"id":1,"name":"Ext","source":"chrome_extensions"}],"count":1}`))
		case "/api/v1/fleet/software/versions":
			_, _ = w.Write([]byte(`{"software":[{"id":2,"name":"Ext","version":"1.0","source":"chrome_extensions"}],"count":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %q
  api_key        = "test-token"
}

data "fleetdm_software_titles" "test" {
  source        = ["firefox_addons", "chrome_extensions"]
  extension_for = ["edge", "brave"]
}

data "fleetdm_software_versions" "test" {
  source        = ["firefox_addons", "chrome_extensions"]
  extension_for = ["edge", "brave"]
}
`, server.URL)

	expectQuery := func(path string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			q, ok := queries[path]
			if !ok {
				return fmt.Errorf("no request to %s", path)
			}
			if got := q.Get("source"); got != "chrome_extensions,firefox_addons" {
				return fmt.Errorf("%s: source=%q", path, got)
			}
			if got := q.Get("extension_for"); got != "brave,edge" {
				return fmt.Errorf("%s: extension_for=%q", path, got)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_software_titles.test", "software_titles.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_software_titles.test", "source.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_software_versions.test", "software_versions.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_software_versions.test", "extension_for.#", "2"),
					expectQuery("/api/v1/fleet/software/titles"),
					expectQuery("/api/v1/fleet/software/versions"),
				),
			},
		},
	})
}

// TestAccSoftwareDataSources_typeFilterValidation pins the plan-time copies
// of Fleet's ParseSoftwareTypeFilter checks. No server is reached.
func TestAccSoftwareDataSources_typeFilterValidation(t *testing.T) {
	cfg := func(dataSource, attrs string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = "http://127.0.0.1:1"
  api_key        = "test-token"
}

data %q "test" {
%s
}
`, dataSource, attrs)
	}

	cases := []struct {
		name  string
		attrs string
		err   string
	}{
		{"unknown source", `  source = ["not_a_source"]`, `(?s)Invalid\s+Attribute\s+Value\s+Match.*must\s+be\s+one\s+of`},
		{"unknown extension_for", "  source = [\"chrome_extensions\"]\n  extension_for = [\"netscape\"]", `(?s)Invalid\s+Attribute\s+Value\s+Match.*must\s+be\s+one\s+of`},
		{"extension_for without source", `  extension_for = ["brave"]`, `(?s)Invalid\s+Attribute\s+Combination.*"source"\s+must\s+be\s+specified`},
		{"source not selected", "  source = [\"apps\"]\n  extension_for = [\"brave\"]", `(?s)Source\s+not\s+selected.*"brave"\s+is\s+a\s+"chrome_extensions"\s+value`},
	}
	for _, ds := range []string{"fleetdm_software_titles", "fleetdm_software_versions"} {
		for _, tc := range cases {
			t.Run(ds+"/"+tc.name, func(t *testing.T) {
				resource.Test(t, resource.TestCase{
					ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
					Steps: []resource.TestStep{
						{
							Config:      cfg(ds, tc.attrs),
							ExpectError: regexp.MustCompile(tc.err),
						},
					},
				})
			})
		}
	}
}

// TestAccSoftwareTitlesDataSource_sourceFilterLive needs no enrolled hosts:
// it uploads a `.py` and a `.sh` script package to a new fleet, which Fleet
// files under the py_packages and sh_packages sources, and checks that
// filtering on one source returns that title and not the other. Requires
// Fleet Premium 4.94+.
func TestAccSoftwareTitlesDataSource_sourceFilterLive(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	tmpDir := t.TempDir()
	pyName := fmt.Sprintf("tf-acc-%s.py", suffix)
	shName := fmt.Sprintf("tf-acc-%s.sh", suffix)
	// Unique contents keep Fleet from deduplicating against an earlier run.
	if err := os.WriteFile(filepath.Join(tmpDir, pyName), []byte(fmt.Sprintf("#!/usr/bin/env python3\nprint(%q)\n", suffix)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, shName), []byte(fmt.Sprintf("#!/bin/sh\necho %q\n", suffix)), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := func(source string) string {
		return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = "tf-acc-team-swfilter-%[1]s"
}

resource "fleetdm_software_custom_package" "py" {
  team_id      = fleetdm_fleet.test.id
  package_path = %[2]q
  filename     = %[3]q
}

resource "fleetdm_software_custom_package" "sh" {
  team_id      = fleetdm_fleet.test.id
  package_path = %[4]q
  filename     = %[5]q
}

data "fleetdm_software_titles" "test" {
  team_id               = fleetdm_fleet.test.id
  available_for_install = true
  source                = [%[6]q]
  depends_on            = [fleetdm_software_custom_package.py, fleetdm_software_custom_package.sh]
}
`, suffix, filepath.Join(tmpDir, pyName), pyName, filepath.Join(tmpDir, shName), shName, source)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("py_packages"),
				Check:  checkSourceFilteredTitles("py_packages", "fleetdm_software_custom_package.py", "fleetdm_software_custom_package.sh"),
			},
			{
				Config: cfg("sh_packages"),
				Check:  checkSourceFilteredTitles("sh_packages", "fleetdm_software_custom_package.sh", "fleetdm_software_custom_package.py"),
			},
		},
	})
}

// checkSourceFilteredTitles asserts every listed title has the given source,
// the wanted package's title is listed, and the other package's is not.
func checkSourceFilteredTitles(source, wantRes, notRes string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ds, ok := s.RootModule().Resources["data.fleetdm_software_titles.test"]
		if !ok {
			return fmt.Errorf("data source not in state")
		}
		titleID := func(name string) (string, error) {
			rs, ok := s.RootModule().Resources[name]
			if !ok {
				return "", fmt.Errorf("%s not in state", name)
			}
			return rs.Primary.Attributes["title_id"], nil
		}
		want, err := titleID(wantRes)
		if err != nil {
			return err
		}
		not, err := titleID(notRes)
		if err != nil {
			return err
		}
		n, err := strconv.Atoi(ds.Primary.Attributes["software_titles.#"])
		if err != nil {
			return err
		}
		found := false
		for i := range n {
			prefix := fmt.Sprintf("software_titles.%d.", i)
			if got := ds.Primary.Attributes[prefix+"source"]; got != source {
				return fmt.Errorf("title %s has source %q, want only %q", ds.Primary.Attributes[prefix+"id"], got, source)
			}
			switch ds.Primary.Attributes[prefix+"id"] {
			case want:
				found = true
			case not:
				return fmt.Errorf("title %s from %s should be filtered out by source = %q", not, notRes, source)
			}
		}
		if !found {
			return fmt.Errorf("title %s from %s not listed with source = %q (got %d titles)", want, wantRes, source, n)
		}
		return nil
	}
}
