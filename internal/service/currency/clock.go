package currency

import (
	"context"
	"time"
)

// asOfKey carries the evaluation instant in a context.
type asOfKey struct{}

// withNow returns ctx evaluating currency at now instead of the current time.
func withNow(ctx context.Context, now time.Time) context.Context {
	return context.WithValue(ctx, asOfKey{}, now)
}

// nowFrom returns the evaluation instant carried by ctx, or the current time.
func nowFrom(ctx context.Context) time.Time {
	if t, ok := ctx.Value(asOfKey{}).(time.Time); ok {
		return t
	}
	return time.Now()
}

// daysUntil returns the whole days from the evaluation instant to t.
func daysUntil(ctx context.Context, t time.Time) int {
	return int(t.Sub(nowFrom(ctx)).Hours() / 24)
}

// dateAt returns noon UTC on the date of t, the instant an as-of-date evaluation runs at.
func dateAt(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, time.UTC)
}
