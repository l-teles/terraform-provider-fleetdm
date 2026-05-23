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
	uploadIncludeFieldSet bool
	uploadExcludeFieldSet bool
	uploadInstallScript   string
	uploadCount           int

	// Wire observations from PATCH /software/titles/{id}/package.
	patchCount            int
	patchIncludeFieldSeen bool
	patchExcludeFieldSeen bool
	patchIncludeLabels    []string
	patchExcludeLabels    []string
	patchInstallScript    string
	patchSelfService      string

	// Wire observations from POST /software/app_store_apps (VPP).
	vppCreateCount int
	vppAppStoreID  string
	vppPlatform    string
	vppSelfService bool
	vppTeamID      int

	// Wire observations from PATCH /software/titles/{id}/app_store_app
	// (VPP update — JSON, not multipart).
	vppPatchCount         int
	vppPatchSelfService   bool
	vppPatchIncludeLabels []string
	vppPatchExcludeLabels []string

	// Wire observations from POST /software/fleet_maintained_apps.
	fmaCreateCount   int
	fmaCreateAppID   int
	fmaInstallScript string
	fmaTeamID        int
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
			f.titleInstallScript = f.uploadInstallScript
			f.titleSelfService = r.FormValue("self_service") == "true"
			f.titleSource = "pkg"
			_, f.uploadIncludeFieldSet = r.MultipartForm.Value["labels_include_any"]
			_, f.uploadExcludeFieldSet = r.MultipartForm.Value["labels_exclude_any"]
			id := f.titleID
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_package": map[string]any{"title_id": id, "team_id": 0},
			})

		// POST /software/app_store_apps — VPP Create (JSON).
		case r.URL.Path == "/api/v1/fleet/software/app_store_apps" && r.Method == http.MethodPost:
			var body struct {
				AppStoreID  string `json:"app_store_id"`
				TeamID      int    `json:"team_id"`
				Platform    string `json:"platform"`
				SelfService bool   `json:"self_service"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.vppCreateCount++
			f.vppAppStoreID = body.AppStoreID
			f.vppPlatform = body.Platform
			f.vppSelfService = body.SelfService
			f.vppTeamID = body.TeamID
			f.titleAppStoreID = body.AppStoreID
			f.titlePlatform = body.Platform
			f.titleSelfService = body.SelfService
			f.titleSource = "app_store_app"
			id := f.titleID
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"software_title_id": id})

		// POST /software/fleet_maintained_apps — FMA Create (JSON).
		case r.URL.Path == "/api/v1/fleet/software/fleet_maintained_apps" && r.Method == http.MethodPost:
			var body struct {
				FleetMaintainedAppID int    `json:"fleet_maintained_app_id"`
				TeamID               int    `json:"team_id"`
				InstallScript        string `json:"install_script"`
				SelfService          bool   `json:"self_service"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.fmaCreateCount++
			f.fmaCreateAppID = body.FleetMaintainedAppID
			f.fmaInstallScript = body.InstallScript
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
				"source":         "pkg",
				"hosts_count":    0,
				"versions_count": 1,
				"versions": []map[string]any{
					{"id": 1, "version": "1.0.0", "hosts_count": 0},
				},
			}
			switch source {
			case "app_store_app":
				payload["app_store_app"] = map[string]any{
					"app_store_id":   f.titleAppStoreID,
					"platform":       f.titlePlatform,
					"name":           f.titleName,
					"latest_version": "1.0.0",
					"self_service":   f.titleSelfService,
				}
			default: // "pkg" or "fma" — both Fleet-side use the software_package shape
				payload["software_package"] = map[string]any{
					"title_id":       f.titleID,
					"platform":       "darwin",
					"hash_sha256":    hex.EncodeToString(sumOf([]byte("FAKEPKG"))),
					"self_service":   f.titleSelfService,
					"install_script": f.titleInstallScript,
				}
			}
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"software_title": payload})

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
			f.patchInstallScript = r.FormValue("install_script")
			f.patchSelfService = r.FormValue("self_service")
			if f.patchInstallScript != "" {
				f.titleInstallScript = f.patchInstallScript
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
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.vppPatchCount++
			f.vppPatchSelfService = body.SelfService
			f.vppPatchIncludeLabels = body.LabelsIncludeAny
			f.vppPatchExcludeLabels = body.LabelsExcludeAny
			f.titleSelfService = body.SelfService
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
