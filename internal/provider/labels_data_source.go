package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &LabelsDataSource{}

// NewLabelsDataSource creates a new labels data source.
func NewLabelsDataSource() datasource.DataSource {
	return &LabelsDataSource{}
}

// LabelsDataSource defines the data source implementation.
type LabelsDataSource struct {
	client *fleetdm.Client
}

// LabelsDataSourceModel describes the data source data model.
type LabelsDataSourceModel struct {
	Labels []LabelModel `tfsdk:"labels"`
}

// LabelModel describes a single label in the list.
type LabelModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Query       types.String `tfsdk:"query"`
	Platform    types.String `tfsdk:"platform"`
	LabelType   types.String `tfsdk:"label_type"`
	HostCount   types.Int64  `tfsdk:"host_count"`
}

func (d *LabelsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_labels"
}

func (d *LabelsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about all FleetDM labels.",

		Attributes: map[string]schema.Attribute{
			"labels": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of all labels.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The unique identifier of the label.",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The name of the label.",
						},
						"description": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "A description of the label.",
						},
						"query": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The SQL query that defines which hosts belong to this label.",
						},
						"platform": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The platform the label is restricted to.",
						},
						"label_type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The type of the label.",
						},
						"host_count": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The number of hosts that belong to this label.",
						},
					},
				},
			},
		},
	}
}

func (d *LabelsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *LabelsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data LabelsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	labels, err := d.client.ListLabels(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list labels: %s", err))
		return
	}

	// Map response to model
	data.Labels = make([]LabelModel, len(labels))
	for i, label := range labels {
		data.Labels[i] = LabelModel{
			ID:          types.StringValue(strconv.Itoa(label.ID)),
			Name:        types.StringValue(label.Name),
			Description: types.StringValue(label.Description),
			Query:       types.StringValue(label.Query),
			Platform:    types.StringValue(label.Platform),
			LabelType:   types.StringValue(label.LabelType),
			HostCount:   types.Int64Value(int64(label.HostCount)),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
