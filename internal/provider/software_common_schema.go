package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// packageSource exposes the three fields the binary-source helpers
// (buildS3Source / resolveRemoteSHA / readPackageContentForUpload /
// deriveFilename in software_package_resource.go) need to read. Both
// the legacy softwarePackageResourceModel and the new
// softwareCustomPackageResourceModel implement this interface so the
// helpers stay shared across resources during the deprecation window.
// Declared in this file so that removal of the legacy resource at the
// next major doesn't drag the interface declaration with it.
type packageSource interface {
	PackagePathField() types.String
	PackageS3Field() types.Object
	FilenameField() types.String
}

// softwareCommonSchemaAttributes returns the schema attributes shared by
// fleetdm_software_custom_package, fleetdm_software_app_store_app, and
// fleetdm_software_fleet_maintained_app. Each new resource merges this map
// with its own type-specific attributes before assembling its final schema.
//
// The legacy fleetdm_software_package resource intentionally does NOT use
// this helper — it predates the split, has additional discriminator
// attributes (type), and keeps the older `automatic_install` attribute
// during the deprecation window. Keeping the legacy schema inline avoids
// the risk of accidentally narrowing it during helper edits.
//
// Attributes returned:
//   - id, title_id, team_id      — identification (id == title_id internally)
//   - name, version              — Fleet-computed metadata
//   - platform                   — Optional+Computed; values vary per type
//   - display_name               — Optional+Computed; Fleet auto-derives when unset
//   - self_service               — Optional+Computed bool, default false
//   - install_during_setup       — Optional+Computed bool, default false
//     (replaces PR E's broken `automatic_install` attribute. This routes
//     to Fleet's PUT /setup_experience/software endpoint via the
//     SetSetupExperienceSoftwareInclude/Exclude helpers on the API
//     client. Distinct from the policy-based `automatic_install_policy`
//     which is exposed only by resources whose Create endpoint supports
//     it.)
//   - labels_include_any         — Optional list, ConflictsWith labels_exclude_any AND labels_include_all
//   - labels_exclude_any         — Optional list, ConflictsWith labels_include_all
//   - labels_include_all         — Optional list, no validators (covered by the others)
//   - automatic_install_policies — Computed list of {id, name} pairs; Fleet
//     returns the auto-install policies for the
//     title so users can reference them without
//     leaving the Fleet UI.
func softwareCommonSchemaAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.Int64Attribute{
			Description: "The unique identifier (internal, same as title_id).",
			Computed:    true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		},
		"title_id": schema.Int64Attribute{
			Description: "The software title ID.",
			Computed:    true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		},
		"team_id": schema.Int64Attribute{
			Description: "The ID of the team this software belongs to. Required for Fleet Premium.",
			Optional:    true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.RequiresReplace(),
			},
		},
		"name": schema.StringAttribute{
			Description: "The name of the software, as parsed by Fleet from the installer or App Store metadata.",
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"version": schema.StringAttribute{
			Description: "The version of the software.",
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"platform": schema.StringAttribute{
			Description: "The platform the software targets (`darwin`, `windows`, `linux`, `ios`, `ipados`).",
			Optional:    true,
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"display_name": schema.StringAttribute{
			Description: "End-user-visible name shown for this software in Fleet's UI (e.g. on the Self Service page). " +
				"Optional override for Fleet's auto-derived name (the installer's intrinsic name for custom packages, the App Store metadata for VPP, " +
				"the catalog name for Fleet Maintained Apps). Computed when omitted.",
			Optional: true,
			Computed: true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"self_service": schema.BoolAttribute{
			Description: "Whether the software is available for self-service installation by end users. Defaults to false.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"install_during_setup": schema.BoolAttribute{
			Description: "Whether to install this software during the device's Setup Assistant / first-boot setup experience. " +
				"Routes to Fleet's `PUT /setup_experience/software` endpoint, which manages a per-team-per-platform set " +
				"of titles flagged for setup-time installation. Distinct from `automatic_install_policy`, which creates " +
				"a Fleet policy that installs the software on hosts missing it (the policy-based path). " +
				"\n\n" +
				"When the attribute is **omitted from HCL**, the provider leaves Fleet's setup-experience set " +
				"untouched and adopts whatever value Fleet returns — managing the field is opt-in. Set it " +
				"explicitly to `true` or `false` to drive the value from Terraform. This avoids spurious " +
				"true → false flips after `terraform import` of a title that was previously in the set. " +
				"\n\n" +
				"Multi-resource race: when two `fleetdm_software_*` resources on the same team and platform both flip " +
				"`install_during_setup = true` in a single `terraform apply`, the provider serializes the updates " +
				"per-(team, platform) inside the API client to avoid losing one — but cross-process race conditions " +
				"(another `terraform apply` against the same team/platform at the same time, or a concurrent Fleet UI " +
				"change) remain a user concern.",
			Optional: true,
			Computed: true,
			PlanModifiers: []planmodifier.Bool{
				boolplanmodifier.UseStateForUnknown(),
			},
		},
		"labels_include_any": schema.ListAttribute{
			Description: "List of label names. The software will be available for hosts that match *any* of these labels. " +
				"Mutually exclusive with `labels_exclude_any` and `labels_include_all` — Fleet's API rejects requests that set more than one of the three. " +
				"To clear previously-set labels, set this attribute to `[]` explicitly; omitting the attribute preserves Fleet's existing labels.",
			Optional:    true,
			ElementType: types.StringType,
			Validators: []validator.List{
				listvalidator.ConflictsWith(path.Expressions{
					path.MatchRoot("labels_exclude_any"),
					path.MatchRoot("labels_include_all"),
				}...),
			},
		},
		"labels_exclude_any": schema.ListAttribute{
			Description: "List of label names. The software will not be available for hosts that match any of these labels. " +
				"Mutually exclusive with `labels_include_any` and `labels_include_all`. " +
				"To clear previously-set labels, set this attribute to `[]` explicitly; omitting the attribute preserves Fleet's existing labels.",
			Optional:    true,
			ElementType: types.StringType,
			Validators: []validator.List{
				listvalidator.ConflictsWith(path.Expressions{
					path.MatchRoot("labels_include_all"),
				}...),
			},
		},
		"labels_include_all": schema.ListAttribute{
			Description: "List of label names. The software will be available for hosts that match *all* of these labels. " +
				"Mutually exclusive with `labels_include_any` and `labels_exclude_any`; the conflict is enforced by validators on the other two. " +
				"To clear previously-set labels, set this attribute to `[]` explicitly; omitting the attribute preserves Fleet's existing labels.",
			Optional:    true,
			ElementType: types.StringType,
		},
		"automatic_install_policies": schema.ListNestedAttribute{
			Description: "**Read-only.** List of Fleet policies whose `install_software` automation currently points at this title. " +
				"Each entry exposes the policy `id` and `name` so you can reference them from other Terraform resources. " +
				"\n\n" +
				"This attribute is read-only because Fleet's REST API does not accept a policies array on any of the " +
				"software-title endpoints (`POST /software/package`, `PATCH /software/titles/{id}/package`, the Fleet " +
				"Maintained Apps add endpoint, or the VPP add endpoint) — the relationship is owned on the *policy* " +
				"side. To attach an install-software policy to this title from Terraform, create or update a " +
				"`fleetdm_policy` resource and set its `software_title_id` to this resource's `title_id`. To detach, " +
				"clear `software_title_id` on the policy (or delete the policy). " +
				"\n\n" +
				"The list is populated when Fleet creates the auto-install policy because `automatic_install_policy = " +
				"true` was set at this resource's Create time, when a `fleetdm_policy` elsewhere in your configuration " +
				"points at this title, or when an admin attaches an `install_software` automation via Fleet's UI.",
			Computed: true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.Int64Attribute{
						Description: "The Fleet policy ID.",
						Computed:    true,
					},
					"name": schema.StringAttribute{
						Description: "The Fleet policy name.",
						Computed:    true,
					},
				},
			},
			PlanModifiers: []planmodifier.List{
				automaticInstallPoliciesUseStateForUnknown{},
			},
		},
	}
}

// softwareScriptAttributes returns the install/uninstall/pre-install/post-install
// script attributes shared by fleetdm_software_custom_package and
// fleetdm_software_fleet_maintained_app. VPP doesn't accept scripts —
// Apple manages the install flow — so the app_store_app resource does
// NOT merge this helper.
func softwareScriptAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"install_script": schema.StringAttribute{
			Description: "Script to run during installation. Optional — Fleet picks a default for the package type when omitted.",
			Optional:    true,
		},
		"uninstall_script": schema.StringAttribute{
			Description: "Script to run during uninstallation. Optional — Fleet picks a default for the package type when omitted.",
			Optional:    true,
		},
		"pre_install_query": schema.StringAttribute{
			Description: "An osquery SQL query to run before installation. Installation proceeds only if the query returns results. Optional.",
			Optional:    true,
		},
		"post_install_script": schema.StringAttribute{
			Description: "Script to run after installation. Optional.",
			Optional:    true,
		},
	}
}

// softwareAutomaticInstallPolicyAttribute returns the schema attribute for
// the policy-based auto-install feature. Only fleetdm_software_custom_package
// and fleetdm_software_fleet_maintained_app support it — VPP's Add endpoint
// has no equivalent. ForceNew because Fleet creates the policy at Create
// time only; changing the value would require a new resource.
func softwareAutomaticInstallPolicyAttribute() schema.Attribute {
	return schema.BoolAttribute{
		Description: "One-shot create-time shortcut: when `true`, Fleet itself mints a single `install_software` " +
			"policy pointing at this title (via the `automatic_install` body field on the Add Package / Add Fleet " +
			"Maintained App endpoint). The generated policy is owned by Fleet, not by Terraform — its IDs surface " +
			"in the read-only `automatic_install_policies` attribute, but it is not represented as a " +
			"`fleetdm_policy` resource in your state. " +
			"\n\n" +
			"Forces resource replacement because Fleet only honors this flag at title creation; toggling it after " +
			"the title exists has no supported wire path. " +
			"\n\n" +
			"**To manage install-software policies as first-class Terraform resources** — multiple policies, custom " +
			"queries, label scoping, drift detection on the policy itself — leave this attribute unset and instead " +
			"declare one or more `fleetdm_policy` resources with `software_title_id = <this title_id>`. That is " +
			"how Fleet's API models the relationship; the software-title endpoints do not accept a policy list on " +
			"input. " +
			"\n\n" +
			"Distinct from `install_during_setup`, which flags the title for installation during the first-boot " +
			"Setup Assistant flow via a separate Fleet endpoint.",
		Optional: true,
		Computed: true,
		Default:  booldefault.StaticBool(false),
		PlanModifiers: []planmodifier.Bool{
			boolplanmodifier.RequiresReplace(),
		},
	}
}

// softwareCategoriesAttribute returns the schema attribute for self-service
// categories. Only the custom_package and fleet_maintained_app resources
// support categories — VPP's API doesn't expose them.
func softwareCategoriesAttribute() schema.Attribute {
	return schema.ListAttribute{
		Description: "Zero or more self-service categories the software appears under on the end-user's *My device* page. " +
			"Supported values are documented under the `software` section at https://fleetdm.com/docs/configuration/yaml-files — " +
			"at time of writing: `Browsers`, `Communication`, `Developer tools`, `Productivity`, `Security`, `Utilities`. " +
			"To clear previously-set categories, set this attribute to `[]` explicitly; omitting it preserves Fleet's existing categories.",
		Optional:    true,
		ElementType: types.StringType,
	}
}

// stringSliceToStringList converts a []string from Fleet's API response
// (used for categories, etc.) into a types.List of strings. nil input
// becomes a null list; non-nil but empty becomes an empty list. Mirrors
// labelsToStringListValue's nil/empty semantics.
func stringSliceToStringList(items []string) types.List {
	if items == nil {
		return types.ListNull(types.StringType)
	}
	values := make([]attr.Value, 0, len(items))
	for _, s := range items {
		values = append(values, types.StringValue(s))
	}
	return types.ListValueMust(types.StringType, values)
}

// automaticInstallPolicyObjectType describes one element of the
// automatic_install_policies Computed list.
var automaticInstallPolicyObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"id":   types.Int64Type,
		"name": types.StringType,
	},
}

// automaticInstallPoliciesFromTitle converts the Fleet response's
// automatic_install_policies array (returned on both SoftwarePackageInfo
// and AppStoreAppInfo) into a types.List for resource state.
//
// The provider treats a nil response slice as null state — Fleet's JSON
// can't distinguish "no policies attached" from "policies field absent"
// once Go-decoded, so we pick null which is the more conservative
// default for Computed-only attributes.
func automaticInstallPoliciesFromTitle(title *fleetdm.SoftwareTitle) types.List {
	var refs []fleetdm.AutomaticInstallPolicyRef
	switch {
	case title.SoftwarePackage != nil:
		refs = title.SoftwarePackage.AutomaticInstallPolicies
	case title.AppStoreApp != nil:
		refs = title.AppStoreApp.AutomaticInstallPolicies
	}
	if refs == nil {
		return types.ListNull(automaticInstallPolicyObjectType)
	}
	elems := make([]attr.Value, 0, len(refs))
	for _, p := range refs {
		obj, _ := types.ObjectValue(
			automaticInstallPolicyObjectType.AttrTypes,
			map[string]attr.Value{
				"id":   types.Int64Value(int64(p.ID)),
				"name": types.StringValue(p.Name),
			},
		)
		elems = append(elems, obj)
	}
	v, _ := types.ListValue(automaticInstallPolicyObjectType, elems)
	return v
}

// automaticInstallPoliciesUseStateForUnknown is a list-attribute plan
// modifier that preserves the prior state value when the value would
// otherwise be marked Unknown. Computed-only attributes default to
// "known after apply" on every plan; using state-for-unknown collapses
// that noise when nothing about the title has actually changed.
type automaticInstallPoliciesUseStateForUnknown struct{}

func (m automaticInstallPoliciesUseStateForUnknown) Description(_ context.Context) string {
	return "Preserve the prior state value for automatic_install_policies on every plan unless the resource is being created or replaced."
}

func (m automaticInstallPoliciesUseStateForUnknown) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m automaticInstallPoliciesUseStateForUnknown) PlanModifyList(_ context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if req.StateValue.IsNull() {
		return
	}
	if !req.PlanValue.IsUnknown() {
		return
	}
	resp.PlanValue = req.StateValue
}
