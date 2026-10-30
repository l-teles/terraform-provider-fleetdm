package provider

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// softwareTypeFilterSources mirrors Fleet 4.94's softwareTypeFilterSources
// (server/fleet/software.go): every `source` the software list endpoints
// accept, mapped to the `extension_for` values that narrow it. Fleet rejects
// anything else, so a source Fleet adds later needs an update here.
var softwareTypeFilterSources = map[string][]string{
	"adobe_plugins":       nil,
	"android_apps":        nil,
	"apps":                nil,
	"chocolatey_packages": nil,
	"deb_packages":        nil,
	"go_binaries":         nil,
	"homebrew_packages":   nil,
	"ie_extensions":       nil,
	"ios_apps":            nil,
	"ipados_apps":         nil,
	"nix_packages":        nil,
	"npm_packages":        nil,
	"pacman_packages":     nil,
	"pkg_packages":        nil,
	"portage_packages":    nil,
	"programs":            nil,
	"ps1_packages":        nil,
	"py_packages":         nil,
	"python_packages":     nil,
	"rpm_packages":        nil,
	"safari_extensions":   nil,
	"sh_packages":         nil,
	"tgz_packages":        nil,
	"chrome_extensions":   {"chrome", "chromium", "brave", "edge", "edge_beta", "opera", "yandex"},
	"firefox_addons":      {"firefox"},
	"vscode_extensions":   {"vscode", "vscode_insiders", "vscodium", "vscodium_insiders", "cursor", "windsurf", "trae"},
	"jetbrains_plugins": {
		"clion", "datagrip", "goland", "intellij_idea", "intellij_idea_community_edition", "phpstorm", "pycharm",
		"pycharm_community_edition", "resharper", "rider", "rubymine", "rust_rov", "webstorm",
	},
}

// softwareSourceByExtensionFor maps each extension_for value to its source.
var softwareSourceByExtensionFor = func() map[string]string {
	out := make(map[string]string)
	for source, exts := range softwareTypeFilterSources {
		for _, ext := range exts {
			out[ext] = source
		}
	}
	return out
}()

// softwareTypeFilterAttributes returns the `source` and `extension_for`
// schema attributes shared by the software list data sources.
func softwareTypeFilterAttributes() map[string]schema.Attribute {
	sources := slices.Sorted(maps.Keys(softwareTypeFilterSources))
	exts := slices.Sorted(maps.Keys(softwareSourceByExtensionFor))
	return map[string]schema.Attribute{
		"source": schema.SetAttribute{
			Optional:    true,
			ElementType: types.StringType,
			MarkdownDescription: "Only return software from any of these sources (osquery tables), e.g. `[\"apps\", \"chrome_extensions\"]`. " +
				"One of: " + backtickList(sources) + ". _Requires Fleet 4.94+._",
			Validators: []validator.Set{
				setvalidator.ValueStringsAre(stringvalidator.OneOf(sources...)),
			},
		},
		"extension_for": schema.SetAttribute{
			Optional:    true,
			ElementType: types.StringType,
			MarkdownDescription: "Narrow an extension or plugin source to the browsers or IDEs it is for, e.g. `[\"brave\"]` with `source = [\"chrome_extensions\"]`. " +
				"Each value's own source must be listed in `source`; a source without any of its values here matches all of its software. " +
				"One of: " + backtickList(exts) + ". _Requires Fleet 4.94+._",
			Validators: []validator.Set{
				setvalidator.ValueStringsAre(stringvalidator.OneOf(exts...)),
				setvalidator.AlsoRequires(path.MatchRoot("source")),
			},
		},
	}
}

func backtickList(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "`" + v + "`"
	}
	return strings.Join(quoted, ", ")
}

// validateSoftwareTypeFilter repeats Fleet's check that every extension_for
// value belongs to a source listed in `source`. Unknown values defer to Fleet.
func validateSoftwareTypeFilter(sources, extensionFor types.Set, diags *diag.Diagnostics) {
	if sources.IsUnknown() || extensionFor.IsNull() || extensionFor.IsUnknown() {
		return
	}
	// A known set can still hold unknown elements; the selection isn't known yet.
	for _, elem := range sources.Elements() {
		if elem.IsUnknown() {
			return
		}
	}
	selected := knownStringSetValues(sources)
	for _, ext := range knownStringSetValues(extensionFor) {
		source, ok := softwareSourceByExtensionFor[ext]
		if !ok || slices.Contains(selected, source) {
			continue
		}
		diags.AddAttributeError(
			path.Root("extension_for"),
			"Source not selected",
			fmt.Sprintf("%q is a %q value, but source doesn't include %q.", ext, source, source),
		)
	}
}

// softwareTypeFilterFromConfig converts the two set attributes to the
// client's filter, sorted so the query string is deterministic.
func softwareTypeFilterFromConfig(ctx context.Context, sources, extensionFor types.Set, diags *diag.Diagnostics) fleetdm.SoftwareTypeFilter {
	toSorted := func(set types.Set) []string {
		if set.IsNull() || set.IsUnknown() {
			return nil
		}
		var out []string
		diags.Append(set.ElementsAs(ctx, &out, false)...)
		slices.Sort(out)
		return out
	}
	return fleetdm.SoftwareTypeFilter{Sources: toSorted(sources), ExtensionFor: toSorted(extensionFor)}
}

// checkSoftwareTypeFilterSupported rejects a type filter on Fleet older than
// 4.94, which ignores the source and extension_for parameters and would
// return the unfiltered list.
func checkSoftwareTypeFilterSupported(ctx context.Context, client *fleetdm.Client, filter fleetdm.SoftwareTypeFilter, diags *diag.Diagnostics) {
	if len(filter.Sources) == 0 && len(filter.ExtensionFor) == 0 {
		return
	}
	before, known := fleetServerVersionBefore(ctx, client, 4, 94)
	if !known {
		diags.AddWarning(
			"Could Not Confirm Software Type Filter Support",
			"The Fleet server version could not be read. Filtering by source or extension_for requires Fleet 4.94 or "+
				"later; older Fleet ignores the filter and returns every item.",
		)
	}
	if before {
		diags.AddError(
			"Software Type Filter Not Supported by This Fleet Server",
			"Filtering by source or extension_for requires Fleet 4.94 or later. Older Fleet ignores the filter "+
				"and would return every item. Upgrade Fleet or remove source and extension_for.",
		)
	}
}
