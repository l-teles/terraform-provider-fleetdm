package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ datasource.DataSource              = &SoftwareSelfServiceCategoriesDataSource{}
	_ datasource.DataSourceWithConfigure = &SoftwareSelfServiceCategoriesDataSource{}
)

// NewSoftwareSelfServiceCategoriesDataSource creates a new self-service categories data source.
func NewSoftwareSelfServiceCategoriesDataSource() datasource.DataSource {
	return &SoftwareSelfServiceCategoriesDataSource{}
}

// SoftwareSelfServiceCategoriesDataSource defines the data source implementation.
type SoftwareSelfServiceCategoriesDataSource struct {
	client *fleetdm.Client
}

// SoftwareSelfServiceCategoriesDataSourceModel describes the data source data model.
type SoftwareSelfServiceCategoriesDataSourceModel struct {
	FleetID    types.Int64                            `tfsdk:"fleet_id"`
	Categories []SoftwareSelfServiceCategoryListModel `tfsdk:"categories"`
}

// SoftwareSelfServiceCategoryListModel describes a single self-service category
// in the list.
type SoftwareSelfServiceCategoryListModel struct {
	ID   types.Int64  `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func (d *SoftwareSelfServiceCategoriesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_self_service_categories"
}

func (d *SoftwareSelfServiceCategoriesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves the self-service software categories on a fleet, including the defaults Fleet seeds on fleet creation. Requires Fleet Premium and Fleet >= 4.90.",

		Attributes: map[string]schema.Attribute{
			"fleet_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The ID of the fleet (team) to list categories for. Use `0` for hosts that are not assigned to a fleet. Fleet rejects the request without it.",
			},
			"categories": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The self-service categories on the fleet.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The unique identifier of the category.",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The category name.",
						},
					},
				},
			},
		},
	}
}

func (d *SoftwareSelfServiceCategoriesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *SoftwareSelfServiceCategoriesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SoftwareSelfServiceCategoriesDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	categories, err := d.client.ListSelfServiceCategories(ctx, data.FleetID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FleetDM Self-Service Categories",
			err.Error(),
		)
		return
	}

	data.Categories = make([]SoftwareSelfServiceCategoryListModel, len(categories))
	for i, category := range categories {
		data.Categories[i] = SoftwareSelfServiceCategoryListModel{
			ID:   types.Int64Value(category.ID),
			Name: types.StringValue(category.Name),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
