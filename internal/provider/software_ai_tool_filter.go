package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// aiToolFilterAttribute is the optional `ai_tool` filter shared by the
// software list data sources.
func aiToolFilterAttribute() schema.Attribute {
	return schema.BoolAttribute{
		Optional: true,
		MarkdownDescription: "Only return software that fleetd reported as an AI tool. `false` or unset returns all software; " +
			"it does not select software that isn't an AI tool. Fleet Free rejects `true`. _Requires Fleet Premium 4.95+._",
	}
}

// aiToolItemAttribute is the computed `ai_tool` flag on a returned software
// item.
func aiToolItemAttribute() schema.Attribute {
	return schema.BoolAttribute{
		Computed: true,
		MarkdownDescription: "Whether fleetd reported this software as an AI tool. Null when Fleet does not report it " +
			"(Fleet Free, or Fleet before 4.95). _Available in Fleet Premium 4.95+._",
	}
}

// aiToolFilterFromConfig reports whether the ai_tool filter is known to be true.
func aiToolFilterFromConfig(v types.Bool) bool {
	return !v.IsNull() && !v.IsUnknown() && v.ValueBool()
}

// checkAIToolFilterSupported rejects ai_tool = true on Fleet older than 4.95,
// which ignores the parameter and would return the unfiltered list.
func checkAIToolFilterSupported(ctx context.Context, client *fleetdm.Client, aiTool bool, diags *diag.Diagnostics) {
	if !aiTool {
		return
	}
	before, known := fleetServerVersionBefore(ctx, client, 4, 95)
	if !known {
		diags.AddWarning(
			"Could Not Confirm AI Tool Filter Support",
			"The Fleet server version could not be read. Filtering by ai_tool requires Fleet Premium 4.95 or later; "+
				"older Fleet ignores the filter and returns every item.",
		)
	}
	if before {
		diags.AddError(
			"AI Tool Filter Not Supported by This Fleet Server",
			"Filtering by ai_tool requires Fleet Premium 4.95 or later. Older Fleet ignores the filter and would "+
				"return every item. Upgrade Fleet or remove ai_tool.",
		)
	}
}
