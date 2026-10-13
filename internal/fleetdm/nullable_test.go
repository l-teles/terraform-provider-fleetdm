package fleetdm

import (
	"encoding/json"
	"testing"
)

func TestNullableMarshal(t *testing.T) {
	type body struct {
		ID Nullable[int64] `json:"id,omitzero"`
	}
	for _, tc := range []struct {
		name string
		in   Nullable[int64]
		want string
	}{
		{"zero value is left out", Nullable[int64]{}, `{}`},
		{"null is sent as null", Null[int64](), `{"id":null}`},
		{"value is sent", Value(int64(42)), `{"id":42}`},
		{"zero is a value, not omitted", Value(int64(0)), `{"id":0}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(body{ID: tc.in})
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != tc.want {
				t.Errorf("got %s, want %s", raw, tc.want)
			}
		})
	}
}
