package provider

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
// with its own type-specific attributes (e.g. package_path, app_store_id,
// fleet_maintained_app_id) before assembling its final schema.
//
// The legacy fleetdm_software_package resource intentionally does NOT use
// this helper — it predates the split and has additional discriminator
// attributes (type) plus a wider superset of fields. Keeping the legacy
// schema inline avoids the risk of accidentally narrowing it during
// helper edits.
//
// Attributes returned:
//   - id, title_id, team_id      — identification (id == title_id internally)
//   - name, version              — Fleet-computed metadata
//   - platform                   — Optional+Computed; values vary per type
//   - self_service               — Optional+Computed bool, default false
//   - automatic_install          — Optional+Computed bool, default false
//     (NOTE: the wire encoding of this field is broken for type=package;
//     parity with the legacy resource is preserved here intentionally
//     pending the PR F follow-up that fixes the semantics per type.)
//   - labels_include_any         — Optional list, ConflictsWith labels_exclude_any
//   - labels_exclude_any         — Optional list
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
		"self_service": schema.BoolAttribute{
			Description: "Whether the software is available for self-service installation by end users. Defaults to false.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"automatic_install": schema.BoolAttribute{
			Description: "Whether to automatically install the software during device setup (install during setup). Defaults to false.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"labels_include_any": schema.ListAttribute{
			Description: "List of label names. The software will be available for hosts that match any of these labels. " +
				"Mutually exclusive with `labels_exclude_any` (Fleet's API rejects requests that set both). " +
				"To clear previously-set labels, set this attribute to `[]` explicitly; omitting the attribute preserves Fleet's existing labels.",
			Optional:    true,
			ElementType: types.StringType,
			Validators: []validator.List{
				listvalidator.ConflictsWith(path.Expressions{
					path.MatchRoot("labels_exclude_any"),
				}...),
			},
		},
		"labels_exclude_any": schema.ListAttribute{
			Description: "List of label names. The software will not be available for hosts that match any of these labels. " +
				"Mutually exclusive with `labels_include_any`; the conflict is enforced by the validator on `labels_include_any`. " +
				"To clear previously-set labels, set this attribute to `[]` explicitly; omitting the attribute preserves Fleet's existing labels.",
			Optional:    true,
			ElementType: types.StringType,
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
