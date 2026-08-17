package registration

import (
	"strings"
	"testing"
	"time"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		country string // expected ISO code; "" means the input must not match
	}{
		// EASA states: the hyphen is mandatory and gets inserted, moved or
		// kept as needed.
		{"german already canonical", "D-EABC", "D-EABC", "DE"},
		{"german missing hyphen", "DEABC", "D-EABC", "DE"},
		{"german lowercase", "d-eabc", "D-EABC", "DE"},
		{"german hyphen in wrong place", "DE-ABC", "D-EABC", "DE"},
		{"german space separator", "D EABC", "D-EABC", "DE"},
		{"german surrounding whitespace", "  D-EABC  ", "D-EABC", "DE"},
		{"german airliner", "DAIBL", "D-AIBL", "DE"},
		{"german ultralight", "DMABC", "D-MABC", "DE"},
		{"german glider is numeric", "D1234", "D-1234", "DE"},
		{"uk", "GABCD", "G-ABCD", "GB"},
		{"france", "FGABC", "F-GABC", "FR"},
		{"italy", "IABCD", "I-ABCD", "IT"},
		{"austria", "OEABC", "OE-ABC", "AT"},
		{"switzerland", "HBPNT", "HB-PNT", "CH"},
		{"netherlands", "PHABC", "PH-ABC", "NL"},
		{"belgium", "OOABC", "OO-ABC", "BE"},
		{"spain", "ECABC", "EC-ABC", "ES"},
		{"sweden", "SEABC", "SE-ABC", "SE"},
		{"poland", "SPABC", "SP-ABC", "PL"},
		{"czechia", "OKABC", "OK-ABC", "CZ"},
		{"isle of man", "MABCD", "M-ABCD", "IM"},

		// Non-hyphenating states: a hyphen the pilot typed is removed.
		{"us canonical", "N12345", "N12345", "US"},
		{"us with hyphen", "N-12345", "N12345", "US"},
		{"us lowercase", "n123ab", "N123AB", "US"},
		{"us short", "N1", "N1", "US"},
		{"us alphanumeric", "N737BA", "N737BA", "US"},
		{"japan", "JA-8089", "JA8089", "JP"},
		{"japan alphanumeric", "JA01AA", "JA01AA", "JP"},
		{"south korea", "HL-7747", "HL7747", "KR"},

		// Hyphenating states with numeric registration marks — the reason
		// the Annex 7 letter/digit rule is not enough on its own.
		{"china", "B1234", "B-1234", "CN"},
		{"taiwan", "B12345", "B-12345", "CN"},
		{"hong kong", "BHKA", "B-HKA", "CN"},
		{"russia", "RA12345", "RA-12345", "RU"},
		{"cuba", "CUT1234", "CU-T1234", "CU"},
		{"bolivia", "CP1234", "CP-1234", "BO"},

		// Longest-mark-first matching.
		{"angola beats germany", "D2ABC", "D2-ABC", "AO"},
		{"cabo verde beats germany", "D4ABC", "D4-ABC", "CV"},
		{"fiji beats germany", "DQFAB", "DQ-FAB", "FJ"},
		{"chile beats canada", "CCAAA", "CC-AAA", "CL"},
		{"portugal beats canada", "CSTMB", "CS-TMB", "PT"},
		{"canada when no two-letter mark applies", "CFABC", "C-FABC", "CA"},
		{"png beats north korea", "P2ABC", "P2-ABC", "PG"},
		{"north korea", "P618", "P-618", "KP"},
		{"albania beats zimbabwe", "ZAABC", "ZA-ABC", "AL"},
		{"zimbabwe", "ZWKM", "Z-WKM", "ZW"},
		{"new zealand", "ZKABC", "ZK-ABC", "NZ"},
		{"rwanda three-character mark", "9XRAA", "9XR-AA", "RW"},
		{"bahrain three-character mark", "A9CAA", "A9C-AA", "BH"},
		{"laos four-character mark", "RDPL34123", "RDPL-34123", "LA"},

		// The disambiguation that motivates the suffix patterns: a German
		// glider whose registration mark starts with a digit that also names
		// another state.
		{"german glider not angola", "D-2345", "D-2345", "DE"},
		{"german glider not cabo verde", "D4123", "D-4123", "DE"},
		{"german glider not comoros", "D6789", "D-6789", "DE"},

		// Unrecognised input is cleaned but never rewritten.
		{"simulator", "SIM", "SIM", ""},
		{"fnpt", "FNPT2", "FNPT2", ""},
		{"aircraft type in the wrong field", "B738", "B738", ""},
		{"cessna type", "C172", "C172", ""},
		{"avionics type", "G1000", "G1000", ""},
		{"placeholder", "N/A", "N/A", ""},
		{"unknown mark", "QQ-ABC", "QQ-ABC", ""},
		{"empty", "", "", ""},
		{"whitespace only", "   ", "", ""},
		{"separators only", "---", "---", ""},
		{"lowercase unknown is still uppercased", "sim", "SIM", ""},
		{"too long to be a registration", "DEABCDEFGH", "DEABCDEFGH", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Normalize(tc.in)
			if got.Value != tc.want {
				t.Errorf("Normalize(%q).Value = %q, want %q", tc.in, got.Value, tc.want)
			}
			if tc.country == "" {
				if got.Matched {
					t.Errorf("Normalize(%q) matched %s, want no match", tc.in, got.Entry.Prefix)
				}
				return
			}
			if !got.Matched {
				t.Fatalf("Normalize(%q) did not match, want country %s", tc.in, tc.country)
			}
			if got.Entry.Country != tc.country {
				t.Errorf("Normalize(%q) country = %s, want %s", tc.in, got.Entry.Country, tc.country)
			}
		})
	}
}

// Normalising an already-normalised registration must be a no-op, or repeated
// imports and recalculations would keep rewriting the same rows.
func TestNormalizeIsIdempotent(t *testing.T) {
	for _, in := range []string{
		"D-EABC", "N12345", "JA8089", "HL7747", "B-1234", "9XR-AA",
		"RDPL-34123", "SIM", "N/A", "C172", "",
	} {
		once := Canonical(in)
		twice := Canonical(once)
		if once != twice {
			t.Errorf("Canonical(%q) = %q, but Canonical(%q) = %q", in, once, once, twice)
		}
	}
}

// A normalised registration must still fit the 20-character registration
// column. The longest mark is four characters and every suffix pattern caps
// the registration mark at five, so the bound holds by construction — this
// test keeps it holding as the table grows.
func TestNormalizeStaysWithinColumnLength(t *testing.T) {
	const maxRegistrationLength = 20
	for _, e := range Entries() {
		// The longest string this entry can produce is its mark, a hyphen,
		// and the longest registration mark its pattern admits.
		longest := len(e.Prefix) + 1 + 5
		if longest > maxRegistrationLength {
			t.Errorf("mark %q can normalise to %d characters, exceeding the %d-character column",
				e.Prefix, longest, maxRegistrationLength)
		}
	}
}

func TestTableIntegrity(t *testing.T) {
	seen := make(map[string]bool)
	for _, e := range Entries() {
		switch {
		case e.Prefix == "":
			t.Error("entry with empty nationality mark")
		case seen[e.Prefix]:
			t.Errorf("duplicate nationality mark %q", e.Prefix)
		case strings.ToUpper(e.Prefix) != e.Prefix:
			t.Errorf("nationality mark %q is not uppercase", e.Prefix)
		case len(e.Country) != 2:
			t.Errorf("mark %q has country code %q, want ISO 3166-1 alpha-2", e.Prefix, e.Country)
		case e.CountryName == "":
			t.Errorf("mark %q has no country name", e.Prefix)
		}
		seen[e.Prefix] = true
	}
	if len(seen) < 150 {
		t.Errorf("table has %d marks, expected the full ICAO set (>150)", len(seen))
	}
}

// Every entry must be reachable: a mark shadowed by a longer one that accepts
// a superset of its registration marks could never win, which would silently
// misattribute a whole country.
func TestEveryMarkIsReachable(t *testing.T) {
	for _, e := range Entries() {
		sample, ok := sampleFor(e)
		if !ok {
			t.Errorf("mark %q: could not build a sample registration", e.Prefix)
			continue
		}
		got := Normalize(sample)
		if !got.Matched {
			t.Errorf("mark %q: sample %q did not match", e.Prefix, sample)
			continue
		}
		if got.Entry.Prefix != e.Prefix {
			t.Errorf("mark %q: sample %q matched %q (%s) instead",
				e.Prefix, sample, got.Entry.Prefix, got.Entry.CountryName)
		}
	}
}

// sampleFor builds a registration that the entry's own pattern accepts, by
// trying the registration-mark shapes the table actually uses.
func sampleFor(e Entry) (string, bool) {
	candidates := []string{
		"ABC", "ABCD", "AB", "1234", "12345", "123", "T1234", "HKA", "123A", "AA",
	}
	for _, mark := range candidates {
		if e.re.MatchString(mark) {
			return e.Prefix + "-" + mark, true
		}
	}
	return "", false
}

// LastReviewed is reported to operators as a date via GET /admin/config, and
// is the only signal that the vendored table may be stale — a typo would show
// them the zero date.
func TestLastReviewedIsADate(t *testing.T) {
	if _, err := time.Parse("2006-01-02", LastReviewed); err != nil {
		t.Errorf("LastReviewed = %q, want YYYY-MM-DD: %v", LastReviewed, err)
	}
}

func TestCount(t *testing.T) {
	if got, want := Count(), len(Entries()); got != want {
		t.Errorf("Count() = %d, want %d", got, want)
	}
}

func TestLookup(t *testing.T) {
	if e, ok := Lookup("d"); !ok || e.Country != "DE" {
		t.Errorf("Lookup(%q) = %+v, %v; want Germany", "d", e, ok)
	}
	if e, ok := Lookup(" N "); !ok || e.Hyphenated() {
		t.Errorf("Lookup(%q) = %+v, %v; want the United States, unhyphenated", " N ", e, ok)
	}
	if _, ok := Lookup("QQ"); ok {
		t.Error("Lookup(\"QQ\") matched, want no entry")
	}
}
