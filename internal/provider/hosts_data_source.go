package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &HostsDataSource{}

// NewHostsDataSource creates a new hosts data source.
func NewHostsDataSource() datasource.DataSource {
	return &HostsDataSource{}
}

// HostsDataSource defines the data source implementation.
type HostsDataSource struct {
	client *fleetdm.Client
}

// HostsDataSourceModel describes the data source data model.
type HostsDataSourceModel struct {
	Query    types.String    `tfsdk:"query"`
	Status   types.String    `tfsdk:"status"`
	TeamID   types.Int64     `tfsdk:"team_id"`
	Platform types.String    `tfsdk:"platform"`
	LabelID  types.Int64     `tfsdk:"label_id"`
	PolicyID types.Int64     `tfsdk:"policy_id"`
	PerPage  types.Int64     `tfsdk:"per_page"`
	Page     types.Int64     `tfsdk:"page"`
	Hosts    []HostItemModel `tfsdk:"hosts"`
}

// HostItemModel describes a host in the list.
type HostItemModel struct {
	ID             types.Int64  `tfsdk:"id"`
	UUID           types.String `tfsdk:"uuid"`
	Hostname       types.String `tfsdk:"hostname"`
	DisplayName    types.String `tfsdk:"display_name"`
	Platform       types.String `tfsdk:"platform"`
	OSVersion      types.String `tfsdk:"os_version"`
	HardwareVendor types.String `tfsdk:"hardware_vendor"`
	HardwareModel  types.String `tfsdk:"hardware_model"`
	HardwareSerial types.String `tfsdk:"hardware_serial"`
	PrimaryIP      types.String `tfsdk:"primary_ip"`
	TeamID         types.Int64  `tfsdk:"team_id"`
	TeamName       types.String `tfsdk:"team_name"`
	Status         types.String `tfsdk:"status"`
}

// Metadata returns the data source type name.
func (d *HostsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hosts"
}

// Schema defines the schema for the data source.
func (d *HostsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to get a list of FleetDM hosts with optional filtering.",

		Attributes: map[string]schema.Attribute{
			"query": schema.StringAttribute{
				MarkdownDescription: "Search query string to filter hosts by hostname, display name, or IP.",
				Optional:            true,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Filter by host status (online, offline, mia, new, missing).",
				Optional:            true,
			},
			"team_id": schema.Int64Attribute{
				MarkdownDescription: "Filter by team ID.",
				Optional:            true,
			},
			"platform": schema.StringAttribute{
				MarkdownDescription: "Filter by platform (darwin, windows, ubuntu, etc.).",
				Optional:            true,
			},
			"label_id": schema.Int64Attribute{
				MarkdownDescription: "Filter by label ID.",
				Optional:            true,
			},
			"policy_id": schema.Int64Attribute{
				MarkdownDescription: "Filter by policy ID.",
				Optional:            true,
			},
			"per_page": schema.Int64Attribute{
				MarkdownDescription: "Number of results per page. Default is 100.",
				Optional:            true,
			},
			"page": schema.Int64Attribute{
				MarkdownDescription: "Page number for pagination (1-indexed). Default is 1.",
				Optional:            true,
			},
			"hosts": schema.ListNestedAttribute{
				MarkdownDescription: "List of hosts matching the filter criteria.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							MarkdownDescription: "Host ID.",
							Computed:            true,
						},
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
						"platform": schema.StringAttribute{
							MarkdownDescription: "Host platform.",
							Computed:            true,
						},
						"os_version": schema.StringAttribute{
							MarkdownDescription: "Host OS version.",
							Computed:            true,
						},
						"hardware_vendor": schema.StringAttribute{
							MarkdownDescription: "Hardware vendor.",
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
						"team_id": schema.Int64Attribute{
							MarkdownDescription: "Team ID.",
							Computed:            true,
						},
						"team_name": schema.StringAttribute{
							MarkdownDescription: "Team name.",
							Computed:            true,
						},
						"status": schema.StringAttribute{
							MarkdownDescription: "Host status.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *HostsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read reads the data source.
func (d *HostsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data HostsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	opts := fleetdm.ListHostsOptions{}

	if !data.Query.IsNull() {
		opts.Query = data.Query.ValueString()
	}
	if !data.Status.IsNull() {
		opts.Status = data.Status.ValueString()
	}
	if !data.TeamID.IsNull() {
		opts.TeamID = int(data.TeamID.ValueInt64())
	}
	if !data.LabelID.IsNull() {
		opts.LabelID = int(data.LabelID.ValueInt64())
	}
	if !data.PolicyID.IsNull() {
		opts.PolicyID = int(data.PolicyID.ValueInt64())
	}
	if !data.PerPage.IsNull() {
		opts.PerPage = int(data.PerPage.ValueInt64())
	} else {
		opts.PerPage = 100 // Default
	}
	if !data.Page.IsNull() {
		opts.Page = int(data.Page.ValueInt64())
	} else {
		opts.Page = 1 // Default
	}

	hosts, err := d.client.ListHosts(ctx, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to List Hosts",
			fmt.Sprintf("Unable to list hosts: %s", err),
		)
		return
	}

	// Map response to model
	data.Hosts = make([]HostItemModel, len(hosts))
	for i, host := range hosts {
		item := HostItemModel{
			ID:             types.Int64Value(int64(host.ID)),
			UUID:           types.StringValue(host.UUID),
			Hostname:       types.StringValue(host.Hostname),
			DisplayName:    types.StringValue(host.DisplayName),
			Platform:       types.StringValue(host.Platform),
			OSVersion:      types.StringValue(host.OSVersion),
			HardwareVendor: types.StringValue(host.HardwareVendor),
			HardwareModel:  types.StringValue(host.HardwareModel),
			HardwareSerial: types.StringValue(host.HardwareSerial),
			PrimaryIP:      types.StringValue(host.PrimaryIP),
			TeamName:       types.StringValue(host.TeamName),
			Status:         types.StringValue(host.Status),
		}

		item.TeamID = intPtrToInt64(host.TeamID)

		data.Hosts[i] = item
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
