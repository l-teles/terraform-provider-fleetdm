package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &restAPIEndpointsDataSource{}
	_ datasource.DataSourceWithConfigure = &restAPIEndpointsDataSource{}
)

// NewRESTAPIEndpointsDataSource creates a new REST API endpoints data source.
func NewRESTAPIEndpointsDataSource() datasource.DataSource {
	return &restAPIEndpointsDataSource{}
}

// restAPIEndpointsDataSource is the data source implementation.
type restAPIEndpointsDataSource struct {
	client *fleetdm.Client
}

// restAPIEndpointsDataSourceModel maps the data source schema data.
type restAPIEndpointsDataSourceModel struct {
	APIEndpoints []restAPIEndpointModel `tfsdk:"api_endpoints"`
}

// restAPIEndpointModel maps an individual catalog entry.
type restAPIEndpointModel struct {
	Method      types.String `tfsdk:"method"`
	Path        types.String `tfsdk:"path"`
	DisplayName types.String `tfsdk:"display_name"`
	Deprecated  types.Bool   `tfsdk:"deprecated"`
}

// Metadata returns the data source type name.
func (d *restAPIEndpointsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rest_api_endpoints"
}

// Schema defines the schema for the data source.
func (d *restAPIEndpointsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves FleetDM's REST API endpoint catalog. This is a Premium feature.",
		MarkdownDescription: `Retrieves FleetDM's REST API endpoint catalog.

The catalog is the authoritative list of endpoints that the ` + "`api_endpoints`" + ` attribute of a ` + "`fleetdm_user`" + ` resource may reference — Fleet validates every method/path pair against it and rejects unknown pairs. Use this data source to discover the exact ` + "`path`" + ` templates to grant.

This is a Fleet Premium feature and requires Fleet 4.90 or later.

## Example Usage

` + "```hcl" + `
data "fleetdm_rest_api_endpoints" "all" {}

# Every non-deprecated read-only endpoint.
locals {
  read_only_endpoints = [
    for e in data.fleetdm_rest_api_endpoints.all.api_endpoints :
    { method = e.method, path = e.path }
    if e.method == "GET" && !e.deprecated
  ]
}

resource "fleetdm_user" "read_only_bot" {
  name        = "Read-only bot"
  email       = "read-only-bot@example.com"
  password    = var.bot_password
  global_role = "observer"
  api_only    = true

  api_endpoints = local.read_only_endpoints
}
` + "```",
		Attributes: map[string]schema.Attribute{
			"api_endpoints": schema.ListNestedAttribute{
				Description:         "The API endpoints Fleet exposes.",
				MarkdownDescription: "The API endpoints Fleet exposes.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"method": schema.StringAttribute{
							Description:         "The HTTP method of the endpoint, upper-cased (GET, POST, PUT, PATCH or DELETE).",
							MarkdownDescription: "The HTTP method of the endpoint, upper-cased (`GET`, `POST`, `PUT`, `PATCH` or `DELETE`).",
							Computed:            true,
						},
						"path": schema.StringAttribute{
							Description:         "The route template of the endpoint. Path parameters appear as :name placeholders, for example /api/v1/fleet/hosts/:id.",
							MarkdownDescription: "The route template of the endpoint. Path parameters appear as `:name` placeholders, for example `/api/v1/fleet/hosts/:id`.",
							Computed:            true,
						},
						"display_name": schema.StringAttribute{
							Description:         "The human-readable name of the endpoint, as shown in the Fleet UI.",
							MarkdownDescription: "The human-readable name of the endpoint, as shown in the Fleet UI.",
							Computed:            true,
						},
						"deprecated": schema.BoolAttribute{
							Description:         "Whether Fleet has deprecated the endpoint. Deprecated endpoints can still be granted, but should be avoided in new configurations.",
							MarkdownDescription: "Whether Fleet has deprecated the endpoint. Deprecated endpoints can still be granted, but should be avoided in new configurations.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *restAPIEndpointsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (d *restAPIEndpointsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state restAPIEndpointsDataSourceModel

	endpoints, err := d.client.ListAPIEndpoints(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FleetDM REST API Endpoints",
			err.Error(),
		)
		return
	}

	state.APIEndpoints = make([]restAPIEndpointModel, 0, len(endpoints))
	for _, e := range endpoints {
		state.APIEndpoints = append(state.APIEndpoints, restAPIEndpointModel{
			Method:      types.StringValue(e.Method),
			Path:        types.StringValue(e.Path),
			DisplayName: types.StringValue(e.DisplayName),
			Deprecated:  types.BoolValue(e.Deprecated),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
