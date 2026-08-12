package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// customHostVitalNameMaxLen mirrors Fleet's limit, which counts runes rather
// than bytes.
const customHostVitalNameMaxLen = 255

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &CustomHostVitalResource{}
	_ resource.ResourceWithImportState = &CustomHostVitalResource{}
)

// NewCustomHostVitalResource creates a new custom host vital resource.
func NewCustomHostVitalResource() resource.Resource {
	return &CustomHostVitalResource{}
}

// CustomHostVitalResource defines the resource implementation.
type CustomHostVitalResource struct {
	client *fleetdm.Client
}

// CustomHostVitalResourceModel describes the resource data model.
type CustomHostVitalResourceModel struct {
	ID        types.Int64  `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

// noSurroundingWhitespaceValidator rejects strings Fleet would reject for
// having leading or trailing whitespace.
//
// The check is `strings.TrimSpace(v) != v`, deliberately matching Fleet's own
// implementation. A `^\S...\S$` regex would not: Go's `\S` is ASCII-only, so it
// treats U+00A0 (and the rest of the Unicode space class) as a non-space
// character and would wave through names Fleet then rejects with a 422.
type noSurroundingWhitespaceValidator struct{}

func (v noSurroundingWhitespaceValidator) Description(_ context.Context) string {
	return "must not have leading or trailing whitespace"
}

func (v noSurroundingWhitespaceValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v noSurroundingWhitespaceValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	if strings.TrimSpace(value) != value {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Custom Host Vital Name",
			fmt.Sprintf("Name %q has leading or trailing whitespace, which Fleet rejects. Note that Unicode spaces (e.g. U+00A0) count too.", value),
		)
	}
}

func (r *CustomHostVitalResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_host_vital"
}

func (r *CustomHostVitalResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a FleetDM custom host vital.\n\n" +
			"A custom host vital is a named slot for host-specific data that Fleet itself does not collect — an asset tag, " +
			"a cost centre, a room number. Terraform manages the slot; the per-host values are pushed in out-of-band with " +
			"`PUT /api/v1/fleet/hosts/{host_id}/custom_host_vitals/{id}` (there is no Terraform resource for a value, since " +
			"host inventory is not infrastructure-as-code).\n\n" +
			"Once defined, a vital can be:\n" +
			"- interpolated into configuration profiles, scripts and software installers as `$FLEET_HOST_VITAL_<id>`, and\n" +
			"- matched by a `fleetdm_label` `criteria` block to build a label whose membership follows the value.\n\n" +
			"~> **The vital carries no query.** In Fleet 4.90 a custom host vital is `(id, name)` only, so there is nothing " +
			"here to express \"how to collect this value\" — values arrive via the API above. A label that should be driven " +
			"by a SQL query wants `fleetdm_label`'s `query` attribute instead.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the custom host vital. Use this as the `<id>` in `$FLEET_HOST_VITAL_<id>` references and in a label's `criteria.custom_host_vital_id`.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The name of the custom host vital. Must be unique across the Fleet instance, at most " +
					"255 characters, and free of leading or trailing whitespace. Renaming updates the vital in place; " +
					"existing `$FLEET_HOST_VITAL_<id>` references keep working because they resolve by id, not name.",
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, customHostVitalNameMaxLen),
					noSurroundingWhitespaceValidator{},
				},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Timestamp when the custom host vital was created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Timestamp when the custom host vital was last updated.",
			},
		},
	}
}

func (r *CustomHostVitalResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

func (r *CustomHostVitalResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CustomHostVitalResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vital, err := r.client.CreateCustomHostVital(ctx, fleetdm.CreateCustomHostVitalRequest{
		Name: data.Name.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating FleetDM Custom Host Vital", fmt.Sprintf("Unable to create custom host vital: %s", err))
		return
	}

	r.mapVitalToModel(vital, &data)
	r.fillTimestamps(ctx, &data, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CustomHostVitalResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CustomHostVitalResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vital, err := r.client.GetCustomHostVital(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading FleetDM Custom Host Vital", fmt.Sprintf("Unable to read custom host vital: %s", err))
		return
	}

	r.mapVitalToModel(vital, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CustomHostVitalResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data CustomHostVitalResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vital, err := r.client.UpdateCustomHostVital(ctx, int(data.ID.ValueInt64()), fleetdm.UpdateCustomHostVitalRequest{
		Name: data.Name.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Updating FleetDM Custom Host Vital", fmt.Sprintf("Unable to update custom host vital: %s", err))
		return
	}

	r.mapVitalToModel(vital, &data)
	r.fillTimestamps(ctx, &data, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CustomHostVitalResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CustomHostVitalResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteCustomHostVital(ctx, int(data.ID.ValueInt64())); err != nil {
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting FleetDM Custom Host Vital",
			fmt.Sprintf("Unable to delete custom host vital: %s\n\n"+
				"Fleet refuses to delete a vital that is still referenced. If the message above names a configuration "+
				"profile, script, software installer or label, remove that reference first — Terraform cannot order this "+
				"for you unless the referencing resource depends on this one.", err),
		)
		return
	}
}

func (r *CustomHostVitalResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, ok := parseIDFromString(req.ID, "Custom Host Vital", &resp.Diagnostics)
	if !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// fillTimestamps refreshes created_at/updated_at from the list endpoint.
//
// Fleet's create and update responses return both timestamps as empty strings,
// so without this the values only appear on the next refresh — which would show
// up as an unexplained state change and break `ImportStateVerify`. A failure
// here is downgraded to a warning: the vital already exists at this point, so
// erroring out would leak it.
func (r *CustomHostVitalResource) fillTimestamps(ctx context.Context, data *CustomHostVitalResourceModel, diags *diag.Diagnostics) {
	if isKnown(data.CreatedAt) && isKnown(data.UpdatedAt) {
		return
	}

	vital, err := r.client.GetCustomHostVital(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		diags.AddWarning(
			"Could Not Read Back FleetDM Custom Host Vital Timestamps",
			fmt.Sprintf("The custom host vital was written successfully, but reading its timestamps back failed: %s\n\n"+
				"`created_at`/`updated_at` will be populated on the next refresh.", err),
		)
	} else {
		r.mapVitalToModel(vital, data)
	}

	// Normalize unconditionally: a Computed attribute may end an apply null,
	// but never unknown. The read-back succeeding is not enough on its own —
	// a listed vital whose timestamps came back empty would leave these
	// unknown and fail the apply.
	if data.CreatedAt.IsUnknown() {
		data.CreatedAt = types.StringNull()
	}
	if data.UpdatedAt.IsUnknown() {
		data.UpdatedAt = types.StringNull()
	}
}

// mapVitalToModel copies an API vital onto the Terraform model.
//
// Timestamps are only overwritten when the API actually supplied them. Fleet's
// create/update responses return them empty, and blanking `created_at` there
// would contradict the plan (which carries the prior value forward via
// UseStateForUnknown) and fail the apply.
func (r *CustomHostVitalResource) mapVitalToModel(vital *fleetdm.CustomHostVital, data *CustomHostVitalResourceModel) {
	data.ID = types.Int64Value(int64(vital.ID))
	data.Name = types.StringValue(vital.Name)
	if vital.CreatedAt != "" {
		data.CreatedAt = types.StringValue(vital.CreatedAt)
	}
	if vital.UpdatedAt != "" {
		data.UpdatedAt = types.StringValue(vital.UpdatedAt)
	}
}

// isKnown reports whether a types.String holds a usable value (neither null nor
// unknown).
func isKnown(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown()
}
