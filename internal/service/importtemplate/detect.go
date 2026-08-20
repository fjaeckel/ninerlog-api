package importtemplate

import (
	"sort"
	"strings"
	"unicode"
)

// normalizeHeader canonicalises a CSV header for alias lookup: lower-cased,
// stripped of a leading byte-order mark and surrounding quotes, with runs of
// whitespace collapsed to a single space.
//
// Internal punctuation is deliberately preserved. "AircraftID" (ForeFlight) and
// "Aircraft ID" (LogTen Pro) are different columns from different applications,
// and flattening them together would make the two formats indistinguishable.
// The looser, punctuation-free match is applied only as a second pass, after
// the exact aliases have had their chance.
func normalizeHeader(h string) string {
	h = strings.TrimPrefix(h, "\ufeff")
	h = strings.TrimSpace(h)
	h = strings.Trim(h, `"'`)
	h = strings.ToLower(strings.TrimSpace(h))

	var b strings.Builder
	b.Grow(len(h))
	space := false
	for _, r := range h {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

// Detect picks the template that best explains a file's header row, or nil when
// no template clears its threshold.
//
// Scoring weights signature columns far above ordinary ones: every logbook has
// a "Date" and a "Total Time", so a match is only meaningful when it rests on
// spellings the source application is more or less alone in using.
func Detect(headers []string) *Template {
	type candidate struct {
		tpl   *Template
		score int
	}
	var candidates []candidate

	for _, t := range All() {
		if len(t.Signature) == 0 || t.MinSignatureHits == 0 {
			// Catalogue-only entry (e.g. plain CSV): never auto-detected.
			continue
		}
		cols, sig := t.Matches(headers)
		if sig < t.MinSignatureHits {
			continue
		}
		candidates = append(candidates, candidate{tpl: t, score: cols + signatureWeight*sig})
	}
	if len(candidates) == 0 {
		return nil
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].tpl.Priority < candidates[j].tpl.Priority
	})
	return candidates[0].tpl
}

// signatureWeight is how much one matched signature column counts for relative
// to one ordinary recognised column.
const signatureWeight = 4

// Suggest returns column mappings for a file. When tpl is nil the shared
// cross-vendor alias table is used, which still resolves the common spellings
// of every format in the catalogue.
func Suggest(tpl *Template, headers []string) []Mapping {
	if tpl != nil {
		return tpl.Suggest(headers)
	}
	return genericTemplate.Suggest(headers)
}

// DetectAndSuggest is the one call the upload handler needs: it returns the
// detected template (nil when unrecognised), the import-format value to record,
// and the suggested mappings.
func DetectAndSuggest(headers []string) (*Template, string, []Mapping) {
	tpl := Detect(headers)
	format := FormatGenericCSV
	if tpl != nil {
		format = tpl.ID
	}
	return tpl, format, Suggest(tpl, headers)
}
