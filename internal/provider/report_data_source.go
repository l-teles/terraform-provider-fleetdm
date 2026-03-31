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
var _ datasource.DataSource = &ReportDataSource{}

// NewReportDataSource creates a new report data source.
func NewReportDataSource() datasource.DataSource {
	return &ReportDataSource{}
}

// ReportDataSource defines the data source implementation.
type ReportDataSource struct {
	client *fleetdm.Client
}

// ReportDataSourceModel describes the data source data model.
type ReportDataSourceModel struct {
	ID                 types.Int64  `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	Query              types.String `tfsdk:"query"`
	Platform           types.List   `tfsdk:"platform"`
	MinOsqueryVersion  types.String `tfsdk:"min_osquery_version"`
	Interval           types.Int64  `tfsdk:"interval"`
	ObserverCanRun     types.Bool   `tfsdk:"observer_can_run"`
	AutomationsEnabled types.Bool   `tfsdk:"automations_enabled"`
	Logging            types.String `tfsdk:"logging"`
	DiscardData        types.Bool   `tfsdk:"discard_data"`
	FleetID            types.Int64  `tfsdk:"fleet_id"`
	AuthorID           types.Int64  `tfsdk:"author_id"`
	AuthorName         types.String `tfsdk:"author_name"`
	AuthorEmail        types.String `tfsdk:"author_email"`
}

func (d *ReportDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_report"
}

func (d *ReportDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about a specific FleetDM report.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The unique identifier of the report.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the report.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A description of the report.",
			},
			"query": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The SQL query.",
			},
			"platform": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of platforms this report is compatible with (darwin, linux, windows, chrome). Empty list means all platforms.",
			},
			"min_osquery_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The minimum osquery version required.",
			},
			"interval": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The scheduled report interval in seconds.",
			},
			"observer_can_run": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether observers can run this report.",
			},
			"automations_enabled": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether automations are enabled.",
			},
			"logging": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The logging type for this report.",
			},
			"discard_data": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether to discard report results after logging.",
			},
			"fleet_id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The ID of the fleet this report belongs to.",
			},
			"author_id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The ID of the user who created the report.",
			},
			"author_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the user who created the report.",
			},
			"author_email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The email of the user who created the report.",
			},
		},
	}
}

func (d *ReportDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *ReportDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ReportDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	query, err := d.client.GetQuery(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read report: %s", err))
		return
	}

	data.ID = types.Int64Value(int64(query.ID))
	data.Name = types.StringValue(query.Name)
	data.Description = types.StringValue(query.Description)
	data.Query = types.StringValue(query.Query)
	data.Platform = platformStringToList(query.Platform)
	data.MinOsqueryVersion = types.StringValue(query.MinOsqueryVersion)
	data.Interval = types.Int64Value(int64(query.Interval))
	data.ObserverCanRun = types.BoolValue(query.ObserverCanRun)
	data.AutomationsEnabled = types.BoolValue(query.AutomationsEnabled)
	data.Logging = types.StringValue(query.Logging)
	data.DiscardData = types.BoolValue(query.DiscardData)
	data.AuthorID = types.Int64Value(int64(query.AuthorID))
	data.AuthorName = types.StringValue(query.AuthorName)
	data.AuthorEmail = types.StringValue(query.AuthorEmail)
	data.FleetID = intPtrToInt64(query.TeamID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
