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
var _ datasource.DataSource = &PolicyDataSource{}

// NewPolicyDataSource creates a new policy data source.
func NewPolicyDataSource() datasource.DataSource {
	return &PolicyDataSource{}
}

// PolicyDataSource defines the data source implementation.
type PolicyDataSource struct {
	client *fleetdm.Client
}

// PolicyDataSourceModel describes the data source data model.
type PolicyDataSourceModel struct {
	ID               types.Int64  `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	Query            types.String `tfsdk:"query"`
	Critical         types.Bool   `tfsdk:"critical"`
	Resolution       types.String `tfsdk:"resolution"`
	Platform         types.String `tfsdk:"platform"`
	TeamID           types.Int64  `tfsdk:"team_id"`
	AuthorID         types.Int64  `tfsdk:"author_id"`
	AuthorName       types.String `tfsdk:"author_name"`
	AuthorEmail      types.String `tfsdk:"author_email"`
	PassingHostCount types.Int64  `tfsdk:"passing_host_count"`
	FailingHostCount types.Int64  `tfsdk:"failing_host_count"`
}

func (d *PolicyDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_policy"
}

func (d *PolicyDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about a specific FleetDM policy.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The unique identifier of the policy.",
			},
			"team_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "The ID of the team this policy belongs to. Required for team policies.",
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
			"platform": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Comma-separated platforms this policy applies to.",
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
		},
	}
}

func (d *PolicyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *PolicyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data PolicyDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policy, err := d.client.GetPolicy(ctx, int(data.ID.ValueInt64()), optionalIntPtr(data.TeamID))
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read policy: %s", err))
		return
	}

	// Map response to model
	data.ID = types.Int64Value(int64(policy.ID))
	data.Name = types.StringValue(policy.Name)
	data.Description = types.StringValue(policy.Description)
	data.Query = types.StringValue(policy.Query)
	data.Critical = types.BoolValue(policy.Critical)
	data.Resolution = types.StringValue(policy.Resolution)
	data.Platform = types.StringValue(policy.Platform)
	data.AuthorID = types.Int64Value(int64(policy.AuthorID))
	data.AuthorName = types.StringValue(policy.AuthorName)
	data.AuthorEmail = types.StringValue(policy.AuthorEmail)
	data.PassingHostCount = types.Int64Value(int64(policy.PassingHostCount))
	data.FailingHostCount = types.Int64Value(int64(policy.FailingHostCount))

	data.TeamID = intPtrToInt64(policy.TeamID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
