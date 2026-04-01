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
	_ resource.Resource                = &ReportResource{}
	_ resource.ResourceWithImportState = &ReportResource{}
	_ resource.ResourceWithMoveState   = &ReportResource{}
)

// NewReportResource creates a new report resource.
func NewReportResource() resource.Resource {
	return &ReportResource{}
}

// ReportResource defines the resource implementation.
type ReportResource struct {
	client *fleetdm.Client
}

// ReportResourceModel describes the resource data model.
type ReportResourceModel struct {
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

func (r *ReportResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_report"
}

func (r *ReportResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a FleetDM report. Reports are SQL statements that can be run against hosts to collect system information.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the report.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the report. Must be unique.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "A description of the report.",
			},
			"query": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The SQL query to run against hosts.",
			},
			"platform": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of platforms this report is compatible with (darwin, linux, windows, chrome). Empty list means all platforms.",
			},
			"min_osquery_version": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "The minimum osquery version required to run this report.",
			},
			"interval": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				MarkdownDescription: "The interval in seconds at which to run this report as a scheduled report. 0 means the report is not scheduled.",
			},
			"observer_can_run": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether observers can run this report.",
			},
			"automations_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether automations are enabled for this report.",
			},
			"logging": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("snapshot"),
				MarkdownDescription: "The logging type for this report (snapshot, differential, differential_ignore_removals).",
			},
			"discard_data": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether to discard the report results after logging.",
			},
			"fleet_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "The ID of the fleet this report belongs to. If not specified, the report is global. Changing this value forces a new resource to be created.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
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

func (r *ReportResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

func (r *ReportResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ReportResourceModel

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

	createReq.TeamID = optionalIntPtr(data.FleetID)

	query, err := r.client.CreateQuery(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error Creating FleetDM Report", fmt.Sprintf("Unable to create report: %s", err))
		return
	}

	r.mapQueryToModel(query, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ReportResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ReportResourceModel

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
		resp.Diagnostics.AddError("Error Reading FleetDM Report", fmt.Sprintf("Unable to read report: %s", err))
		return
	}

	r.mapQueryToModel(query, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ReportResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ReportResourceModel

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
		resp.Diagnostics.AddError("Error Updating FleetDM Report", fmt.Sprintf("Unable to update report: %s", err))
		return
	}

	r.mapQueryToModel(query, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ReportResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ReportResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteQuery(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error Deleting FleetDM Report", fmt.Sprintf("Unable to delete report: %s", err))
		return
	}
}

func (r *ReportResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, ok := parseIDFromString(req.ID, "Report", &resp.Diagnostics)
	if !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// MoveState supports moving state from the deprecated fleetdm_query resource to fleetdm_report.
// The only schema difference is that fleetdm_query used "team_id" while fleetdm_report uses "fleet_id".
func (r *ReportResource) MoveState(ctx context.Context) []resource.StateMover {
	return []resource.StateMover{
		{
			SourceSchema: &schema.Schema{Attributes: querySchemaAttributes()},
			StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
				var src QueryResourceModel
				resp.Diagnostics.Append(req.SourceState.Get(ctx, &src)...)
				if resp.Diagnostics.HasError() {
					return
				}
				target := ReportResourceModel{
					ID:                 src.ID,
					Name:               src.Name,
					Description:        src.Description,
					Query:              src.Query,
					Platform:           src.Platform,
					MinOsqueryVersion:  src.MinOsqueryVersion,
					Interval:           src.Interval,
					ObserverCanRun:     src.ObserverCanRun,
					AutomationsEnabled: src.AutomationsEnabled,
					Logging:            src.Logging,
					DiscardData:        src.DiscardData,
					FleetID:            src.TeamID,
					AuthorID:           src.AuthorID,
					AuthorName:         src.AuthorName,
					AuthorEmail:        src.AuthorEmail,
				}
				resp.Diagnostics.Append(resp.TargetState.Set(ctx, &target)...)
			},
		},
	}
}

func (r *ReportResource) mapQueryToModel(query *fleetdm.Query, data *ReportResourceModel) {
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
}
