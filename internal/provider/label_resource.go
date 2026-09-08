package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                   = &LabelResource{}
	_ resource.ResourceWithImportState    = &LabelResource{}
	_ resource.ResourceWithValidateConfig = &LabelResource{}
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
	ID                  types.Int64         `tfsdk:"id"`
	Name                types.String        `tfsdk:"name"`
	Description         types.String        `tfsdk:"description"`
	Query               types.String        `tfsdk:"query"`
	Criteria            *LabelCriteriaModel `tfsdk:"criteria"`
	Platform            types.String        `tfsdk:"platform"`
	LabelType           types.String        `tfsdk:"label_type"`
	LabelMembershipType types.String        `tfsdk:"label_membership_type"`
	HostCount           types.Int64         `tfsdk:"host_count"`
}

// LabelCriteriaModel describes the host-vitals criteria block.
type LabelCriteriaModel struct {
	Vital             types.String `tfsdk:"vital"`
	Operator          types.String `tfsdk:"operator"`
	Value             types.String `tfsdk:"value"`
	CustomHostVitalID types.Int64  `tfsdk:"custom_host_vital_id"`
}

func (r *LabelResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_label"
}

func (r *LabelResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a FleetDM label. Labels group hosts so you can scope profiles, software and policies to them.\n\n" +
			"A label picks its members one of three ways, and the choice is fixed for the label's lifetime:\n" +
			"- **dynamic** — set `query`; hosts join when the SQL matches.\n" +
			"- **host vitals** — set `criteria`; hosts join when a host attribute (IdP group or department, or a " +
			"`fleetdm_custom_host_vital` value) compares true.\n" +
			"- **manual** — set neither; membership is assigned host-by-host outside Terraform.\n\n" +
			"`query` and `criteria` are mutually exclusive, and changing either one replaces the label.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the label.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The name of the label. At most 255 characters — Fleet's API surfaces a longer value as a raw MySQL " +
					"`Data too long` error, so the limit is enforced at plan time.",
				Validators: []validator.String{
					stringvalidator.LengthAtMost(fleetdm.MaxNameLength),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "A description of the label. At most 255 characters.",
				Validators: []validator.String{
					stringvalidator.LengthAtMost(fleetdm.MaxNameLength),
				},
			},
			"query": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "The SQL query that defines which hosts belong to this label. Hosts are automatically added to the label based on query results. " +
					"Mutually exclusive with `criteria`; omit both for a manual label.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("criteria")),
				},
			},
			"criteria": schema.SingleNestedAttribute{
				Optional: true,
				MarkdownDescription: "Defines a **host vitals** label: membership follows a host attribute instead of a SQL query. " +
					"Mutually exclusive with `query`.\n\n" +
					"Fleet cannot change a label's criteria after creation — its modify-label endpoint has no field for it and " +
					"silently ignores one — so any change here replaces the label.",
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
				// Declared on both sides of each pair so the diagnostic can be
				// attributed to whichever attribute the user is looking at.
				Validators: []validator.Object{
					objectvalidator.ConflictsWith(
						path.MatchRoot("query"),
						path.MatchRoot("platform"),
					),
				},
				Attributes: map[string]schema.Attribute{
					"vital": schema.StringAttribute{
						Required: true,
						MarkdownDescription: "The host attribute to match on. One of `end_user_idp_group`, `end_user_idp_department` " +
							"(both sourced from the host's end user via SCIM) or `custom_host_vital` (a `fleetdm_custom_host_vital` value, " +
							"which additionally requires `custom_host_vital_id`).",
						Validators: []validator.String{
							stringvalidator.OneOf(
								fleetdm.HostVitalEndUserIDPGroup,
								fleetdm.HostVitalEndUserIDPDepartment,
								fleetdm.HostVitalCustomHostVital,
							),
						},
					},
					"operator": schema.StringAttribute{
						Optional: true,
						MarkdownDescription: "How to compare the vital against `value`. One of `=`, `!=`, `>`, `<` or `LIKE`. " +
							"Defaults to equality when omitted.",
						Validators: []validator.String{
							stringvalidator.OneOf(
								fleetdm.HostVitalOperatorEqual,
								fleetdm.HostVitalOperatorNotEqual,
								fleetdm.HostVitalOperatorGreater,
								fleetdm.HostVitalOperatorLess,
								fleetdm.HostVitalOperatorLike,
							),
						},
					},
					"value": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "The value to compare the vital against.",
					},
					"custom_host_vital_id": schema.Int64Attribute{
						Optional: true,
						MarkdownDescription: "The `id` of the `fleetdm_custom_host_vital` to match. Required when `vital` is " +
							"`custom_host_vital`, and rejected for the other vitals. Reference the resource (for example " +
							"`fleetdm_custom_host_vital.asset_tag.id`) so Terraform creates the vital first and deletes it last.",
					},
				},
			},
			"platform": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Restricts this label to a specific platform. Fleet accepts `darwin`, `windows` and `linux` (`linux` matches any distribution and requires Fleet 4.91.0 or later); " +
					"any other value, including `chrome`, is rejected with `has invalid platform`. If not specified, the label applies to all platforms. " +
					"Cannot be combined with `criteria` — Fleet rejects a platform on a host vitals label.",
				PlanModifiers: []planmodifier.String{
					// UseStateForUnknown must come first. Without it this
					// Optional+Computed attribute plans as unknown whenever the
					// config omits it, which never equals the stored value, so
					// RequiresReplace fired on *every* update — a rename or a
					// description edit silently destroyed and recreated the
					// label, churning its id and dropping manual membership.
					// Keeping the prior value means replacement is reserved for
					// a platform the user actually changed, which Fleet's
					// modify-label endpoint genuinely cannot apply.
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("criteria")),
				},
			},
			"label_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The type of the label (regular or builtin).",
			},
			"label_membership_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "How Fleet resolves membership for this label: `dynamic` (driven by `query`), `host_vitals` (driven by `criteria`) or `manual`.",
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

// ValidateConfig enforces the coupling between `criteria.vital` and
// `criteria.custom_host_vital_id`, which Fleet only reports at apply time:
//   - `vital = "custom_host_vital"` requires `custom_host_vital_id`, because the
//     vital name alone doesn't say which custom vital to read.
//   - the other vitals reject `custom_host_vital_id`.
func (r *LabelResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	// Read `criteria` as an opaque object first. Decoding straight into the
	// model would fail with a "Value Conversion Error" when the whole block is
	// unknown — e.g. `criteria = var.enabled ? { ... } : null` where the
	// condition isn't known until apply — because an unknown object has no
	// null-or-struct representation to convert into *LabelCriteriaModel.
	var criteriaObj types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("criteria"), &criteriaObj)...)
	if resp.Diagnostics.HasError() || criteriaObj.IsNull() || criteriaObj.IsUnknown() {
		return
	}

	var criteria LabelCriteriaModel
	resp.Diagnostics.Append(criteriaObj.As(ctx, &criteria, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	vital := criteria.Vital
	id := criteria.CustomHostVitalID
	if vital.IsNull() || vital.IsUnknown() || id.IsUnknown() {
		return
	}

	switch {
	case vital.ValueString() == fleetdm.HostVitalCustomHostVital && id.IsNull():
		resp.Diagnostics.AddAttributeError(
			path.Root("criteria").AtName("custom_host_vital_id"),
			"Missing Custom Host Vital ID",
			"`criteria.custom_host_vital_id` is required when `criteria.vital` is \"custom_host_vital\", "+
				"since that vital does not identify which custom host vital to match. "+
				"Reference a `fleetdm_custom_host_vital` resource's `id`.",
		)
	case vital.ValueString() != fleetdm.HostVitalCustomHostVital && !id.IsNull():
		resp.Diagnostics.AddAttributeError(
			path.Root("criteria").AtName("custom_host_vital_id"),
			"Unexpected Custom Host Vital ID",
			fmt.Sprintf("`criteria.custom_host_vital_id` only applies when `criteria.vital` is \"custom_host_vital\", but vital is %q. Remove it.",
				vital.ValueString()),
		)
	}
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
		Criteria:    criteriaToAPI(data.Criteria),
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
	// Fleet echoes an empty query for host-vitals and manual labels. `query` is
	// Optional and not Computed, so writing "" over a null config value would
	// fail the apply with an inconsistent-result error; keep null as null.
	if label.Query == "" && data.Query.IsNull() {
		data.Query = types.StringNull()
	} else {
		data.Query = types.StringValue(label.Query)
	}
	// Keep the criteria we already hold when the response omits it. Fleet
	// echoes criteria on every label endpoint today, but that isn't a contract:
	// if a modify response ever dropped it, blanking the field would abort a
	// plain rename with "inconsistent result after apply (.criteria: was
	// object, now null)". Criteria is immutable server-side, so state is
	// authoritative whenever the API declines to restate it.
	if label.Criteria != nil || data.Criteria == nil {
		data.Criteria = criteriaFromAPI(label.Criteria)
	}
	data.Platform = types.StringValue(label.Platform)
	data.LabelType = types.StringValue(label.LabelType)
	data.LabelMembershipType = types.StringValue(label.LabelMembershipType)
	data.HostCount = types.Int64Value(int64(label.HostCount))
}

// criteriaToAPI converts the Terraform criteria block into an API payload,
// returning nil when no criteria block is configured so the field is omitted
// (Fleet rejects a request carrying both criteria and a query).
func criteriaToAPI(criteria *LabelCriteriaModel) *fleetdm.HostVitalCriteria {
	if criteria == nil {
		return nil
	}
	return &fleetdm.HostVitalCriteria{
		Vital:             criteria.Vital.ValueString(),
		Value:             criteria.Value.ValueString(),
		Operator:          criteria.Operator.ValueString(),
		CustomHostVitalID: optionalIntPtr(criteria.CustomHostVitalID),
	}
}

// criteriaFromAPI converts an API criteria echo back into the Terraform model.
//
// Fleet omits `operator` and `custom_host_vital_id` from the echo when they
// weren't sent, so those map back to null and round-trip cleanly against a
// config that left them out.
func criteriaFromAPI(criteria *fleetdm.HostVitalCriteria) *LabelCriteriaModel {
	if criteria == nil {
		return nil
	}
	return &LabelCriteriaModel{
		Vital:             types.StringValue(criteria.Vital),
		Value:             types.StringValue(criteria.Value),
		Operator:          stringEmptyToNull(criteria.Operator),
		CustomHostVitalID: intPtrToInt64(criteria.CustomHostVitalID),
	}
}

// stringEmptyToNull maps the empty string to null, used for API fields Fleet
// omits rather than returning empty.
func stringEmptyToNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}
