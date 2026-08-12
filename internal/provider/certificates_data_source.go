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
	_ datasource.DataSource              = &certificatesDataSource{}
	_ datasource.DataSourceWithConfigure = &certificatesDataSource{}
)

// NewCertificatesDataSource is a helper function to simplify the provider
// implementation.
func NewCertificatesDataSource() datasource.DataSource {
	return &certificatesDataSource{}
}

// certificatesDataSource is the data source implementation.
type certificatesDataSource struct {
	client *fleetdm.Client
}

// certificatesDataSourceModel maps the data source schema data.
type certificatesDataSourceModel struct {
	FleetID      types.Int64               `tfsdk:"fleet_id"`
	Certificates []certificateSummaryModel `tfsdk:"certificates"`
}

// certificateSummaryModel maps an individual certificate template.
type certificateSummaryModel struct {
	ID                       types.Int64  `tfsdk:"id"`
	Name                     types.String `tfsdk:"name"`
	SubjectName              types.String `tfsdk:"subject_name"`
	SubjectAlternativeName   types.String `tfsdk:"subject_alternative_name"`
	CertificateAuthorityID   types.Int64  `tfsdk:"certificate_authority_id"`
	CertificateAuthorityName types.String `tfsdk:"certificate_authority_name"`
	CreatedAt                types.String `tfsdk:"created_at"`
}

// Metadata returns the data source type name.
func (d *certificatesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificates"
}

// Schema defines the schema for the data source.
func (d *certificatesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves the certificate templates on one fleet. This is a Fleet Premium feature and requires Fleet >= 4.90.\n\n" +
			"~> Fleet's list endpoint is scoped to a single fleet and has no \"all fleets\" mode, so this data source " +
			"reads one fleet at a time. Omitting `fleet_id` reads the templates that target hosts not assigned to a " +
			"fleet, not every template on the server.\n\n" +
			"~> The list endpoint does not report `certificate_authority_type`. Read an individual template with " +
			"`fleetdm_certificate` if you need it.",
		Attributes: map[string]schema.Attribute{
			"fleet_id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the fleet (team) whose certificate templates to read. Defaults to `0`, which selects the templates targeting hosts that are not assigned to a fleet.",
				Optional:            true,
			},
			"certificates": schema.ListNestedAttribute{
				MarkdownDescription: "List of certificate templates on the fleet, ordered by `id` ascending.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							MarkdownDescription: "Identifier of the certificate template.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Name of the certificate template.",
							Computed:            true,
						},
						"subject_name": schema.StringAttribute{
							MarkdownDescription: "The certificate subject, as an X.500 distinguished name. Any `$FLEET_VAR_*` reference is returned unexpanded.",
							Computed:            true,
						},
						"subject_alternative_name": schema.StringAttribute{
							MarkdownDescription: "The certificate subject alternative names, as a comma-separated list of `KEY=value` entries. Null when the template has none.",
							Computed:            true,
						},
						"certificate_authority_id": schema.Int64Attribute{
							MarkdownDescription: "Identifier of the certificate authority that issues certificates for this template.",
							Computed:            true,
						},
						"certificate_authority_name": schema.StringAttribute{
							MarkdownDescription: "Name of the certificate authority that issues certificates for this template.",
							Computed:            true,
						},
						"created_at": schema.StringAttribute{
							MarkdownDescription: "Timestamp when the certificate template was created.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *certificatesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

// Read refreshes the Terraform state with the latest data.
func (d *certificatesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state certificatesDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A null fleet_id means fleet 0, matching the resource's default.
	fleetID := state.FleetID.ValueInt64()

	templates, err := d.client.ListCertificateTemplates(ctx, fleetID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FleetDM Certificate Templates",
			fmt.Sprintf("Could not list certificate templates for fleet %d: %s", fleetID, err.Error()),
		)
		return
	}

	state.Certificates = make([]certificateSummaryModel, 0, len(templates))
	for _, template := range templates {
		// Absent API strings map to null rather than "", matching the
		// resource's mapTemplateToModel. The list route always reports
		// certificate_authority_name and created_at on Fleet 4.90, so the
		// null case for those two is defensive only.
		state.Certificates = append(state.Certificates, certificateSummaryModel{
			ID:                       types.Int64Value(template.ID),
			Name:                     types.StringValue(template.Name),
			SubjectName:              types.StringValue(template.SubjectName),
			SubjectAlternativeName:   emptyStringToNull(template.SubjectAlternativeName),
			CertificateAuthorityID:   types.Int64Value(template.CertificateAuthorityID),
			CertificateAuthorityName: emptyStringToNull(template.CertificateAuthorityName),
			CreatedAt:                emptyStringToNull(template.CreatedAt),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
