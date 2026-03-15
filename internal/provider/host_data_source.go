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
var _ datasource.DataSource = &HostDataSource{}

// NewHostDataSource creates a new host data source.
func NewHostDataSource() datasource.DataSource {
	return &HostDataSource{}
}

// HostDataSource defines the data source implementation.
type HostDataSource struct {
	client *fleetdm.Client
}

// HostDataSourceModel describes the data source data model.
type HostDataSourceModel struct {
	ID         types.Int64  `tfsdk:"id"`
	Identifier types.String `tfsdk:"identifier"`
	hostDetailFields
}

// Metadata returns the data source type name.
func (d *HostDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_host"
}

// Schema defines the schema for the data source.
func (d *HostDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := hostComputedAttributes()
	attrs["id"] = schema.Int64Attribute{
		MarkdownDescription: "Host ID. Exactly one of `id` or `identifier` must be specified.",
		Optional:            true,
		Computed:            true,
	}
	attrs["identifier"] = schema.StringAttribute{
		MarkdownDescription: "Host identifier — can be hostname, UUID, or hardware serial number. Exactly one of `id` or `identifier` must be specified.",
		Optional:            true,
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to get information about a FleetDM host. " +
			"Look up by numeric `id` or by `identifier` (hostname, UUID, or serial number).",
		Attributes: attrs,
	}
}

// Configure adds the provider configured client to the data source.
func (d *HostDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read reads the data source.
func (d *HostDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data HostDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !data.ID.IsNull() && !data.ID.IsUnknown()
	hasIdentifier := !data.Identifier.IsNull() && !data.Identifier.IsUnknown()

	if hasID == hasIdentifier {
		resp.Diagnostics.AddError(
			"Invalid Host Lookup",
			"Exactly one of \"id\" or \"identifier\" must be specified.",
		)
		return
	}

	var host *fleetdm.Host
	var err error

	if hasID {
		host, err = d.client.GetHost(ctx, int(data.ID.ValueInt64()))
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to Read Host",
				fmt.Sprintf("Unable to read host with ID %d: %s", data.ID.ValueInt64(), err),
			)
			return
		}
	} else {
		host, err = d.client.GetHostByIdentifier(ctx, data.Identifier.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to Read Host",
				fmt.Sprintf("Unable to read host with identifier %q: %s", data.Identifier.ValueString(), err),
			)
			return
		}
	}

	data.ID = types.Int64Value(int64(host.ID))
	data.hostDetailFields = mapHostToDetailFields(host)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
