package fleetdm

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// Host represents a FleetDM host.
type Host struct {
	ID                          int        `json:"id"`
	UUID                        string     `json:"uuid"`
	Hostname                    string     `json:"hostname"`
	DisplayName                 string     `json:"display_name"`
	ComputerName                string     `json:"computer_name"`
	Platform                    string     `json:"platform"`
	OSVersion                   string     `json:"os_version"`
	Build                       string     `json:"build"`
	PlatformLike                string     `json:"platform_like"`
	CodeName                    string     `json:"code_name"`
	Uptime                      int64      `json:"uptime"`
	Memory                      int64      `json:"memory"`
	CPUType                     string     `json:"cpu_type"`
	CPUSubtype                  string     `json:"cpu_subtype"`
	CPUBrand                    string     `json:"cpu_brand"`
	CPUPhysicalCores            int        `json:"cpu_physical_cores"`
	CPULogicalCores             int        `json:"cpu_logical_cores"`
	HardwareVendor              string     `json:"hardware_vendor"`
	HardwareModel               string     `json:"hardware_model"`
	HardwareVersion             string     `json:"hardware_version"`
	HardwareSerial              string     `json:"hardware_serial"`
	PrimaryIP                   string     `json:"primary_ip"`
	PrimaryMac                  string     `json:"primary_mac"`
	PublicIP                    string     `json:"public_ip"`
	DistributedInterval         int        `json:"distributed_interval"`
	ConfigTLSRefresh            int        `json:"config_tls_refresh"`
	LoggerTLSPeriod             int        `json:"logger_tls_period"`
	TeamID                      *int       `json:"team_id"`
	TeamName                    string     `json:"team_name"`
	GigsDiskSpaceAvailable      float64    `json:"gigs_disk_space_available"`
	PercentDiskSpaceAvailable   float64    `json:"percent_disk_space_available"`
	GigsTotalDiskSpace          float64    `json:"gigs_total_disk_space"`
	PacksCount                  int        `json:"packs_count,omitempty"`
	PoliciesCount               int        `json:"policies_count,omitempty"`
	IssuesCount                 int        `json:"issues_count,omitempty"`
	Status                      string     `json:"status"`
	SeenTime                    time.Time  `json:"seen_time"`
	CreatedAt                   time.Time  `json:"created_at"`
	UpdatedAt                   time.Time  `json:"updated_at"`
	RefetchRequested            bool       `json:"refetch_requested"`
	LabelUpdatedAt              time.Time  `json:"label_updated_at"`
	LastEnrolledAt              time.Time  `json:"last_enrolled_at"`
	PolicyUpdatedAt             time.Time  `json:"policy_updated_at"`
	RefetchCriticalQueriesUntil *time.Time `json:"refetch_critical_queries_until,omitempty"`
	DetailUpdatedAt             time.Time  `json:"detail_updated_at"`
	SoftwareUpdatedAt           time.Time  `json:"software_updated_at"`
	LastRestartedAt             time.Time  `json:"last_restarted_at"`

	// MDM information
	MDM *HostMDM `json:"mdm,omitempty"`

	// Geolocation
	Geolocation *Geolocation `json:"geolocation,omitempty"`

	// Device mapping
	DeviceMapping []DeviceMapping `json:"device_mapping,omitempty"`

	// Labels
	Labels []HostLabel `json:"labels,omitempty"`

	// Packs
	Packs []HostPack `json:"packs,omitempty"`

	// Policies
	Policies []HostPolicy `json:"policies,omitempty"`

	// Software
	Software []HostSoftware `json:"software,omitempty"`

	// Users
	Users []HostUser `json:"users,omitempty"`

	// Batteries
	Batteries []HostBattery `json:"batteries,omitempty"`
}

// HostMDM contains MDM information for a host.
type HostMDM struct {
	EnrollmentStatus   string             `json:"enrollment_status"`
	ServerURL          string             `json:"server_url"`
	DEPProfileAssigned bool               `json:"dep_profile_assigned"`
	DEPProfilePending  bool               `json:"dep_profile_pending"`
	DEPProfileError    bool               `json:"dep_profile_error"`
	ConnectedToFleet   bool               `json:"connected_to_fleet"`
	Name               string             `json:"name"`
	MacOSSettings      *HostMacOSSettings `json:"macos_settings,omitempty"`
	Profiles           []MDMProfile       `json:"profiles,omitempty"`
}

// HostMacOSSettings contains macOS settings for a host's MDM.
type HostMacOSSettings struct {
	DiskEncryption *string `json:"disk_encryption"`
	ActionRequired *string `json:"action_required"`
}

// MDMProfile contains MDM profile information.
type MDMProfile struct {
	ProfileUUID   string `json:"profile_uuid"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	OperationType string `json:"operation_type"`
	Detail        string `json:"detail"`
}

// Geolocation contains geolocation information for a host.
type Geolocation struct {
	CityName   string `json:"city_name"`
	CountryISO string `json:"country_iso"`
}

// DeviceMapping contains device mapping information.
type DeviceMapping struct {
	Email  string `json:"email"`
	Source string `json:"source"`
}

// HostLabel contains label information for a host.
type HostLabel struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Query       string    `json:"query"`
	Platform    string    `json:"platform"`
	LabelType   string    `json:"label_type"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// HostPack contains pack information for a host.
type HostPack struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Platform    string    `json:"platform"`
	Disabled    bool      `json:"disabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// HostPolicy contains policy information for a host.
type HostPolicy struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Query       string    `json:"query"`
	Response    string    `json:"response"`
	Critical    bool      `json:"critical"`
	Resolution  string    `json:"resolution"`
	Platform    string    `json:"platform"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// HostSoftware contains software information for a host.
type HostSoftware struct {
	ID               int             `json:"id"`
	Name             string          `json:"name"`
	Version          string          `json:"version"`
	Source           string          `json:"source"`
	GenerateCPE      bool            `json:"generate_cpe"`
	BundleIdentifier string          `json:"bundle_identifier"`
	Vulnerabilities  []Vulnerability `json:"vulnerabilities,omitempty"`
	InstalledPaths   []string        `json:"installed_paths,omitempty"`
	LastOpenedAt     *time.Time      `json:"last_opened_at,omitempty"`
}

// Vulnerability contains CVE information.
type Vulnerability struct {
	CVE               string     `json:"cve"`
	CVSSScore         *float64   `json:"cvss_score"`
	EPSSProbability   *float64   `json:"epss_probability"`
	CISAKnownExploit  bool       `json:"cisa_known_exploit"`
	CVEPublished      *time.Time `json:"cve_published,omitempty"`
	CVEDescription    string     `json:"cve_description,omitempty"`
	ResolvedInVersion *string    `json:"resolved_in_version,omitempty"`
}

// HostUser contains user information for a host.
type HostUser struct {
	UID       int    `json:"uid"`
	Username  string `json:"username"`
	Type      string `json:"type"`
	GroupName string `json:"groupname"`
	Shell     string `json:"shell"`
}

// HostBattery contains battery information for a host.
type HostBattery struct {
	CycleCount int    `json:"cycle_count"`
	Health     string `json:"health"`
}

// ListHostsOptions contains options for listing hosts.
type ListHostsOptions struct {
	Page                        int
	PerPage                     int
	OrderKey                    string
	OrderDirection              string
	Query                       string
	Status                      string
	LabelID                     int
	PolicyID                    int
	PolicyResponse              string
	SoftwareID                  int
	SoftwareVersionID           int
	SoftwareTitleID             int
	OSName                      string
	OSVersion                   string
	OSVersionID                 int
	MunkiIssueID                int
	LowDiskSpace                int
	MDMEnrollmentStatus         string
	MDMName                     string
	MacOSSettingsDiskEncryption string
	BootstrapPackage            string
	MacOSSetupAssistant         string
	TeamID                      int
	Vulnerable                  bool
	DeviceMapping               bool
	Columns                     []string
}

// ListHosts returns a list of hosts with optional filtering.
func (c *Client) ListHosts(ctx context.Context, opts ListHostsOptions) ([]Host, error) {
	params := make(map[string]string)

	if opts.Page > 0 {
		params["page"] = strconv.Itoa(opts.Page - 1) // API uses 0-based paging
	}
	if opts.PerPage > 0 {
		params["per_page"] = strconv.Itoa(opts.PerPage)
	}
	if opts.OrderKey != "" {
		params["order_key"] = opts.OrderKey
	}
	if opts.OrderDirection != "" {
		params["order_direction"] = opts.OrderDirection
	}
	if opts.Query != "" {
		params["query"] = opts.Query
	}
	if opts.Status != "" {
		params["status"] = opts.Status
	}
	if opts.LabelID > 0 {
		params["label_id"] = strconv.Itoa(opts.LabelID)
	}
	if opts.PolicyID > 0 {
		params["policy_id"] = strconv.Itoa(opts.PolicyID)
	}
	if opts.PolicyResponse != "" {
		params["policy_response"] = opts.PolicyResponse
	}
	if opts.SoftwareID > 0 {
		params["software_id"] = strconv.Itoa(opts.SoftwareID)
	}
	if opts.SoftwareVersionID > 0 {
		params["software_version_id"] = strconv.Itoa(opts.SoftwareVersionID)
	}
	if opts.SoftwareTitleID > 0 {
		params["software_title_id"] = strconv.Itoa(opts.SoftwareTitleID)
	}
	if opts.OSName != "" {
		params["os_name"] = opts.OSName
	}
	if opts.OSVersion != "" {
		params["os_version"] = opts.OSVersion
	}
	if opts.OSVersionID > 0 {
		params["os_version_id"] = strconv.Itoa(opts.OSVersionID)
	}
	if opts.TeamID > 0 {
		params["team_id"] = strconv.Itoa(opts.TeamID)
	}
	if opts.MDMEnrollmentStatus != "" {
		params["mdm_enrollment_status"] = opts.MDMEnrollmentStatus
	}
	if opts.MDMName != "" {
		params["mdm_name"] = opts.MDMName
	}
	if opts.MacOSSettingsDiskEncryption != "" {
		params["macos_settings_disk_encryption"] = opts.MacOSSettingsDiskEncryption
	}
	if opts.BootstrapPackage != "" {
		params["bootstrap_package"] = opts.BootstrapPackage
	}
	if opts.MacOSSetupAssistant != "" {
		params["macos_setup_assistant"] = opts.MacOSSetupAssistant
	}
	if opts.LowDiskSpace > 0 {
		params["low_disk_space"] = strconv.Itoa(opts.LowDiskSpace)
	}
	if opts.Vulnerable {
		params["vulnerable"] = "true"
	}
	if opts.DeviceMapping {
		params["device_mapping"] = "true"
	}

	var response struct {
		Hosts []Host `json:"hosts"`
	}

	err := c.Get(ctx, "/hosts", params, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to list hosts: %w", err)
	}

	return response.Hosts, nil
}

// GetHost returns a single host by ID.
func (c *Client) GetHost(ctx context.Context, id int) (*Host, error) {
	var response struct {
		Host Host `json:"host"`
	}

	err := c.Get(ctx, fmt.Sprintf("/hosts/%d", id), nil, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get host %d: %w", id, err)
	}

	return &response.Host, nil
}

// GetHostByIdentifier returns a host by identifier (hostname, UUID, or serial number).
func (c *Client) GetHostByIdentifier(ctx context.Context, identifier string) (*Host, error) {
	var response struct {
		Host Host `json:"host"`
	}

	err := c.Get(ctx, fmt.Sprintf("/hosts/identifier/%s", identifier), nil, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get host by identifier %q: %w", identifier, err)
	}

	return &response.Host, nil
}
