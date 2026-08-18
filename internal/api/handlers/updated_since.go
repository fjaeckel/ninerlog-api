package handlers

import "time"

// deltaWatermark normalises the generated `updatedSince` query parameter: a
// pointer to a zero time.Time (an explicitly empty `?updatedSince=`) becomes
// "no filter"; a bare `YYYY-MM-DD` passes through as midnight UTC.
func deltaWatermark(updatedSince *time.Time) *time.Time {
	if updatedSince == nil || updatedSince.IsZero() {
		return nil
	}
	return updatedSince
}
