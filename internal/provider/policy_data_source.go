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
	ID                       types.Int64  `tfsdk:"id"`
	Name                     types.String `tfsdk:"name"`
	Description              types.String `tfsdk:"description"`
	Query                    types.String `tfsdk:"query"`
	Critical                 types.Bool   `tfsdk:"critical"`
	Resolution               types.String `tfsdk:"resolution"`
	Platform                 types.List   `tfsdk:"platform"`
	TeamID                   types.Int64  `tfsdk:"team_id"`
	Type                     types.String `tfsdk:"type"`
	PatchSoftwareTitleID     types.Int64  `tfsdk:"patch_software_title_id"`
	SoftwareTitleID          types.Int64  `tfsdk:"software_title_id"`
	ScriptID                 types.Int64  `tfsdk:"script_id"`
	LabelsIncludeAny         types.Set    `tfsdk:"labels_include_any"`
	LabelsExcludeAny         types.Set    `tfsdk:"labels_exclude_any"`
	CalendarEventsEnabled    types.Bool   `tfsdk:"calendar_events_enabled"`
	ConditionalAccessEnabled types.Bool   `tfsdk:"conditional_access_enabled"`
	AuthorID                 types.Int64  `tfsdk:"author_id"`
	AuthorName               types.String `tfsdk:"author_name"`
	AuthorEmail              types.String `tfsdk:"author_email"`
	PassingHostCount         types.Int64  `tfsdk:"passing_host_count"`
	FailingHostCount         types.Int64  `tfsdk:"failing_host_count"`
	FleetMaintained          types.Bool   `tfsdk:"fleet_maintained"`
	CreatedAt                types.String `tfsdk:"created_at"`
	UpdatedAt                types.String `tfsdk:"updated_at"`
	HostCountUpdatedAt       types.String `tfsdk:"host_count_updated_at"`
	InstallSoftware          types.Object `tfsdk:"install_software"`
	RunScript                types.Object `tfsdk:"run_script"`
	PatchSoftware            types.Object `tfsdk:"patch_software"`
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
			"platform": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of platforms this policy applies to (darwin, linux, windows, chrome). Empty list means all platforms.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The type of the policy (`dynamic` or `patch`).",
			},
			"patch_software_title_id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "ID of the Fleet-maintained software title for `type = \"patch\"` policies.",
			},
			"software_title_id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "ID of the software title to install if the policy fails (install-software automation).",
			},
			"script_id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "ID of the script to run if the policy fails (run-script automation).",
			},
			"labels_include_any": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Labels whose hosts are targeted by this policy (any-of semantics).",
			},
			"labels_exclude_any": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Labels whose hosts are excluded from this policy (any-of semantics).",
			},
			"calendar_events_enabled": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether calendar events are triggered when the policy fails.",
			},
			"conditional_access_enabled": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether conditional access (SSO blocking) is enabled for failing hosts.",
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
			"fleet_maintained": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the policy is maintained by Fleet.",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Timestamp when the policy was created.",
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Timestamp when the policy was last updated.",
			},
			"host_count_updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Timestamp when the passing/failing host counts were last refreshed.",
			},
			"install_software": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Echo of the install-software automation attached to this policy.",
				Attributes: map[string]schema.Attribute{
					"name":              schema.StringAttribute{Computed: true},
					"software_title_id": schema.Int64Attribute{Computed: true},
				},
			},
			"run_script": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Echo of the run-script automation attached to this policy.",
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{Computed: true},
					"id":   schema.Int64Attribute{Computed: true},
				},
			},
			"patch_software": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Echo of the patch-software target for `type = \"patch\"` policies.",
				Attributes: map[string]schema.Attribute{
					"name":              schema.StringAttribute{Computed: true},
					"display_name":      schema.StringAttribute{Computed: true},
					"software_title_id": schema.Int64Attribute{Computed: true},
				},
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

	data.ID = types.Int64Value(int64(policy.ID))
	data.Name = types.StringValue(policy.Name)
	data.Description = types.StringValue(policy.Description)
	data.Query = types.StringValue(policy.Query)
	data.Critical = types.BoolValue(policy.Critical)
	data.Resolution = types.StringValue(policy.Resolution)
	data.Platform = platformStringToList(policy.Platform)
	data.AuthorID = types.Int64Value(int64(policy.AuthorID))
	data.AuthorName = types.StringValue(policy.AuthorName)
	data.AuthorEmail = types.StringValue(policy.AuthorEmail)
	data.PassingHostCount = types.Int64Value(int64(policy.PassingHostCount))
	data.FailingHostCount = types.Int64Value(int64(policy.FailingHostCount))
	data.TeamID = intPtrToInt64(policy.TeamID)

	// Empty `type` from the API normalizes to "dynamic" — see the resource
	// for context.
	policyType := policy.Type
	if policyType == "" {
		policyType = "dynamic"
	}
	data.Type = types.StringValue(policyType)
	data.LabelsIncludeAny = policyLabelsToSet(policy.LabelsIncludeAny)
	data.LabelsExcludeAny = policyLabelsToSet(policy.LabelsExcludeAny)
	data.CalendarEventsEnabled = types.BoolValue(policy.CalendarEventsEnabled)
	data.ConditionalAccessEnabled = types.BoolValue(policy.ConditionalAccessEnabled)
	data.FleetMaintained = types.BoolValue(policy.FleetMaintained)
	data.CreatedAt = types.StringValue(policy.CreatedAt)
	data.UpdatedAt = types.StringValue(policy.UpdatedAt)
	data.HostCountUpdatedAt = stringPtrToString(policy.HostCountUpdatedAt)

	data.SoftwareTitleID, data.InstallSoftware = mapInstallSoftware(policy.InstallSoftware, &resp.Diagnostics)
	data.ScriptID, data.RunScript = mapRunScript(policy.RunScript, &resp.Diagnostics)
	data.PatchSoftwareTitleID, data.PatchSoftware = mapPatchSoftware(policy.PatchSoftware, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
