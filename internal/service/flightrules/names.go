package flightrules

import (
	"strings"
	"unicode"
)

// NormalizeName returns a comparable form of a person name: lower-case,
// trimmed of leading/trailing whitespace, with all runs of internal
// whitespace collapsed to a single ASCII space, and Unicode whitespace
// (e.g. U+00A0 non-breaking space, tabs, newlines) folded to ASCII space.
//
// This is the only place name normalization should happen. Empty input
// yields empty output.
func NormalizeName(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := true // skip leading whitespace
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(unicode.ToLower(r))
		prevSpace = false
	}
	out := b.String()
	return strings.TrimRight(out, " ")
}

// MatchesUser reports whether `candidate` refers to the same person as
// `userName`. Comparison is case- and whitespace-insensitive, and also
// recognises the "Last, First" contact-picker form against a stored
// "First Last" profile (and vice versa).
//
// Examples (all match "Amelia Earhart"):
//   - "Amelia Earhart", "amelia earhart", "AMELIA EARHART"
//   - "  Amelia  Earhart  ", "Amelia\u00a0Earhart"
//   - "Earhart, Amelia"
//
// Examples (do NOT match): "Amelia M. Earhart", "A. Earhart".
func MatchesUser(candidate, userName string) bool {
	c := NormalizeName(candidate)
	u := NormalizeName(userName)
	if c == "" || u == "" {
		return false
	}
	if c == u {
		return true
	}
	// Try reversing "Last, First" → "First Last" on either side.
	if r := reverseCommaForm(c); r != "" && r == u {
		return true
	}
	if r := reverseCommaForm(u); r != "" && r == c {
		return true
	}
	return false
}

// reverseCommaForm turns a normalized "last, first" into "first last".
// Returns "" if the input does not contain a single comma.
func reverseCommaForm(s string) string {
	idx := strings.Index(s, ",")
	if idx <= 0 || idx == len(s)-1 {
		return ""
	}
	if strings.Count(s, ",") != 1 {
		return ""
	}
	last := strings.TrimSpace(s[:idx])
	first := strings.TrimSpace(s[idx+1:])
	if last == "" || first == "" {
		return ""
	}
	return first + " " + last
}
