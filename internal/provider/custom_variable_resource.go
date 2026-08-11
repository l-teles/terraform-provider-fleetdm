package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                     = &CustomVariableResource{}
	_ resource.ResourceWithConfigure        = &CustomVariableResource{}
	_ resource.ResourceWithImportState      = &CustomVariableResource{}
	_ resource.ResourceWithConfigValidators = &CustomVariableResource{}
	_ resource.ResourceWithModifyPlan       = &CustomVariableResource{}
)

// customVariableNameFormat mirrors Fleet's own server-side rule for custom
// variable names (uppercase letters, digits and underscores only). Validating
// it at plan time turns a 422 mid-apply into an error the practitioner sees
// before anything is written.
var customVariableNameFormat = regexp.MustCompile(`^[A-Z0-9_]+$`)

// customVariableNameMaxLen matches Fleet's server-side limit.
const customVariableNameMaxLen = 255

// reservedCustomVariableNamePrefix is the prefix Fleet itself prepends when a
// custom variable is referenced from a script or configuration profile
// (`$FLEET_SECRET_MY_VAR` resolves the variable named `MY_VAR`). Fleet's create
// endpoint stores a name carrying this prefix verbatim, which would then have to
// be referenced as `$FLEET_SECRET_FLEET_SECRET_MY_VAR`, while Fleet's spec
// endpoint strips the prefix instead — so the same name means two different
// things depending on which endpoint wrote it. The provider rejects the prefix
// outright rather than pick a side.
const reservedCustomVariableNamePrefix = "FLEET_SECRET_"

// customVariablesUnsupportedSummary titles the diagnostic raised when Fleet
// answers 404 for the custom variables collection endpoint.
const customVariablesUnsupportedSummary = "Custom Variables Not Supported By This Fleet Server"

// customVariablesUnsupportedDetail explains a 404 from the collection endpoint.
//
// The provider resolves a custom variable by listing the collection, because
// Fleet offers no GET-by-id. That makes 404 unambiguous in the opposite
// direction from most resources: a 404 is the *route* missing (Fleet older than
// 4.90), never the variable missing — an absent variable comes back as an empty
// result from a successful 200 list. Treating it as a deletion would make a
// refresh against a pre-4.90 server silently drop the resource from state and
// then propose recreating it on a server that cannot accept it.
func customVariablesUnsupportedDetail(err error) string {
	return "Fleet returned 404 for the /custom_variables endpoint, which means this Fleet server does not implement the custom " +
		"variables API. The fleetdm_custom_variable resource requires Fleet 4.90 or later.\n\n" +
		"This is a missing API route, not a missing variable: a variable that no longer exists comes back as an empty result " +
		"from a successful list, and the provider removes it from state in that case.\n\nFleet reported: " + err.Error()
}

// NewCustomVariableResource creates a new custom variable resource.
func NewCustomVariableResource() resource.Resource {
	return &CustomVariableResource{}
}

// CustomVariableResource defines the resource implementation.
type CustomVariableResource struct {
	client *fleetdm.Client
}

// CustomVariableResourceModel describes the resource data model.
type CustomVariableResourceModel struct {
	ID             types.Int64  `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Value          types.String `tfsdk:"value"`
	ValueWO        types.String `tfsdk:"value_wo"`
	ValueWOVersion types.Int64  `tfsdk:"value_wo_version"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

// Metadata returns the resource type name.
func (r *CustomVariableResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_variable"
}

// Schema defines the schema for the resource.
func (r *CustomVariableResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manages a FleetDM custom variable — a named secret Fleet stores encrypted and substitutes into scripts and configuration profiles.

Reference a custom variable from a script or profile as ` + "`$FLEET_SECRET_MY_VAR`" + ` or ` + "`${FLEET_SECRET_MY_VAR}`" + `, where ` + "`MY_VAR`" + ` is this resource's ` + "`name`" + `. Fleet adds the ` + "`FLEET_SECRET_`" + ` prefix itself, so the name configured here must **not** include it.

Requires Fleet 4.90 or later, and the Fleet server must be started with a private key (` + "`FLEET_SERVER_PRIVATE_KEY`" + `) — without one Fleet rejects every write with "Missing required private key".

## Drift detection

**Fleet never returns a custom variable's value from any API endpoint.** The provider therefore cannot detect a value changed outside Terraform: a refresh confirms only that a variable of this ` + "`name`" + ` still exists, and the value recorded in state (or, for ` + "`value_wo`" + `, not recorded at all) is assumed to still be current. A value edited in the Fleet UI will not show up as drift.

To deliberately push a new value:

* with ` + "`value`" + ` — change the attribute; Terraform sees the change in state and updates in place.
* with ` + "`value_wo`" + ` — change the attribute **and** increment ` + "`value_wo_version`" + `. Write-only attributes are absent from both plan and state, so ` + "`value_wo_version`" + ` is the only signal Terraform can act on; editing ` + "`value_wo`" + ` alone produces no diff and no update.

## Deleting a variable in use

Fleet refuses (409) to delete a custom variable that is still referenced by a script, configuration profile or declaration, naming the referencing object in the error. Remove or edit that object before destroying this resource. Renaming the variable is subject to the same rule, because Fleet has no rename endpoint and a rename is therefore a replacement.
`,

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The unique identifier Fleet assigned to the custom variable.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the custom variable, referenced in scripts and profiles as `$FLEET_SECRET_<name>`. " +
					"Fleet allows uppercase letters, digits and underscores only (`^[A-Z0-9_]+$`), up to 255 characters. " +
					"Must not start with `FLEET_SECRET_` — Fleet adds that prefix when resolving references. " +
					"Fleet has no rename endpoint, so changing the name replaces the variable.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, customVariableNameMaxLen),
					stringvalidator.RegexMatches(
						customVariableNameFormat,
						"must contain only uppercase letters (A-Z), digits (0-9) and underscores (_)",
					),
					customVariableNameNotReserved{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "The secret value of the custom variable. Exactly one of `value` or `value_wo` must be set. " +
					"This attribute is stored in Terraform state — anyone who can read the state file can read the value. " +
					"Prefer `value_wo` on Terraform 1.11 and later.",
				Optional:  true,
				Sensitive: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"value_wo": schema.StringAttribute{
				MarkdownDescription: "The secret value of the custom variable, as a write-only attribute: Terraform never persists it to the plan or the state file. " +
					"Exactly one of `value` or `value_wo` must be set. Requires Terraform 1.11 or later. " +
					"Because Terraform cannot see a write-only value, changing it has no effect on its own — increment `value_wo_version` in the same change to push the new value.",
				Optional:  true,
				Sensitive: true,
				WriteOnly: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"value_wo_version": schema.Int64Attribute{
				MarkdownDescription: "An arbitrary version counter for `value_wo`. Increment it to make Terraform push the current `value_wo` to Fleet. " +
					"Only meaningful alongside `value_wo`; it conflicts with `value`, which Terraform can diff directly.",
				Optional: true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the custom variable was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the custom variable's value was last written. Fleet updates this on every value rotation, so it is the only server-side evidence that a rotation took effect.",
				Computed:            true,
			},
		},
	}
}

// ConfigValidators enforces the value/value_wo relationship across attributes.
func (r *CustomVariableResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		// A custom variable without a value is meaningless, and Fleet rejects an
		// empty one, so exactly one source must be configured.
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("value"),
			path.MatchRoot("value_wo"),
		),
		// value_wo_version exists only to make a write-only change visible to
		// Terraform. Paired with `value` it would silently do nothing, so reject
		// the combination instead of ignoring it.
		resourcevalidator.Conflicting(
			path.MatchRoot("value"),
			path.MatchRoot("value_wo_version"),
		),
		// Nudge practitioners on Terraform >= 1.11 towards the write-only path.
		resourcevalidator.PreferWriteOnlyAttribute(
			path.MatchRoot("value"),
			path.MatchRoot("value_wo"),
		),
	}
}

// customVariableNameNotReserved rejects names carrying Fleet's reserved
// reference prefix.
type customVariableNameNotReserved struct{}

func (v customVariableNameNotReserved) Description(ctx context.Context) string {
	return fmt.Sprintf("must not start with the reserved prefix %q", reservedCustomVariableNamePrefix)
}

func (v customVariableNameNotReserved) MarkdownDescription(ctx context.Context) string {
	return fmt.Sprintf("must not start with the reserved prefix `%s`", reservedCustomVariableNamePrefix)
}

func (v customVariableNameNotReserved) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	name := req.ConfigValue.ValueString()
	if !strings.HasPrefix(name, reservedCustomVariableNamePrefix) {
		return
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Reserved Custom Variable Name Prefix",
		fmt.Sprintf(
			"The name %q starts with %q, which Fleet reserves. Fleet prepends that prefix itself when resolving a reference, "+
				"so a variable named %q would have to be referenced as $%s%s. Drop the prefix: name the variable %q and reference it as $%s%s.",
			name, reservedCustomVariableNamePrefix,
			name, reservedCustomVariableNamePrefix, name,
			strings.TrimPrefix(name, reservedCustomVariableNamePrefix),
			reservedCustomVariableNamePrefix, strings.TrimPrefix(name, reservedCustomVariableNamePrefix),
		),
	)
}

// Configure adds the provider configured client to the resource.
func (r *CustomVariableResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// ModifyPlan marks updated_at unknown when a value change is planned.
//
// Fleet advances updated_at on every value write, so leaving the prior value in
// the plan would make Terraform reject the apply with "Provider produced
// inconsistent result after apply". The guard matters both ways: setting it
// unknown unconditionally would manufacture a diff on every plan and leave the
// resource permanently proposing an update.
func (r *CustomVariableResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Null raw state means create; null raw plan means destroy. Neither can
	// produce an in-place value change.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var stateValue, planValue types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("value"), &stateValue)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("value"), &planValue)...)

	var stateVersion, planVersion types.Int64
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("value_wo_version"), &stateVersion)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("value_wo_version"), &planVersion)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if planValue.Equal(stateValue) && planVersion.Equal(stateVersion) {
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("updated_at"), types.StringUnknown())...)
}

// resolveCustomVariableValue returns the value to send to Fleet.
//
// `value_wo` is a write-only attribute, so it is present only in the
// configuration for the current apply — never in the plan or the state. It must
// therefore be read straight from the config rather than from the model built
// out of the plan.
func resolveCustomVariableValue(ctx context.Context, config tfsdk.Config, plannedValue types.String, diags *diag.Diagnostics) string {
	var writeOnly types.String
	diags.Append(config.GetAttribute(ctx, path.Root("value_wo"), &writeOnly)...)
	if diags.HasError() {
		return ""
	}

	if !writeOnly.IsNull() && !writeOnly.IsUnknown() {
		return writeOnly.ValueString()
	}
	if !plannedValue.IsNull() && !plannedValue.IsUnknown() {
		return plannedValue.ValueString()
	}

	diags.AddError(
		"Missing Custom Variable Value",
		"Neither `value` nor `value_wo` resolved to a usable value at apply time. Set exactly one of them to a non-empty string.",
	)
	return ""
}

// Create creates the resource and sets the initial Terraform state.
func (r *CustomVariableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CustomVariableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	value := resolveCustomVariableValue(ctx, req.Config, data.Value, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	tflog.Debug(ctx, "Creating FleetDM custom variable", map[string]interface{}{"name": name})

	created, err := r.client.CreateCustomVariable(ctx, name, value)
	if err != nil {
		if isConflict(err) {
			resp.Diagnostics.AddError(
				"Custom Variable Already Exists",
				fmt.Sprintf(
					"Fleet already has a custom variable named %q. Custom variable names are global, and Fleet has no endpoint that "+
						"returns a value, so the provider cannot adopt it implicitly. Import it instead:\n\n"+
						"  terraform import fleetdm_custom_variable.<label> %s\n\n"+
						"Fleet reported: %s",
					name, name, err.Error(),
				),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Error Creating FleetDM Custom Variable",
			fmt.Sprintf("Could not create custom variable %q: %s", name, err.Error()),
		)
		return
	}

	data.ID = types.Int64Value(int64(created.ID))

	// The secret now exists in Fleet. From here on, every path must persist state:
	// returning an error without setting state would leave the variable behind in
	// Fleet with nothing managing it — an orphaned secret that a later apply
	// cannot even recreate, because the name is taken. refreshTimestamps
	// therefore only ever warns, and always leaves the computed attributes known.
	r.refreshTimestamps(ctx, &data, &resp.Diagnostics)

	// value_wo is write-only: it must never reach state. The framework nullifies
	// write-only attributes in the response state as well, but being explicit
	// keeps the intent local to the code that handles the secret.
	data.ValueWO = types.StringNull()

	tflog.Info(ctx, "Created FleetDM custom variable", map[string]interface{}{
		"name": name,
		"id":   created.ID,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read refreshes the Terraform state with the latest data.
//
// Fleet exposes no GET-by-id for custom variables, so the read lists and matches
// on name. It is deliberately state-preserving: Fleet never returns a value, so
// the prior state is the only record of it and must survive the refresh
// untouched. Nulling it here would erase the practitioner's secret from state
// and make the next plan look like a fresh create.
func (r *CustomVariableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CustomVariableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	tflog.Debug(ctx, "Reading FleetDM custom variable", map[string]interface{}{"name": name})

	found, err := r.client.FindCustomVariableByName(ctx, name)
	if err != nil {
		if isNotFound(err) {
			// A 404 from the collection endpoint means the route is absent, not
			// the variable. Removing the resource from state here would hide an
			// unsupported-server problem behind a spurious "recreate" plan.
			resp.Diagnostics.AddError(customVariablesUnsupportedSummary, customVariablesUnsupportedDetail(err))
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading FleetDM Custom Variable",
			fmt.Sprintf("Could not read custom variable %q: %s", name, err.Error()),
		)
		return
	}

	// An empty result from a successful list is the real "deleted" signal.
	if found == nil {
		tflog.Warn(ctx, "FleetDM custom variable no longer exists, removing from state", map[string]interface{}{"name": name})
		resp.State.RemoveResource(ctx)
		return
	}

	data.ID = types.Int64Value(int64(found.ID))
	data.CreatedAt = timestampOrNull(found.CreatedAt)
	data.UpdatedAt = timestampOrNull(found.UpdatedAt)
	data.ValueWO = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update pushes a new value for an existing custom variable.
//
// Fleet has no PATCH/PUT on /custom_variables/{id}; the only in-place write is
// the spec upsert, which the client wraps. Destroy-and-recreate is not a viable
// alternative: Fleet refuses (409) to delete a custom variable that is still
// referenced by a script or profile, which is the normal state for any variable
// that is actually doing something.
func (r *CustomVariableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan CustomVariableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	var state CustomVariableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	value := resolveCustomVariableValue(ctx, req.Config, plan.Value, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	tflog.Debug(ctx, "Updating FleetDM custom variable value", map[string]interface{}{"name": name})

	if err := r.client.UpsertCustomVariable(ctx, name, value); err != nil {
		resp.Diagnostics.AddError(
			"Error Updating FleetDM Custom Variable",
			fmt.Sprintf("Could not update custom variable %q: %s", name, err.Error()),
		)
		return
	}

	// Re-read to pick up the new updated_at and to confirm the upsert landed on
	// the variable this resource owns rather than re-creating a deleted one under
	// a new id.
	found, err := r.client.FindCustomVariableByName(ctx, name)
	if err != nil {
		if isNotFound(err) {
			resp.Diagnostics.AddError(customVariablesUnsupportedSummary, customVariablesUnsupportedDetail(err))
			return
		}
		// Unlike Create, erroring here loses nothing: the resource is already in
		// state, so Terraform keeps the prior state and the next apply repeats the
		// upsert, which is idempotent. Prior state briefly records the superseded
		// value, and re-running converges.
		resp.Diagnostics.AddError(
			"Error Reading FleetDM Custom Variable After Update",
			fmt.Sprintf(
				"The new value was written to Fleet, but reading %q back to record its timestamps failed. Terraform state still "+
					"records the previous value; re-run to converge (writing the value again is idempotent).\n\nFleet reported: %s",
				name, err.Error(),
			),
		)
		return
	}
	if found == nil {
		resp.Diagnostics.AddError(
			"Custom Variable Missing After Update",
			fmt.Sprintf("Fleet accepted the update but no custom variable named %q exists. It was probably deleted concurrently; re-run to recreate it.", name),
		)
		return
	}
	if !state.ID.IsNull() && int64(found.ID) != state.ID.ValueInt64() {
		resp.Diagnostics.AddError(
			"Custom Variable Recreated Outside Terraform",
			fmt.Sprintf(
				"Custom variable %q now has id %d, but Terraform state records id %d — it was deleted and recreated outside Terraform. "+
					"The new value has been written. Refresh state (`terraform apply -refresh-only`) to adopt the new id.",
				name, found.ID, state.ID.ValueInt64(),
			),
		)
		return
	}

	plan.ID = types.Int64Value(int64(found.ID))
	plan.CreatedAt = timestampOrNull(found.CreatedAt)
	plan.UpdatedAt = timestampOrNull(found.UpdatedAt)
	plan.ValueWO = types.StringNull()

	tflog.Info(ctx, "Updated FleetDM custom variable value", map[string]interface{}{
		"name": name,
		"id":   found.ID,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the resource and clears the Terraform state.
func (r *CustomVariableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CustomVariableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	id := int(data.ID.ValueInt64())
	tflog.Debug(ctx, "Deleting FleetDM custom variable", map[string]interface{}{"name": name, "id": id})

	if err := r.client.DeleteCustomVariable(ctx, id); err != nil {
		if isNotFound(err) {
			// Already gone — nothing to do, and the framework drops the resource
			// from state once Delete returns without error.
			return
		}
		if isConflict(err) {
			resp.Diagnostics.AddError(
				"Custom Variable Still In Use",
				fmt.Sprintf(
					"Fleet refuses to delete custom variable %q while it is still referenced by a script, configuration profile or "+
						"declaration. Remove or edit the referencing object first, then destroy this resource.\n\nFleet reported: %s",
					name, err.Error(),
				),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting FleetDM Custom Variable",
			fmt.Sprintf("Could not delete custom variable %q: %s", name, err.Error()),
		)
	}
}

// ImportState imports an existing custom variable by name.
//
// The value cannot be imported — Fleet never returns it — so the imported state
// carries a null `value`. On the next plan Terraform sees the configured value
// as a change and pushes it in place, which is exactly the intended adopt-then-
// converge flow. With `value_wo` there is nothing for Terraform to diff, so set
// or increment `value_wo_version` after importing to push a known value.
func (r *CustomVariableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	name := strings.TrimSpace(req.ID)

	tflog.Debug(ctx, "Importing FleetDM custom variable", map[string]interface{}{"name": name})

	if strings.HasPrefix(name, reservedCustomVariableNamePrefix) {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf(
				"The import ID %q starts with the reserved prefix %q, which this provider does not manage. "+
					"Fleet adds that prefix when resolving a reference, so a stored name that also carries it cannot be referenced "+
					"unambiguously. Recreate the variable in Fleet without the prefix.",
				name, reservedCustomVariableNamePrefix,
			),
		)
		return
	}
	if !customVariableNameFormat.MatchString(name) {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf(
				"Custom variables are imported by name, and %q is not a valid Fleet custom variable name "+
					"(uppercase letters, digits and underscores only, 1-255 characters).",
				name,
			),
		)
		return
	}

	found, err := r.client.FindCustomVariableByName(ctx, name)
	if err != nil {
		if isNotFound(err) {
			resp.Diagnostics.AddError(customVariablesUnsupportedSummary, customVariablesUnsupportedDetail(err))
			return
		}
		resp.Diagnostics.AddError(
			"Error Importing FleetDM Custom Variable",
			fmt.Sprintf("Could not list custom variables to find %q: %s", name, err.Error()),
		)
		return
	}
	if found == nil {
		resp.Diagnostics.AddError(
			"Custom Variable Not Found",
			fmt.Sprintf("No custom variable named %q exists in Fleet. Import custom variables by name, not by id.", name),
		)
		return
	}

	data := CustomVariableResourceModel{
		ID:        types.Int64Value(int64(found.ID)),
		Name:      types.StringValue(found.Name),
		CreatedAt: timestampOrNull(found.CreatedAt),
		UpdatedAt: timestampOrNull(found.UpdatedAt),
		// Fleet does not return values, so there is nothing to import here.
		Value:          types.StringNull(),
		ValueWO:        types.StringNull(),
		ValueWOVersion: types.Int64Null(),
	}

	tflog.Info(ctx, "Imported FleetDM custom variable", map[string]interface{}{
		"name": found.Name,
		"id":   found.ID,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// refreshTimestamps fills created_at/updated_at from the API. Fleet's create
// response carries only the id and name, so the timestamps require a follow-up
// list call.
//
// It deliberately never raises an error diagnostic. It runs only after the
// variable already exists in Fleet, and an error would abort Create before state
// was written, orphaning the secret. A failed read-back is degraded information,
// not a failed apply: the timestamps are left null and the next refresh fills
// them in.
func (r *CustomVariableResource) refreshTimestamps(ctx context.Context, data *CustomVariableResourceModel, diags *diag.Diagnostics) {
	name := data.Name.ValueString()

	found, err := r.client.FindCustomVariableByName(ctx, name)
	if err != nil {
		if isNotFound(err) {
			diags.AddWarning(customVariablesUnsupportedSummary, customVariablesUnsupportedDetail(err))
		} else {
			diags.AddWarning(
				"Custom Variable Created But Not Read Back",
				fmt.Sprintf(
					"Fleet created custom variable %q, but reading it back to record its timestamps failed. The variable is saved "+
						"in Terraform state so it stays managed; created_at and updated_at are unset until the next refresh.\n\n"+
						"Fleet reported: %s",
					name, err.Error(),
				),
			)
		}
		data.CreatedAt = types.StringNull()
		data.UpdatedAt = types.StringNull()
		return
	}
	if found == nil {
		// Should not happen; keep the computed attributes known rather than
		// leaving them unknown, which the framework rejects.
		diags.AddWarning(
			"Custom Variable Not Listed After Create",
			fmt.Sprintf("Fleet created custom variable %q but did not list it back; timestamps are unavailable.", name),
		)
		data.CreatedAt = types.StringNull()
		data.UpdatedAt = types.StringNull()
		return
	}

	data.ID = types.Int64Value(int64(found.ID))
	data.CreatedAt = timestampOrNull(found.CreatedAt)
	data.UpdatedAt = timestampOrNull(found.UpdatedAt)
}

// timestampOrNull maps an absent API timestamp to a null value rather than an
// empty string, so state does not distinguish "" from "not reported".
func timestampOrNull(ts string) types.String {
	if ts == "" {
		return types.StringNull()
	}
	return types.StringValue(ts)
}
