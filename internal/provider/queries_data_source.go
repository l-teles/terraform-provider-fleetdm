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
var _ datasource.DataSource = &QueriesDataSource{}

// NewQueriesDataSource creates a new queries data source.
func NewQueriesDataSource() datasource.DataSource {
	return &QueriesDataSource{}
}

// QueriesDataSource defines the data source implementation.
type QueriesDataSource struct {
	client *fleetdm.Client
}

// QueriesDataSourceModel describes the data source data model.
type QueriesDataSourceModel struct {
	TeamID  types.Int64  `tfsdk:"team_id"`
	Queries []QueryModel `tfsdk:"queries"`
}

// QueryModel describes a single query in the list.
type QueryModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	Query              types.String `tfsdk:"query"`
	Platform           types.String `tfsdk:"platform"`
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

func (d *QueriesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_queries"
}

func (d *QueriesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about all FleetDM queries.",

		Attributes: map[string]schema.Attribute{
			"team_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Filter queries by team ID. If not specified, all global queries are returned.",
			},
			"queries": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of queries.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
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
						"platform": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Comma-separated platforms this query is compatible with.",
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
				},
			},
		},
	}
}

func (d *QueriesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *QueriesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data QueriesDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var queries []fleetdm.Query
	var err error

	if !data.TeamID.IsNull() && !data.TeamID.IsUnknown() {
		queries, err = d.client.ListQueriesByTeam(ctx, int(data.TeamID.ValueInt64()))
	} else {
		queries, err = d.client.ListQueries(ctx)
	}

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list queries: %s", err))
		return
	}

	// Map response to model
	data.Queries = make([]QueryModel, len(queries))
	for i, query := range queries {
		data.Queries[i] = QueryModel{
			ID:                 types.StringValue(strconv.Itoa(query.ID)),
			Name:               types.StringValue(query.Name),
			Description:        types.StringValue(query.Description),
			Query:              types.StringValue(query.Query),
			Platform:           types.StringValue(query.Platform),
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

		data.Queries[i].TeamID = intPtrToInt64(query.TeamID)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
