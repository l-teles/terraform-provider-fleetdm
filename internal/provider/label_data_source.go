package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &LabelDataSource{}

// NewLabelDataSource creates a new label data source.
func NewLabelDataSource() datasource.DataSource {
	return &LabelDataSource{}
}

// LabelDataSource defines the data source implementation.
type LabelDataSource struct {
	client *fleetdm.Client
}

// LabelDataSourceModel describes the data source data model.
type LabelDataSourceModel struct {
	ID                  types.Int64  `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	Description         types.String `tfsdk:"description"`
	Query               types.String `tfsdk:"query"`
	Criteria            types.Object `tfsdk:"criteria"`
	Platform            types.String `tfsdk:"platform"`
	LabelType           types.String `tfsdk:"label_type"`
	LabelMembershipType types.String `tfsdk:"label_membership_type"`
	HostCount           types.Int64  `tfsdk:"host_count"`
}

func (d *LabelDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_label"
}

func (d *LabelDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about a specific FleetDM label.\n\n" +
			"`label_membership_type` says how the label selects hosts, and for a host vitals label `criteria` shows " +
			"the attribute comparison that drives it.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The unique identifier of the label.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the label.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A description of the label.",
			},
			"query": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The SQL query that defines which hosts belong to this label. Empty for manual and host vitals labels.",
			},
			"criteria": labelCriteriaDataSourceSchema(),
			"platform": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The platform the label is restricted to (darwin, windows, linux, chrome).",
			},
			"label_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The type of the label (regular or builtin).",
			},
			"label_membership_type": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "How Fleet resolves membership for this label: `dynamic` (driven by `query`), " +
					"`host_vitals` (driven by `criteria`) or `manual` (assigned host-by-host).",
			},
			"host_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The number of hosts that belong to this label.",
			},
		},
	}
}

func (d *LabelDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics, "Data Source")
}

func (d *LabelDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data LabelDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	label, err := d.client.GetLabel(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read label: %s", err))
		return
	}

	// Map response to model
	data.ID = types.Int64Value(int64(label.ID))
	data.Name = types.StringValue(label.Name)
	data.Description = types.StringValue(label.Description)
	data.Query = types.StringValue(label.Query)
	data.Criteria = labelCriteriaToObject(label.Criteria, &resp.Diagnostics)
	data.Platform = types.StringValue(label.Platform)
	data.LabelType = types.StringValue(label.LabelType)
	data.LabelMembershipType = types.StringValue(label.LabelMembershipType)
	data.HostCount = types.Int64Value(int64(label.HostCount))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// labelCriteriaAttrTypes mirrors the `criteria` block on the fleetdm_label
// resource, so a host vitals label reads back through either label data source
// with the same shape it was declared with.
var labelCriteriaAttrTypes = map[string]attr.Type{
	"vital":                types.StringType,
	"operator":             types.StringType,
	"value":                types.StringType,
	"custom_host_vital_id": types.Int64Type,
}

// labelCriteriaDataSourceSchema returns the Computed `criteria` attribute
// shared by the fleetdm_label and fleetdm_labels data sources.
func labelCriteriaDataSourceSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Computed: true,
		MarkdownDescription: "The host attribute comparison that drives membership for a **host vitals** label " +
			"(`label_membership_type = \"host_vitals\"`). Null for dynamic and manual labels, which Fleet omits it for.",
		Attributes: map[string]schema.Attribute{
			"vital": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The host attribute matched on: `end_user_idp_group`, `end_user_idp_department` " +
					"or `custom_host_vital`.",
			},
			"operator": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "How the vital is compared against `value`. Null when the label was created " +
					"without one — Fleet omits the field rather than echoing its default.",
			},
			"value": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The value the vital is compared against.",
			},
			"custom_host_vital_id": schema.Int64Attribute{
				Computed: true,
				MarkdownDescription: "The `fleetdm_custom_host_vital` id being matched. Only set when `vital` is " +
					"`custom_host_vital`; null otherwise.",
			},
		},
	}
}

// labelCriteriaToObject converts a label's criteria echo into the computed
// object, yielding a null object rather than an empty one for the dynamic and
// manual labels that have no criteria.
//
// Fleet omits `operator` and `custom_host_vital_id` from the echo when they
// weren't set, so both map back to null instead of a zero value.
func labelCriteriaToObject(criteria *fleetdm.HostVitalCriteria, diags *diag.Diagnostics) types.Object {
	if criteria == nil {
		return types.ObjectNull(labelCriteriaAttrTypes)
	}
	obj, dd := types.ObjectValue(labelCriteriaAttrTypes, map[string]attr.Value{
		"vital":                types.StringValue(criteria.Vital),
		"operator":             stringEmptyToNull(criteria.Operator),
		"value":                types.StringValue(criteria.Value),
		"custom_host_vital_id": intPtrToInt64(criteria.CustomHostVitalID),
	})
	diags.Append(dd...)
	return obj
}
