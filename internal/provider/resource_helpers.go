package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// configureClient extracts the *fleetdm.Client from provider data during
// Configure. Works for both resources and data sources since both pass
// providerData as any and expose Diagnostics.
func configureClient(providerData any, diagnostics *diag.Diagnostics, typeName string) *fleetdm.Client {
	if providerData == nil {
		return nil
	}

	client, ok := providerData.(*fleetdm.Client)
	if !ok {
		diagnostics.AddError(
			fmt.Sprintf("Unexpected %s Configure Type", typeName),
			fmt.Sprintf("Expected *fleetdm.Client, got: %T. Please report this issue to the provider developers.", providerData),
		)
		return nil
	}
	return client
}

// isNotFound returns true if the error (or any error it wraps) is a FleetDM
// API 404 response. Domain client methods wrap *fleetdm.APIError with
// fmt.Errorf("...: %w", err), so we must unwrap with errors.As.
func isNotFound(err error) bool {
	var apiErr *fleetdm.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 404
}

// isForbidden returns true if the error (or any error it wraps) is a FleetDM
// API 403 response.
func isForbidden(err error) bool {
	var apiErr *fleetdm.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 403
}

// isConflict returns true if the error (or any error it wraps) is a FleetDM
// API 409 response. Fleet uses 409 both for "this name already exists" on
// create and for "this object is still referenced" on delete.
func isConflict(err error) bool {
	var apiErr *fleetdm.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 409
}

// parseIDFromString parses a numeric string ID and adds a diagnostic error on failure.
// Returns the parsed int and true on success, or 0 and false on failure.
//
// Negative values are rejected: no Fleet ID is ever negative, so "-1" is a
// typo or a mangled composite import ID, and accepting it would push the bad
// value into state and turn a clear import error into a confusing 404 (or, for
// the fleet_id half of a composite ID, a wrong-scope lookup).
//
// Zero is deliberately accepted. It is a meaningful value in several places:
// fleet_id/team_id 0 is Fleet's "no team" scope, which is why import IDs like
// "0:5" are legitimate.
func parseIDFromString(id string, resourceName string, diagnostics *diag.Diagnostics) (int64, bool) {
	parsed, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		diagnostics.AddError(
			fmt.Sprintf("Error Importing FleetDM %s", resourceName),
			fmt.Sprintf("Could not parse %s ID '%s': %s", resourceName, id, err),
		)
		return 0, false
	}
	if parsed < 0 {
		diagnostics.AddError(
			fmt.Sprintf("Error Importing FleetDM %s", resourceName),
			fmt.Sprintf("Could not parse %s ID '%s': ID must not be negative", resourceName, id),
		)
		return 0, false
	}
	return parsed, true
}

// optionalIntPtr converts an optional types.Int64 to a *int.
// Returns nil if the value is null or unknown.
func optionalIntPtr(val types.Int64) *int {
	if val.IsNull() || val.IsUnknown() {
		return nil
	}
	v := int(val.ValueInt64())
	return &v
}

// optionalBoolPtr converts an optional types.Bool to a *bool.
// Returns nil if the value is null or unknown, so the field is either omitted
// or marshalled as `null` depending on the request struct's tags.
func optionalBoolPtr(val types.Bool) *bool {
	if val.IsNull() || val.IsUnknown() {
		return nil
	}
	v := val.ValueBool()
	return &v
}

// optionalStringPtr converts an optional types.String to a *string.
// Returns nil if the value is null or unknown. Note that an explicitly
// configured empty string yields a pointer to "", which is how a setting is
// cleared on Fleet's PATCH endpoints.
func optionalStringPtr(val types.String) *string {
	if val.IsNull() || val.IsUnknown() {
		return nil
	}
	v := val.ValueString()
	return &v
}

// The refreshOptional* helpers implement the opt-in convention used by the
// nested settings blocks on fleetdm_fleet: an attribute that is Optional but
// not Computed must stay null when the practitioner never configured it.
//
// Fleet returns a concrete value for every field regardless of whether it was
// ever set (an unset host_batch_size reads back as 0), so refreshing
// unconditionally would write that 0 into state. For an Optional-only
// attribute that makes state disagree with config, which Terraform rejects as
// "provider produced inconsistent result after apply". Keeping the prior null
// also means these attributes never manufacture a permanent diff.
//
// A nil apiValue likewise leaves the prior value alone: the API omitted the
// field rather than reporting it as absent.

// refreshOptionalBool refreshes a bool attribute only if it was already set.
func refreshOptionalBool(prior types.Bool, apiValue *bool) types.Bool {
	if prior.IsNull() || apiValue == nil {
		return prior
	}
	return types.BoolValue(*apiValue)
}

// refreshOptionalString refreshes a string attribute only if it was already set.
func refreshOptionalString(prior types.String, apiValue *string) types.String {
	if prior.IsNull() || apiValue == nil {
		return prior
	}
	return types.StringValue(*apiValue)
}

// refreshOptionalInt64 refreshes an int attribute only if it was already set.
func refreshOptionalInt64(prior types.Int64, apiValue *int) types.Int64 {
	if prior.IsNull() || apiValue == nil {
		return prior
	}
	return types.Int64Value(int64(*apiValue))
}

// refreshOptionalFloat64 refreshes a float attribute only if it was already set.
func refreshOptionalFloat64(prior types.Float64, apiValue *float64) types.Float64 {
	if prior.IsNull() || apiValue == nil {
		return prior
	}
	return types.Float64Value(*apiValue)
}

// intPtrToInt64 converts a *int to a types.Int64, returning Null for nil pointers.
func intPtrToInt64(val *int) types.Int64 {
	if val != nil {
		return types.Int64Value(int64(*val))
	}
	return types.Int64Null()
}

// stringPtrToString converts a *string to a types.String, returning Null for nil pointers.
func stringPtrToString(val *string) types.String {
	if val != nil {
		return types.StringValue(*val)
	}
	return types.StringNull()
}

// platformListToString converts a types.List of platform strings to a comma-separated string for the API.
func platformListToString(ctx context.Context, list types.List) string {
	if list.IsNull() || list.IsUnknown() || len(list.Elements()) == 0 {
		return ""
	}
	var platforms []string
	list.ElementsAs(ctx, &platforms, false)
	return strings.Join(platforms, ",")
}

// platformStringToList converts a comma-separated platform string from the API to a types.List.
func platformStringToList(s string) types.List {
	if s == "" {
		return types.ListValueMust(types.StringType, []attr.Value{})
	}
	parts := strings.Split(s, ",")
	values := make([]attr.Value, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			values = append(values, types.StringValue(p))
		}
	}
	if len(values) == 0 {
		return types.ListValueMust(types.StringType, []attr.Value{})
	}
	return types.ListValueMust(types.StringType, values)
}

// platformPlanClears reports whether the planned platform list goes from a
// non-empty state to an empty (or null) plan. This is the exact case Fleet's
// PATCH endpoints cannot honor: the request marshals to `platform: ""` which
// the `omitempty` JSON tag drops, leaving Fleet's stored value unchanged and
// producing a "Provider produced inconsistent result after apply" error.
//
// Subset shrinks (e.g. `["darwin","linux"] -> ["darwin"]`) and swaps
// (`["darwin"] -> ["linux"]`) are NOT considered clears — Fleet honors those
// in-place because a non-empty platform string is sent and overwrites the
// stored value.
//
// Null/unknown plan values short-circuit to false because we can't evaluate
// the change yet.
func platformPlanClears(ctx context.Context, state, plan types.List) (bool, diag.Diagnostics) {
	if plan.IsUnknown() {
		return false, nil
	}

	stateEmpty := state.IsNull() || state.IsUnknown() || len(state.Elements()) == 0
	planEmpty := plan.IsNull() || len(plan.Elements()) == 0
	if stateEmpty || !planEmpty {
		return false, nil
	}

	// Sanity-check we can actually read the state list — surfaces conversion
	// errors instead of silently swallowing them.
	var entries []string
	d := state.ElementsAs(ctx, &entries, false)
	if d.HasError() {
		return false, d
	}
	return len(entries) > 0, nil
}

// requireReplaceOnPlatformShrink returns a list plan modifier that forces
// resource replacement when the user clears a previously-set platform list
// (non-empty -> empty/null). Fleet's PATCH endpoints for queries/reports and
// policies cannot clear `platform` once it has been set to a non-empty value —
// the request body omits the field via `omitempty`, Fleet treats that as
// "no change", and Terraform aborts with a
// "Provider produced inconsistent result after apply" error.
//
// Subset shrinks and swaps are left in-place because Fleet honors any
// non-empty platform value sent in PATCH.
//
// Name kept as "...PlatformShrink" rather than "...PlatformClear" because
// "shrink" is the user-facing operation (removing entries) that ends up
// triggering replacement; the implementation-level trigger happens to be the
// total clear specifically.
func requireReplaceOnPlatformShrink() planmodifier.List {
	return listplanmodifier.RequiresReplaceIf(
		func(ctx context.Context, req planmodifier.ListRequest, resp *listplanmodifier.RequiresReplaceIfFuncResponse) {
			clears, d := platformPlanClears(ctx, req.StateValue, req.PlanValue)
			resp.Diagnostics.Append(d...)
			if d.HasError() {
				return
			}
			resp.RequiresReplace = clears
		},
		"Replace the resource if the planned platform list clears a previously-set non-empty value.",
		"Replace the resource if the planned `platform` list clears a previously-set non-empty value. Fleet's API drops empty `platform` from PATCH requests (`omitempty`), so an in-place clear would silently leave the stored value unchanged.",
	)
}

// userTeamAttrTypes defines the Terraform object type for user team assignments.
var userTeamAttrTypes = map[string]attr.Type{
	"id":   types.Int64Type,
	"name": types.StringType,
	"role": types.StringType,
}

// mapUserTeamsToList converts a FleetDM UserTeam slice to a Terraform List.
// Used by both user and users data sources.
func mapUserTeamsToList(_ context.Context, teams []fleetdm.UserTeam, diags *diag.Diagnostics) types.List {
	if len(teams) == 0 {
		return types.ListNull(types.ObjectType{AttrTypes: userTeamAttrTypes})
	}

	teamElements := make([]attr.Value, len(teams))
	for i, t := range teams {
		teamObj, dd := types.ObjectValue(
			userTeamAttrTypes,
			map[string]attr.Value{
				"id":   types.Int64Value(t.ID),
				"name": types.StringValue(t.Name),
				"role": types.StringValue(t.Role),
			},
		)
		if dd.HasError() {
			diags.Append(dd...)
			return types.ListNull(types.ObjectType{AttrTypes: userTeamAttrTypes})
		}
		teamElements[i] = teamObj
	}

	teamList, dd := types.ListValue(types.ObjectType{AttrTypes: userTeamAttrTypes}, teamElements)
	if dd.HasError() {
		diags.Append(dd...)
		return types.ListNull(types.ObjectType{AttrTypes: userTeamAttrTypes})
	}

	return teamList
}
