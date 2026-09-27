package engine

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Date is a calendar date without time or zone.
type Date struct {
	t time.Time
}

// NewDate returns the date y-m-d.
func NewDate(y int, m time.Month, d int) Date {
	return Date{time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
}

// ParseDate parses YYYY-MM-DD.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return Date{}, fmt.Errorf("invalid date %q: want YYYY-MM-DD", s)
	}
	return Date{t}, nil
}

// MustDate parses YYYY-MM-DD and panics on error.
func MustDate(s string) Date {
	d, err := ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

// IsZero reports whether d is unset.
func (d Date) IsZero() bool { return d.t.IsZero() }

func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.t.Format(time.DateOnly)
}

// Year, Month and Day return the date's parts.
func (d Date) Year() int          { return d.t.Year() }
func (d Date) Month() time.Month  { return d.t.Month() }
func (d Date) Day() int           { return d.t.Day() }
func (d Date) Before(o Date) bool { return d.t.Before(o.t) }
func (d Date) After(o Date) bool  { return d.t.After(o.t) }
func (d Date) Equal(o Date) bool  { return d.t.Equal(o.t) }

// Compare returns -1, 0 or 1.
func (d Date) Compare(o Date) int { return d.t.Compare(o.t) }

// AddDays returns d plus n days.
func (d Date) AddDays(n int) Date { return Date{d.t.AddDate(0, 0, n)} }

// AddMonths returns d plus n months, clamping the day to the target month's last day.
func (d Date) AddMonths(n int) Date {
	first := time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, n, 0)
	last := daysIn(first.Year(), first.Month())
	day := min(d.Day(), last)
	return NewDate(first.Year(), first.Month(), day)
}

// FirstOfMonth returns the first day of d's month.
func (d Date) FirstOfMonth() Date { return NewDate(d.Year(), d.Month(), 1) }

// LastOfMonth returns the last day of d's month.
func (d Date) LastOfMonth() Date { return NewDate(d.Year(), d.Month(), daysIn(d.Year(), d.Month())) }

// DaysUntil returns the number of days from d to o.
func (d Date) DaysUntil(o Date) int { return int(o.t.Sub(d.t).Hours() / 24) }

// YearsBetween returns completed years from d to o (age on o for a birth date d).
func (d Date) YearsBetween(o Date) int {
	y := o.Year() - d.Year()
	if o.Month() < d.Month() || (o.Month() == d.Month() && o.Day() < d.Day()) {
		y--
	}
	return y
}

func daysIn(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// UnmarshalYAML accepts YYYY-MM-DD.
func (d *Date) UnmarshalYAML(n *yaml.Node) error {
	p, err := ParseDate(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: %w", n.Line, err)
	}
	*d = p
	return nil
}

// MarshalYAML writes YYYY-MM-DD.
func (d Date) MarshalYAML() (any, error) { return d.String(), nil }

// MarshalJSON writes "YYYY-MM-DD".
func (d Date) MarshalJSON() ([]byte, error) { return []byte(`"` + d.String() + `"`), nil }

func minDate(a, b Date) Date {
	if b.Before(a) {
		return b
	}
	return a
}
