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
var _ datasource.DataSource = &LabelDataSource{}

// NewLabelDataSource creates a new label data source.
func NewLabelDataSource() datasource.DataSource {
	return &LabelDataSource{}
}

// LabelDataSource defines the data source implementation.
type LabelDataSource struct {
	client *fleetdm.Client
}

// LabelDataSourceModel describes the data source data model.
type LabelDataSourceModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Query       types.String `tfsdk:"query"`
	Platform    types.String `tfsdk:"platform"`
	LabelType   types.String `tfsdk:"label_type"`
	HostCount   types.Int64  `tfsdk:"host_count"`
}

func (d *LabelDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_label"
}

func (d *LabelDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about a specific FleetDM label.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Required:            true,
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
				MarkdownDescription: "The platform the label is restricted to (darwin, windows, linux, chrome).",
			},
			"label_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The type of the label (regular or builtin).",
			},
			"host_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The number of hosts that belong to this label.",
			},
		},
	}
}

func (d *LabelDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *LabelDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data LabelDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	label, err := d.client.GetLabel(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read label: %s", err))
		return
	}

	// Map response to model
	data.ID = types.Int64Value(int64(label.ID))
	data.Name = types.StringValue(label.Name)
	data.Description = types.StringValue(label.Description)
	data.Query = types.StringValue(label.Query)
	data.Platform = types.StringValue(label.Platform)
	data.LabelType = types.StringValue(label.LabelType)
	data.HostCount = types.Int64Value(int64(label.HostCount))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
