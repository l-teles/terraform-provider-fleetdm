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
var _ datasource.DataSource = &CustomHostVitalsDataSource{}

// NewCustomHostVitalsDataSource creates a new custom host vitals data source.
func NewCustomHostVitalsDataSource() datasource.DataSource {
	return &CustomHostVitalsDataSource{}
}

// CustomHostVitalsDataSource defines the data source implementation.
type CustomHostVitalsDataSource struct {
	client *fleetdm.Client
}

// CustomHostVitalsDataSourceModel describes the data source data model.
type CustomHostVitalsDataSourceModel struct {
	CustomHostVitals []CustomHostVitalModel `tfsdk:"custom_host_vitals"`
}

// CustomHostVitalModel describes a single custom host vital in the list.
type CustomHostVitalModel struct {
	ID        types.Int64  `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

func (d *CustomHostVitalsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_host_vitals"
}

func (d *CustomHostVitalsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve all FleetDM custom host vitals, including any created outside " +
			"Terraform. Useful for resolving a vital's `id` — needed to interpolate `$FLEET_HOST_VITAL_<id>` into a " +
			"configuration profile or to point a label's `criteria` at it — when the vital is not managed here.\n\n" +
			"Results come back ordered by name.",

		Attributes: map[string]schema.Attribute{
			"custom_host_vitals": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of all custom host vitals, ordered by name.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The unique identifier of the custom host vital.",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The name of the custom host vital.",
						},
						"created_at": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Timestamp when the custom host vital was created.",
						},
						"updated_at": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Timestamp when the custom host vital was last updated.",
						},
					},
				},
			},
		},
	}
}

func (d *CustomHostVitalsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *CustomHostVitalsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CustomHostVitalsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vitals, err := d.client.ListCustomHostVitals(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list custom host vitals: %s", err))
		return
	}

	data.CustomHostVitals = make([]CustomHostVitalModel, len(vitals))
	for i, v := range vitals {
		data.CustomHostVitals[i] = CustomHostVitalModel{
			ID:        types.Int64Value(int64(v.ID)),
			Name:      types.StringValue(v.Name),
			CreatedAt: types.StringValue(v.CreatedAt),
			UpdatedAt: types.StringValue(v.UpdatedAt),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
