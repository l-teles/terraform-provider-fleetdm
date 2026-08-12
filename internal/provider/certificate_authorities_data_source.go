package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &certificateAuthoritiesDataSource{}
	_ datasource.DataSourceWithConfigure = &certificateAuthoritiesDataSource{}
)

// NewCertificateAuthoritiesDataSource is a helper function to simplify the
// provider implementation.
func NewCertificateAuthoritiesDataSource() datasource.DataSource {
	return &certificateAuthoritiesDataSource{}
}

// certificateAuthoritiesDataSource is the data source implementation.
type certificateAuthoritiesDataSource struct {
	client *fleetdm.Client
}

// certificateAuthoritiesDataSourceModel maps the data source schema data.
type certificateAuthoritiesDataSourceModel struct {
	CertificateAuthorities []certificateAuthoritySummaryModel `tfsdk:"certificate_authorities"`
}

// certificateAuthoritySummaryModel maps an individual certificate authority.
type certificateAuthoritySummaryModel struct {
	ID   types.Int64  `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Type types.String `tfsdk:"type"`
}

// Metadata returns the data source type name.
func (d *certificateAuthoritiesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate_authorities"
}

// Schema defines the schema for the data source.
func (d *certificateAuthoritiesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves every certificate authority configured in FleetDM. This is a Fleet Premium feature.\n\n" +
			"~> **Note:** Fleet's list endpoint returns identity only, so this data source exposes `id`, `name` and " +
			"`type` and no configuration. Fleet never returns certificate authority secrets through any endpoint.",
		Attributes: map[string]schema.Attribute{
			"certificate_authorities": schema.ListNestedAttribute{
				MarkdownDescription: "List of certificate authorities.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							MarkdownDescription: "Identifier of the certificate authority.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Name of the certificate authority.",
							Computed:            true,
						},
						"type": schema.StringAttribute{
							MarkdownDescription: "Type of the certificate authority. One of `digicert`, `ndes_scep_proxy`, " +
								"`custom_scep_proxy`, `custom_est_proxy`, `hydrant`, `smallstep`.",
							Computed: true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *certificateAuthoritiesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (d *certificateAuthoritiesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state certificateAuthoritiesDataSourceModel

	cas, err := d.client.ListCertificateAuthorities(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FleetDM Certificate Authorities",
			fmt.Sprintf("Could not list certificate authorities: %s", err.Error()),
		)
		return
	}

	state.CertificateAuthorities = make([]certificateAuthoritySummaryModel, 0, len(cas))
	for _, ca := range cas {
		state.CertificateAuthorities = append(state.CertificateAuthorities, certificateAuthoritySummaryModel{
			ID:   types.Int64Value(int64(ca.ID)),
			Name: types.StringValue(ca.Name),
			Type: types.StringValue(ca.Type),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
