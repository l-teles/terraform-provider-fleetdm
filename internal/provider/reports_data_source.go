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
var _ datasource.DataSource = &ReportsDataSource{}

// NewReportsDataSource creates a new reports data source.
func NewReportsDataSource() datasource.DataSource {
	return &ReportsDataSource{}
}

// ReportsDataSource defines the data source implementation.
type ReportsDataSource struct {
	client *fleetdm.Client
}

// ReportsDataSourceModel describes the data source data model.
type ReportsDataSourceModel struct {
	FleetID types.Int64   `tfsdk:"fleet_id"`
	Reports []ReportModel `tfsdk:"reports"`
}

// ReportModel describes a single report in the list.
type ReportModel struct {
	ID                 types.String `tfsdk:"id"`
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

func (d *ReportsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_reports"
}

func (d *ReportsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about all FleetDM reports.",

		Attributes: map[string]schema.Attribute{
			"fleet_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Filter reports by fleet ID. If not specified, all global reports are returned.",
			},
			"reports": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of reports.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
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
				},
			},
		},
	}
}

func (d *ReportsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *ReportsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ReportsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var queries []fleetdm.Query
	var err error

	if !data.FleetID.IsNull() && !data.FleetID.IsUnknown() {
		queries, err = d.client.ListQueriesByTeam(ctx, int(data.FleetID.ValueInt64()))
	} else {
		queries, err = d.client.ListQueries(ctx)
	}

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list reports: %s", err))
		return
	}

	data.Reports = make([]ReportModel, len(queries))
	for i, query := range queries {
		data.Reports[i] = ReportModel{
			ID:                 types.StringValue(strconv.Itoa(query.ID)),
			Name:               types.StringValue(query.Name),
			Description:        types.StringValue(query.Description),
			Query:              types.StringValue(query.Query),
			Platform:           platformStringToList(query.Platform),
			MinOsqueryVersion:  types.StringValue(query.MinOsqueryVersion),
			Interval:           types.Int64Value(int64(query.Interval)),
			ObserverCanRun:     types.BoolValue(query.ObserverCanRun),
			AutomationsEnabled: types.BoolValue(query.AutomationsEnabled),
			Logging:            types.StringValue(query.Logging),
			DiscardData:        types.BoolValue(query.DiscardData),
			AuthorID:           types.Int64Value(int64(query.AuthorID)),
			AuthorName:         types.StringValue(query.AuthorName),
			AuthorEmail:        types.StringValue(query.AuthorEmail),
		}
		data.Reports[i].FleetID = intPtrToInt64(query.TeamID)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
