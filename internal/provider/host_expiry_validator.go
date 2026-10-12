package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// hostExpiryValidator mirrors Fleet's rule that turning on host expiry needs a
// window of at least one day. Fleet enforces it per fleet, and since 4.93 on
// the global config too (422). A window left out of the configuration
// defaults to 0, so it fails the rule as well.
type hostExpiryValidator struct{}

var _ resource.ConfigValidator = hostExpiryValidator{}

func (hostExpiryValidator) Description(context.Context) string {
	return "host_expiry_window must be at least 1 when host_expiry_enabled is true"
}

func (hostExpiryValidator) MarkdownDescription(context.Context) string {
	return "`host_expiry_window` must be at least 1 when `host_expiry_enabled` is true"
}

func (hostExpiryValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var enabled types.Bool
	var window types.Int64
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("host_expiry_enabled"), &enabled)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("host_expiry_window"), &window)...)
	if resp.Diagnostics.HasError() || !boolKnownTrue(enabled) || window.IsUnknown() {
		return
	}
	if window.IsNull() || window.ValueInt64() < 1 {
		resp.Diagnostics.AddAttributeError(
			path.Root("host_expiry_window"),
			"Invalid host expiry window",
			"host_expiry_window must be at least 1 when host_expiry_enabled is true.",
		)
	}
}
