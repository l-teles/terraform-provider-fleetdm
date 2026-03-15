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
var _ datasource.DataSource = &ConfigurationDataSource{}

func NewConfigurationDataSource() datasource.DataSource {
	return &ConfigurationDataSource{}
}

// ConfigurationDataSource defines the data source implementation.
type ConfigurationDataSource struct {
	client *fleetdm.Client
}

// ConfigurationDataSourceModel describes the data source data model.
type ConfigurationDataSourceModel struct {
	// Organization Info
	OrgName                   types.String `tfsdk:"org_name"`
	OrgLogoURL                types.String `tfsdk:"org_logo_url"`
	OrgLogoURLLightBackground types.String `tfsdk:"org_logo_url_light_background"`
	ContactURL                types.String `tfsdk:"contact_url"`

	// Server Settings
	ServerURL            types.String `tfsdk:"server_url"`
	LiveQueryDisabled    types.Bool   `tfsdk:"live_query_disabled"`
	EnableAnalytics      types.Bool   `tfsdk:"enable_analytics"`
	QueryReportsDisabled types.Bool   `tfsdk:"query_reports_disabled"`
	ScriptsDisabled      types.Bool   `tfsdk:"scripts_disabled"`
	AIFeaturesDisabled   types.Bool   `tfsdk:"ai_features_disabled"`

	// Host Expiry Settings
	HostExpiryEnabled types.Bool  `tfsdk:"host_expiry_enabled"`
	HostExpiryWindow  types.Int64 `tfsdk:"host_expiry_window"`

	// Activity Expiry Settings
	ActivityExpiryEnabled types.Bool  `tfsdk:"activity_expiry_enabled"`
	ActivityExpiryWindow  types.Int64 `tfsdk:"activity_expiry_window"`

	// Features
	EnableHostUsers         types.Bool `tfsdk:"enable_host_users"`
	EnableSoftwareInventory types.Bool `tfsdk:"enable_software_inventory"`

	// Fleet Desktop
	TransparencyURL types.String `tfsdk:"transparency_url"`

	// License
	LicenseTier         types.String `tfsdk:"license_tier"`
	LicenseOrganization types.String `tfsdk:"license_organization"`
	LicenseDeviceCount  types.Int64  `tfsdk:"license_device_count"`
	LicenseExpiration   types.String `tfsdk:"license_expiration"`

	// MDM
	MDMEnabledAndConfigured        types.Bool `tfsdk:"mdm_enabled_and_configured"`
	MDMAppleBMEnabledAndConfigured types.Bool `tfsdk:"mdm_apple_bm_enabled_and_configured"`
	MDMAppleBMTermsExpired         types.Bool `tfsdk:"mdm_apple_bm_terms_expired"`

	// Sandbox
	SandboxEnabled types.Bool `tfsdk:"sandbox_enabled"`
}

func (d *ConfigurationDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_configuration"
}

func (d *ConfigurationDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches the Fleet application configuration.",

		Attributes: map[string]schema.Attribute{
			// Organization Info
			"org_name": schema.StringAttribute{
				MarkdownDescription: "The name of the organization using Fleet.",
				Computed:            true,
			},
			"org_logo_url": schema.StringAttribute{
				MarkdownDescription: "The URL of the organization logo.",
				Computed:            true,
			},
			"org_logo_url_light_background": schema.StringAttribute{
				MarkdownDescription: "The URL of the organization logo for light backgrounds.",
				Computed:            true,
			},
			"contact_url": schema.StringAttribute{
				MarkdownDescription: "The URL for contacting support.",
				Computed:            true,
			},

			// Server Settings
			"server_url": schema.StringAttribute{
				MarkdownDescription: "The Fleet server URL.",
				Computed:            true,
			},
			"live_query_disabled": schema.BoolAttribute{
				MarkdownDescription: "Whether live queries are disabled.",
				Computed:            true,
			},
			"enable_analytics": schema.BoolAttribute{
				MarkdownDescription: "Whether analytics are enabled.",
				Computed:            true,
			},
			"query_reports_disabled": schema.BoolAttribute{
				MarkdownDescription: "Whether query reports are disabled.",
				Computed:            true,
			},
			"scripts_disabled": schema.BoolAttribute{
				MarkdownDescription: "Whether scripts are disabled.",
				Computed:            true,
			},
			"ai_features_disabled": schema.BoolAttribute{
				MarkdownDescription: "Whether AI features are disabled.",
				Computed:            true,
			},

			// Host Expiry Settings
			"host_expiry_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether automatic host expiry is enabled.",
				Computed:            true,
			},
			"host_expiry_window": schema.Int64Attribute{
				MarkdownDescription: "The number of days after which hosts are expired.",
				Computed:            true,
			},

			// Activity Expiry Settings
			"activity_expiry_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether automatic activity cleanup is enabled.",
				Computed:            true,
			},
			"activity_expiry_window": schema.Int64Attribute{
				MarkdownDescription: "The number of days after which activities are cleaned up.",
				Computed:            true,
			},

			// Features
			"enable_host_users": schema.BoolAttribute{
				MarkdownDescription: "Whether host user collection is enabled.",
				Computed:            true,
			},
			"enable_software_inventory": schema.BoolAttribute{
				MarkdownDescription: "Whether software inventory collection is enabled.",
				Computed:            true,
			},

			// Fleet Desktop
			"transparency_url": schema.StringAttribute{
				MarkdownDescription: "The transparency URL shown in Fleet Desktop.",
				Computed:            true,
			},

			// License
			"license_tier": schema.StringAttribute{
				MarkdownDescription: "The Fleet license tier (e.g., 'free', 'premium').",
				Computed:            true,
			},
			"license_organization": schema.StringAttribute{
				MarkdownDescription: "The organization name in the license.",
				Computed:            true,
			},
			"license_device_count": schema.Int64Attribute{
				MarkdownDescription: "The maximum number of devices allowed by the license.",
				Computed:            true,
			},
			"license_expiration": schema.StringAttribute{
				MarkdownDescription: "The expiration date of the license.",
				Computed:            true,
			},

			// MDM
			"mdm_enabled_and_configured": schema.BoolAttribute{
				MarkdownDescription: "Whether MDM is enabled and configured.",
				Computed:            true,
			},
			"mdm_apple_bm_enabled_and_configured": schema.BoolAttribute{
				MarkdownDescription: "Whether Apple Business Manager is enabled and configured.",
				Computed:            true,
			},
			"mdm_apple_bm_terms_expired": schema.BoolAttribute{
				MarkdownDescription: "Whether Apple Business Manager terms have expired.",
				Computed:            true,
			},

			// Sandbox
			"sandbox_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether this is a sandbox instance.",
				Computed:            true,
			},
		},
	}
}

func (d *ConfigurationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *ConfigurationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ConfigurationDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Call API
	config, err := d.client.GetAppConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read Fleet configuration, got error: %s", err))
		return
	}

	// Map response to model
	// Organization Info
	data.OrgName = types.StringValue(config.OrgInfo.OrgName)
	data.OrgLogoURL = types.StringValue(config.OrgInfo.OrgLogoURL)
	data.OrgLogoURLLightBackground = types.StringValue(config.OrgInfo.OrgLogoURLLightBackground)
	data.ContactURL = types.StringValue(config.OrgInfo.ContactURL)

	// Server Settings
	data.ServerURL = types.StringValue(config.ServerSettings.ServerURL)
	data.LiveQueryDisabled = types.BoolValue(config.ServerSettings.LiveQueryDisabled)
	data.EnableAnalytics = types.BoolValue(config.ServerSettings.EnableAnalytics)
	data.QueryReportsDisabled = types.BoolValue(config.ServerSettings.QueryReportsDisabled)
	data.ScriptsDisabled = types.BoolValue(config.ServerSettings.ScriptsDisabled)
	data.AIFeaturesDisabled = types.BoolValue(config.ServerSettings.AIFeaturesDisabled)

	// Host Expiry Settings
	data.HostExpiryEnabled = types.BoolValue(config.HostExpirySettings.HostExpiryEnabled)
	data.HostExpiryWindow = types.Int64Value(int64(config.HostExpirySettings.HostExpiryWindow))

	// Activity Expiry Settings
	data.ActivityExpiryEnabled = types.BoolValue(config.ActivityExpirySettings.ActivityExpiryEnabled)
	data.ActivityExpiryWindow = types.Int64Value(int64(config.ActivityExpirySettings.ActivityExpiryWindow))

	// Features
	data.EnableHostUsers = types.BoolValue(config.Features.EnableHostUsers)
	data.EnableSoftwareInventory = types.BoolValue(config.Features.EnableSoftwareInventory)

	// Fleet Desktop
	data.TransparencyURL = types.StringValue(config.FleetDesktop.TransparencyURL)

	// License
	if config.License != nil {
		data.LicenseTier = types.StringValue(config.License.Tier)
		data.LicenseOrganization = types.StringValue(config.License.Organization)
		data.LicenseDeviceCount = types.Int64Value(int64(config.License.DeviceCount))
		data.LicenseExpiration = types.StringValue(config.License.Expiration)
	} else {
		data.LicenseTier = types.StringValue("")
		data.LicenseOrganization = types.StringValue("")
		data.LicenseDeviceCount = types.Int64Value(0)
		data.LicenseExpiration = types.StringValue("")
	}

	// MDM
	if config.MDM != nil {
		data.MDMEnabledAndConfigured = types.BoolValue(config.MDM.EnabledAndConfigured)
		data.MDMAppleBMEnabledAndConfigured = types.BoolValue(config.MDM.AppleBMEnabledAndConfigured)
		data.MDMAppleBMTermsExpired = types.BoolValue(config.MDM.AppleBMTermsExpired)
	} else {
		data.MDMEnabledAndConfigured = types.BoolValue(false)
		data.MDMAppleBMEnabledAndConfigured = types.BoolValue(false)
		data.MDMAppleBMTermsExpired = types.BoolValue(false)
	}

	// Sandbox
	data.SandboxEnabled = types.BoolValue(config.SandboxEnabled)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
