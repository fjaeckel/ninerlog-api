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

// letterFolds maps letters without a canonical decomposition to ASCII.
var letterFolds = map[rune]string{
	'ß': "ss", 'ł': "l", 'Ł': "l", 'ø': "o", 'Ø': "o", 'æ': "ae", 'Æ': "ae",
	'œ': "oe", 'Œ': "oe", 'đ': "d", 'Đ': "d", 'ð': "d", 'Ð': "d", 'ı': "i",
	'þ': "th", 'Þ': "th", 'ħ': "h", 'Ħ': "h",
}

// foldName lower-cases s, strips diacritics and reduces it to single-space
// separated runs of letters and digits.
func foldName(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if f, ok := letterFolds[r]; ok {
			b.WriteString(f)
			continue
		}
		switch {
		case unicode.Is(unicode.Mn, r):
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
