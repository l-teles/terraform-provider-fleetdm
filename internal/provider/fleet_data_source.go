package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &FleetDataSource{}

// NewFleetDataSource creates a new fleet data source.
func NewFleetDataSource() datasource.DataSource {
	return &FleetDataSource{}
}

// NewTeamDataSource creates a deprecated team data source (alias for FleetDataSource).
func NewTeamDataSource() datasource.DataSource {
	return &FleetDataSource{deprecated: true}
}

// FleetDataSource defines the data source implementation.
type FleetDataSource struct {
	client     *fleetdm.Client
	deprecated bool
}

// FleetDataSourceModel describes the data source data model.
type FleetDataSourceModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`

	// Host expiry settings
	HostExpiryEnabled types.Bool  `tfsdk:"host_expiry_enabled"`
	HostExpiryWindow  types.Int64 `tfsdk:"host_expiry_window"`

	// MDM Settings
	EnableDiskEncryption types.Bool `tfsdk:"enable_disk_encryption"`

	// Computed fields
	UserCount types.Int64 `tfsdk:"user_count"`
	HostCount types.Int64 `tfsdk:"host_count"`
}

// Metadata returns the data source type name.
func (d *FleetDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	if d.deprecated {
		resp.TypeName = req.ProviderTypeName + "_team"
	} else {
		resp.TypeName = req.ProviderTypeName + "_fleet"
	}
}

// Schema defines the schema for the data source.
func (d *FleetDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	deprecationMsg := ""
	if d.deprecated {
		deprecationMsg = "fleetdm_team is deprecated and will be removed in a future version. Use fleetdm_fleet instead (requires Fleet 4.82.0+)."
	}

	resp.Schema = schema.Schema{
		DeprecationMessage:  deprecationMsg,
		Description:         "Retrieves information about a FleetDM fleet.",
		MarkdownDescription: "Retrieves information about a FleetDM fleet.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description:         "The unique identifier of the fleet.",
				MarkdownDescription: "The unique identifier of the fleet.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				Description:         "The name of the fleet.",
				MarkdownDescription: "The name of the fleet.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				Description:         "A description of the fleet.",
				MarkdownDescription: "A description of the fleet.",
				Computed:            true,
			},
			"host_expiry_enabled": schema.BoolAttribute{
				Description:         "Whether host expiry is enabled for this fleet.",
				MarkdownDescription: "Whether host expiry is enabled for this fleet.",
				Computed:            true,
			},
			"host_expiry_window": schema.Int64Attribute{
				Description:         "The number of days after which hosts are considered expired.",
				MarkdownDescription: "The number of days after which hosts are considered expired.",
				Computed:            true,
			},
			"enable_disk_encryption": schema.BoolAttribute{
				Description:         "Whether disk encryption is enforced for hosts in this fleet.",
				MarkdownDescription: "Whether disk encryption is enforced for hosts in this fleet.",
				Computed:            true,
			},
			"user_count": schema.Int64Attribute{
				Description:         "The number of users in the fleet.",
				MarkdownDescription: "The number of users in the fleet.",
				Computed:            true,
			},
			"host_count": schema.Int64Attribute{
				Description:         "The number of hosts in the fleet.",
				MarkdownDescription: "The number of hosts in the fleet.",
				Computed:            true,
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *FleetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (d *FleetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config FleetDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading fleet data source", map[string]interface{}{
		"id": config.ID.ValueInt64(),
	})

	// Get the fleet from the API
	team, err := d.client.GetTeam(ctx, config.ID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading FleetDM Fleet",
			fmt.Sprintf("Could not read fleet ID %d: %s", config.ID.ValueInt64(), err.Error()),
		)
		return
	}

	// Map response to model
	config.ID = types.Int64Value(team.ID)
	config.Name = types.StringValue(team.Name)
	config.Description = types.StringValue(team.Description)
	config.UserCount = types.Int64Value(int64(team.UserCount))
	config.HostCount = types.Int64Value(int64(team.HostCount))

	if team.HostExpirySettings != nil {
		config.HostExpiryEnabled = types.BoolValue(team.HostExpirySettings.HostExpiryEnabled)
		config.HostExpiryWindow = types.Int64Value(int64(team.HostExpirySettings.HostExpiryWindow))
	} else {
		config.HostExpiryEnabled = types.BoolValue(false)
		config.HostExpiryWindow = types.Int64Null()
	}

	if team.MDM != nil {
		config.EnableDiskEncryption = types.BoolValue(team.MDM.EnableDiskEncryption)
	} else {
		config.EnableDiskEncryption = types.BoolValue(false)
	}

	tflog.Debug(ctx, "Fleet data source read", map[string]interface{}{
		"id":   team.ID,
		"name": team.Name,
	})

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
