package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &LabelResource{}
	_ resource.ResourceWithImportState = &LabelResource{}
)

// NewLabelResource creates a new label resource.
func NewLabelResource() resource.Resource {
	return &LabelResource{}
}

// LabelResource defines the resource implementation.
type LabelResource struct {
	client *fleetdm.Client
}

// LabelResourceModel describes the resource data model.
type LabelResourceModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Query       types.String `tfsdk:"query"`
	Platform    types.String `tfsdk:"platform"`
	LabelType   types.String `tfsdk:"label_type"`
	HostCount   types.Int64  `tfsdk:"host_count"`
}

func (r *LabelResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_label"
}

func (r *LabelResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a FleetDM label. Labels are used to group hosts based on SQL queries.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the label.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the label.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "A description of the label.",
			},
			"query": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The SQL query that defines which hosts belong to this label. Hosts are automatically added to the label based on query results.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"platform": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Restricts this label to a specific platform (darwin, windows, linux, chrome). If not specified, the label applies to all platforms.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"label_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The type of the label (regular or builtin).",
			},
			"host_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The number of hosts that belong to this label.",
			},
		},
	}
}

func (r *LabelResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

func (r *LabelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data LabelResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := fleetdm.CreateLabelRequest{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Query:       data.Query.ValueString(),
		Platform:    data.Platform.ValueString(),
	}

	label, err := r.client.CreateLabel(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error Creating FleetDM Label", fmt.Sprintf("Unable to create label: %s", err))
		return
	}

	// Map response to model
	r.mapLabelToModel(label, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LabelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data LabelResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	label, err := r.client.GetLabel(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading FleetDM Label", fmt.Sprintf("Unable to read label: %s", err))
		return
	}

	r.mapLabelToModel(label, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LabelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data LabelResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := fleetdm.UpdateLabelRequest{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
	}

	label, err := r.client.UpdateLabel(ctx, int(data.ID.ValueInt64()), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating FleetDM Label", fmt.Sprintf("Unable to update label: %s", err))
		return
	}

	r.mapLabelToModel(label, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LabelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data LabelResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteLabel(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error Deleting FleetDM Label", fmt.Sprintf("Unable to delete label: %s", err))
		return
	}
}

func (r *LabelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, ok := parseIDFromString(req.ID, "Label", &resp.Diagnostics)
	if !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func (r *LabelResource) mapLabelToModel(label *fleetdm.Label, data *LabelResourceModel) {
	data.ID = types.Int64Value(int64(label.ID))
	data.Name = types.StringValue(label.Name)
	data.Description = types.StringValue(label.Description)
	data.Query = types.StringValue(label.Query)
	data.Platform = types.StringValue(label.Platform)
	data.LabelType = types.StringValue(label.LabelType)
	data.HostCount = types.Int64Value(int64(label.HostCount))
}
