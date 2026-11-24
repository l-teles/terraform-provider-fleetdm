package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// TestAccSoftwareDataSources_aiToolMock checks that ai_tool = true reaches
// Fleet on both list data sources, an unset filter sends nothing, and the
// per-item flag maps true, false and omitted (Fleet Free) to true, false and
// null.
func TestAccSoftwareDataSources_aiToolMock(t *testing.T) {
	var mu sync.Mutex
	queries := map[string][]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if r.URL.Query().Has("ai_tool") {
			queries[r.URL.Path] = append(queries[r.URL.Path], r.URL.Query().Get("ai_tool"))
		} else {
			queries[r.URL.Path] = append(queries[r.URL.Path], "<absent>")
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/fleet/version":
			_, _ = w.Write([]byte(`{"version":"4.95.0"}`))
		case "/api/v1/fleet/software/titles":
			_, _ = w.Write([]byte(`{"count":3,"software_titles":[{"id":1,"name":"A","ai_tool":true},{"id":2,"name":"B","ai_tool":false},{"id":3,"name":"C"}]}`))
		case "/api/v1/fleet/software/versions":
			_, _ = w.Write([]byte(`{"count":2,"software":[{"id":4,"name":"A","version":"1","ai_tool":true},{"id":5,"name":"C","version":"1"}]}`))
		case "/api/v1/fleet/software/titles/1":
			_, _ = w.Write([]byte(`{"software_title":{"id":1,"name":"A","ai_tool":true}}`))
		case "/api/v1/fleet/software/versions/5":
			_, _ = w.Write([]byte(`{"software":{"id":5,"name":"C","version":"1"}}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %q
  api_key        = "test-token"
}

data "fleetdm_software_titles" "ai" {
  ai_tool = true
}

data "fleetdm_software_versions" "ai" {
  ai_tool = true
}

data "fleetdm_software_versions" "unfiltered" {
  ai_tool = false
}

data "fleetdm_software_title" "one" {
  id = "1"
}

data "fleetdm_software_version" "five" {
  id = "5"
}
`, server.URL)

	expectQueries := func(path string, want ...string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			got := map[string]int{}
			for _, v := range queries[path] {
				got[v]++
			}
			for _, w := range want {
				if got[w] == 0 {
					return fmt.Errorf("%s: expected a request with ai_tool=%s, got %q", path, w, queries[path])
				}
			}
			for v := range got {
				found := false
				for _, w := range want {
					found = found || v == w
				}
				if !found {
					return fmt.Errorf("%s: unexpected ai_tool=%s, got %q", path, v, queries[path])
				}
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
					resource.TestCheckResourceAttr("data.fleetdm_software_titles.ai", "software_titles.0.ai_tool", "true"),
					resource.TestCheckResourceAttr("data.fleetdm_software_titles.ai", "software_titles.1.ai_tool", "false"),
					resource.TestCheckNoResourceAttr("data.fleetdm_software_titles.ai", "software_titles.2.ai_tool"),
					resource.TestCheckResourceAttr("data.fleetdm_software_versions.ai", "software_versions.0.ai_tool", "true"),
					resource.TestCheckNoResourceAttr("data.fleetdm_software_versions.ai", "software_versions.1.ai_tool"),
					resource.TestCheckResourceAttr("data.fleetdm_software_title.one", "ai_tool", "true"),
					resource.TestCheckNoResourceAttr("data.fleetdm_software_version.five", "ai_tool"),
					expectQueries("/api/v1/fleet/software/titles", "true"),
					expectQueries("/api/v1/fleet/software/versions", "true", "<absent>"),
				),
			},
		},
	})
}

// TestAccSoftwareDataSources_aiToolOldFleet checks that ai_tool = true on
// Fleet older than 4.95, which ignores the parameter, fails instead of
// returning every title.
func TestAccSoftwareDataSources_aiToolOldFleet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/fleet/version":
			_, _ = w.Write([]byte(`{"version":"4.94.0"}`))
		case "/api/v1/fleet/software/titles", "/api/v1/fleet/software/versions":
			if r.URL.Query().Has("ai_tool") {
				t.Errorf("filtered request reached a pre-4.95 server: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"count":0,"software_titles":[],"software":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := func(ds, attrs string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %q
  api_key        = "test-token"
}

data %q "test" {
%s
}
`, server.URL, ds, attrs)
	}

	for _, ds := range []string{"fleetdm_software_titles", "fleetdm_software_versions"} {
		t.Run(ds, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      cfg(ds, "  ai_tool = true"),
						ExpectError: regexp.MustCompile(`(?s)requires\s+Fleet\s+Premium\s+4\.95\s+or\s+later`),
					},
					{
						Config: cfg(ds, "  ai_tool = false"),
					},
				},
			})
		})
	}
}

// TestCheckAIToolFilterSupported covers the version gate directly, including
// the error when the version can't be read.
func TestCheckAIToolFilterSupported(t *testing.T) {
	for _, tc := range []struct {
		name        string
		version     string // "" makes /version fail
		aiTool      bool
		wantErr     bool
		wantWarning bool
		wantCall    bool
	}{
		{name: "filter off", version: "4.94.0", wantCall: false},
		{name: "4.95", version: "4.95.0", aiTool: true, wantCall: true},
		{name: "4.94", version: "4.94.2", aiTool: true, wantErr: true, wantCall: true},
		{name: "unreadable version", version: "", aiTool: true, wantErr: true, wantCall: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if tc.version == "" {
					http.Error(w, "boom", http.StatusInternalServerError)
					return
				}
				_, _ = fmt.Fprintf(w, `{"version":%q}`, tc.version)
			}))
			defer server.Close()
			client, err := fleetdm.NewClient(fleetdm.ClientConfig{ServerAddress: server.URL, APIKey: "k"})
			if err != nil {
				t.Fatal(err)
			}

			var diags diag.Diagnostics
			checkAIToolFilterSupported(context.Background(), client, tc.aiTool, &diags)
			if diags.HasError() != tc.wantErr {
				t.Errorf("HasError = %v, want %v: %v", diags.HasError(), tc.wantErr, diags)
			}
			if (diags.WarningsCount() > 0) != tc.wantWarning {
				t.Errorf("warnings = %v, want warning %v", diags.Warnings(), tc.wantWarning)
			}
			if called != tc.wantCall {
				t.Errorf("version request made = %v, want %v", called, tc.wantCall)
			}
		})
	}
}

// TestAccSoftwareTitlesDataSource_aiToolLive reads a fleet that holds a
// Fleet-maintained app and checks that Premium reports ai_tool as a non-null
// false for it, and that the ai_tool filter is accepted. Requires Fleet
// Premium 4.95+.
func TestAccSoftwareTitlesDataSource_aiToolLive(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for live acceptance tests")
	}
	testAccPreCheck(t)
	fleetID, titleID := addFleetMaintainedAppOutOfBand(t, "tf-acc-team-ai-"+acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum), "1Password", "darwin")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
data "fleetdm_software_titles" "all" {
  team_id               = %[1]d
  available_for_install = true
}

data "fleetdm_software_titles" "ai" {
  team_id               = %[1]d
  available_for_install = true
  ai_tool               = true
}

data "fleetdm_software_versions" "ai" {
  team_id = %[1]d
  ai_tool = true
}

data "fleetdm_software_title" "one" {
  id      = "%[2]d"
  team_id = %[1]d
}
`, fleetID, titleID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_software_titles.all", "software_titles.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_software_titles.all", "software_titles.0.id", fmt.Sprint(titleID)),
					resource.TestCheckResourceAttr("data.fleetdm_software_titles.all", "software_titles.0.ai_tool", "false"),
					// No host has reported the app, so nothing is flagged as an AI tool.
					resource.TestCheckResourceAttr("data.fleetdm_software_titles.ai", "software_titles.#", "0"),
					resource.TestCheckResourceAttr("data.fleetdm_software_title.one", "ai_tool", "false"),
					resource.TestCheckResourceAttr("data.fleetdm_software_versions.ai", "software_versions.#", "0"),
				),
			},
		},
	})
}
