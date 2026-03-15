package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &ConfigurationResource{}
	_ resource.ResourceWithImportState = &ConfigurationResource{}
)

// NewConfigurationResource creates a new configuration resource.
func NewConfigurationResource() resource.Resource {
	return &ConfigurationResource{}
}

// ConfigurationResource defines the resource implementation.
type ConfigurationResource struct {
	client *fleetdm.Client
}

// ConfigurationResourceModel describes the resource data model.
type ConfigurationResourceModel struct {
	ID types.String `tfsdk:"id"`

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

	// Agent Options (JSON)
	AgentOptions types.String `tfsdk:"agent_options"`
}

// Metadata returns the resource type name.
func (r *ConfigurationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_configuration"
}

// Schema defines the schema for the resource.
func (r *ConfigurationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the FleetDM server configuration.",
		MarkdownDescription: `Manages the FleetDM server configuration.

This resource allows you to configure global Fleet settings including organization info, server settings, host expiry, and agent options.

~> **Note:** There is only one Fleet configuration per server, so this resource manages a singleton. The ID is always "configuration".

## Example Usage

` + "```hcl" + `
resource "fleetdm_configuration" "main" {
  org_name    = "My Organization"
  org_logo_url = "https://example.com/logo.png"
  contact_url  = "https://example.com/support"

  host_expiry_enabled = true
  host_expiry_window  = 30

  enable_host_users         = true
  enable_software_inventory = true
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The identifier of the configuration (always 'configuration').",
			},
			// Organization Info
			"org_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the organization.",
			},
			"org_logo_url": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "URL of the organization logo.",
			},
			"org_logo_url_light_background": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "URL of the organization logo for light backgrounds.",
			},
			"contact_url": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("https://fleetdm.com/company/contact"),
				MarkdownDescription: "URL for contacting support.",
			},
			// Server Settings
			"server_url": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The Fleet server URL. Changing this requires enrolled hosts to re-enroll.",
			},
			"live_query_disabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether live queries are disabled.",
			},
			"enable_analytics": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether usage analytics are enabled.",
			},
			"query_reports_disabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether query reports are disabled.",
			},
			"scripts_disabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether scripts are disabled.",
			},
			"ai_features_disabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether AI features are disabled.",
			},
			// Host Expiry Settings
			"host_expiry_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether to automatically remove hosts that have not checked in.",
			},
			"host_expiry_window": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				MarkdownDescription: "Number of days after which a host is removed if it hasn't checked in.",
			},
			// Activity Expiry Settings
			"activity_expiry_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether to automatically remove old activities.",
			},
			"activity_expiry_window": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				MarkdownDescription: "Number of days after which activities are removed.",
			},
			// Features
			"enable_host_users": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether to collect user information from hosts.",
			},
			"enable_software_inventory": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether to collect software inventory from hosts.",
			},
			// Fleet Desktop
			"transparency_url": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "URL shown in Fleet Desktop transparency modal.",
			},
			// Agent Options
			"agent_options": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "JSON-encoded osquery agent options.",
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *ConfigurationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// Create creates the resource and sets the initial Terraform state.
func (r *ConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating Fleet configuration resource")

	// Build the update request
	updateReq := r.buildUpdateRequest(&data)

	// Update the configuration
	config, err := r.client.UpdateAppConfig(ctx, updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", "Unable to update Fleet configuration: "+err.Error())
		return
	}

	// Set ID
	data.ID = types.StringValue("configuration")

	// Update state from response
	r.updateModelFromConfig(&data, config)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *ConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config, err := r.client.GetAppConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", "Unable to read Fleet configuration: "+err.Error())
		return
	}

	data.ID = types.StringValue("configuration")
	r.updateModelFromConfig(&data, config)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update updates the resource and sets the updated Terraform state.
func (r *ConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating Fleet configuration resource")

	// Build the update request
	updateReq := r.buildUpdateRequest(&data)

	// Update the configuration
	config, err := r.client.UpdateAppConfig(ctx, updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", "Unable to update Fleet configuration: "+err.Error())
		return
	}

	// Set ID (singleton resource)
	data.ID = types.StringValue("configuration")

	// Update state from response
	r.updateModelFromConfig(&data, config)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete removes the resource from Terraform state.
// Note: Fleet configuration cannot be deleted, only reset to defaults.
func (r *ConfigurationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Configuration cannot be deleted - it always exists
	// Just remove from state
	tflog.Info(ctx, "Fleet configuration resource removed from state (configuration still exists on server)")
}

// ImportState imports an existing resource into Terraform state.
func (r *ConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// buildUpdateRequest creates an UpdateAppConfigRequest from the resource model.
func (r *ConfigurationResource) buildUpdateRequest(data *ConfigurationResourceModel) *fleetdm.UpdateAppConfigRequest {
	req := &fleetdm.UpdateAppConfigRequest{
		OrgInfo: &fleetdm.OrgInfo{
			OrgName:                   data.OrgName.ValueString(),
			OrgLogoURL:                data.OrgLogoURL.ValueString(),
			OrgLogoURLLightBackground: data.OrgLogoURLLightBackground.ValueString(),
			ContactURL:                data.ContactURL.ValueString(),
		},
		ServerSettings: r.buildServerSettingsUpdate(data),
		HostExpirySettings: &fleetdm.HostExpirySettings{
			HostExpiryEnabled: data.HostExpiryEnabled.ValueBool(),
			HostExpiryWindow:  int(data.HostExpiryWindow.ValueInt64()),
		},
		ActivityExpirySettings: &fleetdm.ActivityExpirySettings{
			ActivityExpiryEnabled: data.ActivityExpiryEnabled.ValueBool(),
			ActivityExpiryWindow:  int(data.ActivityExpiryWindow.ValueInt64()),
		},
		Features: &fleetdm.Features{
			EnableHostUsers:         data.EnableHostUsers.ValueBool(),
			EnableSoftwareInventory: data.EnableSoftwareInventory.ValueBool(),
		},
	}

	// Only send FleetDesktop.TransparencyURL when the user has explicitly set it.
	// Sending "" would override Fleet's existing value with an empty string.
	if !data.TransparencyURL.IsNull() && !data.TransparencyURL.IsUnknown() {
		req.FleetDesktop = &fleetdm.FleetDesktopSettings{
			TransparencyURL: data.TransparencyURL.ValueString(),
		}
	}

	// Handle agent options if provided
	if !data.AgentOptions.IsNull() && !data.AgentOptions.IsUnknown() && data.AgentOptions.ValueString() != "" {
		agentOptionsJSON := json.RawMessage(data.AgentOptions.ValueString())
		req.AgentOptions = &agentOptionsJSON
	}

	return req
}

// buildServerSettingsUpdate constructs the server_settings update payload.
// server_url is included only when the user has explicitly provided a value,
// preventing an empty string from failing Fleet's URL validation.
func (r *ConfigurationResource) buildServerSettingsUpdate(data *ConfigurationResourceModel) *fleetdm.ServerSettingsUpdate {
	s := &fleetdm.ServerSettingsUpdate{
		LiveQueryDisabled:    data.LiveQueryDisabled.ValueBool(),
		EnableAnalytics:      data.EnableAnalytics.ValueBool(),
		QueryReportsDisabled: data.QueryReportsDisabled.ValueBool(),
		ScriptsDisabled:      data.ScriptsDisabled.ValueBool(),
		AIFeaturesDisabled:   data.AIFeaturesDisabled.ValueBool(),
	}
	if !data.ServerURL.IsNull() && !data.ServerURL.IsUnknown() && data.ServerURL.ValueString() != "" {
		v := data.ServerURL.ValueString()
		s.ServerURL = &v
	}
	return s
}

// updateModelFromConfig updates the resource model from the API response.
func (r *ConfigurationResource) updateModelFromConfig(data *ConfigurationResourceModel, config *fleetdm.AppConfig) {
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

	// Agent Options
	if config.AgentOptions != nil {
		data.AgentOptions = types.StringValue(string(*config.AgentOptions))
	} else {
		data.AgentOptions = types.StringValue("")
	}
}
