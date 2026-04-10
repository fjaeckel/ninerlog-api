// Package duration provides helpers for converting between flight time representations.
// All internal storage uses integer minutes for lossless precision.
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

// hmRe matches "1h23m", "1h 23m", "1h", "23m" formats.
var hmRe = regexp.MustCompile(`^(?:(\d+)\s*h)?\s*(?:(\d+)\s*m)?$`)

// ParseDuration parses a flexible duration string into integer minutes.
// Accepted formats:
//   - "1:23" or "01:23" → 83 minutes (colon-separated hours:minutes)
//   - "1h23m" or "1h 23m" → 83 minutes
//   - "1h" → 60 minutes
//   - "23m" → 23 minutes
//   - "83" → 83 minutes (bare integer = minutes)
//   - "1.38" or "1.5" → decimal hours converted to minutes (83 or 90)
func ParseDuration(input string) (int, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
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
			hours := 0
			mins := 0
			if m[1] != "" {
				hours, _ = strconv.Atoi(m[1])
			}
			if m[2] != "" {
				mins, _ = strconv.Atoi(m[2])
			}
			return hours*60 + mins, nil
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
