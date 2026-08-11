package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &SoftwareSelfServiceCategoryResource{}
	_ resource.ResourceWithImportState = &SoftwareSelfServiceCategoryResource{}
)

// NewSoftwareSelfServiceCategoryResource creates a new self-service category resource.
func NewSoftwareSelfServiceCategoryResource() resource.Resource {
	return &SoftwareSelfServiceCategoryResource{}
}

// SoftwareSelfServiceCategoryResource manages a self-service software category
// on a fleet. Categories are fleet-scoped: Fleet has no endpoint for moving one
// between fleets, so fleet_id forces replacement.
type SoftwareSelfServiceCategoryResource struct {
	client *fleetdm.Client
}

// SoftwareSelfServiceCategoryResourceModel describes the resource data model.
type SoftwareSelfServiceCategoryResourceModel struct {
	ID      types.Int64  `tfsdk:"id"`
	FleetID types.Int64  `tfsdk:"fleet_id"`
	Name    types.String `tfsdk:"name"`
}

func (r *SoftwareSelfServiceCategoryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software_self_service_category"
}

func (r *SoftwareSelfServiceCategoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a FleetDM self-service software category. Categories group self-service software on a fleet so end users can browse and install it by category on the **My device > Self-service** page.\n\n" +
			"Requires Fleet Premium and Fleet >= 4.90.\n\n" +
			"~> Fleet seeds every new fleet with a set of default categories (for example `🌎 Browsers`, `🧰 Developer tools`). Those are not managed by Terraform; creating a category whose name collides with one of them fails because names must be unique within a fleet (case-insensitive). Import the seeded category instead, or pick a different name.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the self-service category.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"fleet_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The ID of the fleet (team) the category belongs to. Use `0` for hosts that are not assigned to a fleet. Changing this forces a new category to be created.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: fmt.Sprintf("The category name. Must be unique within the fleet (case-insensitive) and at most %d characters. "+
					"Emojis are supported and are part of the name (for example `\"🌎 Browsers\"`). Renaming updates the category in place.",
					fleetdm.SelfServiceCategoryNameMaxLength),
				Validators: []validator.String{
					// Fleet measures the limit with utf8.RuneCountInString, so
					// the rune-counting validator is the matching check —
					// stringvalidator.LengthAtMost counts bytes and would
					// reject valid emoji names well before Fleet does.
					stringvalidator.UTF8LengthAtMost(fleetdm.SelfServiceCategoryNameMaxLength),
					untrimmedNameValidator{},
				},
			},
		},
	}
}

// untrimmedNameValidator rejects a category name that is empty or that Fleet
// would trim.
//
// Fleet applies strings.TrimSpace to the name server-side and rejects the empty
// result. A name Fleet trims applies successfully and then fails with an opaque
// "Provider produced inconsistent result after apply", because the value Fleet
// stored differs from the one Terraform planned.
//
// The predicate is strings.TrimSpace itself rather than a regex on purpose. Go's
// RE2 defines \s as the ASCII set [\t\n\f\r ], while strings.TrimSpace uses
// unicode.IsSpace — so a `^\S(.*\S)?$` pattern accepts U+000B, U+0085, U+00A0,
// U+1680, U+2028, U+202F and U+3000 padding that Fleet goes on to strip.
// Reusing Fleet's own predicate keeps the two in lockstep by construction.
type untrimmedNameValidator struct{}

var _ validator.String = untrimmedNameValidator{}

func (untrimmedNameValidator) Description(_ context.Context) string {
	return "must not be empty or have leading/trailing whitespace"
}

func (v untrimmedNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (untrimmedNameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	if value != "" && strings.TrimSpace(value) == value {
		return
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid Attribute Value",
		fmt.Sprintf("Attribute %s must not be empty or have leading/trailing whitespace: Fleet trims the name, "+
			"which would leave the applied value different from the planned one. Got: %q",
			req.Path, value),
	)
}

func (r *SoftwareSelfServiceCategoryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

func (r *SoftwareSelfServiceCategoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SoftwareSelfServiceCategoryResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	category, err := r.client.CreateSelfServiceCategory(ctx, fleetdm.CreateSelfServiceCategoryRequest{
		FleetID: data.FleetID.ValueInt64(),
		Name:    data.Name.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating FleetDM Self-Service Category",
			fmt.Sprintf("Unable to create self-service category: %s", err),
		)
		return
	}

	r.mapCategoryToModel(category, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SoftwareSelfServiceCategoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SoftwareSelfServiceCategoryResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Fleet has no per-category GET endpoint; the client lists the fleet's
	// categories and matches on ID.
	category, err := r.client.GetSelfServiceCategory(ctx, data.FleetID.ValueInt64(), data.ID.ValueInt64())
	if err != nil {
		// A deleted fleet answers 404 — the category is gone with it.
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading FleetDM Self-Service Category",
			fmt.Sprintf("Unable to read self-service category: %s", err),
		)
		return
	}
	if category == nil {
		// Deleted out of band.
		resp.State.RemoveResource(ctx)
		return
	}

	r.mapCategoryToModel(category, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SoftwareSelfServiceCategoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SoftwareSelfServiceCategoryResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// id is Computed and carried over from state by UseStateForUnknown, but a
	// fleet_id change would have forced replacement rather than an update, so
	// the plan always has a concrete ID here.
	var state SoftwareSelfServiceCategoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	category, err := r.client.UpdateSelfServiceCategory(ctx, state.ID.ValueInt64(), fleetdm.UpdateSelfServiceCategoryRequest{
		Name: data.Name.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating FleetDM Self-Service Category",
			fmt.Sprintf("Unable to rename self-service category %d: %s", state.ID.ValueInt64(), err),
		)
		return
	}

	r.mapCategoryToModel(category, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SoftwareSelfServiceCategoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SoftwareSelfServiceCategoryResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteSelfServiceCategory(ctx, data.ID.ValueInt64()); err != nil {
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting FleetDM Self-Service Category",
			fmt.Sprintf("Unable to delete self-service category %d: %s", data.ID.ValueInt64(), err),
		)
		return
	}
}

// ImportState imports a category using the composite ID "fleet_id:id". Both
// parts are required: the category ID alone is not enough because reads go
// through the fleet-scoped list endpoint.
func (r *SoftwareSelfServiceCategoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be in format: fleet_id:id",
		)
		return
	}

	fleetID, ok := parseIDFromString(parts[0], "Self-Service Category Fleet", &resp.Diagnostics)
	if !ok {
		return
	}
	id, ok := parseIDFromString(parts[1], "Self-Service Category", &resp.Diagnostics)
	if !ok {
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("fleet_id"), fleetID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// mapCategoryToModel copies an API category onto the resource model. fleet_id
// is only overwritten when the API reports one, so a response that omits both
// the new and legacy team key leaves the configured value intact.
func (r *SoftwareSelfServiceCategoryResource) mapCategoryToModel(category *fleetdm.SelfServiceCategory, data *SoftwareSelfServiceCategoryResourceModel) {
	data.ID = types.Int64Value(category.ID)
	data.Name = types.StringValue(category.Name)
	if category.FleetID != nil {
		data.FleetID = types.Int64Value(*category.FleetID)
	}
}
