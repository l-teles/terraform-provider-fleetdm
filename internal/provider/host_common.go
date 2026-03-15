package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// hostDetailFields holds the computed fields common to both host data sources.
type hostDetailFields struct {
	UUID                      types.String  `tfsdk:"uuid"`
	Hostname                  types.String  `tfsdk:"hostname"`
	DisplayName               types.String  `tfsdk:"display_name"`
	ComputerName              types.String  `tfsdk:"computer_name"`
	Platform                  types.String  `tfsdk:"platform"`
	OSVersion                 types.String  `tfsdk:"os_version"`
	Build                     types.String  `tfsdk:"build"`
	PlatformLike              types.String  `tfsdk:"platform_like"`
	CPUType                   types.String  `tfsdk:"cpu_type"`
	CPUBrand                  types.String  `tfsdk:"cpu_brand"`
	CPUPhysicalCores          types.Int64   `tfsdk:"cpu_physical_cores"`
	CPULogicalCores           types.Int64   `tfsdk:"cpu_logical_cores"`
	Memory                    types.Int64   `tfsdk:"memory"`
	HardwareVendor            types.String  `tfsdk:"hardware_vendor"`
	HardwareModel             types.String  `tfsdk:"hardware_model"`
	HardwareSerial            types.String  `tfsdk:"hardware_serial"`
	PrimaryIP                 types.String  `tfsdk:"primary_ip"`
	PrimaryMac                types.String  `tfsdk:"primary_mac"`
	PublicIP                  types.String  `tfsdk:"public_ip"`
	TeamID                    types.Int64   `tfsdk:"team_id"`
	TeamName                  types.String  `tfsdk:"team_name"`
	Status                    types.String  `tfsdk:"status"`
	GigsDiskSpaceAvailable    types.Float64 `tfsdk:"gigs_disk_space_available"`
	PercentDiskSpaceAvailable types.Float64 `tfsdk:"percent_disk_space_available"`
	SeenTime                  types.String  `tfsdk:"seen_time"`
	CreatedAt                 types.String  `tfsdk:"created_at"`
	UpdatedAt                 types.String  `tfsdk:"updated_at"`
}

// hostComputedAttributes returns the schema attributes shared by both host data sources.
func hostComputedAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"uuid": schema.StringAttribute{
			MarkdownDescription: "Host UUID.",
			Computed:            true,
		},
		"hostname": schema.StringAttribute{
			MarkdownDescription: "Host hostname.",
			Computed:            true,
		},
		"display_name": schema.StringAttribute{
			MarkdownDescription: "Host display name.",
			Computed:            true,
		},
		"computer_name": schema.StringAttribute{
			MarkdownDescription: "Host computer name.",
			Computed:            true,
		},
		"platform": schema.StringAttribute{
			MarkdownDescription: "Host platform (darwin, windows, ubuntu, etc.).",
			Computed:            true,
		},
		"os_version": schema.StringAttribute{
			MarkdownDescription: "Host OS version string.",
			Computed:            true,
		},
		"build": schema.StringAttribute{
			MarkdownDescription: "Host OS build string.",
			Computed:            true,
		},
		"platform_like": schema.StringAttribute{
			MarkdownDescription: "Host platform family.",
			Computed:            true,
		},
		"cpu_type": schema.StringAttribute{
			MarkdownDescription: "Host CPU type.",
			Computed:            true,
		},
		"cpu_brand": schema.StringAttribute{
			MarkdownDescription: "Host CPU brand.",
			Computed:            true,
		},
		"cpu_physical_cores": schema.Int64Attribute{
			MarkdownDescription: "Number of physical CPU cores.",
			Computed:            true,
		},
		"cpu_logical_cores": schema.Int64Attribute{
			MarkdownDescription: "Number of logical CPU cores.",
			Computed:            true,
		},
		"memory": schema.Int64Attribute{
			MarkdownDescription: "Total memory in bytes.",
			Computed:            true,
		},
		"hardware_vendor": schema.StringAttribute{
			MarkdownDescription: "Hardware vendor name.",
			Computed:            true,
		},
		"hardware_model": schema.StringAttribute{
			MarkdownDescription: "Hardware model.",
			Computed:            true,
		},
		"hardware_serial": schema.StringAttribute{
			MarkdownDescription: "Hardware serial number.",
			Computed:            true,
		},
		"primary_ip": schema.StringAttribute{
			MarkdownDescription: "Primary IP address.",
			Computed:            true,
		},
		"primary_mac": schema.StringAttribute{
			MarkdownDescription: "Primary MAC address.",
			Computed:            true,
		},
		"public_ip": schema.StringAttribute{
			MarkdownDescription: "Public IP address.",
			Computed:            true,
		},
		"team_id": schema.Int64Attribute{
			MarkdownDescription: "Team ID the host belongs to.",
			Computed:            true,
		},
		"team_name": schema.StringAttribute{
			MarkdownDescription: "Team name the host belongs to.",
			Computed:            true,
		},
		"status": schema.StringAttribute{
			MarkdownDescription: "Host status (online, offline, mia, new).",
			Computed:            true,
		},
		"gigs_disk_space_available": schema.Float64Attribute{
			MarkdownDescription: "Available disk space in GB.",
			Computed:            true,
		},
		"percent_disk_space_available": schema.Float64Attribute{
			MarkdownDescription: "Percentage of disk space available.",
			Computed:            true,
		},
		"seen_time": schema.StringAttribute{
			MarkdownDescription: "Last seen timestamp.",
			Computed:            true,
		},
		"created_at": schema.StringAttribute{
			MarkdownDescription: "Creation timestamp.",
			Computed:            true,
		},
		"updated_at": schema.StringAttribute{
			MarkdownDescription: "Last update timestamp.",
			Computed:            true,
		},
	}
}

// mapHostToDetailFields populates the shared host fields from an API host object.
func mapHostToDetailFields(host *fleetdm.Host) hostDetailFields {
	f := hostDetailFields{
		UUID:                      types.StringValue(host.UUID),
		Hostname:                  types.StringValue(host.Hostname),
		DisplayName:               types.StringValue(host.DisplayName),
		ComputerName:              types.StringValue(host.ComputerName),
		Platform:                  types.StringValue(host.Platform),
		OSVersion:                 types.StringValue(host.OSVersion),
		Build:                     types.StringValue(host.Build),
		PlatformLike:              types.StringValue(host.PlatformLike),
		CPUType:                   types.StringValue(host.CPUType),
		CPUBrand:                  types.StringValue(host.CPUBrand),
		CPUPhysicalCores:          types.Int64Value(int64(host.CPUPhysicalCores)),
		CPULogicalCores:           types.Int64Value(int64(host.CPULogicalCores)),
		Memory:                    types.Int64Value(host.Memory),
		HardwareVendor:            types.StringValue(host.HardwareVendor),
		HardwareModel:             types.StringValue(host.HardwareModel),
		HardwareSerial:            types.StringValue(host.HardwareSerial),
		PrimaryIP:                 types.StringValue(host.PrimaryIP),
		PrimaryMac:                types.StringValue(host.PrimaryMac),
		PublicIP:                  types.StringValue(host.PublicIP),
		Status:                    types.StringValue(host.Status),
		GigsDiskSpaceAvailable:    types.Float64Value(host.GigsDiskSpaceAvailable),
		PercentDiskSpaceAvailable: types.Float64Value(host.PercentDiskSpaceAvailable),
		TeamName:                  types.StringValue(host.TeamName),
		SeenTime:                  types.StringValue(host.SeenTime.Format("2006-01-02T15:04:05Z")),
		CreatedAt:                 types.StringValue(host.CreatedAt.Format("2006-01-02T15:04:05Z")),
		UpdatedAt:                 types.StringValue(host.UpdatedAt.Format("2006-01-02T15:04:05Z")),
	}

	f.TeamID = intPtrToInt64(host.TeamID)

	return f
}
