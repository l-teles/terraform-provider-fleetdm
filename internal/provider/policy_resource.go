package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &PolicyResource{}
	_ resource.ResourceWithImportState = &PolicyResource{}
)

// NewPolicyResource creates a new policy resource.
func NewPolicyResource() resource.Resource {
	return &PolicyResource{}
}

// PolicyResource defines the resource implementation.
type PolicyResource struct {
	client *fleetdm.Client
}

// PolicyResourceModel describes the resource data model.
type PolicyResourceModel struct {
	ID               types.Int64  `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	Query            types.String `tfsdk:"query"`
	Critical         types.Bool   `tfsdk:"critical"`
	Resolution       types.String `tfsdk:"resolution"`
	Platform         types.List   `tfsdk:"platform"`
	TeamID           types.Int64  `tfsdk:"team_id"`
	AuthorID         types.Int64  `tfsdk:"author_id"`
	AuthorName       types.String `tfsdk:"author_name"`
	AuthorEmail      types.String `tfsdk:"author_email"`
	PassingHostCount types.Int64  `tfsdk:"passing_host_count"`
	FailingHostCount types.Int64  `tfsdk:"failing_host_count"`
}

func (r *PolicyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_policy"
}

func (r *PolicyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a FleetDM policy. Policies are yes/no questions that define compliance checks for hosts.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the policy.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the policy. Must be unique.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "A description of the policy.",
			},
			"query": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The SQL query that defines the policy. The policy passes if the query returns results.",
			},
			"critical": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether the policy is critical. Critical policies are highlighted in the UI.",
			},
			"resolution": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "Instructions for resolving a failing policy check.",
			},
			"platform": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of platforms this policy applies to (darwin, linux, windows, chrome). Empty list means all platforms.",
			},
			"team_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "The ID of the team this policy belongs to. If not specified, the policy is global.",
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

func (r *PolicyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

func (r *PolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := fleetdm.CreatePolicyRequest{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Query:       data.Query.ValueString(),
		Critical:    data.Critical.ValueBool(),
		Resolution:  data.Resolution.ValueString(),
		Platform:    platformListToString(ctx, data.Platform),
	}

	policy, err := r.client.CreatePolicy(ctx, optionalIntPtr(data.TeamID), createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error Creating FleetDM Policy", fmt.Sprintf("Unable to create policy: %s", err))
		return
	}

	// Map response to model
	r.mapPolicyToModel(policy, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PolicyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policy, err := r.client.GetPolicy(ctx, int(data.ID.ValueInt64()), optionalIntPtr(data.TeamID))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading FleetDM Policy", fmt.Sprintf("Unable to read policy: %s", err))
		return
	}

	// Map response to model
	r.mapPolicyToModel(policy, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := fleetdm.UpdatePolicyRequest{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Query:       data.Query.ValueString(),
		Critical:    data.Critical.ValueBool(),
		Resolution:  data.Resolution.ValueString(),
		Platform:    platformListToString(ctx, data.Platform),
	}

	policy, err := r.client.UpdatePolicy(ctx, int(data.ID.ValueInt64()), optionalIntPtr(data.TeamID), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating FleetDM Policy", fmt.Sprintf("Unable to update policy: %s", err))
		return
	}

	// Map response to model
	r.mapPolicyToModel(policy, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PolicyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeletePolicy(ctx, int(data.ID.ValueInt64()), optionalIntPtr(data.TeamID))
	if err != nil {
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error Deleting FleetDM Policy", fmt.Sprintf("Unable to delete policy: %s", err))
		return
	}
}

func (r *PolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, ok := parseIDFromString(req.ID, "Policy", &resp.Diagnostics)
	if !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func (r *PolicyResource) mapPolicyToModel(policy *fleetdm.Policy, data *PolicyResourceModel) {
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
}
