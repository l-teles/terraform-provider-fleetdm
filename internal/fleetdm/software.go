package fleetdm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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
	SoftwarePackage  *SoftwarePackageInfo   `json:"software_package,omitempty"`
	AppStoreApp      *AppStoreAppInfo       `json:"app_store_app,omitempty"`
	CountsUpdatedAt  *time.Time             `json:"counts_updated_at,omitempty"`
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
	Name               string `json:"name,omitempty"`
	Version            string `json:"version,omitempty"`
	Platform           string `json:"platform,omitempty"`
	SelfService        bool   `json:"self_service,omitempty"`
	InstallDuringSetup *bool  `json:"install_during_setup,omitempty"`
	InstallScript      string `json:"install_script,omitempty"`
	UninstallScript    string `json:"uninstall_script,omitempty"`
	PreInstallQuery    string `json:"pre_install_query,omitempty"`
	PostInstallScript  string `json:"post_install_script,omitempty"`
	HashSHA256         string `json:"hash_sha256,omitempty"`
}

// AppStoreAppInfo represents App Store app info.
type AppStoreAppInfo struct {
	AdamID             string          `json:"app_store_id,omitempty"`
	Platform           string          `json:"platform,omitempty"`
	Name               string          `json:"name,omitempty"`
	LatestVersion      string          `json:"latest_version,omitempty"`
	SelfService        bool            `json:"self_service,omitempty"`
	InstallDuringSetup *bool           `json:"install_during_setup,omitempty"`
	LabelsIncludeAny   []SoftwareLabel `json:"labels_include_any,omitempty"`
	LabelsExcludeAny   []SoftwareLabel `json:"labels_exclude_any,omitempty"`
}

// AddAppStoreAppRequest represents the request body for adding a VPP app.
type AddAppStoreAppRequest struct {
	AppStoreID  string `json:"app_store_id"`
	TeamID      int    `json:"team_id"`
	Platform    string `json:"platform,omitempty"`
	SelfService bool   `json:"self_service,omitempty"`
}

// UpdateAppStoreAppRequest represents the request body for updating a VPP app.
type UpdateAppStoreAppRequest struct {
	TeamID           int      `json:"team_id"`
	SelfService      bool     `json:"self_service"`
	DisplayName      string   `json:"display_name,omitempty"`
	LabelsIncludeAny []string `json:"labels_include_any"`
	LabelsExcludeAny []string `json:"labels_exclude_any"`
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
type AddFleetMaintainedAppRequest struct {
	FleetMaintainedAppID int      `json:"fleet_maintained_app_id"`
	TeamID               int      `json:"team_id"`
	InstallScript        string   `json:"install_script,omitempty"`
	PreInstallQuery      string   `json:"pre_install_query,omitempty"`
	PostInstallScript    string   `json:"post_install_script,omitempty"`
	SelfService          bool     `json:"self_service,omitempty"`
	AutomaticInstall     bool     `json:"automatic_install,omitempty"`
	LabelsIncludeAny     []string `json:"labels_include_any,omitempty"`
	LabelsExcludeAny     []string `json:"labels_exclude_any,omitempty"`
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
type UploadSoftwarePackageRequest struct {
	TeamID            *int     // Required for Premium
	Software          []byte   // The software package file (pkg, msi, deb, rpm, exe)
	Filename          string   // The filename of the package
	InstallScript     string   // Script to run during install
	UninstallScript   string   // Script to run during uninstall
	PreInstallQuery   string   // Osquery to check before install
	PostInstallScript string   // Script to run after install
	SelfService       bool     // Enable self-service
	AutomaticInstall  bool     // Automatically install on hosts
	LabelsIncludeAny  []string // Labels to include (any match)
	LabelsExcludeAny  []string // Labels to exclude
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
		fields["install_during_setup"] = "true"
	}
	if len(req.LabelsIncludeAny) > 0 {
		labelsJSON, err := json.Marshal(req.LabelsIncludeAny)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal labels_include_any: %w", err)
		}
		fields["labels_include_any"] = string(labelsJSON)
	}
	if len(req.LabelsExcludeAny) > 0 {
		labelsJSON, err := json.Marshal(req.LabelsExcludeAny)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal labels_exclude_any: %w", err)
		}
		fields["labels_exclude_any"] = string(labelsJSON)
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
func (c *Client) DeleteSoftwarePackage(ctx context.Context, titleID int, teamID *int) error {
	endpoint := fmt.Sprintf("/software/titles/%d/available_for_install", titleID)
	if teamID != nil {
		endpoint = fmt.Sprintf("%s?team_id=%d", endpoint, *teamID)
	}

	return c.Delete(ctx, endpoint, nil, nil)
}

// PatchSoftwarePackageRequest contains fields that can be updated on an existing software package.
type PatchSoftwarePackageRequest struct {
	TeamID             *int     `json:"team_id,omitempty"`
	InstallScript      string   `json:"install_script"`
	UninstallScript    string   `json:"uninstall_script"`
	PreInstallQuery    string   `json:"pre_install_query"`
	PostInstallScript  string   `json:"post_install_script"`
	SelfService        bool     `json:"self_service"`
	InstallDuringSetup bool     `json:"install_during_setup"`
	LabelsIncludeAny   []string `json:"labels_include_any"`
	LabelsExcludeAny   []string `json:"labels_exclude_any"`
}

// PatchSoftwarePackage updates the metadata of an existing software package (scripts, labels, flags).
// The package binary itself cannot be changed in-place; use DeleteSoftwarePackage + UploadSoftwarePackage instead.
func (c *Client) PatchSoftwarePackage(ctx context.Context, titleID int, req *PatchSoftwarePackageRequest) error {
	endpoint := fmt.Sprintf("/software/titles/%d/package", titleID)
	if req.TeamID != nil {
		endpoint = fmt.Sprintf("%s?team_id=%d", endpoint, *req.TeamID)
	}
	return c.Patch(ctx, endpoint, req, nil)
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

// GetFleetMaintainedApp retrieves a Fleet Maintained App by ID.
func (c *Client) GetFleetMaintainedApp(ctx context.Context, id int) (*FleetMaintainedApp, error) {
	var resp getFleetMaintainedAppResponse
	if err := c.Get(ctx, fmt.Sprintf("/software/fleet_maintained_apps/%d", id), nil, &resp); err != nil {
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
