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
