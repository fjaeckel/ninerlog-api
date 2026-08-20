// Package registration normalises aircraft registrations to the canonical
// notation of their state of registry, looked up in the nationality mark
// table in prefixes.go.
//
// A registration is only rewritten when its nationality mark is recognised
// and the remainder matches the shape that state issues. Anything else is
// returned uppercased and trimmed, otherwise unchanged.
//
// Design and maintenance: docs/AIRCRAFT_REGISTRATIONS.md.
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
	// NoHyphen marks states that write the two marks without a separator.
	// The zero value is hyphenated.
	NoHyphen bool
	// Suffix constrains the registration mark; empty means defaultSuffix.
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
	// registration mark had the shape that state issues.
	Matched bool
}

// matchOrder is entries sorted longest mark first. Built at init.
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
// of registry, leaving unrecognised input uppercased and trimmed.
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

// Entries returns a copy of the nationality mark table, longest mark first.
func Entries() []Entry {
	out := make([]Entry, len(matchOrder))
	copy(out, matchOrder)
	return out
}

// Count is how many nationality marks the table holds.
func Count() int { return len(matchOrder) }

// clean uppercases, trims, and collapses internal whitespace runs to a single
// space. It never reorders or drops characters.
func clean(raw string) string {
	return strings.Join(strings.Fields(strings.ToUpper(raw)), " ")
}

// bareForm strips every separator from a cleaned registration, leaving the
// alphanumerics the nationality mark is matched against.
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
