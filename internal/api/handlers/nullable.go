package handlers

import "github.com/oapi-codegen/nullable"

// applyNullable implements JSON Merge Patch (RFC 7386) semantics for a single
// optional, nullable request field: if the field was omitted, dst is left
// untouched; if it was explicitly set to JSON null, dst is cleared; otherwise
// dst is set to the provided value.
func applyNullable[T any](dst **T, n nullable.Nullable[T]) {
	if !n.IsSpecified() {
		return
	}
	if n.IsNull() {
		*dst = nil
		return
	}
	v, _ := n.Get()
	*dst = &v
}

// applyOverride applies an optional, nullable request field to an
// auto-calculated value and its override flag: omitted leaves both untouched;
// JSON null zeroes the value and clears the flag so the next calculation
// derives it; a number sets the value and the flag.
func applyOverride(dst *int, override *bool, n nullable.Nullable[int]) {
	if !n.IsSpecified() {
		return
	}
	if n.IsNull() {
		*dst = 0
		*override = false
		return
	}
	v, _ := n.Get()
	*dst = v
	*override = true
}
