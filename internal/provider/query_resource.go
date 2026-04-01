package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &QueryResource{}
	_ resource.ResourceWithImportState = &QueryResource{}
)

// NewQueryResource creates a deprecated query resource (use fleetdm_report instead).
func NewQueryResource() resource.Resource {
	return &QueryResource{}
}

// QueryResource defines the resource implementation.
type QueryResource struct {
	client *fleetdm.Client
}

// QueryResourceModel describes the resource data model.
type QueryResourceModel struct {
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

func (r *QueryResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_query"
}

func (r *QueryResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		DeprecationMessage:  "fleetdm_query is deprecated and will be removed in a future version. Use fleetdm_report instead (requires Fleet 4.82.0+).",
		MarkdownDescription: "Manages a FleetDM query. Queries are SQL statements that can be run against hosts to collect system information.",
		Attributes:          querySchemaAttributes(),
	}
}

// querySchemaAttributes returns the schema attributes for the fleetdm_query resource.
// Extracted to allow reuse in fleetdm_report's MoveState, keeping the source schema
// in sync with the query resource definition.
func querySchemaAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "The unique identifier of the query.",
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		},
		"name": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The name of the query. Must be unique.",
		},
		"description": schema.StringAttribute{
			Optional:            true,
			Computed:            true,
			Default:             stringdefault.StaticString(""),
			MarkdownDescription: "A description of the query.",
		},
		"query": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The SQL query to run against hosts.",
		},
		"platform": schema.ListAttribute{
			Optional:            true,
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "List of platforms this query is compatible with (darwin, linux, windows, chrome). Empty list means all platforms.",
		},
		"min_osquery_version": schema.StringAttribute{
			Optional:            true,
			Computed:            true,
			Default:             stringdefault.StaticString(""),
			MarkdownDescription: "The minimum osquery version required to run this query.",
		},
		"interval": schema.Int64Attribute{
			Optional:            true,
			Computed:            true,
			Default:             int64default.StaticInt64(0),
			MarkdownDescription: "The interval in seconds at which to run this query as a scheduled query. 0 means the query is not scheduled.",
		},
		"observer_can_run": schema.BoolAttribute{
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(false),
			MarkdownDescription: "Whether observers can run this query.",
		},
		"automations_enabled": schema.BoolAttribute{
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(false),
			MarkdownDescription: "Whether automations are enabled for this query.",
		},
		"logging": schema.StringAttribute{
			Optional:            true,
			Computed:            true,
			Default:             stringdefault.StaticString("snapshot"),
			MarkdownDescription: "The logging type for this query (snapshot, differential, differential_ignore_removals).",
		},
		"discard_data": schema.BoolAttribute{
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(false),
			MarkdownDescription: "Whether to discard the query results after logging.",
		},
		"team_id": schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "The ID of the team this query belongs to. If not specified, the query is global.",
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
	}
}

func (r *QueryResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

func (r *QueryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data QueryResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := fleetdm.CreateQueryRequest{
		Name:               data.Name.ValueString(),
		Description:        data.Description.ValueString(),
		Query:              data.Query.ValueString(),
		Platform:           platformListToString(ctx, data.Platform),
		MinOsqueryVersion:  data.MinOsqueryVersion.ValueString(),
		Interval:           int(data.Interval.ValueInt64()),
		ObserverCanRun:     data.ObserverCanRun.ValueBool(),
		AutomationsEnabled: data.AutomationsEnabled.ValueBool(),
		Logging:            data.Logging.ValueString(),
		DiscardData:        data.DiscardData.ValueBool(),
	}

	createReq.TeamID = optionalIntPtr(data.TeamID)

	query, err := r.client.CreateQuery(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error Creating FleetDM Query", fmt.Sprintf("Unable to create query: %s", err))
		return
	}

	// Map response to model
	r.mapQueryToModel(query, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *QueryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data QueryResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	query, err := r.client.GetQuery(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading FleetDM Query", fmt.Sprintf("Unable to read query: %s", err))
		return
	}

	// Map response to model
	r.mapQueryToModel(query, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *QueryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data QueryResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := fleetdm.UpdateQueryRequest{
		Name:               data.Name.ValueString(),
		Description:        data.Description.ValueString(),
		Query:              data.Query.ValueString(),
		Platform:           platformListToString(ctx, data.Platform),
		MinOsqueryVersion:  data.MinOsqueryVersion.ValueString(),
		Interval:           int(data.Interval.ValueInt64()),
		ObserverCanRun:     data.ObserverCanRun.ValueBool(),
		AutomationsEnabled: data.AutomationsEnabled.ValueBool(),
		Logging:            data.Logging.ValueString(),
		DiscardData:        data.DiscardData.ValueBool(),
	}

	query, err := r.client.UpdateQuery(ctx, int(data.ID.ValueInt64()), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating FleetDM Query", fmt.Sprintf("Unable to update query: %s", err))
		return
	}

	// Map response to model
	r.mapQueryToModel(query, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *QueryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data QueryResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteQuery(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error Deleting FleetDM Query", fmt.Sprintf("Unable to delete query: %s", err))
		return
	}
}

func (r *QueryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, ok := parseIDFromString(req.ID, "Query", &resp.Diagnostics)
	if !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func (r *QueryResource) mapQueryToModel(query *fleetdm.Query, data *QueryResourceModel) {
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
}
