package fleetdm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// SoftwareTitle represents a software title in FleetDM.
type SoftwareTitle struct {
	ID               int                    `json:"id"`
	Name             string                 `json:"name"`
	DisplayName      string                 `json:"display_name,omitempty"`
	Source           string                 `json:"source"`
	IconURL          string                 `json:"icon_url,omitempty"`
	HostsCount       int                    `json:"hosts_count"`
	VersionsCount    int                    `json:"versions_count"`
	Versions         []SoftwareTitleVersion `json:"versions,omitempty"`
	BundleIdentifier string                 `json:"bundle_identifier,omitempty"`
	Categories       []string               `json:"categories,omitempty"`
	SoftwarePackage  *SoftwarePackageInfo   `json:"software_package,omitempty"`
	AppStoreApp      *AppStoreAppInfo       `json:"app_store_app,omitempty"`
	CountsUpdatedAt  *time.Time             `json:"counts_updated_at,omitempty"`

	// AutoUpdateEnabled / AutoUpdateWindowStart / AutoUpdateWindowEnd are the
	// App Store (VPP) automatic-update settings. Fleet embeds them at the
	// *title* level (SoftwareAutoUpdateConfig on fleet.SoftwareTitle), not
	// inside app_store_app, and omits them entirely when unset — verified
	// against a live Fleet v4.90.0: an untouched title's GET response carries
	// none of the three keys. Pointers so "absent" stays distinguishable from
	// "explicitly false / empty", which the resource layer's opt-in Read
	// convention depends on.
	AutoUpdateEnabled     *bool   `json:"auto_update_enabled,omitempty"`
	AutoUpdateWindowStart *string `json:"auto_update_window_start,omitempty"`
	AutoUpdateWindowEnd   *string `json:"auto_update_window_end,omitempty"`
}

// AutomaticInstallPolicyRef points at a Fleet policy that auto-installs a
// software title on hosts that fail the policy. Returned as part of
// software_package.automatic_install_policies / app_store_app's policies
// list. The provider exposes this as a Computed list attribute on each
// software resource so users can see (and reference) the auto-created
// policies without leaving the Fleet UI.
type AutomaticInstallPolicyRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// SoftwareTitleVersion represents a version of a software title.
type SoftwareTitleVersion struct {
	ID              int      `json:"id"`
	Version         string   `json:"version"`
	Vulnerabilities []string `json:"vulnerabilities,omitempty"`
	HostsCount      int      `json:"hosts_count,omitempty"`
}

// SoftwarePackageInfo represents software package installation info.
type SoftwarePackageInfo struct {
	Name               string   `json:"name,omitempty"`
	Version            string   `json:"version,omitempty"`
	Platform           string   `json:"platform,omitempty"`
	SelfService        bool     `json:"self_service,omitempty"`
	InstallDuringSetup *bool    `json:"install_during_setup,omitempty"`
	InstallScript      string   `json:"install_script,omitempty"`
	UninstallScript    string   `json:"uninstall_script,omitempty"`
	PreInstallQuery    string   `json:"pre_install_query,omitempty"`
	PostInstallScript  string   `json:"post_install_script,omitempty"`
	HashSHA256         string   `json:"hash_sha256,omitempty"`
	Categories         []string `json:"categories,omitempty"`
	// PinnedVersion is the Fleet-maintained-app version pin currently in
	// effect for this package, echoed by GET /software/titles/{id} as
	// `software_package.pinned_version`. Fleet omits the key when the title
	// tracks the catalog's latest version (no pin), so a nil pointer means
	// "not pinned" — verified against a live Fleet v4.90.0.
	PinnedVersion            *string                     `json:"pinned_version,omitempty"`
	LabelsIncludeAny         []SoftwareLabel             `json:"labels_include_any,omitempty"`
	LabelsExcludeAny         []SoftwareLabel             `json:"labels_exclude_any,omitempty"`
	LabelsIncludeAll         []SoftwareLabel             `json:"labels_include_all,omitempty"`
	AutomaticInstallPolicies []AutomaticInstallPolicyRef `json:"automatic_install_policies,omitempty"`
}

// AppStoreAppInfo represents App Store app info.
type AppStoreAppInfo struct {
	AdamID                   string                      `json:"app_store_id,omitempty"`
	Platform                 string                      `json:"platform,omitempty"`
	Name                     string                      `json:"name,omitempty"`
	LatestVersion            string                      `json:"latest_version,omitempty"`
	SelfService              bool                        `json:"self_service,omitempty"`
	InstallDuringSetup       *bool                       `json:"install_during_setup,omitempty"`
	LabelsIncludeAny         []SoftwareLabel             `json:"labels_include_any,omitempty"`
	LabelsExcludeAny         []SoftwareLabel             `json:"labels_exclude_any,omitempty"`
	LabelsIncludeAll         []SoftwareLabel             `json:"labels_include_all,omitempty"`
	AutomaticInstallPolicies []AutomaticInstallPolicyRef `json:"automatic_install_policies,omitempty"`
	// Configuration is the app's managed app configuration, echoed back on
	// read. Fleet's wire shape is dual: a JSON object for Android Play Store
	// apps, and a JSON *string* containing XML for iOS/iPadOS. Kept as
	// json.RawMessage so both survive the round trip; use
	// DecodeAppConfiguration to turn it back into the provider's raw string.
	Configuration json.RawMessage `json:"configuration,omitempty"`
}

// AddAppStoreAppRequest represents the request body for adding a VPP app.
//
// Fleet's Add endpoint accepts `configuration` but NOT the auto_update_*
// fields — those only exist on the Update endpoint, so the resource layer
// applies them with a follow-up PATCH after create.
type AddAppStoreAppRequest struct {
	AppStoreID  string `json:"app_store_id"`
	TeamID      int    `json:"team_id"`
	Platform    string `json:"platform,omitempty"`
	SelfService bool   `json:"self_service,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	// Configuration is the app's managed app configuration. Omitted when nil.
	Configuration json.RawMessage `json:"configuration,omitempty"`
}

// UpdateAppStoreAppRequest represents the request body for updating a VPP app.
//
// Label slice fields follow the convention documented on UpdatePolicyRequest
// in policies.go: nil slice → JSON `null` → "no change"; empty slice → JSON
// `[]` → "clear all labels"; populated → set. No `omitempty` on the three
// label fields so the null/empty/populated distinction reaches Fleet. Only
// one of labels_include_all, labels_include_any, labels_exclude_any may be
// non-nil per request; the resource schema's ConflictsWith validators
// enforce that at plan time.
//
// The Fleet 4.90 additions (Configuration, AutoUpdate*) all carry `omitempty`
// and pointer/RawMessage types so they are absent from the body unless the
// caller opted in. That keeps this request byte-identical to its pre-4.90
// shape for configurations that don't use the new attributes, so the provider
// remains usable against older Fleet servers.
//
// Fleet validates the auto-update window server-side: enabling automatic
// updates without both window bounds fails with "Start and end time must both
// be set" (verified against a live Fleet v4.90.0). The resource schema mirrors
// that with AlsoRequires validators plus a ValidateConfig check so the failure
// surfaces at plan time instead of mid-apply.
type UpdateAppStoreAppRequest struct {
	TeamID           int      `json:"team_id"`
	SelfService      bool     `json:"self_service"`
	DisplayName      string   `json:"display_name,omitempty"`
	LabelsIncludeAny []string `json:"labels_include_any"`
	LabelsExcludeAny []string `json:"labels_exclude_any"`
	LabelsIncludeAll []string `json:"labels_include_all"`
	// Configuration is the app's managed app configuration (JSON object for
	// Android, JSON string of XML for iOS/iPadOS). Omitted when nil.
	Configuration json.RawMessage `json:"configuration,omitempty"`
	// AutoUpdateEnabled and the window bounds drive Fleet's automatic-update
	// maintenance window for iOS/iPadOS App Store apps. Pointers + omitempty:
	// a nil pointer leaves Fleet's current setting untouched.
	AutoUpdateEnabled     *bool   `json:"auto_update_enabled,omitempty"`
	AutoUpdateWindowStart *string `json:"auto_update_window_start,omitempty"`
	AutoUpdateWindowEnd   *string `json:"auto_update_window_end,omitempty"`
}

// EncodeAppConfiguration converts the provider's raw `configuration` string
// into the JSON shape Fleet's app_store_app endpoints expect.
//
// Fleet decodes the field as json.RawMessage and then branches on platform
// (ee/server/service/vpp.go):
//
//   - Android Play Store apps: the value must be a JSON *object* (the managed
//     configuration itself).
//   - iOS / iPadOS apps: the value must be a JSON *string* whose contents are
//     the XML payload — Fleet json.Unmarshal's it into a string and errors with
//     "expected configuration as a JSON string containing the XML" otherwise.
//
// The provider exposes a single raw string attribute for both, so this helper
// picks the encoding from the content: JSON (the Android case) is passed
// through untouched, and anything else — notably raw XML — is marshalled into a
// JSON string. Callers pass the payload in its natural form; pre-encoding XML
// with jsonencode() is NOT the supported contract (see
// SameAppConfiguration for how an already-encoded value is kept stable).
func EncodeAppConfiguration(raw string) (json.RawMessage, error) {
	if raw == "" {
		return nil, nil
	}
	if json.Valid([]byte(raw)) {
		return json.RawMessage(raw), nil
	}
	// Input that clearly means to be a JSON object/array but doesn't parse must
	// fail here. Wrapping it in a JSON string would "succeed" and then draw a
	// confusing complaint from Fleet about the configuration not being an
	// object — pointing at the wrong problem. A leading byte-order mark is the
	// classic offender (a BOM-prefixed document is not valid JSON), so it gets
	// named explicitly. Detection trims leading whitespace and a BOM; the
	// payload itself is never silently rewritten.
	trimmed := strings.TrimLeft(raw, " \t\r\n")
	const bom = "\uFEFF"
	hadBOM := strings.HasPrefix(trimmed, bom)
	trimmed = strings.TrimLeft(strings.TrimPrefix(trimmed, bom), " \t\r\n")
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		hint := ""
		if hadBOM {
			hint = " The value starts with a byte-order mark (BOM), which is not valid JSON — strip it (for example with a file() read of a UTF-8 file without a BOM)."
		}
		return nil, fmt.Errorf("invalid JSON configuration: the value looks like JSON (it starts with %q) but does not parse.%s", trimmed[:1], hint)
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to encode app configuration: %w", err)
	}
	// Note on the wire form: encoding/json escapes <, > and & as <, >
	// and & — which the XML case hits on every tag. This is cosmetic:
	// Fleet json.Unmarshal's the field back into a string and gets the original
	// XML byte for byte. Suppressing the escaping here would be pointless
	// anyway, because the request body is marshalled again as a whole struct by
	// the shared doRequest helper, which re-escapes it.
	return encoded, nil
}

// SameAppConfiguration reports whether two provider-side `configuration`
// strings mean the same thing to Fleet.
//
// It exists because Encode and Decode are not inverses for every input, so a
// byte comparison between what the user wrote and what Fleet echoes back can
// report a difference where there is none, producing a diff that no apply can
// ever settle. Two cases:
//
//   - A value that is already an encoded JSON string — `jsonencode("<dict/>")`,
//     i.e. `"<dict/>"` quotes included — encodes as a passthrough but decodes to
//     the *unquoted* XML. Byte-comparing the echo against state would flip
//     state between the two forms forever.
//   - Android JSON objects are semantically insensitive to key order and
//     whitespace, but Fleet may return them normalized.
//
// The fix is to compare canonical forms rather than bytes: the decoded payload,
// and for JSON payloads a key-sorted re-encoding of it. Callers use this to
// decide whether to adopt Fleet's echo at all — when the answer is "same", the
// stored value is left exactly as the user wrote it.
func SameAppConfiguration(a, b string) bool {
	return canonicalAppConfiguration(a) == canonicalAppConfiguration(b)
}

// canonicalAppConfiguration reduces a raw configuration string to a comparable
// form: run it through the same encoding Fleet receives, decode it back (which
// collapses "raw XML" and "pre-encoded XML" onto the same value), then, if the
// result is JSON, re-marshal it so map keys are sorted and whitespace is
// normalized. Any error along the way falls back to the input unchanged —
// this is a comparison helper, so a failure to canonicalize must never be
// mistaken for equality.
func canonicalAppConfiguration(raw string) string {
	if raw == "" {
		return ""
	}
	encoded, err := EncodeAppConfiguration(raw)
	if err != nil {
		return raw
	}
	decoded := DecodeAppConfiguration(encoded)
	var parsed any
	if err := json.Unmarshal([]byte(decoded), &parsed); err != nil {
		// Not JSON (the XML case) — the decoded string is already canonical.
		return decoded
	}
	// reflect is not needed for equality here: marshalling a decoded value
	// sorts object keys, so the resulting strings compare directly.
	normalized, err := json.Marshal(parsed)
	if err != nil {
		return decoded
	}
	return string(normalized)
}

// DecodeAppConfiguration is the inverse of EncodeAppConfiguration: it turns
// the wire value back into the raw string the provider stores in state. A JSON
// string is unquoted (the iOS/iPadOS XML case); anything else is returned
// verbatim (the Android JSON-object case).
func DecodeAppConfiguration(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	return string(raw)
}

// FleetMaintainedApp represents a Fleet Maintained App.
type FleetMaintainedApp struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	Slug            string `json:"slug"`
	Platform        string `json:"platform"`
	Version         string `json:"version,omitempty"`
	SoftwareTitleID *int   `json:"software_title_id,omitempty"`
	Filename        string `json:"filename,omitempty"`
	URL             string `json:"url,omitempty"`
	InstallScript   string `json:"install_script,omitempty"`
	UninstallScript string `json:"uninstall_script,omitempty"`
}

// AddFleetMaintainedAppRequest represents the request body for adding a Fleet Maintained App.
//
// AutomaticInstall maps to Fleet's documented `automatic_install` body field
// (creates a policy that triggers install on hosts missing the software);
// this is the policy-based auto-install, distinct from the
// setup-experience flag which is set via the separate
// PUT /setup_experience/software endpoint.
type AddFleetMaintainedAppRequest struct {
	FleetMaintainedAppID int      `json:"fleet_maintained_app_id"`
	TeamID               int      `json:"team_id"`
	InstallScript        string   `json:"install_script,omitempty"`
	UninstallScript      string   `json:"uninstall_script,omitempty"`
	PreInstallQuery      string   `json:"pre_install_query,omitempty"`
	PostInstallScript    string   `json:"post_install_script,omitempty"`
	SelfService          bool     `json:"self_service,omitempty"`
	AutomaticInstall     bool     `json:"automatic_install,omitempty"`
	LabelsIncludeAny     []string `json:"labels_include_any,omitempty"`
	LabelsExcludeAny     []string `json:"labels_exclude_any,omitempty"`
	LabelsIncludeAll     []string `json:"labels_include_all,omitempty"`
}

// SoftwareVersion represents a software version in FleetDM.
type SoftwareVersion struct {
	ID               int                     `json:"id"`
	Name             string                  `json:"name"`
	Version          string                  `json:"version"`
	Source           string                  `json:"source"`
	BundleIdentifier string                  `json:"bundle_identifier,omitempty"`
	Release          string                  `json:"release,omitempty"`
	Vendor           string                  `json:"vendor,omitempty"`
	Arch             string                  `json:"arch,omitempty"`
	GeneratedCPE     string                  `json:"generated_cpe,omitempty"`
	HostsCount       int                     `json:"hosts_count"`
	Vulnerabilities  []SoftwareVulnerability `json:"vulnerabilities,omitempty"`
	CountsUpdatedAt  time.Time               `json:"counts_updated_at"`
	TitleID          int                     `json:"title_id,omitempty"`
}

// SoftwareVulnerability represents a software vulnerability.
type SoftwareVulnerability struct {
	CVE               string   `json:"cve"`
	DetailsLink       string   `json:"details_link,omitempty"`
	CVSSScore         *float64 `json:"cvss_score,omitempty"`
	EPSSProbability   *float64 `json:"epss_probability,omitempty"`
	CISAKnownExploit  bool     `json:"cisa_known_exploit,omitempty"`
	CVEPublished      string   `json:"cve_published,omitempty"`
	CVEDescription    string   `json:"cve_description,omitempty"`
	ResolvedInVersion *string  `json:"resolved_in_version,omitempty"`
}

// ListOptions contains common pagination and ordering options.
type ListOptions struct {
	Page           int
	PerPage        int
	OrderKey       string
	OrderDirection string
}

// applyListParams adds pagination and ordering parameters to a params map.
func (o ListOptions) applyListParams(params map[string]string) {
	if o.Page > 0 {
		params["page"] = strconv.Itoa(o.Page)
	}
	if o.PerPage > 0 {
		params["per_page"] = strconv.Itoa(o.PerPage)
	}
	if o.OrderKey != "" {
		params["order_key"] = o.OrderKey
	}
	if o.OrderDirection != "" {
		params["order_direction"] = o.OrderDirection
	}
}

// SoftwareTitleListOptions represents options for listing software titles.
type SoftwareTitleListOptions struct {
	ListOptions
	TeamID              *int
	Query               string
	AvailableForInstall bool
	SelfService         bool
	VulnerableOnly      bool
}

// SoftwareVersionListOptions represents options for listing software versions.
type SoftwareVersionListOptions struct {
	ListOptions
	TeamID         *int
	Query          string
	VulnerableOnly bool
}

// listSoftwareTitlesResponse is the API response for listing software titles.
type listSoftwareTitlesResponse struct {
	SoftwareTitles  []SoftwareTitle `json:"software_titles"`
	Count           int             `json:"count"`
	CountsUpdatedAt *time.Time      `json:"counts_updated_at,omitempty"`
	Meta            *PaginationMeta `json:"meta,omitempty"`
}

// getSoftwareTitleResponse is the API response for getting a software title.
type getSoftwareTitleResponse struct {
	SoftwareTitle *SoftwareTitle `json:"software_title"`
}

// listSoftwareVersionsResponse is the API response for listing software versions.
type listSoftwareVersionsResponse struct {
	Software        []SoftwareVersion `json:"software"`
	Count           int               `json:"count"`
	CountsUpdatedAt *time.Time        `json:"counts_updated_at,omitempty"`
	Meta            *PaginationMeta   `json:"meta,omitempty"`
}

// getSoftwareVersionResponse is the API response for getting a software version.
type getSoftwareVersionResponse struct {
	Software *SoftwareVersion `json:"software"`
}

// ListSoftwareTitles retrieves all software titles.
func (c *Client) ListSoftwareTitles(ctx context.Context, opts SoftwareTitleListOptions) ([]SoftwareTitle, int, error) {
	params := make(map[string]string)

	if opts.TeamID != nil {
		params["team_id"] = strconv.Itoa(*opts.TeamID)
	}
	if opts.Query != "" {
		params["query"] = opts.Query
	}
	if opts.AvailableForInstall {
		params["available_for_install"] = "true"
	}
	if opts.SelfService {
		params["self_service"] = "true"
	}
	if opts.VulnerableOnly {
		params["vulnerable"] = "true"
	}
	opts.applyListParams(params)

	var resp listSoftwareTitlesResponse
	err := c.Get(ctx, "/software/titles", params, &resp)
	if err != nil {
		return nil, 0, err
	}
	return resp.SoftwareTitles, resp.Count, nil
}

// GetSoftwareTitle retrieves a software title by ID.
func (c *Client) GetSoftwareTitle(ctx context.Context, id int, teamID *int) (*SoftwareTitle, error) {
	params := make(map[string]string)
	if teamID != nil {
		params["team_id"] = strconv.Itoa(*teamID)
	}

	var resp getSoftwareTitleResponse
	err := c.Get(ctx, fmt.Sprintf("/software/titles/%d", id), params, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to get software title %d: %w", id, err)
	}
	return resp.SoftwareTitle, nil
}

// ListSoftwareVersions retrieves all software versions.
func (c *Client) ListSoftwareVersions(ctx context.Context, opts SoftwareVersionListOptions) ([]SoftwareVersion, int, error) {
	params := make(map[string]string)

	if opts.TeamID != nil {
		params["team_id"] = strconv.Itoa(*opts.TeamID)
	}
	if opts.Query != "" {
		params["query"] = opts.Query
	}
	if opts.VulnerableOnly {
		params["vulnerable"] = "true"
	}
	opts.applyListParams(params)

	var resp listSoftwareVersionsResponse
	err := c.Get(ctx, "/software/versions", params, &resp)
	if err != nil {
		return nil, 0, err
	}
	return resp.Software, resp.Count, nil
}

// GetSoftwareVersion retrieves a software version by ID.
func (c *Client) GetSoftwareVersion(ctx context.Context, id int, teamID *int) (*SoftwareVersion, error) {
	params := make(map[string]string)
	if teamID != nil {
		params["team_id"] = strconv.Itoa(*teamID)
	}

	var resp getSoftwareVersionResponse
	err := c.Get(ctx, fmt.Sprintf("/software/versions/%d", id), params, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to get software version %d: %w", id, err)
	}
	return resp.Software, nil
}

// SoftwareInstaller represents a software installer/package in FleetDM.
type SoftwareInstaller struct {
	TitleID           int              `json:"software_title_id"`
	TeamID            *int             `json:"team_id,omitempty"`
	Name              string           `json:"name"`
	Version           string           `json:"version"`
	Filename          string           `json:"filename,omitempty"`
	Platform          string           `json:"platform,omitempty"`
	InstallScript     string           `json:"install_script,omitempty"`
	UninstallScript   string           `json:"uninstall_script,omitempty"`
	PreInstallQuery   string           `json:"pre_install_query,omitempty"`
	PostInstallScript string           `json:"post_install_script,omitempty"`
	SelfService       bool             `json:"self_service,omitempty"`
	AutomaticInstall  bool             `json:"automatic_install,omitempty"`
	LabelsIncludeAny  []SoftwareLabel  `json:"labels_include_any,omitempty"`
	LabelsExcludeAny  []SoftwareLabel  `json:"labels_exclude_any,omitempty"`
	UploadedAt        time.Time        `json:"uploaded_at,omitempty"`
	Status            *InstallerStatus `json:"status,omitempty"`
}

// InstallerStatus represents the status of a software installer.
type InstallerStatus struct {
	Installed        int `json:"installed,omitempty"`
	Pending          int `json:"pending,omitempty"`
	Failed           int `json:"failed,omitempty"`
	PendingUninstall int `json:"pending_uninstall,omitempty"`
	FailedUninstall  int `json:"failed_uninstall,omitempty"`
}

// SoftwareLabel represents a label reference in software installers.
// This is a simplified label struct used in software package responses.
type SoftwareLabel struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name"`
}

// UploadSoftwarePackageRequest contains parameters for uploading a software package.
//
// LabelsIncludeAny / LabelsExcludeAny follow the same nil/empty/populated
// semantics documented on PatchSoftwarePackageRequest:
//
//   - nil pointer        → field is omitted from the form entirely
//   - pointer to empty   → field is sent as "[]"
//   - pointer to a slice → field is sent as the JSON-encoded array
//
// Fleet's "Only one of labels_include_all, labels_include_any or
// labels_exclude_any can be specified" rule applies to this endpoint too;
// callers must not set both pointers non-nil. Note that Fleet's GET
// response collapses "no labels" and "empty label list" into the same
// absent/nil shape, so a subsequent Read cannot distinguish a pointer-to-
// empty round-trip from a never-set one — the resource layer handles
// that asymmetry by gating Read-side state refresh on prior-state being
// non-null.
type UploadSoftwarePackageRequest struct {
	TeamID            *int      // Required for Premium
	Software          []byte    // The software package file. Fleet validates the type from Filename's extension: pkg, msi, exe, zip, deb, rpm, tar.gz, ipa, or a script installer (sh, py, ps1)
	Filename          string    // The filename of the package
	DisplayName       string    // Override for the end-user-visible name; defaults to Filename when empty
	Categories        []string  // Self-service categories (e.g. "Productivity", "Security"); empty = none
	InstallScript     string    // Script to run during install
	UninstallScript   string    // Script to run during uninstall
	PreInstallQuery   string    // Osquery to check before install
	PostInstallScript string    // Script to run after install
	SelfService       bool      // Enable self-service
	AutomaticInstall  bool      // Create a Fleet policy that auto-installs on hosts missing the software (POLICY-based; distinct from the setup-experience flag set via PUT /setup_experience/software)
	LabelsIncludeAny  *[]string // Labels to include (any match)
	LabelsExcludeAny  *[]string // Labels to exclude
	LabelsIncludeAll  *[]string // Labels to include (must match all)
}

// uploadSoftwareResponse is the API response when uploading software.
type uploadSoftwareResponse struct {
	SoftwarePackage struct {
		TeamID  int `json:"team_id"`
		TitleID int `json:"title_id"`
	} `json:"software_package"`
}

// UploadSoftwarePackage uploads a software package to FleetDM.
// This is a Premium feature and uses multipart/form-data.
func (c *Client) UploadSoftwarePackage(ctx context.Context, req *UploadSoftwarePackageRequest) (*SoftwareTitle, error) {
	fields := make(map[string]string)
	if req.TeamID != nil {
		fields["team_id"] = strconv.Itoa(*req.TeamID)
	}
	if req.InstallScript != "" {
		fields["install_script"] = req.InstallScript
	}
	if req.UninstallScript != "" {
		fields["uninstall_script"] = req.UninstallScript
	}
	if req.PreInstallQuery != "" {
		fields["pre_install_query"] = req.PreInstallQuery
	}
	if req.PostInstallScript != "" {
		fields["post_install_script"] = req.PostInstallScript
	}
	if req.SelfService {
		fields["self_service"] = "true"
	}
	if req.AutomaticInstall {
		// Fleet's documented Add Package field name is automatic_install
		// (policy-based auto-install). Previously this code sent the
		// undocumented "install_during_setup" key which Fleet silently
		// ignored — see commit history for the bug fix.
		fields["automatic_install"] = "true"
	}
	if req.DisplayName != "" {
		fields["display_name"] = req.DisplayName
	}
	if len(req.Categories) > 0 {
		categoriesJSON, err := json.Marshal(req.Categories)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal categories: %w", err)
		}
		fields["categories"] = string(categoriesJSON)
	}
	// Same nil/empty/populated semantics as PatchSoftwarePackage; nil
	// pointer omits the field, pointer-to-empty sends "[]" so a future
	// Read can refresh state with the explicit "no labels" value.
	if req.LabelsIncludeAny != nil {
		labelsJSON, err := json.Marshal(*req.LabelsIncludeAny)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal labels_include_any: %w", err)
		}
		fields["labels_include_any"] = string(labelsJSON)
	}
	if req.LabelsExcludeAny != nil {
		labelsJSON, err := json.Marshal(*req.LabelsExcludeAny)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal labels_exclude_any: %w", err)
		}
		fields["labels_exclude_any"] = string(labelsJSON)
	}
	if req.LabelsIncludeAll != nil {
		labelsJSON, err := json.Marshal(*req.LabelsIncludeAll)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal labels_include_all: %w", err)
		}
		fields["labels_include_all"] = string(labelsJSON)
	}

	respBody, err := c.doMultipartRequest(ctx, http.MethodPost, "/software/package", "software", req.Filename, req.Software, fields)
	if err != nil {
		return nil, fmt.Errorf("failed to upload software package: %w", err)
	}

	var uploadResp uploadSoftwareResponse
	if err := json.Unmarshal(respBody, &uploadResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w, body: %s", err, string(respBody))
	}

	if uploadResp.SoftwarePackage.TitleID == 0 {
		return nil, fmt.Errorf("upload succeeded but title_id is 0, response body: %s", string(respBody))
	}

	return c.GetSoftwareTitle(ctx, uploadResp.SoftwarePackage.TitleID, req.TeamID)
}

// GetSoftwareInstaller retrieves a software installer by title ID.
func (c *Client) GetSoftwareInstaller(ctx context.Context, titleID int, teamID *int) (*SoftwareInstaller, error) {
	params := make(map[string]string)
	if teamID != nil {
		params["team_id"] = strconv.Itoa(*teamID)
	}

	var response struct {
		Installer SoftwareInstaller `json:"software_installer"`
	}
	err := c.Get(ctx, fmt.Sprintf("/software/titles/%d/package", titleID), params, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get software installer for title %d: %w", titleID, err)
	}
	return &response.Installer, nil
}

// DeleteSoftwarePackage deletes a software package by title ID.
//
// team_id is always sent: Fleet 4.90 rejects the request outright with
// "Param team_id is required" when it is absent, and 0 is Fleet's value for
// "No team" (verified against a live Fleet v4.90.0 — a delete that used to
// omit the param for team-less packages now 400s).
func (c *Client) DeleteSoftwarePackage(ctx context.Context, titleID int, teamID *int) error {
	tid := 0
	if teamID != nil {
		tid = *teamID
	}
	endpoint := fmt.Sprintf("/software/titles/%d/available_for_install?team_id=%d", titleID, tid)

	return c.Delete(ctx, endpoint, nil, nil)
}

// PatchSoftwarePackageRequest contains fields that can be updated on an existing software package.
//
// Label fields use the same semantic convention as UpdatePolicyRequest in
// policies.go: nil = "no change", empty = "clear all labels", populated =
// "set to this exact set". Because this endpoint is multipart/form-data
// (not JSON), an in-band representation of nil-vs-empty isn't possible for
// a plain []string — so we use *[]string and translate at the wire layer
// in PatchSoftwarePackage:
//
//   - nil pointer        → field is omitted from the form entirely
//   - pointer to empty   → field is sent as "[]"
//   - pointer to a slice → field is sent as the JSON-encoded array
//
// Fleet's API enforces "Only one of labels_include_all, labels_include_any
// or labels_exclude_any can be specified" on this endpoint, so the caller
// must never set both label pointers non-nil (the resource layer's schema
// validator catches this at plan time).
//
// Software / Filename are optional. When Software is non-nil, the request
// becomes an in-place binary replacement (Fleet preserves the title_id):
// no policy detach, no DELETE, no re-upload. Fleet's docs note that any
// change to the installer package resets installation counts and cancels
// pending installs for the old package; non-binary metadata-only PATCHes
// (Software == nil) leave install state alone.
type PatchSoftwarePackageRequest struct {
	TeamID *int `json:"team_id,omitempty"`
	// InstallScript / UninstallScript follow the nil-means-omit convention:
	//
	//   - nil              → field is left out of the form, so Fleet keeps
	//     whatever script the package already has. This is
	//     what a caller that does not manage the script
	//     must send, so an unrelated metadata update never
	//     rewrites a script Fleet owns.
	//   - pointer to value → field is sent, replacing the stored script.
	//
	// Fleet's PATCH decoder only reads a script out of the form when the
	// field is present, so omission is a genuine "no change" rather than a
	// clear. PreInstallQuery / PostInstallScript stay plain strings because
	// they have no Fleet-generated default: for those, the empty string is
	// the caller's way of clearing the value.
	InstallScript     *string `json:"install_script"`
	UninstallScript   *string `json:"uninstall_script"`
	PreInstallQuery   string  `json:"pre_install_query"`
	PostInstallScript string  `json:"post_install_script"`
	SelfService       bool    `json:"self_service"`
	// DisplayName, when non-empty, overrides the title's display name.
	// Pass "" to leave Fleet's existing display_name untouched (no clear path
	// is exposed today — Fleet's API doesn't accept an empty-string override).
	DisplayName string `json:"display_name,omitempty"`
	// Categories follows the same nil-vs-populated convention as the label
	// pointers: nil = "no change", empty = "clear", populated = "set".
	Categories       *[]string `json:"categories"`
	LabelsIncludeAny *[]string `json:"labels_include_any"`
	LabelsExcludeAny *[]string `json:"labels_exclude_any"`
	LabelsIncludeAll *[]string `json:"labels_include_all"`
	// Software, when non-nil, is the new installer binary to replace the
	// existing one in-place. Filename must be set when Software is set.
	// When Software is nil the request is metadata-only and Filename is
	// ignored.
	Software []byte `json:"-"`
	Filename string `json:"-"`
}

// PatchSoftwarePackage updates an existing software package via Fleet's
// PATCH /software/titles/{id}/package endpoint.
//
// Two modes:
//
//   - Metadata-only (req.Software == nil): updates scripts, labels, flags,
//     display_name, categories. Fleet leaves install state alone.
//   - Binary replacement (req.Software != nil, req.Filename != ""): replaces
//     the installer binary in-place AND applies any metadata changes in the
//     same request. The title_id is preserved (it's in the URL path), so
//     every policy that references the title — install_software AND
//     patch_software — stays linked across the upgrade. No DELETE, no
//     detach/reattach dance. Fleet's docs flag that binary changes reset
//     installation counts and cancel pending installs for the old package.
//
// Fleet's endpoint requires multipart/form-data — it rejects application/json
// with HTTP 400 ("failed to parse multipart form: request Content-Type isn't
// multipart/form-data"). Field encoding mirrors UploadSoftwarePackage.
//
// Note: install_during_setup is NOT sent here — that field belongs to the
// separate PUT /setup_experience/software endpoint and is managed by the
// resource layer via SetSetupExperienceSoftwareInclude / Exclude.
func (c *Client) PatchSoftwarePackage(ctx context.Context, titleID int, req *PatchSoftwarePackageRequest) error {
	if req.Software != nil && req.Filename == "" {
		return fmt.Errorf("PatchSoftwarePackage: Filename is required when Software is provided")
	}
	endpoint := fmt.Sprintf("/software/titles/%d/package", titleID)

	// Fleet requires the target fleet on this endpoint and rejects the request
	// with HTTP 400 ("fleet_id is required; enter 0 for unassigned") when it is
	// absent, so it is always sent — 0 stands for "no team". A nil TeamID means
	// the resource has no team_id in its configuration, which is Fleet 0, not
	// "unscoped".
	//
	// It goes in the multipart body as `fleet_id` rather than the URL: Fleet's
	// decoder for this endpoint reads the fleet only out of the parsed form, and
	// a `fleet_id` query parameter is silently ignored (verified against a live
	// Fleet v4.90.0 — query `fleet_id` still fails the required check). Fleet
	// does promote a `team_id` form or query value to `fleet_id`, but that path
	// is deprecated and logs a warning server-side.
	tid := 0
	if req.TeamID != nil {
		tid = *req.TeamID
	}

	// pre_install_query, post_install_script and self_service are sent
	// unconditionally — empty strings included — because PATCH semantics here
	// are "set to exactly this", not "merge": for those fields, omitting one
	// that previously had a value would leave the stale value in place. The
	// script and label fields use pointers instead so the caller can
	// distinguish nil (omit) from empty (clear).
	fields := map[string]string{
		"fleet_id":            strconv.Itoa(tid),
		"pre_install_query":   req.PreInstallQuery,
		"post_install_script": req.PostInstallScript,
		"self_service":        strconv.FormatBool(req.SelfService),
	}
	if req.InstallScript != nil {
		fields["install_script"] = *req.InstallScript
	}
	if req.UninstallScript != nil {
		fields["uninstall_script"] = *req.UninstallScript
	}
	if req.DisplayName != "" {
		fields["display_name"] = req.DisplayName
	}

	// A nil label pointer means "don't touch this field". Sending both
	// labels_include_any and labels_exclude_any (even as empty arrays)
	// violates Fleet's "only one of …" invariant for this endpoint and
	// gets rejected with HTTP 400. Empty (non-nil) is the explicit
	// "clear labels" path: marshalling []string{} yields "[]".
	if req.LabelsIncludeAny != nil {
		labelsIncJSON, err := json.Marshal(*req.LabelsIncludeAny)
		if err != nil {
			return fmt.Errorf("failed to marshal labels_include_any: %w", err)
		}
		fields["labels_include_any"] = string(labelsIncJSON)
	}
	if req.LabelsExcludeAny != nil {
		labelsExcJSON, err := json.Marshal(*req.LabelsExcludeAny)
		if err != nil {
			return fmt.Errorf("failed to marshal labels_exclude_any: %w", err)
		}
		fields["labels_exclude_any"] = string(labelsExcJSON)
	}
	if req.LabelsIncludeAll != nil {
		labelsAllJSON, err := json.Marshal(*req.LabelsIncludeAll)
		if err != nil {
			return fmt.Errorf("failed to marshal labels_include_all: %w", err)
		}
		fields["labels_include_all"] = string(labelsAllJSON)
	}
	if req.Categories != nil {
		categoriesJSON, err := json.Marshal(*req.Categories)
		if err != nil {
			return fmt.Errorf("failed to marshal categories: %w", err)
		}
		fields["categories"] = string(categoriesJSON)
	}

	if req.Software != nil {
		if _, err := c.doMultipartRequest(ctx, http.MethodPatch, endpoint, "software", req.Filename, req.Software, fields); err != nil {
			return fmt.Errorf("failed to patch software package (with binary): %w", err)
		}
		return nil
	}
	if _, err := c.doMultipartFormRequest(ctx, http.MethodPatch, endpoint, fields); err != nil {
		return fmt.Errorf("failed to patch software package: %w", err)
	}
	return nil
}

// PatchSoftwarePackagePinnedVersion pins a Fleet-maintained app to a specific
// (or major) version via PATCH /software/titles/{id}/package, sending only the
// `version` form field plus the mandatory `fleet_id` scope.
//
// The dedicated method exists because Fleet rejects `version` combined with
// anything else on this endpoint. Verified against a live Fleet v4.90.0:
//
//	PATCH version=0.15.12                 → 200, pinned_version echoes on GET
//	PATCH version=0.15.12&self_service=1  → 400 "Couldn't update. \"version\"
//	                                        can't be changed at the same time
//	                                        as other fields."
//
// So PatchSoftwarePackage (which always sends the script + self_service
// fields) can never carry a pin, and callers that need to change both metadata
// and the pin in one apply must issue two sequential requests. Metadata-only
// PATCHes preserve an existing pin, so metadata-first-then-pin is safe in
// either order; the resource layer does metadata first.
//
// version semantics (Fleet's docs + probe):
//
//   - "1.2.3" pins to that exact catalog version.
//   - "^147" pins to a major version — Fleet keeps updating within it.
//   - ""      clears the pin, returning the title to "track latest". This is
//     why the parameter is a plain string with no omit path: an empty
//     value is a meaningful request, not "no change". Callers that
//     mean "no change" must simply not call this method.
func (c *Client) PatchSoftwarePackagePinnedVersion(ctx context.Context, titleID int, teamID *int, version string) error {
	endpoint := fmt.Sprintf("/software/titles/%d/package", titleID)

	// fleet_id is mandatory on this endpoint and does not count as one of the
	// "other fields" Fleet refuses to combine with version — a form carrying
	// both is accepted and pins the title (verified against a live Fleet
	// v4.90.0). See PatchSoftwarePackage for why it travels in the body.
	tid := 0
	if teamID != nil {
		tid = *teamID
	}

	// version + fleet_id only — see the doc comment.
	fields := map[string]string{
		"version":  version,
		"fleet_id": strconv.Itoa(tid),
	}

	if _, err := c.doMultipartFormRequest(ctx, http.MethodPatch, endpoint, fields); err != nil {
		return fmt.Errorf("failed to set pinned version on software package: %w", err)
	}
	return nil
}

// addAppStoreAppResponse is the API response when adding a VPP app.
type addAppStoreAppResponse struct {
	SoftwareTitleID int `json:"software_title_id"`
}

// AddAppStoreApp adds a VPP (App Store) app to a team.
func (c *Client) AddAppStoreApp(ctx context.Context, req *AddAppStoreAppRequest) (*SoftwareTitle, error) {
	var resp addAppStoreAppResponse
	if err := c.Post(ctx, "/software/app_store_apps", req, &resp); err != nil {
		return nil, fmt.Errorf("failed to add App Store app: %w", err)
	}
	if resp.SoftwareTitleID == 0 {
		return nil, fmt.Errorf("add App Store app succeeded but software_title_id is 0")
	}
	teamID := &req.TeamID
	return c.GetSoftwareTitle(ctx, resp.SoftwareTitleID, teamID)
}

// UpdateAppStoreApp updates a VPP (App Store) app's metadata.
func (c *Client) UpdateAppStoreApp(ctx context.Context, titleID int, req *UpdateAppStoreAppRequest) error {
	endpoint := fmt.Sprintf("/software/titles/%d/app_store_app", titleID)
	return c.Patch(ctx, endpoint, req, nil)
}

// listFleetMaintainedAppsResponse is the API response for listing Fleet Maintained Apps.
type listFleetMaintainedAppsResponse struct {
	FleetMaintainedApps []FleetMaintainedApp `json:"fleet_maintained_apps"`
}

// ListFleetMaintainedApps retrieves all Fleet Maintained Apps.
func (c *Client) ListFleetMaintainedApps(ctx context.Context, teamID *int) ([]FleetMaintainedApp, error) {
	params := make(map[string]string)
	if teamID != nil {
		params["team_id"] = strconv.Itoa(*teamID)
	}
	var resp listFleetMaintainedAppsResponse
	if err := c.Get(ctx, "/software/fleet_maintained_apps", params, &resp); err != nil {
		return nil, fmt.Errorf("failed to list Fleet Maintained Apps: %w", err)
	}
	return resp.FleetMaintainedApps, nil
}

// getFleetMaintainedAppResponse is the API response for getting a single Fleet Maintained App.
type getFleetMaintainedAppResponse struct {
	FleetMaintainedApp *FleetMaintainedApp `json:"fleet_maintained_app"`
}

// GetFleetMaintainedApp retrieves a Fleet Maintained App by ID. teamID is
// optional and, when set, populates the response's software_title_id with
// that team's title id instead of leaving it null.
func (c *Client) GetFleetMaintainedApp(ctx context.Context, id int, teamID *int) (*FleetMaintainedApp, error) {
	params := make(map[string]string)
	if teamID != nil {
		params["team_id"] = strconv.Itoa(*teamID)
	}
	var resp getFleetMaintainedAppResponse
	if err := c.Get(ctx, fmt.Sprintf("/software/fleet_maintained_apps/%d", id), params, &resp); err != nil {
		return nil, fmt.Errorf("failed to get Fleet Maintained App %d: %w", id, err)
	}
	return resp.FleetMaintainedApp, nil
}

// ListAppStoreAppsResponse is the API response for listing App Store apps.
type ListAppStoreAppsResponse struct {
	AppStoreApps []AppStoreAppListItem `json:"app_store_apps"`
}

// AppStoreAppListItem represents a single App Store app in a list response.
type AppStoreAppListItem struct {
	AppStoreID    string `json:"app_store_id"`
	Name          string `json:"name"`
	DisplayName   string `json:"display_name,omitempty"`
	Platform      string `json:"platform"`
	IconURL       string `json:"icon_url,omitempty"`
	LatestVersion string `json:"latest_version,omitempty"`
}

// ListAppStoreApps lists available App Store (VPP) apps for a team.
func (c *Client) ListAppStoreApps(ctx context.Context, teamID int) ([]AppStoreAppListItem, error) {
	params := map[string]string{
		"team_id": strconv.Itoa(teamID),
	}
	var resp ListAppStoreAppsResponse
	if err := c.Get(ctx, "/software/app_store_apps", params, &resp); err != nil {
		return nil, fmt.Errorf("failed to list App Store apps: %w", err)
	}
	return resp.AppStoreApps, nil
}

// addFleetMaintainedAppResponse is the API response when adding a Fleet Maintained App.
type addFleetMaintainedAppResponse struct {
	SoftwareTitleID int `json:"software_title_id"`
}

// AddFleetMaintainedApp adds a Fleet Maintained App to a team.
func (c *Client) AddFleetMaintainedApp(ctx context.Context, req *AddFleetMaintainedAppRequest) (*SoftwareTitle, error) {
	var resp addFleetMaintainedAppResponse
	if err := c.Post(ctx, "/software/fleet_maintained_apps", req, &resp); err != nil {
		return nil, fmt.Errorf("failed to add Fleet Maintained App: %w", err)
	}
	if resp.SoftwareTitleID == 0 {
		return nil, fmt.Errorf("add Fleet Maintained App succeeded but software_title_id is 0")
	}
	teamID := &req.TeamID
	return c.GetSoftwareTitle(ctx, resp.SoftwareTitleID, teamID)
}
