package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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

// isNotFound returns true if the error is a FleetDM API 404 error.
func isNotFound(err error) bool {
	apiErr, ok := err.(*fleetdm.APIError)
	return ok && apiErr.StatusCode == 404
}

// parseIDFromString parses a numeric string ID and adds a diagnostic error on failure.
// Returns the parsed int and true on success, or 0 and false on failure.
func parseIDFromString(id string, resourceName string, diagnostics *diag.Diagnostics) (int64, bool) {
	parsed, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		diagnostics.AddError(
			fmt.Sprintf("Error Importing FleetDM %s", resourceName),
			fmt.Sprintf("Could not parse %s ID '%s': %s", resourceName, id, err),
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
