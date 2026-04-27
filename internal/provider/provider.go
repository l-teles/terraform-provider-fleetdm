package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure FleetDMProvider satisfies various provider interfaces.
var _ provider.Provider = &FleetDMProvider{}

// FleetDMProvider defines the provider implementation.
type FleetDMProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// FleetDMProviderModel describes the provider data model.
type FleetDMProviderModel struct {
	ServerAddress types.String `tfsdk:"server_address"`
	APIKey        types.String `tfsdk:"api_key"`
	VerifyTLS     types.Bool   `tfsdk:"verify_tls"`
	Timeout       types.Int64  `tfsdk:"timeout"`
}

// New creates a new provider instance.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &FleetDMProvider{
			version: version,
		}
	}
}

// Metadata returns the provider type name.
func (p *FleetDMProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "fleetdm"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data.
func (p *FleetDMProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The FleetDM provider allows you to manage FleetDM resources using Terraform.",
		MarkdownDescription: `
The FleetDM provider allows you to manage FleetDM resources using Terraform.

## Example Usage

` + "```hcl" + `
provider "fleetdm" {
  server_address = "https://fleet.example.com"
  api_key        = var.fleetdm_api_key
}
` + "```" + `

## Authentication

The provider supports authentication via API key. You can obtain an API key from the FleetDM UI
under Settings > Integrations > API.

The API key can be provided via:
1. The ` + "`api_key`" + ` provider attribute
2. The ` + "`FLEETDM_API_TOKEN`" + ` environment variable

Similarly, the server address can be provided via:
1. The ` + "`server_address`" + ` provider attribute
2. The ` + "`FLEETDM_URL`" + ` environment variable
`,
		Attributes: map[string]schema.Attribute{
			"server_address": schema.StringAttribute{
				Description: "The address of the FleetDM server (e.g., 'https://fleet.example.com'). " +
					"Can also be set via the FLEETDM_URL environment variable.",
				MarkdownDescription: "The address of the FleetDM server (e.g., `https://fleet.example.com`). " +
					"Can also be set via the `FLEETDM_URL` environment variable.",
				Optional: true,
			},
			"api_key": schema.StringAttribute{
				Description: "The API key for authenticating with the FleetDM API. " +
					"Can also be set via the FLEETDM_API_TOKEN environment variable.",
				MarkdownDescription: "The API key for authenticating with the FleetDM API. " +
					"Can also be set via the `FLEETDM_API_TOKEN` environment variable.",
				Optional:  true,
				Sensitive: true,
			},
			"verify_tls": schema.BoolAttribute{
				Description: "Whether to verify the server's TLS certificate. Defaults to true. " +
					"Can also be set via the FLEETDM_VERIFY_TLS environment variable (set to 'false' or '0' to disable).",
				MarkdownDescription: "Whether to verify the server's TLS certificate. Defaults to `true`. " +
					"Can also be set via the `FLEETDM_VERIFY_TLS` environment variable (set to `false` or `0` to disable).",
				Optional: true,
			},
			"timeout": schema.Int64Attribute{
				Description:         "The timeout for API requests in seconds. Defaults to 30.",
				MarkdownDescription: "The timeout for API requests in seconds. Defaults to `30`.",
				Optional:            true,
			},
		},
	}
}

// Configure prepares a FleetDM API client for data sources and resources.
func (p *FleetDMProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Info(ctx, "Configuring FleetDM client")

	var config FleetDMProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values from environment variables
	serverAddress := os.Getenv("FLEETDM_URL")
	apiKey := os.Getenv("FLEETDM_API_TOKEN")

	// Override with config values if set
	if !config.ServerAddress.IsNull() {
		serverAddress = config.ServerAddress.ValueString()
	}

	if !config.APIKey.IsNull() {
		apiKey = config.APIKey.ValueString()
	}

	// Validate required configuration
	if serverAddress == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("server_address"),
			"Missing FleetDM Server Address",
			"The provider cannot create the FleetDM API client as there is a missing or empty value for the FleetDM server address. "+
				"Set the server_address value in the configuration or use the FLEETDM_URL environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Missing FleetDM API Key",
			"The provider cannot create the FleetDM API client as there is a missing or empty value for the FleetDM API key. "+
				"Set the api_key value in the configuration or use the FLEETDM_API_TOKEN environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Set defaults for optional values
	verifyTLS := true
	if v := os.Getenv("FLEETDM_VERIFY_TLS"); v == "false" || v == "0" {
		verifyTLS = false
	}
	if !config.VerifyTLS.IsNull() {
		verifyTLS = config.VerifyTLS.ValueBool()
	}

	timeout := int64(30)
	if !config.Timeout.IsNull() {
		timeout = config.Timeout.ValueInt64()
	}

	// Create the FleetDM client
	tflog.Debug(ctx, "Creating FleetDM client", map[string]interface{}{
		"server_address": serverAddress,
		"verify_tls":     verifyTLS,
		"timeout":        timeout,
	})

	client, err := fleetdm.NewClient(fleetdm.ClientConfig{
		ServerAddress: serverAddress,
		APIKey:        apiKey,
		VerifyTLS:     verifyTLS,
		Timeout:       int(timeout),
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create FleetDM API Client",
			"An unexpected error occurred when creating the FleetDM API client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"FleetDM Client Error: "+err.Error(),
		)
		return
	}

	// Make the client available during DataSource and Resource type Configure methods.
	resp.DataSourceData = client
	resp.ResourceData = client

	tflog.Info(ctx, "FleetDM client configured successfully", map[string]interface{}{
		"server_address": serverAddress,
	})
}

// Resources defines the resources implemented in the provider.
func (p *FleetDMProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewFleetResource,
		NewTeamResource, // deprecated alias for NewFleetResource
		NewLabelResource,
		NewReportResource,
		NewQueryResource, // deprecated alias for NewReportResource
		NewPolicyResource,
		NewScriptResource,
		NewEnrollSecretResource,
		NewConfigurationResource,
		NewUserResource,
		NewConfigurationProfileResource,
		NewSoftwarePackageResource,
		NewBootstrapPackageResource,
		NewSetupExperienceResource,
	}
}

// DataSources defines the data sources implemented in the provider.
func (p *FleetDMProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewFleetDataSource,
		NewTeamDataSource, // deprecated alias for NewFleetDataSource
		NewFleetsDataSource,
		NewTeamsDataSource, // deprecated alias for NewFleetsDataSource
		NewLabelDataSource,
		NewLabelsDataSource,
		NewReportDataSource,
		NewQueryDataSource, // deprecated alias for NewReportDataSource
		NewReportsDataSource,
		NewQueriesDataSource, // deprecated alias for NewReportsDataSource
		NewPolicyDataSource,
		NewPoliciesDataSource,
		NewHostDataSource,
		NewHostsDataSource,
		NewScriptDataSource,
		NewScriptsDataSource,
		NewVersionDataSource,
		NewSoftwareTitleDataSource,
		NewSoftwareTitlesDataSource,
		NewSoftwareVersionDataSource,
		NewSoftwareVersionsDataSource,
		NewConfigurationDataSource,
		NewEnrollSecretsDataSource,
		NewConfigurationProfilesDataSource,
		NewMDMSummaryDataSource,
		NewUserDataSource,
		NewUsersDataSource,
		NewActivitiesDataSource,
		NewABMTokensDataSource,
		NewVPPTokensDataSource,
		NewFleetMaintainedAppDataSource,
		NewFleetMaintainedAppsDataSource,
		NewAppStoreAppsDataSource,
	}
}
