package fleetdm

import "encoding/json"

// Nullable is a request field with three states, for PATCH endpoints where
// leaving a key out means "no change" and null means "clear": the zero value
// is omitted (use it with the omitzero tag), Null sends null, and Value sends
// the value.
type Nullable[T any] struct {
	set   bool
	value *T
}

// Null returns a Nullable that is sent as JSON null.
func Null[T any]() Nullable[T] { return Nullable[T]{set: true} }

// Value returns a Nullable that is sent as v.
func Value[T any](v T) Nullable[T] { return Nullable[T]{set: true, value: &v} }

// IsZero reports whether the field is left out of the request.
func (n Nullable[T]) IsZero() bool { return !n.set }

// Get returns the value and whether one is set; a Null field returns false.
func (n Nullable[T]) Get() (T, bool) {
	if n.value == nil {
		var zero T
		return zero, false
	}
	return *n.value, true
}

// MarshalJSON writes null for Null and the value otherwise.
func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if n.value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*n.value)
}
