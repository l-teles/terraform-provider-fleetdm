package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// listPoliciesBlockingTitleDelete enumerates all Fleet policies whose
// automation references the given software title — both `install_software`
// and `patch_software` flavors. Fleet's DELETE /software/titles refuses
// with 409 if any such policy is attached:
//
//   - "Policy automation uses this software." — install_software
//   - "This software has a patch policy."     — patch_software
//
// Callers detach via SetPolicyInstallSoftwareTitleID / SetPolicyPatchSoftwareTitleID
// before the delete (and reattach after a successful re-upload, in the
// Update→replace path).
//
// Returns two lists rather than a unioned set because the detach API
// surface is different per flavor; deduplication by policy id is the
// caller's responsibility if needed.
func listPoliciesBlockingTitleDelete(ctx context.Context, client *fleetdm.Client, titleID int, teamID *int) ([]fleetdm.Policy, []fleetdm.Policy, error) {
	install, err := client.ListPoliciesByInstallSoftwareTitleID(ctx, titleID, teamID)
	if err != nil {
		return nil, nil, fmt.Errorf("list install_software policies: %w", err)
	}
	patch, err := client.ListPoliciesByPatchSoftwareTitleID(ctx, titleID, teamID)
	if err != nil {
		return nil, nil, fmt.Errorf("list patch_software policies: %w", err)
	}
	return install, patch, nil
}

// detachPoliciesBeforeTitleDelete clears install_software and patch_software
// automation from every policy referencing titleID, so a follow-up
// DeleteSoftwarePackage doesn't 409. No reattach: the title is going away.
//
// Returns nil on success or a diag.Diagnostics with a single error to
// append to the caller's response. Caller checks the return value and
// short-circuits before calling DeleteSoftwarePackage.
func detachPoliciesBeforeTitleDelete(ctx context.Context, client *fleetdm.Client, titleID int, teamID *int) diag.Diagnostics {
	install, patch, err := listPoliciesBlockingTitleDelete(ctx, client, titleID, teamID)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError(
			"Error preparing software title for delete",
			"Could not list policies referencing this title (needed to clear policy automation before delete): "+err.Error(),
		)
		return diags
	}
	for _, p := range install {
		if err := client.SetPolicyInstallSoftwareTitleID(ctx, p.ID, teamID, nil); err != nil {
			var diags diag.Diagnostics
			diags.AddError(
				"Error detaching install_software automation",
				fmt.Sprintf("Could not detach install_software automation from policy %d (%q) before delete: %s", p.ID, p.Name, err.Error()),
			)
			return diags
		}
	}
	for _, p := range patch {
		if err := client.SetPolicyPatchSoftwareTitleID(ctx, p.ID, teamID, nil); err != nil {
			var diags diag.Diagnostics
			diags.AddError(
				"Error detaching patch_software automation",
				fmt.Sprintf("Could not detach patch_software automation from policy %d (%q) before delete: %s", p.ID, p.Name, err.Error()),
			)
			return diags
		}
	}
	return nil
}
