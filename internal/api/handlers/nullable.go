package handlers

import (
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

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

// applyNullableULKind applies an optional, nullable ultralight kind field.
func applyNullableULKind(dst **models.ULKind, n nullable.Nullable[string]) {
	if !n.IsSpecified() {
		return
	}
	if n.IsNull() {
		*dst = nil
		return
	}
	v, _ := n.Get()
	k := models.ULKind(v)
	*dst = &k
}

// applyNullableDate is applyNullable for a calendar-date field.
func applyNullableDate(dst **time.Time, n nullable.Nullable[openapi_types.Date]) {
	if !n.IsSpecified() {
		return
	}
	if n.IsNull() {
		*dst = nil
		return
	}
	v, _ := n.Get()
	t := v.Time
	*dst = &t
}

// dateToTimePtr converts an optional calendar date to *time.Time.
func dateToTimePtr(d *openapi_types.Date) *time.Time {
	if d == nil {
		return nil
	}
	t := d.Time
	return &t
}

// timeToDatePtr converts an optional *time.Time to a calendar date.
func timeToDatePtr(t *time.Time) *openapi_types.Date {
	if t == nil {
		return nil
	}
	return &openapi_types.Date{Time: *t}
}
