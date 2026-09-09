package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// unorderedStringListType is a list of strings whose element order carries no
// meaning. Fleet's label and category targeting is a set: it returns names in
// its own order (by label id), which need not match the order written in HCL.
// With a plain list that mismatch is a permanent diff on every plan, and an
// update that rewrites the same set.
//
// Semantic equality lets the framework keep the configured value whenever the
// refreshed value holds the same names, so order-only differences stop being
// changes. The wire type stays a list, so no state migration is needed.
type unorderedStringListType struct {
	basetypes.ListType
}

var _ basetypes.ListTypable = unorderedStringListType{}

func newUnorderedStringListType() unorderedStringListType {
	return unorderedStringListType{
		ListType: basetypes.ListType{ElemType: basetypes.StringType{}},
	}
}

func (t unorderedStringListType) Equal(o attr.Type) bool {
	other, ok := o.(unorderedStringListType)
	if !ok {
		return false
	}
	return t.ListType.Equal(other.ListType)
}

func (t unorderedStringListType) String() string {
	return "provider.unorderedStringListType"
}

func (t unorderedStringListType) ValueFromList(_ context.Context, in basetypes.ListValue) (basetypes.ListValuable, diag.Diagnostics) {
	return unorderedStringList{ListValue: in}, nil
}

func (t unorderedStringListType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	attrValue, err := t.ListType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	listValue, ok := attrValue.(basetypes.ListValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", attrValue)
	}
	listValuable, diags := t.ValueFromList(ctx, listValue)
	if diags.HasError() {
		return nil, fmt.Errorf("unexpected error converting ListValue to ListValuable: %v", diags)
	}
	return listValuable, nil
}

func (t unorderedStringListType) ValueType(ctx context.Context) attr.Value {
	return unorderedStringList{ListValue: t.ListType.ValueType(ctx).(basetypes.ListValue)}
}

// unorderedStringList is the value type for unorderedStringListType.
type unorderedStringList struct {
	basetypes.ListValue
}

var _ basetypes.ListValuableWithSemanticEquals = unorderedStringList{}

func (v unorderedStringList) Type(_ context.Context) attr.Type {
	return newUnorderedStringListType()
}

func (v unorderedStringList) Equal(o attr.Value) bool {
	other, ok := o.(unorderedStringList)
	if !ok {
		return false
	}
	return v.ListValue.Equal(other.ListValue)
}

// ListSemanticEquals treats the two lists as equal when they hold the same
// names, regardless of order. Duplicates are significant to the comparison —
// they are not valid input anyway, and collapsing them here would hide a
// genuine difference.
func (v unorderedStringList) ListSemanticEquals(ctx context.Context, newValuable basetypes.ListValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	newValue, ok := newValuable.(unorderedStringList)
	if !ok {
		return false, diags
	}

	// Null and unknown are never semantically equal to a known value; the
	// framework only calls this for two known values, but be explicit.
	if v.IsNull() != newValue.IsNull() || v.IsUnknown() != newValue.IsUnknown() {
		return false, diags
	}
	if v.IsNull() || v.IsUnknown() {
		return true, diags
	}

	var oldNames, newNames []string
	diags.Append(v.ElementsAs(ctx, &oldNames, false)...)
	diags.Append(newValue.ElementsAs(ctx, &newNames, false)...)
	if diags.HasError() {
		return false, diags
	}
	if len(oldNames) != len(newNames) {
		return false, diags
	}

	counts := make(map[string]int, len(oldNames))
	for _, n := range oldNames {
		counts[n]++
	}
	for _, n := range newNames {
		counts[n]--
		if counts[n] < 0 {
			return false, diags
		}
	}
	return true, diags
}
