package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

func mkUnordered(vals ...string) unorderedStringList {
	elems := make([]attr.Value, len(vals))
	for i, v := range vals {
		elems[i] = types.StringValue(v)
	}
	return unorderedStringList{ListValue: types.ListValueMust(types.StringType, elems)}
}

// TestUnorderedStringList_SemanticEquals covers the reason the type exists:
// Fleet returns label names in its own order, so an order-only difference
// must not read as a change. Anything that is a genuine difference still has
// to compare unequal, or a real drift would be swallowed.
func TestUnorderedStringList_SemanticEquals(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name  string
		prior unorderedStringList
		newer basetypes.ListValuable
		want  bool
	}{
		{"same order", mkUnordered("a", "b"), mkUnordered("a", "b"), true},
		{"reordered", mkUnordered("a", "b"), mkUnordered("b", "a"), true},
		{"reordered three", mkUnordered("a", "b", "c"), mkUnordered("c", "a", "b"), true},
		{"both empty", mkUnordered(), mkUnordered(), true},
		{"single", mkUnordered("a"), mkUnordered("a"), true},

		{"added name", mkUnordered("a"), mkUnordered("a", "b"), false},
		{"removed name", mkUnordered("a", "b"), mkUnordered("a"), false},
		{"swapped name", mkUnordered("a", "b"), mkUnordered("a", "c"), false},
		{"emptied", mkUnordered("a"), mkUnordered(), false},
		{"filled from empty", mkUnordered(), mkUnordered("a"), false},
		// Duplicates are not valid input, but must not be treated as a set
		// collapse — that would hide a difference.
		{"duplicate vs single", mkUnordered("a", "a"), mkUnordered("a"), false},
		{"duplicate vs distinct", mkUnordered("a", "a"), mkUnordered("a", "b"), false},

		{"null vs known", unorderedStringList{ListValue: types.ListNull(types.StringType)}, mkUnordered("a"), false},
		{"null vs null", unorderedStringList{ListValue: types.ListNull(types.StringType)},
			unorderedStringList{ListValue: types.ListNull(types.StringType)}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, diags := tc.prior.ListSemanticEquals(ctx, tc.newer)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if got != tc.want {
				t.Errorf("ListSemanticEquals = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestUnorderedStringList_TypeContract pins the bits the framework relies on:
// the value's Type() must match the schema type, and the type must round-trip
// a list value into the custom value.
func TestUnorderedStringList_TypeContract(t *testing.T) {
	ctx := context.Background()
	typ := newUnorderedStringListType()

	if !typ.Equal(newUnorderedStringListType()) {
		t.Error("type must equal another instance of itself")
	}
	if typ.Equal(types.ListType{ElemType: types.StringType}) {
		t.Error("type must not equal a plain list type")
	}

	v := mkUnordered("a")
	if !v.Type(ctx).Equal(typ) {
		t.Errorf("value Type() = %v, want %v", v.Type(ctx), typ)
	}

	valuable, diags := typ.ValueFromList(ctx, types.ListValueMust(types.StringType, []attr.Value{types.StringValue("a")}))
	if diags.HasError() {
		t.Fatalf("ValueFromList: %v", diags)
	}
	if _, ok := valuable.(unorderedStringList); !ok {
		t.Errorf("ValueFromList returned %T, want unorderedStringList", valuable)
	}

	// The element type must stay string, or state would not round-trip.
	if !typ.ElementType().Equal(types.StringType) {
		t.Errorf("ElementType() = %v, want string", typ.ElementType())
	}
}
