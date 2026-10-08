package airports

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// genericNameWords are trailing words dropped from an airport name to form its
// short name key, e.g. "Konz-Könen Glider Field" → "konz konen".
var genericNameWords = map[string]bool{
	"aerodrome": true, "airfield": true, "airpark": true, "airport": true,
	"airstrip": true, "field": true, "strip": true, "glider": true,
	"gliderfield": true, "gliderport": true, "gliding": true, "site": true,
	"landing": true, "ultralight": true, "ul": true, "ulm": true,
	"flugplatz": true, "flugfeld": true, "landeplatz": true,
	"sonderlandeplatz": true, "segelfluggelande": true, "segelflugplatz": true,
}

// foldName lower-cases s, strips diacritics and reduces it to single-space
// separated runs of letters and digits.
func foldName(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r == 'ß':
			b.WriteString("ss")
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// shortNameKey returns the folded name without trailing generic words, or ""
// when that leaves it unchanged or with fewer than two words.
func shortNameKey(folded string) string {
	words := strings.Fields(folded)
	n := len(words)
	for n > 0 && genericNameWords[words[n-1]] {
		n--
	}
	if n < 2 || n == len(words) {
		return ""
	}
	return strings.Join(words[:n], " ")
}

// nameMatches reports whether the folded query starts at a word boundary of
// the folded name.
func nameMatches(folded, query string) bool {
	return strings.HasPrefix(folded, query) || strings.Contains(folded, " "+query)
}
