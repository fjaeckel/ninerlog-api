// Package duration provides helpers for converting between flight time representations.
// All internal storage uses integer minutes.
package duration

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// MinutesToDecimalHours converts minutes to decimal hours for display/export.
func MinutesToDecimalHours(minutes int) float64 {
	return float64(minutes) / 60.0
}

// DecimalHoursToMinutes converts decimal hours to integer minutes via rounding.
func DecimalHoursToMinutes(hours float64) int {
	return int(math.Round(hours * 60))
}

// FormatHM returns a human-readable hours:minutes string, e.g. "1h 23m".
func FormatHM(minutes int) string {
	if minutes == 0 {
		return "0h 0m"
	}
	h := minutes / 60
	m := minutes % 60
	return fmt.Sprintf("%dh %dm", h, m)
}

// FormatDecimal returns a decimal hours string with 1 decimal place, e.g. "1.4h".
func FormatDecimal(minutes int) string {
	h := float64(minutes) / 60.0
	return fmt.Sprintf("%.1fh", h)
}

// FormatColonHM returns a colon-separated hours:minutes string, e.g. "1:23".
// Suitable for EASA logbook PDF export.
func FormatColonHM(minutes int) string {
	h := minutes / 60
	m := minutes % 60
	return fmt.Sprintf("%d:%02d", h, m)
}

// colonRe matches "H:MM" or "HH:MM" or "HHH:MM" format.
var colonRe = regexp.MustCompile(`^(\d+):(\d{1,2})$`)

// hmRe matches "1h23m", "1h 23m", "1h", "23m", and decimal-hour "1.5h" formats.
var hmRe = regexp.MustCompile(`^(?:(\d+(?:\.\d+)?)\s*h)?\s*(?:(\d+)\s*m)?$`)

// ParseDuration parses a flexible duration string into integer minutes.
// Accepted formats:
//   - "1:23" or "01:23" → 83 minutes (colon-separated hours:minutes)
//   - "1h23m" or "1h 23m" → 83 minutes
//   - "1h" → 60 minutes
//   - "23m" → 23 minutes
//   - "83" → 83 minutes (bare integer = minutes)
//   - "1.38" or "1.5" → decimal hours converted to minutes (83 or 90)
//   - "1.5h" or "1,5" → decimal-hour variants (h suffix, comma separator)
func ParseDuration(input string) (int, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
	}
	// European-style decimal comma ("1,5" / "1,5h") → dot form.
	if strings.Count(s, ",") == 1 && !strings.Contains(s, ".") {
		s = strings.Replace(s, ",", ".", 1)
	}

	// Try colon format: "1:23"
	if m := colonRe.FindStringSubmatch(s); m != nil {
		hours, _ := strconv.Atoi(m[1])
		mins, _ := strconv.Atoi(m[2])
		if mins >= 60 {
			return 0, fmt.Errorf("minutes part must be 0-59, got %d", mins)
		}
		return hours*60 + mins, nil
	}

	// Try "Xh Ym" format
	if strings.ContainsAny(s, "hHmM") {
		lower := strings.ToLower(s)
		if m := hmRe.FindStringSubmatch(lower); m != nil && (m[1] != "" || m[2] != "") {
			hours := 0.0
			mins := 0
			if m[1] != "" {
				hours, _ = strconv.ParseFloat(m[1], 64)
			}
			if m[2] != "" {
				mins, _ = strconv.Atoi(m[2])
			}
			return DecimalHoursToMinutes(hours) + mins, nil
		}
		return 0, fmt.Errorf("invalid hm format: %q", input)
	}

	// Try decimal (contains dot) → decimal hours
	if strings.Contains(s, ".") {
		hours, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid decimal duration: %q", input)
		}
		if hours < 0 {
			return 0, fmt.Errorf("duration cannot be negative")
		}
		return DecimalHoursToMinutes(hours), nil
	}

	// Bare integer → minutes
	mins, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration: %q", input)
	}
	if mins < 0 {
		return 0, fmt.Errorf("duration cannot be negative")
	}
	return mins, nil
}

// Unit is how a bare number in a duration cell is read. Colon ("1:18") and
// suffixed ("1h 18m") forms parse the same under every unit.
type Unit string

const (
	// UnitAuto reads a bare integer as minutes and a fraction as decimal hours.
	UnitAuto Unit = "auto"
	// UnitDecimalHours reads every bare number as hours ("2" → 120).
	UnitDecimalHours Unit = "decimal"
	// UnitMinutes reads every bare number as whole minutes ("90" → 90).
	UnitMinutes Unit = "minutes"
)

// bareNumberRe matches an unsigned bare number with an optional dot fraction.
var bareNumberRe = regexp.MustCompile(`^\d+(?:\.\d+)?$`)

// ParseDurationAs parses input like ParseDuration, reading a bare number in
// the given unit.
func ParseDurationAs(input string, unit Unit) (int, error) {
	s := strings.TrimSpace(input)
	if strings.Count(s, ",") == 1 && !strings.Contains(s, ".") {
		s = strings.Replace(s, ",", ".", 1)
	}
	if !bareNumberRe.MatchString(s) {
		return ParseDuration(input)
	}
	switch unit {
	case UnitDecimalHours:
		hours, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid decimal duration: %q", input)
		}
		return DecimalHoursToMinutes(hours), nil
	case UnitMinutes:
		mins, err := strconv.Atoi(s)
		if err != nil {
			return 0, fmt.Errorf("invalid minutes duration: %q", input)
		}
		return mins, nil
	default:
		return ParseDuration(input)
	}
}

// InferUnit returns UnitDecimalHours when the values are all bare numbers
// and at least one has a fraction ("1.3", "1,5"), and UnitAuto otherwise.
// Empty values are ignored.
func InferUnit(values []string) Unit {
	fraction := false
	for _, v := range values {
		s := strings.TrimSpace(v)
		if s == "" {
			continue
		}
		if strings.Count(s, ",") == 1 && !strings.Contains(s, ".") {
			s = strings.Replace(s, ",", ".", 1)
		}
		if !bareNumberRe.MatchString(s) {
			return UnitAuto
		}
		if strings.Contains(s, ".") {
			fraction = true
		}
	}
	if fraction {
		return UnitDecimalHours
	}
	return UnitAuto
}
