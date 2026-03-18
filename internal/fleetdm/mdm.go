package fleetdm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// MDMConfigProfile represents an MDM configuration profile.
type MDMConfigProfile struct {
	ProfileUUID      string                      `json:"profile_uuid"`
	TeamID           *int                        `json:"team_id,omitempty"`
	Name             string                      `json:"name"`
	Platform         string                      `json:"platform"`
	Identifier       string                      `json:"identifier,omitempty"`
	Checksum         string                      `json:"checksum,omitempty"`
	CreatedAt        string                      `json:"created_at"`
	UploadedAt       string                      `json:"uploaded_at"`
	LabelsIncludeAll []ConfigurationProfileLabel `json:"labels_include_all,omitempty"`
	LabelsIncludeAny []ConfigurationProfileLabel `json:"labels_include_any,omitempty"`
	LabelsExcludeAny []ConfigurationProfileLabel `json:"labels_exclude_any,omitempty"`
}

// ConfigurationProfileLabel represents a label associated with a configuration profile.
type ConfigurationProfileLabel struct {
	LabelName string `json:"name"`
	LabelID   *int   `json:"id,omitempty"`
	Broken    bool   `json:"broken,omitempty"`
}

// MDMProfilesSummary represents the summary of MDM profiles.
type MDMProfilesSummary struct {
	Verified  int `json:"verified"`
	Verifying int `json:"verifying"`
	Failed    int `json:"failed"`
	Pending   int `json:"pending"`
}

// MDMEnrollmentSummary represents the MDM enrollment summary.
type MDMEnrollmentSummary struct {
	EnrolledManualHostsCount    int `json:"enrolled_manual_hosts_count"`
	EnrolledAutomatedHostsCount int `json:"enrolled_automated_hosts_count"`
	EnrolledPersonalHostsCount  int `json:"enrolled_personal_hosts_count"`
	UnenrolledHostsCount        int `json:"unenrolled_hosts_count"`
	PendingHostsCount           int `json:"pending_hosts_count,omitempty"`
	HostsCount                  int `json:"hosts_count"`
}

// MDMSolution represents an MDM solution.
type MDMSolution struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	ServerURL  string `json:"server_url"`
	HostsCount int    `json:"hosts_count"`
}

// MDMSummary represents the full MDM summary response.
type MDMSummary struct {
	CountsUpdatedAt  string               `json:"counts_updated_at"`
	EnrollmentStatus MDMEnrollmentSummary `json:"mobile_device_management_enrollment_status"`
	MDMSolutions     []MDMSolution        `json:"mobile_device_management_solution"`
}

// ListMDMConfigProfilesOptions contains options for listing MDM config profiles.
type ListMDMConfigProfilesOptions struct {
	TeamID  *int
	Page    int
	PerPage int
}

// listMDMConfigProfilesResponse represents the response from the list profiles endpoint.
type listMDMConfigProfilesResponse struct {
	Profiles []MDMConfigProfile `json:"profiles"`
	Meta     *PaginationMeta    `json:"meta,omitempty"`
}

// ListMDMConfigProfiles retrieves MDM configuration profiles.
func (c *Client) ListMDMConfigProfiles(ctx context.Context, opts *ListMDMConfigProfilesOptions) ([]MDMConfigProfile, error) {
	params := make(map[string]string)

	if opts != nil {
		if opts.TeamID != nil {
			params["team_id"] = fmt.Sprintf("%d", *opts.TeamID)
		}
		if opts.Page > 0 {
			params["page"] = fmt.Sprintf("%d", opts.Page)
		}
		if opts.PerPage > 0 {
			params["per_page"] = fmt.Sprintf("%d", opts.PerPage)
		}
	}

	var response listMDMConfigProfilesResponse
	if err := c.Get(ctx, "/configuration_profiles", params, &response); err != nil {
		return nil, fmt.Errorf("failed to list MDM config profiles: %w", err)
	}

	return response.Profiles, nil
}

// GetMDMConfigProfile retrieves a specific MDM configuration profile by UUID.
func (c *Client) GetMDMConfigProfile(ctx context.Context, profileUUID string) (*MDMConfigProfile, error) {
	var response MDMConfigProfile
	if err := c.Get(ctx, fmt.Sprintf("/configuration_profiles/%s", profileUUID), nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get MDM config profile %s: %w", profileUUID, err)
	}

	return &response, nil
}

// GetMDMSummary retrieves the MDM enrollment summary.
func (c *Client) GetMDMSummary(ctx context.Context, platform string, teamID *int) (*MDMSummary, error) {
	params := make(map[string]string)
	if platform != "" {
		params["platform"] = platform
	}
	if teamID != nil {
		params["team_id"] = fmt.Sprintf("%d", *teamID)
	}

	var response MDMSummary
	if err := c.Get(ctx, "/hosts/summary/mdm", params, &response); err != nil {
		return nil, fmt.Errorf("failed to get MDM summary: %w", err)
	}

	return &response, nil
}

// ProfileExtensionFromContent inspects profile bytes and returns the appropriate
// file extension. Fleet uses the extension to detect the platform:
//   - ".mobileconfig" for Apple (macOS/iOS) configuration profiles
//   - ".xml" for Windows configuration profiles
//   - ".json" for Apple declaration (DDM) profiles
func ProfileExtensionFromContent(content []byte) string {
	trimmed := bytes.TrimSpace(content)
	switch {
	case bytes.HasPrefix(trimmed, []byte("<?xml")) || bytes.HasPrefix(trimmed, []byte("<!DOCTYPE")):
		if bytes.Contains(trimmed, []byte("<plist")) || bytes.Contains(trimmed, []byte("PayloadType")) {
			return ".mobileconfig"
		}
		return ".xml"
	case bytes.HasPrefix(trimmed, []byte("{")):
		return ".json"
	default:
		return ".mobileconfig"
	}
}

// CreateConfigProfileRequest contains the parameters for creating a configuration profile.
type CreateConfigProfileRequest struct {
	TeamID           *int     // Optional team ID
	Filename         string   // Upload filename; Fleet derives the Windows profile name from this
	Profile          []byte   // Profile content (mobileconfig or XML)
	Labels           []string // Deprecated: use LabelsIncludeAll instead
	LabelsIncludeAll []string // Labels that must all match
	LabelsIncludeAny []string // Labels where any can match
	LabelsExcludeAny []string // Labels to exclude
}

// CreateConfigProfile creates a new MDM configuration profile.
// This uses multipart/form-data as required by the FleetDM API.
func (c *Client) CreateConfigProfile(ctx context.Context, req *CreateConfigProfileRequest) (*MDMConfigProfile, error) {
	fields := make(map[string]string)
	if req.TeamID != nil {
		fields["team_id"] = strconv.Itoa(*req.TeamID)
	}
	if len(req.Labels) > 0 {
		fields["labels"] = strings.Join(req.Labels, ",")
	}
	if len(req.LabelsIncludeAll) > 0 {
		fields["labels_include_all"] = strings.Join(req.LabelsIncludeAll, ",")
	}
	if len(req.LabelsIncludeAny) > 0 {
		fields["labels_include_any"] = strings.Join(req.LabelsIncludeAny, ",")
	}
	if len(req.LabelsExcludeAny) > 0 {
		fields["labels_exclude_any"] = strings.Join(req.LabelsExcludeAny, ",")
	}

	filename := req.Filename
	if filename == "" {
		filename = "profile.mobileconfig"
	}

	respBody, err := c.doMultipartRequest(ctx, http.MethodPost, "/configuration_profiles", "profile", filename, req.Profile, fields)
	if err != nil {
		return nil, fmt.Errorf("failed to create config profile: %w", err)
	}

	var response struct {
		ProfileUUID string `json:"profile_uuid"`
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return c.GetMDMConfigProfile(ctx, response.ProfileUUID)
}

// GetConfigProfileContent retrieves the raw content of a configuration profile by UUID using the alt=media query parameter.
func (c *Client) GetConfigProfileContent(ctx context.Context, profileUUID string) (string, error) {
	endpoint := fmt.Sprintf("/configuration_profiles/%s?alt=media", profileUUID)
	reqURL := c.BaseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("failed to get config profile content %s: HTTP %d: %s", profileUUID, resp.StatusCode, string(body))
	}

	return string(body), nil
}

// DeleteConfigProfile deletes an MDM configuration profile by UUID.
func (c *Client) DeleteConfigProfile(ctx context.Context, profileUUID string) error {
	return c.Delete(ctx, fmt.Sprintf("/configuration_profiles/%s", profileUUID), nil, nil)
}

// BootstrapPackage represents a bootstrap package in FleetDM.
type BootstrapPackage struct {
	TeamID    int    `json:"team_id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at,omitempty"`
	Sha256    string `json:"sha256,omitempty"`
	Token     string `json:"token,omitempty"`
}

// UploadBootstrapPackageRequest contains parameters for uploading a bootstrap package.
type UploadBootstrapPackageRequest struct {
	TeamID  int    // Required team ID
	Package []byte // The bootstrap package file (pkg)
	Name    string // The package filename
}

// UploadBootstrapPackage uploads a bootstrap package to FleetDM.
// This is a Premium feature and uses multipart/form-data.
func (c *Client) UploadBootstrapPackage(ctx context.Context, req *UploadBootstrapPackageRequest) error {
	fields := map[string]string{
		"team_id": strconv.Itoa(req.TeamID),
	}

	_, err := c.doMultipartRequest(ctx, http.MethodPost, "/bootstrap", "package", req.Name, req.Package, fields)
	if err != nil {
		return fmt.Errorf("failed to upload bootstrap package: %w", err)
	}

	return nil
}

// GetBootstrapPackageMetadata retrieves bootstrap package metadata for a team.
func (c *Client) GetBootstrapPackageMetadata(ctx context.Context, teamID int) (*BootstrapPackage, error) {
	var response BootstrapPackage
	err := c.Get(ctx, fmt.Sprintf("/bootstrap/%d/metadata", teamID), nil, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get bootstrap package metadata for team %d: %w", teamID, err)
	}
	return &response, nil
}

// DeleteBootstrapPackage deletes the bootstrap package for a team.
func (c *Client) DeleteBootstrapPackage(ctx context.Context, teamID int) error {
	return c.Delete(ctx, fmt.Sprintf("/bootstrap/%d", teamID), nil, nil)
}

// SetupExperience represents the setup experience settings for a team.
type SetupExperience struct {
	EnableEndUserAuth     bool            `json:"enable_end_user_authentication"`
	EnableReleaseManually bool            `json:"enable_release_device_manually"`
	Script                *SetupScript    `json:"script,omitempty"`
	Software              []SetupSoftware `json:"software,omitempty"`
	SoftwareTitles        []SetupSoftware `json:"software_titles,omitempty"`
}

// SetupScript represents a script in setup experience.
type SetupScript struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// SetupSoftware represents software in setup experience.
type SetupSoftware struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// UpdateSetupExperienceRequest represents the request to update setup experience.
type UpdateSetupExperienceRequest struct {
	TeamID                int   `json:"team_id"`
	EnableEndUserAuth     *bool `json:"enable_end_user_authentication,omitempty"`
	EnableReleaseManually *bool `json:"enable_release_device_manually,omitempty"`
}

// GetSetupExperience retrieves setup experience settings for a team.
func (c *Client) GetSetupExperience(ctx context.Context, teamID int) (*SetupExperience, error) {
	params := map[string]string{
		"team_id": strconv.Itoa(teamID),
	}
	var response SetupExperience
	err := c.Get(ctx, "/setup_experience", params, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get setup experience for team %d: %w", teamID, err)
	}
	return &response, nil
}

// UpdateSetupExperience updates setup experience settings for a team.
func (c *Client) UpdateSetupExperience(ctx context.Context, req *UpdateSetupExperienceRequest) error {
	return c.Patch(ctx, "/setup_experience", req, nil)
}
