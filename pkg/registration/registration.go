// Package registration normalises aircraft registrations to their canonical
// notation.
//
// A registration is a nationality mark followed by a registration mark. ICAO
// Annex 7 requires the two to be separated by a hyphen when the registration
// mark starts with a letter, which is why most of the world writes D-EABC,
// G-ABCD or HB-PNT — but the United States, Japan and South Korea draw
// numeric registration marks and run them together (N12345, JA8089, HL7747),
// while China and Russia hyphenate numeric marks anyway (B-1234, RA-12345).
// The notation therefore cannot be derived from the string; it has to be
// looked up per state of registry. See prefixes.go for the table and its
// sources.
//
// Normalisation is deliberately conservative. A registration is only
// rewritten when its nationality mark is recognised *and* the rest of the
// string has the shape that state issues. Anything else — a simulator
// identifier, an aircraft type entered in the wrong field, a mark this table
// has not caught up with — is returned uppercased and trimmed but otherwise
// untouched, so the normaliser can never turn a value it does not understand
// into a different value.
package registration

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Entry is one state's nationality mark.
type Entry struct {
	// Prefix is the nationality mark itself, uppercase and without a
	// separator: "D", "N", "9XR", "RDPL".
	Prefix string
	// Country is the ISO 3166-1 alpha-2 code of the state of registry.
	Country string
	// CountryName is the English name of the state of registry.
	CountryName string
	// NoHyphen marks the states that write the nationality mark and the
	// registration mark without a separator. The zero value — hyphenated —
	// is the common case.
	NoHyphen bool
	// Suffix constrains the registration mark; empty means defaultSuffix.
	// See prefixes.go for when a pattern is worth spelling out.
	Suffix string

	re *regexp.Regexp
}

// Hyphenated reports whether this state's canonical notation separates the
// nationality mark from the registration mark with a hyphen.
func (e Entry) Hyphenated() bool { return !e.NoHyphen }

// Result is the outcome of normalising one registration.
type Result struct {
	// Value is the normalised registration: always uppercase and trimmed,
	// and rewritten into canonical notation when Matched is true.
	Value string
	// Entry is the matched nationality mark, zero when Matched is false.
	Entry Entry
	// Matched reports whether a nationality mark was recognised and the
	// registration mark had the shape that state issues. When false, Value
	// carries the input with only case and whitespace cleaned up.
	Matched bool
}

// matchOrder is entries sorted longest mark first, so "D2" is considered
// before "D" and "9XR" before "9X". Built once at init.
var matchOrder []Entry

// byPrefix indexes entries by nationality mark.
var byPrefix map[string]Entry

func init() {
	matchOrder = make([]Entry, len(entries))
	copy(matchOrder, entries)
	byPrefix = make(map[string]Entry, len(entries))

	for i := range matchOrder {
		e := &matchOrder[i]
		pattern := e.Suffix
		if pattern == "" {
			pattern = defaultSuffix
		}
		e.re = regexp.MustCompile("^(?:" + pattern + ")$")
		if _, dup := byPrefix[e.Prefix]; dup {
			panic(fmt.Sprintf("registration: duplicate nationality mark %q in table", e.Prefix))
		}
		byPrefix[e.Prefix] = *e
	}

	sort.SliceStable(matchOrder, func(i, j int) bool {
		return len(matchOrder[i].Prefix) > len(matchOrder[j].Prefix)
	})
}

// Normalize rewrites a registration into the canonical notation of its state
// of registry. See the package comment for what happens to input it does not
// recognise.
func Normalize(raw string) Result {
	cleaned := clean(raw)
	bare := bareForm(cleaned)
	if bare == "" {
		return Result{Value: cleaned}
	}

	for _, e := range matchOrder {
		mark, ok := strings.CutPrefix(bare, e.Prefix)
		if !ok || mark == "" || !e.re.MatchString(mark) {
			continue
		}
		value := e.Prefix + mark
		if e.Hyphenated() {
			value = e.Prefix + "-" + mark
		}
		return Result{Value: value, Entry: e, Matched: true}
	}

	return Result{Value: cleaned}
}

// Canonical is Normalize for callers that only want the normalised string.
func Canonical(raw string) string { return Normalize(raw).Value }

// Lookup returns the table entry for a nationality mark.
func Lookup(prefix string) (Entry, bool) {
	e, ok := byPrefix[strings.ToUpper(strings.TrimSpace(prefix))]
	return e, ok
}

// Entries returns the nationality mark table, longest mark first. The
// returned slice is a copy; the caller may not mutate the package's table
// through it.
func Entries() []Entry {
	out := make([]Entry, len(matchOrder))
	copy(out, matchOrder)
	return out
}

// Count is how many nationality marks the table holds.
func Count() int { return len(matchOrder) }

// clean uppercases, trims, and collapses internal whitespace runs to a single
// space. This is the most that is done to input whose nationality mark is not
// recognised, so it must never reorder or drop characters.
func clean(raw string) string {
	return strings.Join(strings.Fields(strings.ToUpper(raw)), " ")
}

// bareForm reduces a cleaned registration to the characters that carry
// meaning, dropping the separators a pilot might type: hyphens of any dash
// flavour, spaces, dots, slashes, underscores. The result is what the
// nationality mark is matched against.
func bareForm(cleaned string) string {
	var b strings.Builder
	b.Grow(len(cleaned))
	for _, r := range cleaned {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
