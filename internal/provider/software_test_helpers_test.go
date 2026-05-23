package provider

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
)

// fakeFleetSoftwareServer is a multipurpose Fleet API fake used by the
// three new software-resource test files (custom_package, app_store_app,
// fleet_maintained_app). It mocks the minimum endpoint surface each
// resource hits and records the wire shape of every request so tests can
// assert on it.
//
// The legacy fleetdm_software_package test file still uses its own
// fakeFleetForLabels (inline in software_package_resource_test.go) —
// migrating it here would double the diff of this PR for no review
// benefit. When the legacy resource is removed in a future major, that
// file gets deleted wholesale and this helper becomes the only one.
type fakeFleetSoftwareServer struct {
	srv *httptest.Server
	mu  sync.Mutex

	// titleID is what Create endpoints return as the new title's ID. Tests
	// can override before driving the server; defaults to 71.
	titleID int

	// Per-title state mirrored back to subsequent GETs so the resource's
	// Read sees fields it just sent on Create/Update.
	titleName          string
	titleSelfService   bool
	titleInstallScript string
	titleAppStoreID    string
	titlePlatform      string
	titleSource        string // "pkg" / "app_store_app" / "fma" — drives detectSoftwareType branching

	// Wire observations from the most recent multipart Upload (POST
	// /software/package). Tests inspect these to verify Create behavior.
	uploadIncludeFieldSet    bool
	uploadExcludeFieldSet    bool
	uploadIncludeAllFieldSet bool
	uploadIncludeAllLabels   []string
	uploadInstallScript      string
	uploadCount              int

	// Wire observations from PATCH /software/titles/{id}/package.
	patchCount               int
	patchIncludeFieldSeen    bool
	patchExcludeFieldSeen    bool
	patchIncludeAllFieldSeen bool
	patchIncludeLabels       []string
	patchExcludeLabels       []string
	patchIncludeAllLabels    []string
	patchInstallScript       string
	patchSelfService         string

	// Wire observations from POST /software/app_store_apps (VPP).
	vppCreateCount  int
	vppAppStoreID   string
	vppPlatform     string
	vppSelfService  bool
	vppTeamID       int
	vppDisplayName  string
	vppCreateIncAll []string
	vppCreateIncAny []string
	vppCreateExcAny []string

	// Wire observations from PATCH /software/titles/{id}/app_store_app
	// (VPP update — JSON, not multipart).
	vppPatchCount         int
	vppPatchSelfService   bool
	vppPatchIncludeLabels []string
	vppPatchExcludeLabels []string
	vppPatchIncludeAll    []string
	vppPatchDisplayName   string

	// Wire observations from POST /software/fleet_maintained_apps.
	fmaCreateCount      int
	fmaCreateAppID      int
	fmaInstallScript    string
	fmaUninstallScript  string
	fmaAutomaticInstall bool
	fmaCreateIncludeAll []string
	fmaTeamID           int

	// setupExperienceSet is the server-side authoritative list of title
	// IDs currently flagged for setup-experience install for the test's
	// team+platform. Tracks the PUT /setup_experience/software endpoint.
	// Tests inspect this to verify install_during_setup actually flips
	// Fleet's state.
	setupExperienceSet  []int
	setupExperienceGets int
	setupExperiencePuts int

	// titleDisplayName and titleCategories mirror the most recent
	// Create/Patch fields so subsequent GET responses reflect them and
	// the resource's Read populates state correctly.
	titleDisplayName string
	titleCategories  []string

	// titleAutomaticInstallPolicies controls what the GET title response
	// reports for the automatic_install_policies computed list. Tests set
	// this when they want to exercise the policy-attached path.
	titleAutomaticInstallPolicies []map[string]any

	// uploadAutomaticInstall and uploadDisplayName / patch equivalents
	// capture the form values sent during Upload / Patch so tests can
	// assert on the wire shape.
	uploadAutomaticInstall string
	uploadDisplayName      string
	uploadCategories       string
	patchDisplayName       string
	patchCategories        string
}

// newFakeFleetSoftwareServer stands up an httptest server that handles the
// endpoints used by the three new software resources. Tests interact with
// it via f.srv.URL and inspect captured state via the exported fields
// under f.mu.
func newFakeFleetSoftwareServer(t *testing.T) *fakeFleetSoftwareServer {
	t.Helper()
	f := &fakeFleetSoftwareServer{
		titleID:     71,
		titleName:   "test-app",
		titleSource: "pkg",
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		// Policy list — needed by the binary-replace flow (custom_package only).
		case r.URL.Path == "/api/v1/fleet/global/policies" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"policies": []map[string]any{}})

		// POST /software/package — custom-package Create (multipart upload).
		case r.URL.Path == "/api/v1/fleet/software/package" && r.Method == http.MethodPost:
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("ParseMultipartForm (upload): %v", err)
				http.Error(w, "bad multipart", http.StatusBadRequest)
				return
			}
			f.mu.Lock()
			f.uploadCount++
			f.uploadInstallScript = r.FormValue("install_script")
			f.uploadAutomaticInstall = r.FormValue("automatic_install")
			f.uploadDisplayName = r.FormValue("display_name")
			f.uploadCategories = r.FormValue("categories")
			f.titleInstallScript = f.uploadInstallScript
			f.titleSelfService = r.FormValue("self_service") == "true"
			if f.uploadDisplayName != "" {
				f.titleDisplayName = f.uploadDisplayName
			}
			if f.uploadCategories != "" {
				_ = json.Unmarshal([]byte(f.uploadCategories), &f.titleCategories)
			}
			f.titleSource = "pkg"
			_, f.uploadIncludeFieldSet = r.MultipartForm.Value["labels_include_any"]
			_, f.uploadExcludeFieldSet = r.MultipartForm.Value["labels_exclude_any"]
			incAllVals, incAllSeen := r.MultipartForm.Value["labels_include_all"]
			f.uploadIncludeAllFieldSet = incAllSeen
			f.uploadIncludeAllLabels = nil
			if incAllSeen && len(incAllVals) > 0 {
				_ = json.Unmarshal([]byte(incAllVals[0]), &f.uploadIncludeAllLabels)
			}
			id := f.titleID
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_package": map[string]any{"title_id": id, "team_id": 0},
			})

		// POST /software/app_store_apps — VPP Create (JSON).
		case r.URL.Path == "/api/v1/fleet/software/app_store_apps" && r.Method == http.MethodPost:
			var body struct {
				AppStoreID       string   `json:"app_store_id"`
				TeamID           int      `json:"team_id"`
				Platform         string   `json:"platform"`
				SelfService      bool     `json:"self_service"`
				DisplayName      string   `json:"display_name"`
				LabelsIncludeAny []string `json:"labels_include_any"`
				LabelsExcludeAny []string `json:"labels_exclude_any"`
				LabelsIncludeAll []string `json:"labels_include_all"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.vppCreateCount++
			f.vppAppStoreID = body.AppStoreID
			f.vppPlatform = body.Platform
			f.vppSelfService = body.SelfService
			f.vppTeamID = body.TeamID
			f.vppDisplayName = body.DisplayName
			f.vppCreateIncAll = body.LabelsIncludeAll
			f.vppCreateIncAny = body.LabelsIncludeAny
			f.vppCreateExcAny = body.LabelsExcludeAny
			f.titleAppStoreID = body.AppStoreID
			f.titlePlatform = body.Platform
			f.titleSelfService = body.SelfService
			if body.DisplayName != "" {
				f.titleDisplayName = body.DisplayName
			}
			f.titleSource = "app_store_app"
			id := f.titleID
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"software_title_id": id})

		// POST /software/fleet_maintained_apps — FMA Create (JSON).
		case r.URL.Path == "/api/v1/fleet/software/fleet_maintained_apps" && r.Method == http.MethodPost:
			var body struct {
				FleetMaintainedAppID int      `json:"fleet_maintained_app_id"`
				TeamID               int      `json:"team_id"`
				InstallScript        string   `json:"install_script"`
				UninstallScript      string   `json:"uninstall_script"`
				SelfService          bool     `json:"self_service"`
				AutomaticInstall     bool     `json:"automatic_install"`
				LabelsIncludeAll     []string `json:"labels_include_all"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.fmaCreateCount++
			f.fmaCreateAppID = body.FleetMaintainedAppID
			f.fmaInstallScript = body.InstallScript
			f.fmaUninstallScript = body.UninstallScript
			f.fmaAutomaticInstall = body.AutomaticInstall
			f.fmaCreateIncludeAll = body.LabelsIncludeAll
			f.fmaTeamID = body.TeamID
			f.titleInstallScript = body.InstallScript
			f.titleSelfService = body.SelfService
			f.titleSource = "fma"
			id := f.titleID
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"software_title_id": id})

		// GET /software/titles/{id} — shared Read endpoint for all three.
		case r.URL.Path == "/api/v1/fleet/software/titles/"+strconv.Itoa(f.titleID) && r.Method == http.MethodGet:
			f.mu.Lock()
			source := f.titleSource
			payload := map[string]any{
				"id":             f.titleID,
				"name":           f.titleName,
				"display_name":   f.titleDisplayName,
				"source":         "pkg",
				"hosts_count":    0,
				"versions_count": 1,
				"versions": []map[string]any{
					{"id": 1, "version": "1.0.0", "hosts_count": 0},
				},
			}
			if f.titleCategories != nil {
				payload["categories"] = f.titleCategories
			}
			pkgBody := map[string]any{
				"title_id":       f.titleID,
				"platform":       "darwin",
				"hash_sha256":    hex.EncodeToString(sumOf([]byte("FAKEPKG"))),
				"self_service":   f.titleSelfService,
				"install_script": f.titleInstallScript,
			}
			// install_during_setup mirrors the setup_experience set.
			for _, id := range f.setupExperienceSet {
				if id == f.titleID {
					pkgBody["install_during_setup"] = true
					break
				}
			}
			if len(f.titleAutomaticInstallPolicies) > 0 {
				pkgBody["automatic_install_policies"] = f.titleAutomaticInstallPolicies
			}
			switch source {
			case "app_store_app":
				vppBody := map[string]any{
					"app_store_id":   f.titleAppStoreID,
					"platform":       f.titlePlatform,
					"name":           f.titleName,
					"latest_version": "1.0.0",
					"self_service":   f.titleSelfService,
				}
				for _, id := range f.setupExperienceSet {
					if id == f.titleID {
						vppBody["install_during_setup"] = true
						break
					}
				}
				if len(f.titleAutomaticInstallPolicies) > 0 {
					vppBody["automatic_install_policies"] = f.titleAutomaticInstallPolicies
				}
				payload["app_store_app"] = vppBody
			default: // "pkg" or "fma" — both Fleet-side use the software_package shape
				payload["software_package"] = pkgBody
			}
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"software_title": payload})

		// GET /setup_experience/software — the setup-experience set.
		case r.URL.Path == "/api/v1/fleet/setup_experience/software" && r.Method == http.MethodGet:
			f.mu.Lock()
			f.setupExperienceGets++
			arr := []map[string]any{}
			for _, id := range f.setupExperienceSet {
				arr = append(arr, map[string]any{"id": id})
			}
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"software_titles": arr})

		// PUT /setup_experience/software — replace-the-whole-list.
		case r.URL.Path == "/api/v1/fleet/setup_experience/software" && r.Method == http.MethodPut:
			var body struct {
				SoftwareTitleIDs []int `json:"software_title_ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.setupExperienceSet = append([]int{}, body.SoftwareTitleIDs...)
			f.setupExperiencePuts++
			f.mu.Unlock()
			w.WriteHeader(http.StatusOK)

		// PATCH /software/titles/{id}/package — custom_package + FMA Update.
		case r.URL.Path == "/api/v1/fleet/software/titles/"+strconv.Itoa(f.titleID)+"/package" && r.Method == http.MethodPatch:
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("ParseMultipartForm (patch): %v", err)
				http.Error(w, "bad multipart", http.StatusBadRequest)
				return
			}
			f.mu.Lock()
			f.patchCount++
			vals, incSeen := r.MultipartForm.Value["labels_include_any"]
			f.patchIncludeFieldSeen = incSeen
			f.patchIncludeLabels = nil
			if incSeen && len(vals) > 0 {
				_ = json.Unmarshal([]byte(vals[0]), &f.patchIncludeLabels)
			}
			vals, excSeen := r.MultipartForm.Value["labels_exclude_any"]
			f.patchExcludeFieldSeen = excSeen
			f.patchExcludeLabels = nil
			if excSeen && len(vals) > 0 {
				_ = json.Unmarshal([]byte(vals[0]), &f.patchExcludeLabels)
			}
			vals, incAllSeen := r.MultipartForm.Value["labels_include_all"]
			f.patchIncludeAllFieldSeen = incAllSeen
			f.patchIncludeAllLabels = nil
			if incAllSeen && len(vals) > 0 {
				_ = json.Unmarshal([]byte(vals[0]), &f.patchIncludeAllLabels)
			}
			f.patchInstallScript = r.FormValue("install_script")
			f.patchSelfService = r.FormValue("self_service")
			f.patchDisplayName = r.FormValue("display_name")
			f.patchCategories = r.FormValue("categories")
			if f.patchInstallScript != "" {
				f.titleInstallScript = f.patchInstallScript
			}
			if f.patchDisplayName != "" {
				f.titleDisplayName = f.patchDisplayName
			}
			if f.patchCategories != "" {
				_ = json.Unmarshal([]byte(f.patchCategories), &f.titleCategories)
			}
			f.titleSelfService = f.patchSelfService == "true"
			f.mu.Unlock()
			w.WriteHeader(http.StatusOK)

		// PATCH /software/titles/{id}/app_store_app — VPP Update (JSON).
		case r.URL.Path == "/api/v1/fleet/software/titles/"+strconv.Itoa(f.titleID)+"/app_store_app" && r.Method == http.MethodPatch:
			var body struct {
				TeamID           int      `json:"team_id"`
				SelfService      bool     `json:"self_service"`
				LabelsIncludeAny []string `json:"labels_include_any"`
				LabelsExcludeAny []string `json:"labels_exclude_any"`
				LabelsIncludeAll []string `json:"labels_include_all"`
				DisplayName      string   `json:"display_name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.vppPatchCount++
			f.vppPatchSelfService = body.SelfService
			f.vppPatchIncludeLabels = body.LabelsIncludeAny
			f.vppPatchExcludeLabels = body.LabelsExcludeAny
			f.vppPatchIncludeAll = body.LabelsIncludeAll
			f.vppPatchDisplayName = body.DisplayName
			f.titleSelfService = body.SelfService
			if body.DisplayName != "" {
				f.titleDisplayName = body.DisplayName
			}
			f.mu.Unlock()
			w.WriteHeader(http.StatusOK)

		// DELETE /software/titles/{id}/available_for_install — all three.
		case r.URL.Path == "/api/v1/fleet/software/titles/"+strconv.Itoa(f.titleID)+"/available_for_install" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}
