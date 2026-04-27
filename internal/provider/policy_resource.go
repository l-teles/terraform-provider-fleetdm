package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                   = &PolicyResource{}
	_ resource.ResourceWithImportState    = &PolicyResource{}
	_ resource.ResourceWithValidateConfig = &PolicyResource{}
)

// NewPolicyResource creates a new policy resource.
func NewPolicyResource() resource.Resource {
	return &PolicyResource{}
}

// PolicyResource defines the resource implementation.
type PolicyResource struct {
	client *fleetdm.Client
}

// Attribute type maps for the computed nested echo objects returned by the
// Fleet API. Defined once so they can be reused by the data source.
var (
	policyInstallSoftwareAttrTypes = map[string]attr.Type{
		"name":              types.StringType,
		"software_title_id": types.Int64Type,
	}
	policyRunScriptAttrTypes = map[string]attr.Type{
		"name": types.StringType,
		"id":   types.Int64Type,
	}
	policyPatchSoftwareAttrTypes = map[string]attr.Type{
		"name":              types.StringType,
		"display_name":      types.StringType,
		"software_title_id": types.Int64Type,
	}
)

// PolicyResourceModel describes the resource data model.
type PolicyResourceModel struct {
	ID                             types.Int64  `tfsdk:"id"`
	Name                           types.String `tfsdk:"name"`
	Description                    types.String `tfsdk:"description"`
	Query                          types.String `tfsdk:"query"`
	Critical                       types.Bool   `tfsdk:"critical"`
	Resolution                     types.String `tfsdk:"resolution"`
	Platform                       types.List   `tfsdk:"platform"`
	TeamID                         types.Int64  `tfsdk:"team_id"`
	Type                           types.String `tfsdk:"type"`
	PatchSoftwareTitleID           types.Int64  `tfsdk:"patch_software_title_id"`
	SoftwareTitleID                types.Int64  `tfsdk:"software_title_id"`
	ScriptID                       types.Int64  `tfsdk:"script_id"`
	LabelsIncludeAny               types.List   `tfsdk:"labels_include_any"`
	LabelsExcludeAny               types.List   `tfsdk:"labels_exclude_any"`
	CalendarEventsEnabled          types.Bool   `tfsdk:"calendar_events_enabled"`
	ConditionalAccessEnabled       types.Bool   `tfsdk:"conditional_access_enabled"`
	ConditionalAccessBypassEnabled types.Bool   `tfsdk:"conditional_access_bypass_enabled"`
	AuthorID                       types.Int64  `tfsdk:"author_id"`
	AuthorName                     types.String `tfsdk:"author_name"`
	AuthorEmail                    types.String `tfsdk:"author_email"`
	PassingHostCount               types.Int64  `tfsdk:"passing_host_count"`
	FailingHostCount               types.Int64  `tfsdk:"failing_host_count"`
	FleetMaintained                types.Bool   `tfsdk:"fleet_maintained"`
	CreatedAt                      types.String `tfsdk:"created_at"`
	UpdatedAt                      types.String `tfsdk:"updated_at"`
	HostCountUpdatedAt             types.String `tfsdk:"host_count_updated_at"`
	InstallSoftware                types.Object `tfsdk:"install_software"`
	RunScript                      types.Object `tfsdk:"run_script"`
	PatchSoftware                  types.Object `tfsdk:"patch_software"`
}

func (r *PolicyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_policy"
}

func (r *PolicyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a FleetDM policy. Policies are yes/no questions that define compliance checks for hosts.\n\n" +
			"**Note for users upgrading from older provider versions:** if you have a policy whose `script_id`, `software_title_id`, labels, or calendar/conditional-access settings were configured directly in the Fleet UI before this provider supported them, the next `terraform plan` after upgrading will show a diff that proposes to clear those values. Add the relevant fields to your HCL before applying if you want to preserve them.",

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
				MarkdownDescription: "Whether the policy is critical. Critical policies are highlighted in the UI. _Available in Fleet Premium._",
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
				MarkdownDescription: "The ID of the team this policy belongs to. If not specified, the policy is global. The team-only fields below (`type`, `software_title_id`, `script_id`, `calendar_events_enabled`, `conditional_access_enabled`, `conditional_access_bypass_enabled`) require this to be set.",
			},
			"type": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("dynamic"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				MarkdownDescription: "The type of the policy. One of `dynamic` (classic policy with an editable query) or `patch` (tied to `patch_software_title_id` and automatically updated to include the newest Fleet-maintained app version). Immutable after create — changing this triggers a replacement. _Available in Fleet Premium, team policies only._",
			},
			"patch_software_title_id": schema.Int64Attribute{
				Optional: true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				MarkdownDescription: "ID of the Fleet-maintained software title to create a patch policy for. Required when `type = \"patch\"`. Immutable after create — changing this triggers a replacement. _Available in Fleet Premium, team policies only._",
			},
			"software_title_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "ID of the software title to install if the policy fails. Set to `null` to clear the install-software automation. _Available in Fleet Premium, team policies only._",
			},
			"script_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "ID of the script to run if the policy fails. Set to `null` to clear the run-script automation. _Available in Fleet Premium, team policies only._",
			},
			"labels_include_any": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Target only hosts that have any of the specified labels. Mutually exclusive with `labels_exclude_any`. _Available in Fleet Premium._",
			},
			"labels_exclude_any": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Target only hosts that do not have any of the specified labels. Mutually exclusive with `labels_include_any`. _Available in Fleet Premium._",
			},
			"calendar_events_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether to trigger calendar events when the policy is failing. Only applies to team policies; setting this is a no-op on global policies. _Available in Fleet Premium._",
			},
			"conditional_access_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether to block single sign-on for end users whose hosts fail this policy. Only applies to team policies. _Available in Fleet Premium._",
			},
			"conditional_access_bypass_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Allow end users to bypass conditional access for this policy for a single Okta login. Ignored when `conditional_access_enabled` is `false`, when Okta conditional access is not configured, or when bypass is disabled in org settings. _Available in Fleet Premium._",
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
				MarkdownDescription: "Whether the policy is maintained by Fleet (Fleet-maintained policies cannot be edited).",
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
				MarkdownDescription: "Echo of the install-software automation attached to this policy. Populated by the Fleet API; mirror of `software_title_id` with the human-readable software name.",
				Attributes: map[string]schema.Attribute{
					"name":              schema.StringAttribute{Computed: true},
					"software_title_id": schema.Int64Attribute{Computed: true},
				},
			},
			"run_script": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Echo of the run-script automation attached to this policy. Populated by the Fleet API; mirror of `script_id` with the human-readable script name.",
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{Computed: true},
					"id":   schema.Int64Attribute{Computed: true},
				},
			},
			"patch_software": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Echo of the patch-software target for `type = \"patch\"` policies. Populated by the Fleet API.",
				Attributes: map[string]schema.Attribute{
					"name":              schema.StringAttribute{Computed: true},
					"display_name":      schema.StringAttribute{Computed: true},
					"software_title_id": schema.Int64Attribute{Computed: true},
				},
			},
		},
	}
}

func (r *PolicyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics, "Resource")
}

// ValidateConfig enforces:
//   - `type` must be one of "dynamic" or "patch".
//   - `labels_include_any` and `labels_exclude_any` cannot both be set.
//   - When `type = "patch"`, `patch_software_title_id` must be set.
func (r *PolicyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data PolicyResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !data.Type.IsNull() && !data.Type.IsUnknown() {
		switch data.Type.ValueString() {
		case "", "dynamic", "patch":
		default:
			resp.Diagnostics.AddAttributeError(
				path.Root("type"),
				"Invalid type",
				fmt.Sprintf("type must be one of \"dynamic\" or \"patch\", got: %q", data.Type.ValueString()),
			)
		}
	}

	includeSet := !data.LabelsIncludeAny.IsNull() && !data.LabelsIncludeAny.IsUnknown() && len(data.LabelsIncludeAny.Elements()) > 0
	excludeSet := !data.LabelsExcludeAny.IsNull() && !data.LabelsExcludeAny.IsUnknown() && len(data.LabelsExcludeAny.Elements()) > 0
	if includeSet && excludeSet {
		resp.Diagnostics.AddAttributeError(
			path.Root("labels_exclude_any"),
			"Conflicting label selectors",
			"Only one of labels_include_any or labels_exclude_any can be set on a policy.",
		)
	}

	if data.Type.ValueString() == "patch" {
		if data.PatchSoftwareTitleID.IsNull() || data.PatchSoftwareTitleID.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("patch_software_title_id"),
				"Missing required value",
				"patch_software_title_id is required when type = \"patch\".",
			)
		}
	}
}

func (r *PolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := fleetdm.CreatePolicyRequest{
		Name:                 data.Name.ValueString(),
		Description:          data.Description.ValueString(),
		Query:                data.Query.ValueString(),
		Critical:             data.Critical.ValueBool(),
		Resolution:           data.Resolution.ValueString(),
		Platform:             platformListToString(ctx, data.Platform),
		Type:                 data.Type.ValueString(),
		PatchSoftwareTitleID: optionalIntPtr(data.PatchSoftwareTitleID),
		SoftwareTitleID:      optionalIntPtr(data.SoftwareTitleID),
		ScriptID:             optionalIntPtr(data.ScriptID),
		LabelsIncludeAny:     stringListToSlice(ctx, data.LabelsIncludeAny),
		LabelsExcludeAny:     stringListToSlice(ctx, data.LabelsExcludeAny),
	}

	policy, err := r.client.CreatePolicy(ctx, optionalIntPtr(data.TeamID), createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error Creating FleetDM Policy", fmt.Sprintf("Unable to create policy: %s", err))
		return
	}

	// calendar_events_enabled, conditional_access_enabled, and
	// conditional_access_bypass_enabled are not accepted by the Create
	// endpoint — they can only be set via PATCH. If the user planned any
	// non-default value for these on a team policy, follow up with an
	// immediate Update so the resource ends up in the planned state in a
	// single apply.
	if isTeamPolicy(data.TeamID) && policyNeedsAutomationFollowup(data) {
		createdID := policy.ID
		updated, updateErr := r.client.UpdatePolicy(ctx, createdID, optionalIntPtr(data.TeamID), buildPolicyUpdateRequest(ctx, data))
		if updateErr != nil {
			resp.Diagnostics.AddError("Error Applying Policy Automation Settings",
				fmt.Sprintf("Policy was created (id=%d) but applying calendar/conditional-access settings via PATCH failed: %s", createdID, updateErr))
			return
		}
		policy = updated
	}

	r.mapPolicyToModel(ctx, policy, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

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

	r.mapPolicyToModel(ctx, policy, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policy, err := r.client.UpdatePolicy(ctx, int(data.ID.ValueInt64()), optionalIntPtr(data.TeamID), buildPolicyUpdateRequest(ctx, data))
	if err != nil {
		resp.Diagnostics.AddError("Error Updating FleetDM Policy", fmt.Sprintf("Unable to update policy: %s", err))
		return
	}

	r.mapPolicyToModel(ctx, policy, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

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

// mapPolicyToModel copies an API Policy into the Terraform model. Pulls the
// flat-input fields (script_id, software_title_id) out of the nested response
// objects (run_script, install_software) so users can manage them as scalars.
func (r *PolicyResource) mapPolicyToModel(ctx context.Context, policy *fleetdm.Policy, data *PolicyResourceModel, diags *diag.Diagnostics) {
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

	data.Type = types.StringValue(policy.Type)
	data.LabelsIncludeAny = stringSliceToList(policy.LabelsIncludeAny)
	data.LabelsExcludeAny = stringSliceToList(policy.LabelsExcludeAny)
	data.CalendarEventsEnabled = types.BoolValue(policy.CalendarEventsEnabled)
	data.ConditionalAccessEnabled = types.BoolValue(policy.ConditionalAccessEnabled)
	// Fleet doesn't echo conditional_access_bypass_enabled in the response;
	// the planner-supplied value (config or default) is left unchanged.

	data.FleetMaintained = types.BoolValue(policy.FleetMaintained)
	data.CreatedAt = types.StringValue(policy.CreatedAt)
	data.UpdatedAt = types.StringValue(policy.UpdatedAt)
	data.HostCountUpdatedAt = types.StringValue(policy.HostCountUpdatedAt)

	data.SoftwareTitleID, data.InstallSoftware = mapInstallSoftware(policy.InstallSoftware, diags)
	data.ScriptID, data.RunScript = mapRunScript(policy.RunScript, diags)
	data.PatchSoftwareTitleID, data.PatchSoftware = mapPatchSoftware(policy.PatchSoftware, diags)
}

// stringListToSlice converts a types.List of strings to a []string, returning
// nil when the list is null/unknown. nil is significant here: it lets the
// JSON marshaler emit `null` for the no-omitempty Update fields when the
// user has cleared a labels list.
func stringListToSlice(ctx context.Context, list types.List) []string {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	out := make([]string, 0, len(list.Elements()))
	list.ElementsAs(ctx, &out, false)
	return out
}

// stringSliceToList converts a []string from the API into a types.List,
// returning ListNull when the slice is empty/nil so HCL omitting the field
// produces no diff against the API's "no labels" response.
func stringSliceToList(s []string) types.List {
	if len(s) == 0 {
		return types.ListNull(types.StringType)
	}
	values := make([]attr.Value, 0, len(s))
	for _, v := range s {
		values = append(values, types.StringValue(v))
	}
	return types.ListValueMust(types.StringType, values)
}

// isTeamPolicy returns true if the model's team_id is set to a positive value.
func isTeamPolicy(teamID types.Int64) bool {
	return !teamID.IsNull() && !teamID.IsUnknown() && teamID.ValueInt64() > 0
}

// policyNeedsAutomationFollowup is true when the planned model has any
// PATCH-only field set to a non-default value, requiring a follow-up Update
// after Create.
func policyNeedsAutomationFollowup(data PolicyResourceModel) bool {
	if !data.CalendarEventsEnabled.IsNull() && data.CalendarEventsEnabled.ValueBool() {
		return true
	}
	if !data.ConditionalAccessEnabled.IsNull() && data.ConditionalAccessEnabled.ValueBool() {
		return true
	}
	// The API default for bypass is true. Only follow up if user explicitly
	// set it to false.
	if !data.ConditionalAccessBypassEnabled.IsNull() && !data.ConditionalAccessBypassEnabled.ValueBool() {
		return true
	}
	return false
}

// buildPolicyUpdateRequest builds an UpdatePolicyRequest from the planned
// model. Fields that the API treats as "send null to clear" use pointers
// without omitempty (see UpdatePolicyRequest doc comment).
func buildPolicyUpdateRequest(ctx context.Context, data PolicyResourceModel) fleetdm.UpdatePolicyRequest {
	return fleetdm.UpdatePolicyRequest{
		Name:                           data.Name.ValueString(),
		Description:                    data.Description.ValueString(),
		Query:                          data.Query.ValueString(),
		Critical:                       data.Critical.ValueBool(),
		Resolution:                     data.Resolution.ValueString(),
		Platform:                       platformListToString(ctx, data.Platform),
		SoftwareTitleID:                optionalIntPtr(data.SoftwareTitleID),
		ScriptID:                       optionalIntPtr(data.ScriptID),
		CalendarEventsEnabled:          optionalBoolPtr(data.CalendarEventsEnabled),
		ConditionalAccessEnabled:       optionalBoolPtr(data.ConditionalAccessEnabled),
		ConditionalAccessBypassEnabled: optionalBoolPtr(data.ConditionalAccessBypassEnabled),
		LabelsIncludeAny:               stringListToSlice(ctx, data.LabelsIncludeAny),
		LabelsExcludeAny:               stringListToSlice(ctx, data.LabelsExcludeAny),
	}
}

// mapInstallSoftware extracts the flat software_title_id from the nested
// install_software response and builds the matching computed object.
func mapInstallSoftware(s *fleetdm.PolicyAutomationSoftware, diags *diag.Diagnostics) (types.Int64, types.Object) {
	if s == nil {
		return types.Int64Null(), types.ObjectNull(policyInstallSoftwareAttrTypes)
	}
	obj, dd := types.ObjectValue(policyInstallSoftwareAttrTypes, map[string]attr.Value{
		"name":              types.StringValue(s.Name),
		"software_title_id": types.Int64Value(int64(s.SoftwareTitleID)),
	})
	diags.Append(dd...)
	return types.Int64Value(int64(s.SoftwareTitleID)), obj
}

// mapRunScript extracts the flat script_id from the nested run_script
// response and builds the matching computed object.
func mapRunScript(s *fleetdm.PolicyAutomationScript, diags *diag.Diagnostics) (types.Int64, types.Object) {
	if s == nil {
		return types.Int64Null(), types.ObjectNull(policyRunScriptAttrTypes)
	}
	obj, dd := types.ObjectValue(policyRunScriptAttrTypes, map[string]attr.Value{
		"name": types.StringValue(s.Name),
		"id":   types.Int64Value(int64(s.ID)),
	})
	diags.Append(dd...)
	return types.Int64Value(int64(s.ID)), obj
}

// mapPatchSoftware extracts the flat patch_software_title_id from the nested
// patch_software response and builds the matching computed object.
func mapPatchSoftware(s *fleetdm.PolicyAutomationPatchSoftware, diags *diag.Diagnostics) (types.Int64, types.Object) {
	if s == nil {
		return types.Int64Null(), types.ObjectNull(policyPatchSoftwareAttrTypes)
	}
	obj, dd := types.ObjectValue(policyPatchSoftwareAttrTypes, map[string]attr.Value{
		"name":              types.StringValue(s.Name),
		"display_name":      types.StringValue(s.DisplayName),
		"software_title_id": types.Int64Value(int64(s.SoftwareTitleID)),
	})
	diags.Append(dd...)
	return types.Int64Value(int64(s.SoftwareTitleID)), obj
}

// optionalBoolPtr converts an optional types.Bool to a *bool, returning nil
// for null/unknown so the JSON marshaler emits `null` (per the no-omitempty
// design on UpdatePolicyRequest).
func optionalBoolPtr(val types.Bool) *bool {
	if val.IsNull() || val.IsUnknown() {
		return nil
	}
	v := val.ValueBool()
	return &v
}
