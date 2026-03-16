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
var _ datasource.DataSource = &QueryDataSource{}

// NewQueryDataSource creates a new query data source.
func NewQueryDataSource() datasource.DataSource {
	return &QueryDataSource{}
}

// QueryDataSource defines the data source implementation.
type QueryDataSource struct {
	client *fleetdm.Client
}

// QueryDataSourceModel describes the data source data model.
type QueryDataSourceModel struct {
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
	TeamID             types.Int64  `tfsdk:"team_id"`
	AuthorID           types.Int64  `tfsdk:"author_id"`
	AuthorName         types.String `tfsdk:"author_name"`
	AuthorEmail        types.String `tfsdk:"author_email"`
}

func (d *QueryDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_query"
}

func (d *QueryDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about a specific FleetDM query.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The unique identifier of the query.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the query.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A description of the query.",
			},
			"query": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The SQL query.",
			},
			"platform": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of platforms this query is compatible with (darwin, linux, windows, chrome). Empty list means all platforms.",
			},
			"min_osquery_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The minimum osquery version required.",
			},
			"interval": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The scheduled query interval in seconds.",
			},
			"observer_can_run": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether observers can run this query.",
			},
			"automations_enabled": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether automations are enabled.",
			},
			"logging": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The logging type for this query.",
			},
			"discard_data": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether to discard query results after logging.",
			},
			"team_id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The ID of the team this query belongs to.",
			},
			"author_id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The ID of the user who created the query.",
			},
			"author_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the user who created the query.",
			},
			"author_email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The email of the user who created the query.",
			},
		},
	}
}

func (d *QueryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *QueryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data QueryDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	query, err := d.client.GetQuery(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read query: %s", err))
		return
	}

	// Map response to model
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

	data.TeamID = intPtrToInt64(query.TeamID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
