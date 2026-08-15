package importtemplate

import (
	"sort"
	"strings"
)

// Confidence describes how well the template's column list is known.
//
// It is not a quality rating of the source application — it records how the
// alias table was built, which is what decides whether a file is recognised
// automatically or lands on the mapping screen.
type Confidence string

const (
	// ConfidenceExact means the header row is known verbatim (NinerLog's own
	// exports, or a format whose column list is published and stable).
	ConfidenceExact Confidence = "exact"
	// ConfidenceBestEffort means the aliases cover the columns the vendor
	// documents plus the usual spelling variants, but the export has not been
	// verified byte for byte. Detection may miss; the mapping screen catches it.
	ConfidenceBestEffort Confidence = "best-effort"
)

// Template is one recognised logbook export format.
type Template struct {
	// ID is the stable identifier for this template. It doubles as the
	// import_format value persisted on flight_imports, so it must match a
	// member of the ImportFormat enum in the OpenAPI spec and the database.
	ID string

	Name        string
	Vendor      string
	Website     string
	Description string
	Confidence  Confidence

	// Regions lists the regulatory layouts the source targets ("EASA", "FAA").
	// Informational — it drives the badge on the import screen only.
	Regions []string

	// ExportSteps tells the pilot how to get the file out of the source
	// application. This is the part that actually makes a migration easy, and
	// it is useful even when auto-detection misses.
	ExportSteps []string

	// DateFormat is the Go layout hint attached to suggested date mappings.
	DateFormat string

	// Columns maps a normalised header (see normalizeHeader) to its target
	// field. Several aliases may point at the same field.
	Columns map[string]Field

	// Signature lists normalised headers that identify this source. Generic
	// columns ("date", "from", "to") make poor signatures — prefer spellings
	// the source is alone in using.
	Signature []string

	// MinSignatureHits is how many Signature entries a file must contain
	// before this template is considered a match at all.
	MinSignatureHits int

	// Priority breaks scoring ties; lower wins. Source-specific templates sit
	// below the generic regulatory layouts so a CapZLog file is reported as
	// CapZLog rather than as a bare EASA logbook.
	Priority int

	// looseCache is Columns re-keyed on the loose form. Built once at
	// registration — templates are immutable afterwards, so concurrent
	// requests read it without synchronisation.
	looseCache map[string]Field
}

// registry holds every template, keyed by ID.
var registry = map[string]*Template{}

// order preserves a stable catalogue order for the API response.
var order []string

func register(t *Template) *Template {
	if _, dup := registry[t.ID]; dup {
		panic("importtemplate: duplicate template ID " + t.ID)
	}
	t.looseCache = buildLooseColumns(t.Columns)
	registry[t.ID] = t
	order = append(order, t.ID)
	return t
}

// buildLooseColumns re-keys a column table on the loose form, so "Total Time",
// "Total_Time" and "TotalTime" all resolve even when only one spelling is
// listed. A collision between two headers that mean different things is
// resolved to FieldIgnore: guessing is worse than leaving the column for the
// user to map.
func buildLooseColumns(columns map[string]Field) map[string]Field {
	loose := make(map[string]Field, len(columns))
	for k, v := range columns {
		lk := looseKey(k)
		if existing, dup := loose[lk]; dup && existing != v {
			loose[lk] = FieldIgnore
			continue
		}
		loose[lk] = v
	}
	return loose
}

// All returns every template in catalogue order.
func All() []*Template {
	out := make([]*Template, 0, len(order))
	for _, id := range order {
		out = append(out, registry[id])
	}
	return out
}

// ByID returns the template with the given ID, or nil.
func ByID(id string) *Template {
	return registry[id]
}

// merge builds a column table from a base table plus overrides. Later maps win,
// which lets a source template refine a shared regulatory layout without
// restating it.
func merge(tables ...map[string]Field) map[string]Field {
	out := make(map[string]Field)
	for _, tbl := range tables {
		for k, v := range tbl {
			out[k] = v
		}
	}
	return out
}

// Suggest returns a mapping for every header in the file this template
// recognises. Headers the template does not know are omitted, leaving them
// unmapped ("skip") on the import screen for the user to assign.
// No target field is ever fed by two source columns: the importer assigns each
// mapping in turn, so two columns claiming one field would overwrite each other
// in Go's randomised map order — the same file importing differently on
// different runs. A file can genuinely carry two recognised spellings (NinerLog's
// own export writes both "Remarks" and "Endorsements", which both mean remarks),
// so the tie is broken deterministically: the column that appears first in the
// header row wins, and the later one is left unmapped for the user to reassign.
func (t *Template) Suggest(headers []string) []Mapping {
	var out []Mapping
	seenColumn := make(map[string]bool, len(headers))
	claimedField := make(map[Field]bool, len(headers))

	for _, h := range headers {
		key := normalizeHeader(h)
		if key == "" || seenColumn[key] {
			continue
		}
		field, ok := t.Columns[key]
		if !ok {
			// Fall back to the loose form so "Total Time", "Total_Time" and
			// "TotalTime" all resolve even when only one spelling is listed.
			field, ok = t.looseColumns()[looseKey(key)]
		}
		if !ok || field == FieldIgnore {
			continue
		}
		if claimedField[field] {
			continue
		}
		seenColumn[key] = true
		claimedField[field] = true
		m := Mapping{SourceColumn: h, TargetField: field}
		if field == FieldDate {
			m.DateFormat = t.DateFormat
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SourceColumn < out[j].SourceColumn })
	return out
}

// looseColumns is the template's column table re-keyed on the loose form.
func (t *Template) looseColumns() map[string]Field {
	if t.looseCache == nil {
		// Only reachable for a template built outside register(), i.e. in a test.
		return buildLooseColumns(t.Columns)
	}
	return t.looseCache
}

// Matches reports how strongly headers look like this template's format.
// hits counts recognised columns, sig counts matched signature columns.
func (t *Template) Matches(headers []string) (score int, sig int) {
	present := make(map[string]bool, len(headers))
	for _, h := range headers {
		if key := normalizeHeader(h); key != "" {
			present[key] = true
		}
	}
	for key := range present {
		if _, ok := t.Columns[key]; ok {
			score++
		}
	}
	for _, s := range t.Signature {
		if present[s] {
			sig++
		}
	}
	return score, sig
}

// String makes templates readable in test failures.
func (t *Template) String() string { return t.ID + " (" + t.Name + ")" }

// looseKey strips every character that is not a letter or digit, so spacing,
// punctuation and underscores stop mattering.
func looseKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		}
	}
	return b.String()
}
