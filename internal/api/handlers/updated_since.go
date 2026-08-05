package handlers

import "time"

// deltaWatermark normalises the generated `updatedSince` query parameter.
//
// The generated binder hands back a pointer to a zero time.Time for an
// explicitly empty `?updatedSince=`, which would otherwise reach the database
// as the year-1 timestamp. Treating it as "no filter" keeps that request
// identical to omitting the parameter, which is what a client that serialised
// an unset watermark to an empty string means.
//
// A bare `YYYY-MM-DD` is accepted by the binder and lands here as midnight UTC
// on that date; that is a coarser but well-defined watermark, so it is passed
// through unchanged.
func deltaWatermark(updatedSince *time.Time) *time.Time {
	if updatedSince == nil || updatedSince.IsZero() {
		return nil
	}
	return updatedSince
}
