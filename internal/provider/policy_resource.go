package provider

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

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
	LabelsIncludeAny               types.Set    `tfsdk:"labels_include_any"`
	LabelsExcludeAny               types.Set    `tfsdk:"labels_exclude_any"`
	LabelsIncludeAll               types.Set    `tfsdk:"labels_include_all"`
	LabelsExcludeAll               types.Set    `tfsdk:"labels_exclude_all"`
	CalendarEventsEnabled          types.Bool   `tfsdk:"calendar_events_enabled"`
	ConditionalAccessEnabled       types.Bool   `tfsdk:"conditional_access_enabled"`
	ConditionalAccessBypassEnabled types.Bool   `tfsdk:"conditional_access_bypass_enabled"`
	ContinuousAutomationsEnabled   types.Bool   `tfsdk:"continuous_automations_enabled"`
	PatchWhenClosed                types.Bool   `tfsdk:"patch_when_closed"`
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
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					// No static "" default, and state is reused when the
					// attribute is unset: Fleet generates a description for
					// type = "patch" policies, and a planned "" made the applied
					// value differ from the planned one, so Terraform failed
					// every patch-policy apply with "Provider produced
					// inconsistent result after apply". Mirrors `query`, which
					// Fleet also generates for patch policies.
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: "A description of the policy. Fleet generates one for `type = \"patch\"` policies, so leaving it unset on a patch policy adopts Fleet's text. Once set, removing the attribute keeps the stored value rather than clearing it.",
			},
			"query": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: "The SQL query that defines the policy. The policy passes if the query returns results. Required when `type = \"dynamic\"` (the default). Must be omitted when `type = \"patch\"` — Fleet generates the query automatically for patch policies.",
			},
			"critical": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether the policy is critical. Critical policies are highlighted in the UI. _Available in Fleet Premium._",
			},
			"resolution": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					// Same reasoning as description: Fleet generates a
					// resolution for patch policies.
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: "Instructions for resolving a failing policy check. Fleet generates them for `type = \"patch\"` policies, so leaving this unset on a patch policy adopts Fleet's text rather than an empty string.\n\n**Fleet API limitation:** once set to a non-empty value, `resolution` cannot be cleared via the API. Setting it to `\"\"` after the fact will appear as drift on every plan and never converge — destroy and recreate the policy if you need to remove the resolution.",
			},
			"platform": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of platforms this policy applies to (`darwin`, `linux`, `windows`, `chrome`). Empty list means all platforms. Must be omitted when `type = \"patch\"` — Fleet (4.84+) derives the effective platform from the patch target and rejects an explicit `platform`.\n\n**Fleet API limitation:** once set to a non-empty list, `platform` cannot be cleared via the API. The provider will plan a destroy+recreate when a previously-set list is changed back to an empty list — subset shrinks and value swaps (e.g. `[\"darwin\",\"linux\"]` → `[\"darwin\"]`, `[\"darwin\"]` → `[\"linux\"]`) stay in-place.",
				PlanModifiers: []planmodifier.List{
					requireReplaceOnPlatformShrink(),
				},
			},
			"team_id": schema.Int64Attribute{
				Optional: true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				MarkdownDescription: "The ID of the team this policy belongs to. If not specified, the policy is global. Changing this field forces the policy to be destroyed and recreated — Fleet stores team and global policies under separate endpoints, so a policy cannot be moved in-place. The team-only fields below (`type` = `\"patch\"`, `patch_software_title_id`, `software_title_id`, `script_id`, `calendar_events_enabled`, `conditional_access_enabled`, `conditional_access_bypass_enabled`) require this to be set.",
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
				Optional: true,
				MarkdownDescription: "ID of the software title to install when this policy fails. Wires Fleet's `install_software` policy automation to a software title — this is the **canonical place to attach an install-software policy to a software title from Terraform**, because Fleet's API models the relationship on the policy side (the `fleetdm_software_custom_package` / `fleetdm_software_app_store_app` / `fleetdm_software_fleet_maintained_app` resources expose the resulting attachments through their read-only `automatic_install_policies` attribute, but do not accept them as input). " +
					"Typically referenced as `fleetdm_software_custom_package.<name>.title_id`. " +
					"Set to `null` to clear the install-software automation. " +
					"Note: when a software resource is created with `automatic_install_policy = true`, Fleet mints its own policy under the hood — that policy is not managed by Terraform and should not be re-declared here. " +
					"_Available in Fleet Premium, team policies only._",
			},
			"script_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "ID of the script to run if the policy fails. Set to `null` to clear the run-script automation. _Available in Fleet Premium, team policies only._",
			},
			"labels_include_any": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Target only hosts that have any of the specified labels. Mutually exclusive with `labels_include_all`. Order-insensitive. _Available in Fleet Premium._",
			},
			"labels_exclude_any": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Target only hosts that do not have any of the specified labels. Mutually exclusive with `labels_exclude_all`. Order-insensitive. _Available in Fleet Premium._",
			},
			"labels_include_all": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Target only hosts that have all of the specified labels. Mutually exclusive with `labels_include_any`. Order-insensitive. _Available in Fleet Premium 4.90+._",
			},
			"labels_exclude_all": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Exclude hosts that have all of the specified labels. Mutually exclusive with `labels_exclude_any`. Order-insensitive. _Available in Fleet Premium 4.90+._",
			},
			"calendar_events_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether to trigger calendar events when the policy is failing. Requires `team_id` — only supported on team policies. _Available in Fleet Premium._",
			},
			"conditional_access_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether to block single sign-on for end users whose hosts fail this policy. Requires `team_id` — only supported on team policies. _Available in Fleet Premium._",
			},
			"conditional_access_bypass_enabled": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Allow end users to bypass conditional access for this policy for a single Okta login. Ignored when `conditional_access_enabled` is `false`, when Okta conditional access is not configured, or when bypass is disabled in org settings. When unset, Fleet's default of `true` applies. Requires `team_id` — only supported on team policies. _Available in Fleet Premium._",
			},
			"continuous_automations_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether the policy's automations re-run on every failing check instead of only on the transition into failing. Requires `team_id` — Fleet rejects the field on global policies. _Available in Fleet Premium 4.90+._",
			},
			"patch_when_closed": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Only patch the app on a host while it is not running. Requires `type = \"patch\"`, " +
					"`continuous_automations_enabled = true`, and `team_id` — Fleet silently drops the field on a global policy. " +
					"_Available in Fleet Premium 4.91+._\n\n" +
					"~> **Interaction with the installer resource:** Fleet implements this by writing a managed `pre_install_query` onto the " +
					"patch target's installer. If that installer is managed by `fleetdm_software_fleet_maintained_app`, " +
					"`fleetdm_software_custom_package` or `fleetdm_software_app_store_app` without a `pre_install_query` of its own, " +
					"the next plan will show that query as drift and clear it, turning the behaviour off again. " +
					"Until the installer resources learn to leave Fleet's managed query alone, either keep the installer outside Terraform " +
					"or expect to reconcile the two on every apply.",
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

// policyLabelSelectorConfig pairs one of the four label-scoping attributes
// with its configured value, so ValidateConfig can iterate them.
type policyLabelSelectorConfig struct {
	attrPath string
	value    types.Set
}

// knownStringSetValues returns the decided string elements of a set. A null or
// unknown set yields nothing, as do individual unknown elements: cross-
// attribute checks can only be enforced against values that are already known
// at plan time, and anything else defers to the API's own validation.
func knownStringSetValues(set types.Set) []string {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}
	out := make([]string, 0, len(set.Elements()))
	for _, elem := range set.Elements() {
		str, ok := elem.(types.String)
		if !ok || str.IsNull() || str.IsUnknown() {
			continue
		}
		out = append(out, str.ValueString())
	}
	return out
}

// ValidateConfig enforces:
//   - `type` must be one of "dynamic" or "patch".
//   - At most one "include" label selector and at most one "exclude" label
//     selector; an include and an exclude selector may be combined, but no
//     single label may appear on both sides.
//   - When `type = "patch"`: `patch_software_title_id` and `team_id` are required.
//   - `patch_software_title_id` is only meaningful when `type = "patch"`.
//   - Team-only fields (`script_id`, `software_title_id`, the calendar/CA
//     toggles, `continuous_automations_enabled`) require `team_id` to be set.
//
// Catching these at plan time (instead of letting the API reject them at
// apply time) saves users a wasted apply cycle and produces clearer errors.
func (r *PolicyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data PolicyResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !data.Type.IsNull() && !data.Type.IsUnknown() {
		switch data.Type.ValueString() {
		case "dynamic", "patch":
		default:
			resp.Diagnostics.AddAttributeError(
				path.Root("type"),
				"Invalid type",
				fmt.Sprintf("type must be one of \"dynamic\" or \"patch\", got: %q. Omit the attribute to use the default (\"dynamic\").", data.Type.ValueString()),
			)
		}
	}

	// Fleet 4.90 accepts one "include" selector alongside one "exclude"
	// selector, but rejects two selectors of the same direction with
	// "policy can include at most one of labels_include_any or
	// labels_include_all". Mirror that rule so users get the error at plan
	// time.
	includeSelectors := []policyLabelSelectorConfig{
		{"labels_include_any", data.LabelsIncludeAny},
		{"labels_include_all", data.LabelsIncludeAll},
	}
	excludeSelectors := []policyLabelSelectorConfig{
		{"labels_exclude_any", data.LabelsExcludeAny},
		{"labels_exclude_all", data.LabelsExcludeAll},
	}
	labelSet := func(s types.Set) bool {
		return !s.IsNull() && !s.IsUnknown() && len(s.Elements()) > 0
	}
	for _, pair := range [][]policyLabelSelectorConfig{includeSelectors, excludeSelectors} {
		if labelSet(pair[0].value) && labelSet(pair[1].value) {
			resp.Diagnostics.AddAttributeError(
				path.Root(pair[1].attrPath),
				"Conflicting label selectors",
				fmt.Sprintf("Only one of %s or %s can be set on a policy.", pair[0].attrPath, pair[1].attrPath),
			)
		}
	}

	// Reject explicit empty sets — Fleet's API maps "no labels" to a null
	// response, which we surface as SetNull. An explicit empty set in HCL
	// would never converge with that null state. Tell users to omit the
	// attribute (or set it to null) instead.
	for _, selector := range slices.Concat(includeSelectors, excludeSelectors) {
		if !selector.value.IsNull() && !selector.value.IsUnknown() && len(selector.value.Elements()) == 0 {
			resp.Diagnostics.AddAttributeError(
				path.Root(selector.attrPath),
				"Empty set not allowed",
				fmt.Sprintf("To clear %s, omit the attribute or set it to null. An explicit empty set causes perpetual drift.", selector.attrPath),
			)
		}
	}

	// Fleet also rejects a single label named on both sides of the
	// include/exclude split with `label %q cannot appear in both an include
	// and an exclude list`. That constraint only became reachable in 4.90,
	// where an include and an exclude selector can be combined at all.
	includedLabels := make(map[string]struct{})
	for _, selector := range includeSelectors {
		for _, name := range knownStringSetValues(selector.value) {
			includedLabels[name] = struct{}{}
		}
	}
	for _, selector := range excludeSelectors {
		for _, name := range knownStringSetValues(selector.value) {
			if _, both := includedLabels[name]; both {
				resp.Diagnostics.AddAttributeError(
					path.Root(selector.attrPath),
					"Label in both an include and an exclude selector",
					fmt.Sprintf("Label %q is named by an include selector as well as by %s. Fleet rejects a policy that both includes and excludes the same label.", name, selector.attrPath),
				)
			}
		}
	}

	// team_id often references a not-yet-created fleetdm_fleet resource,
	// so it can be Unknown at plan time. We can only enforce team-only
	// constraints when team_id is fully known: Null means "definitely a
	// global policy", a known positive int means "definitely a team
	// policy". Unknown values defer to the API's runtime check.
	teamKnown := !data.TeamID.IsUnknown()
	teamSet := teamKnown && !data.TeamID.IsNull() && data.TeamID.ValueInt64() > 0
	// `patch_software_title_id` is required when type = "patch", but the
	// "required" check must only fire when the value is KNOWN-null. Treating
	// Unknown as "not set" rejects valid configurations where the field
	// references another resource's attribute (e.g.,
	// `values(fleetdm_software_fleet_maintained_app.X)[0].id`), because
	// ValidateConfig runs before cross-resource references resolve. Mirrors
	// the existing `query`/`type` "decided vs unknown" pattern below.
	patchTitleDecided := !data.PatchSoftwareTitleID.IsUnknown()
	patchTitleSet := patchTitleDecided && !data.PatchSoftwareTitleID.IsNull()
	// Same rationale for type: only enforce patch-vs-dynamic constraints
	// when type is known. An Unknown type (e.g., referenced from another
	// computed value) might still resolve to "patch" at apply time.
	// Treating Null as "known dynamic" — when the user omits type, the
	// schema default of "dynamic" kicks in at apply time, so we can
	// enforce dynamic-policy constraints already at plan time.
	typeKnown := !data.Type.IsUnknown()
	isPatchType := typeKnown && data.Type.ValueString() == "patch"

	// Three predicates capture the states we care about for query:
	//   - queryDecided: not Unknown (Null counts as "decided to be missing")
	//   - queryConfigured: user wrote query in HCL with any value, including ""
	//   - querySet: user wrote query in HCL with a non-empty value
	// Patch policies must omit query entirely (or set it to null); any
	// configured value, including "", is invalid. Dynamic policies must
	// have query set to a non-empty value.
	queryDecided := !data.Query.IsUnknown()
	queryConfigured := queryDecided && !data.Query.IsNull()
	querySet := queryConfigured && data.Query.ValueString() != ""

	// Same null/unknown semantics as query: "configured" means the user
	// wrote the attribute in HCL (with any value, including an empty list).
	// An empty list still causes perpetual drift on patch policies because
	// Fleet returns its derived platform in the response.
	platformConfigured := !data.Platform.IsUnknown() && !data.Platform.IsNull()

	if isPatchType {
		// Only flag the required-missing error when the value is fully known
		// to be null. Unknown values defer to the API's runtime check.
		if patchTitleDecided && !patchTitleSet {
			resp.Diagnostics.AddAttributeError(
				path.Root("patch_software_title_id"),
				"Missing required value",
				"patch_software_title_id is required when type = \"patch\".",
			)
		}
		if teamKnown && !teamSet {
			resp.Diagnostics.AddAttributeError(
				path.Root("team_id"),
				"Missing required value",
				"team_id is required when type = \"patch\" — patch policies are team-only.",
			)
		}
		if queryConfigured {
			resp.Diagnostics.AddAttributeError(
				path.Root("query"),
				"Unsupported field",
				"query is not supported when type = \"patch\" — Fleet generates the query automatically for patch policies. Omit the attribute (or set it to null); explicit empty strings cause perpetual drift because Fleet returns its generated query.",
			)
		}
		if platformConfigured {
			resp.Diagnostics.AddAttributeError(
				path.Root("platform"),
				"Unsupported field",
				"platform is not supported when type = \"patch\" — Fleet (4.84+) rejects `platform` on patch policies and derives the effective platform from the patch target. Omit the attribute (or set it to null).",
			)
		}
	} else {
		if typeKnown && patchTitleSet {
			resp.Diagnostics.AddAttributeError(
				path.Root("patch_software_title_id"),
				"Unsupported field",
				"patch_software_title_id is only meaningful when type = \"patch\".",
			)
		}
		// For dynamic (default) policies, query is required. Only flag
		// when query is fully known to be empty/null — Unknown values
		// defer to the API.
		if typeKnown && queryDecided && !querySet {
			resp.Diagnostics.AddAttributeError(
				path.Root("query"),
				"Missing required value",
				"query is required for type = \"dynamic\" policies (the default).",
			)
		}
	}

	// patch_when_closed only means anything on a patch policy, and Fleet
	// additionally refuses it unless the policy's automations re-run on every
	// failing check. Both were verified against Fleet 4.91:
	//   - on a non-patch policy: 400 `"patch_when_closed" is only supported for patch policies`
	//   - with continuous automations off: 400 `If "patch_when_closed" is true,
	//     "continuous_automations_enabled" can't be set to false.`
	// Only enforce when the flag is known to be true; Unknown defers to the API.
	patchWhenClosedOn := !data.PatchWhenClosed.IsUnknown() && !data.PatchWhenClosed.IsNull() && data.PatchWhenClosed.ValueBool()
	if patchWhenClosedOn {
		if typeKnown && !isPatchType {
			resp.Diagnostics.AddAttributeError(
				path.Root("patch_when_closed"),
				"Unsupported field",
				"patch_when_closed is only supported when type = \"patch\".",
			)
		}
		if !data.ContinuousAutomationsEnabled.IsUnknown() && !data.ContinuousAutomationsEnabled.ValueBool() {
			resp.Diagnostics.AddAttributeError(
				path.Root("continuous_automations_enabled"),
				"Missing required value",
				"continuous_automations_enabled must be true when patch_when_closed is true.",
			)
		}
	}

	// Team-only fields. Each pair: (model attribute, schema path, "set" predicate).
	type teamOnly struct {
		attrPath string
		isSet    bool
	}
	// "Set" means "explicitly written in HCL" (config is non-null and
	// non-unknown). For booleans, an explicit `false` still counts as
	// set — the user wrote the line, even if the value happens to match
	// the default — so we don't gate on .ValueBool() here.
	checks := []teamOnly{
		{"script_id", !data.ScriptID.IsNull() && !data.ScriptID.IsUnknown()},
		{"software_title_id", !data.SoftwareTitleID.IsNull() && !data.SoftwareTitleID.IsUnknown()},
		{"calendar_events_enabled", !data.CalendarEventsEnabled.IsNull() && !data.CalendarEventsEnabled.IsUnknown()},
		{"conditional_access_enabled", !data.ConditionalAccessEnabled.IsNull() && !data.ConditionalAccessEnabled.IsUnknown()},
		{"conditional_access_bypass_enabled", !data.ConditionalAccessBypassEnabled.IsNull() && !data.ConditionalAccessBypassEnabled.IsUnknown()},
		{"continuous_automations_enabled", !data.ContinuousAutomationsEnabled.IsNull() && !data.ContinuousAutomationsEnabled.IsUnknown()},
		{"patch_when_closed", !data.PatchWhenClosed.IsNull() && !data.PatchWhenClosed.IsUnknown()},
	}
	if teamKnown && !teamSet {
		for _, c := range checks {
			if c.isSet {
				resp.Diagnostics.AddAttributeError(
					path.Root(c.attrPath),
					"team_id required",
					fmt.Sprintf("%s is only supported on team policies — set team_id to use it.", c.attrPath),
				)
			}
		}
	}
}

func (r *PolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Apply-side counterpart of the ValidateConfig deferral: ValidateConfig
	// allows `patch_software_title_id` to be Unknown for type=patch policies
	// so cross-resource references can plan. By Create time the reference
	// has resolved — if it resolved to Null, we surface the friendlier
	// "required" error here instead of letting Fleet's API reject the
	// request with a less actionable 4xx.
	if data.Type.ValueString() == "patch" && data.PatchSoftwareTitleID.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("patch_software_title_id"),
			"Missing required value",
			"patch_software_title_id is required when type = \"patch\". The configured value resolved to null at apply time.",
		)
		return
	}

	createReq := fleetdm.CreatePolicyRequest{
		Name:                         data.Name.ValueString(),
		Description:                  data.Description.ValueString(),
		Query:                        policyQueryForRequest(data),
		Critical:                     data.Critical.ValueBool(),
		Resolution:                   data.Resolution.ValueString(),
		Platform:                     policyPlatformForRequest(ctx, data),
		Type:                         data.Type.ValueString(),
		PatchSoftwareTitleID:         optionalIntPtr(data.PatchSoftwareTitleID),
		SoftwareTitleID:              optionalIntPtr(data.SoftwareTitleID),
		ScriptID:                     optionalIntPtr(data.ScriptID),
		ContinuousAutomationsEnabled: data.ContinuousAutomationsEnabled.ValueBool(),
		PatchWhenClosed:              data.PatchWhenClosed.ValueBool(),
		LabelsIncludeAny:             stringSetToSlice(ctx, data.LabelsIncludeAny, &resp.Diagnostics),
		LabelsExcludeAny:             stringSetToSlice(ctx, data.LabelsExcludeAny, &resp.Diagnostics),
		LabelsIncludeAll:             stringSetToSlice(ctx, data.LabelsIncludeAll, &resp.Diagnostics),
		LabelsExcludeAll:             stringSetToSlice(ctx, data.LabelsExcludeAll, &resp.Diagnostics),
	}
	if resp.Diagnostics.HasError() {
		return
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
		updateReq := buildPolicyUpdateRequest(ctx, data, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		updated, updateErr := r.client.UpdatePolicy(ctx, createdID, optionalIntPtr(data.TeamID), updateReq)
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

	updateReq := buildPolicyUpdateRequest(ctx, data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	policy, err := r.client.UpdatePolicy(ctx, int(data.ID.ValueInt64()), optionalIntPtr(data.TeamID), updateReq)
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

// ImportState supports two formats:
//
//   - "<policy_id>" — global policy (e.g., terraform import fleetdm_policy.foo 42).
//   - "<team_id>:<policy_id>" — team policy (e.g., terraform import fleetdm_policy.foo 7:42).
//
// Without the team_id prefix, Read calls Fleet's global-policy endpoint
// and 404s on team policies. The colon-separated form lets users import
// either kind cleanly.
func (r *PolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	switch len(parts) {
	case 1:
		policyID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid import ID",
				fmt.Sprintf("Could not parse policy ID %q as an integer: %s", parts[0], err),
			)
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), policyID)...)
	case 2:
		teamID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid import ID",
				fmt.Sprintf("Could not parse team ID %q as an integer: %s", parts[0], err),
			)
			return
		}
		policyID, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid import ID",
				fmt.Sprintf("Could not parse policy ID %q as an integer: %s", parts[1], err),
			)
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), policyID)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team_id"), teamID)...)
	default:
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected %q for global policies or %q for team policies, got: %q", "<policy_id>", "<team_id>:<policy_id>", req.ID),
		)
	}
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

	// Fleet's API returns an empty string for `type` on legacy policies and
	// occasionally on global policies. Normalize to the schema default so
	// state matches the planner's "dynamic" default and we don't see
	// perpetual diffs.
	policyType := policy.Type
	if policyType == "" {
		policyType = "dynamic"
	}
	data.Type = types.StringValue(policyType)
	data.LabelsIncludeAny = policyLabelsToSet(policy.LabelsIncludeAny)
	data.LabelsExcludeAny = policyLabelsToSet(policy.LabelsExcludeAny)
	data.LabelsIncludeAll = policyLabelsToSet(policy.LabelsIncludeAll)
	data.LabelsExcludeAll = policyLabelsToSet(policy.LabelsExcludeAll)
	data.CalendarEventsEnabled = types.BoolValue(policy.CalendarEventsEnabled)
	data.ConditionalAccessEnabled = types.BoolValue(policy.ConditionalAccessEnabled)
	data.ContinuousAutomationsEnabled = types.BoolValue(policy.ContinuousAutomationsEnabled)
	data.PatchWhenClosed = types.BoolValue(policy.PatchWhenClosed)
	// Fleet doesn't echo conditional_access_bypass_enabled in the response;
	// the planner-supplied value (config or default) is left unchanged.

	data.FleetMaintained = types.BoolValue(policy.FleetMaintained)
	data.CreatedAt = types.StringValue(policy.CreatedAt)
	data.UpdatedAt = types.StringValue(policy.UpdatedAt)
	data.HostCountUpdatedAt = stringPtrToString(policy.HostCountUpdatedAt)

	data.SoftwareTitleID, data.InstallSoftware = mapInstallSoftware(policy.InstallSoftware, diags)
	data.ScriptID, data.RunScript = mapRunScript(policy.RunScript, diags)
	data.PatchSoftwareTitleID, data.PatchSoftware = mapPatchSoftware(policy, diags)
}

// stringSetToSlice converts a types.Set of strings to a []string and
// appends any element-conversion diagnostics.
//
// The Null vs Unknown distinction matters for the no-omitempty Update
// fields: an explicit empty array (`[]`) tells Fleet to clear the
// labels, while `null` is interpreted as "no change". A Null set means
// the user has removed the attribute from HCL → clear; an Unknown set
// means the value is still being computed and we should not touch
// what's already on the server.
func stringSetToSlice(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
	if set.IsUnknown() {
		return nil
	}
	if set.IsNull() {
		return []string{}
	}
	out := make([]string, 0, len(set.Elements()))
	diags.Append(set.ElementsAs(ctx, &out, false)...)
	return out
}

// policyLabelsToSet flattens Fleet's per-label response objects (id+name)
// into a types.Set of label names — what the user-facing schema exposes.
// Sets are used (instead of Lists) because Fleet returns labels in
// nondeterministic order; a List would surface false drift on order
// differences. Returns SetNull on empty input.
func policyLabelsToSet(labels []fleetdm.PolicyLabel) types.Set {
	if len(labels) == 0 {
		return types.SetNull(types.StringType)
	}
	values := make([]attr.Value, 0, len(labels))
	for _, l := range labels {
		values = append(values, types.StringValue(l.Name))
	}
	return types.SetValueMust(types.StringType, values)
}

// isTeamPolicy returns true if the model's team_id is set to a positive value.
func isTeamPolicy(teamID types.Int64) bool {
	return !teamID.IsNull() && !teamID.IsUnknown() && teamID.ValueInt64() > 0
}

// policyNeedsAutomationFollowup is true when the planned model has any
// PATCH-only field set to a non-default value, requiring a follow-up Update
// after Create. Unknown values short-circuit to false — Optional+Computed
// defaults should resolve all of these to Known by the time Create runs,
// but if one is still Unknown we'd rather skip the follow-up and let the
// next apply converge than send `null` to Fleet and risk a misinterpretation.
func policyNeedsAutomationFollowup(data PolicyResourceModel) bool {
	if !data.CalendarEventsEnabled.IsNull() && !data.CalendarEventsEnabled.IsUnknown() && data.CalendarEventsEnabled.ValueBool() {
		return true
	}
	if !data.ConditionalAccessEnabled.IsNull() && !data.ConditionalAccessEnabled.IsUnknown() && data.ConditionalAccessEnabled.ValueBool() {
		return true
	}
	// The API default for bypass is true. Only follow up if user explicitly
	// set it to false.
	if !data.ConditionalAccessBypassEnabled.IsNull() && !data.ConditionalAccessBypassEnabled.IsUnknown() && !data.ConditionalAccessBypassEnabled.ValueBool() {
		return true
	}
	return false
}

// policyQueryForRequest returns the query value to send in a Create or
// Update request — empty string for patch policies so the omitempty JSON
// tag drops the field entirely. Fleet rejects `query` together with
// `type = "patch"` and generates the query itself for patch policies.
func policyQueryForRequest(data PolicyResourceModel) string {
	if data.Type.ValueString() == "patch" {
		return ""
	}
	return data.Query.ValueString()
}

// policyPlatformForRequest returns the platform value to send in a Create or
// Update request — empty string for patch policies so the omitempty JSON tag
// drops the field entirely. Fleet (4.84+) rejects `platform` together with
// `type = "patch"` (status 400: "If the 'type' is 'patch', the 'platform'
// field is not supported"). Fleet derives the effective platform from the
// patch target, so dropping the field is safe.
func policyPlatformForRequest(ctx context.Context, data PolicyResourceModel) string {
	if data.Type.ValueString() == "patch" {
		return ""
	}
	return platformListToString(ctx, data.Platform)
}

// buildPolicyUpdateRequest builds an UpdatePolicyRequest from the planned
// model. Fields that the API treats as "send null to clear" use pointers
// without omitempty (see UpdatePolicyRequest doc comment). Element
// conversion diagnostics from the label sets are appended to diags;
// callers must check diags.HasError() before using the result.
func buildPolicyUpdateRequest(ctx context.Context, data PolicyResourceModel, diags *diag.Diagnostics) fleetdm.UpdatePolicyRequest {
	return fleetdm.UpdatePolicyRequest{
		Name:                           data.Name.ValueString(),
		Description:                    data.Description.ValueString(),
		Query:                          policyQueryForRequest(data),
		Critical:                       data.Critical.ValueBool(),
		Resolution:                     data.Resolution.ValueString(),
		Platform:                       policyPlatformForRequest(ctx, data),
		SoftwareTitleID:                optionalIntPtr(data.SoftwareTitleID),
		ScriptID:                       optionalIntPtr(data.ScriptID),
		CalendarEventsEnabled:          optionalBoolPtr(data.CalendarEventsEnabled),
		ConditionalAccessEnabled:       optionalBoolPtr(data.ConditionalAccessEnabled),
		ConditionalAccessBypassEnabled: optionalBoolPtr(data.ConditionalAccessBypassEnabled),
		ContinuousAutomationsEnabled:   optionalBoolPtr(data.ContinuousAutomationsEnabled),
		PatchWhenClosed:                optionalBoolPtr(data.PatchWhenClosed),
		LabelsIncludeAny:               stringSetToSlice(ctx, data.LabelsIncludeAny, diags),
		LabelsExcludeAny:               stringSetToSlice(ctx, data.LabelsExcludeAny, diags),
		LabelsIncludeAll:               stringSetToSlice(ctx, data.LabelsIncludeAll, diags),
		LabelsExcludeAll:               stringSetToSlice(ctx, data.LabelsExcludeAll, diags),
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
//
// Fleet's policy GET endpoint does not always echo `patch_software` for
// `type = "patch"` policies even though the policy was created with a
// patch_software_title_id. It does always echo `install_software` — Fleet
// auto-creates an install-software automation pointing at the patch
// target, so the two software_title_ids are equal for patch policies. We
// fall back to that here so import (and any subsequent Read) doesn't end
// up with a null patch_software_title_id, which would otherwise force a
// destroy/recreate via the RequiresReplace plan modifier.
func mapPatchSoftware(policy *fleetdm.Policy, diags *diag.Diagnostics) (types.Int64, types.Object) {
	if s := policy.PatchSoftware; s != nil {
		obj, dd := types.ObjectValue(policyPatchSoftwareAttrTypes, map[string]attr.Value{
			"name":              types.StringValue(s.Name),
			"display_name":      types.StringValue(s.DisplayName),
			"software_title_id": types.Int64Value(int64(s.SoftwareTitleID)),
		})
		diags.Append(dd...)
		return types.Int64Value(int64(s.SoftwareTitleID)), obj
	}
	if policy.Type == "patch" && policy.InstallSoftware != nil {
		return types.Int64Value(int64(policy.InstallSoftware.SoftwareTitleID)), types.ObjectNull(policyPatchSoftwareAttrTypes)
	}
	return types.Int64Null(), types.ObjectNull(policyPatchSoftwareAttrTypes)
}
