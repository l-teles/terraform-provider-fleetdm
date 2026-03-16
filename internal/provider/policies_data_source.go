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
var _ datasource.DataSource = &PoliciesDataSource{}

// NewPoliciesDataSource creates a new policies data source.
func NewPoliciesDataSource() datasource.DataSource {
	return &PoliciesDataSource{}
}

// PoliciesDataSource defines the data source implementation.
type PoliciesDataSource struct {
	client *fleetdm.Client
}

// PoliciesDataSourceModel describes the data source data model.
type PoliciesDataSourceModel struct {
	TeamID   types.Int64   `tfsdk:"team_id"`
	Policies []PolicyModel `tfsdk:"policies"`
}

// PolicyModel describes a single policy in the list.
type PolicyModel struct {
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	Description           types.String `tfsdk:"description"`
	Query                 types.String `tfsdk:"query"`
	Critical              types.Bool   `tfsdk:"critical"`
	Resolution            types.String `tfsdk:"resolution"`
	Platform              types.List   `tfsdk:"platform"`
	TeamID                types.Int64  `tfsdk:"team_id"`
	AuthorID              types.Int64  `tfsdk:"author_id"`
	AuthorName            types.String `tfsdk:"author_name"`
	AuthorEmail           types.String `tfsdk:"author_email"`
	PassingHostCount      types.Int64  `tfsdk:"passing_host_count"`
	FailingHostCount      types.Int64  `tfsdk:"failing_host_count"`
	CalendarEventsEnabled types.Bool   `tfsdk:"calendar_events_enabled"`
	CreatedAt             types.String `tfsdk:"created_at"`
	UpdatedAt             types.String `tfsdk:"updated_at"`
}

func (d *PoliciesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_policies"
}

func (d *PoliciesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about all FleetDM policies.",

		Attributes: map[string]schema.Attribute{
			"team_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Filter policies by team ID. If not specified, global policies are returned.",
			},
			"policies": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of policies.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The unique identifier of the policy.",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The name of the policy.",
						},
						"description": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "A description of the policy.",
						},
						"query": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The SQL query that defines the policy.",
						},
						"critical": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the policy is critical.",
						},
						"resolution": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Instructions for resolving a failing policy.",
						},
						"platform": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "List of platforms this policy applies to (darwin, linux, windows, chrome). Empty list means all platforms.",
						},
						"team_id": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The ID of the team this policy belongs to.",
						},
						"author_id": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The ID of the user who created the policy.",
						},
						"author_name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The name of the user who created the policy.",
						},
						"author_email": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The email of the user who created the policy.",
						},
						"passing_host_count": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The number of hosts passing this policy.",
						},
						"failing_host_count": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The number of hosts failing this policy.",
						},
						"calendar_events_enabled": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether calendar events are enabled for this policy.",
						},
						"created_at": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The timestamp when the policy was created.",
						},
						"updated_at": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The timestamp when the policy was last updated.",
						},
					},
				},
			},
		},
	}
}

func (d *PoliciesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *PoliciesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data PoliciesDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := optionalIntPtr(data.TeamID)

	policies, err := d.client.ListPolicies(ctx, teamID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list policies: %s", err))
		return
	}

	// Map response to model
	data.Policies = make([]PolicyModel, len(policies))
	for i, policy := range policies {
		data.Policies[i] = PolicyModel{
			ID:                    types.StringValue(strconv.Itoa(policy.ID)),
			Name:                  types.StringValue(policy.Name),
			Description:           types.StringValue(policy.Description),
			Query:                 types.StringValue(policy.Query),
			Critical:              types.BoolValue(policy.Critical),
			Resolution:            types.StringValue(policy.Resolution),
			Platform:              platformStringToList(policy.Platform),
			AuthorID:              types.Int64Value(int64(policy.AuthorID)),
			AuthorName:            types.StringValue(policy.AuthorName),
			AuthorEmail:           types.StringValue(policy.AuthorEmail),
			PassingHostCount:      types.Int64Value(int64(policy.PassingHostCount)),
			FailingHostCount:      types.Int64Value(int64(policy.FailingHostCount)),
			CalendarEventsEnabled: types.BoolValue(policy.CalendarEventsEnabled),
			CreatedAt:             types.StringValue(policy.CreatedAt),
			UpdatedAt:             types.StringValue(policy.UpdatedAt),
		}

		data.Policies[i].TeamID = intPtrToInt64(policy.TeamID)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
