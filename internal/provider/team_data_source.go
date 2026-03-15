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
var _ datasource.DataSource = &TeamDataSource{}

// NewTeamDataSource creates a new team data source.
func NewTeamDataSource() datasource.DataSource {
	return &TeamDataSource{}
}

// TeamDataSource defines the data source implementation.
type TeamDataSource struct {
	client *fleetdm.Client
}

// TeamDataSourceModel describes the data source data model.
type TeamDataSourceModel struct {
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
func (d *TeamDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team"
}

// Schema defines the schema for the data source.
func (d *TeamDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves information about a FleetDM team.",
		MarkdownDescription: `Retrieves information about a FleetDM team.

## Example Usage

` + "```hcl" + `
data "fleetdm_team" "workstations" {
  id = 1
}

output "team_name" {
  value = data.fleetdm_team.workstations.name
}
` + "```",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description:         "The unique identifier of the team.",
				MarkdownDescription: "The unique identifier of the team.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				Description:         "The name of the team.",
				MarkdownDescription: "The name of the team.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				Description:         "A description of the team.",
				MarkdownDescription: "A description of the team.",
				Computed:            true,
			},
			"host_expiry_enabled": schema.BoolAttribute{
				Description:         "Whether host expiry is enabled for this team.",
				MarkdownDescription: "Whether host expiry is enabled for this team.",
				Computed:            true,
			},
			"host_expiry_window": schema.Int64Attribute{
				Description:         "The number of days after which hosts are considered expired.",
				MarkdownDescription: "The number of days after which hosts are considered expired.",
				Computed:            true,
			},
			"enable_disk_encryption": schema.BoolAttribute{
				Description:         "Whether disk encryption is enforced for hosts in this team.",
				MarkdownDescription: "Whether disk encryption is enforced for hosts in this team.",
				Computed:            true,
			},
			"user_count": schema.Int64Attribute{
				Description:         "The number of users in the team.",
				MarkdownDescription: "The number of users in the team.",
				Computed:            true,
			},
			"host_count": schema.Int64Attribute{
				Description:         "The number of hosts in the team.",
				MarkdownDescription: "The number of hosts in the team.",
				Computed:            true,
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *TeamDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (d *TeamDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TeamDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading team data source", map[string]interface{}{
		"id": config.ID.ValueInt64(),
	})

	// Get the team from the API
	team, err := d.client.GetTeam(ctx, config.ID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading FleetDM Team",
			fmt.Sprintf("Could not read team ID %d: %s", config.ID.ValueInt64(), err.Error()),
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

	tflog.Debug(ctx, "Team data source read", map[string]interface{}{
		"id":   team.ID,
		"name": team.Name,
	})

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
