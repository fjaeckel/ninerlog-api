package engine

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// testCatalogue builds a catalogue from the module vocabulary and keys plus inline rules.
func testCatalogue(t *testing.T, rules ...string) *Catalogue {
	t.Helper()
	v, err := LoadVocabulary("../vocabulary.yaml")
	if err != nil {
		t.Fatal(err)
	}
	k, err := LoadKeys("../messages/keys.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c := &Catalogue{Root: "..", Vocabulary: v, Keys: k, byID: map[string]*Rule{}}
	for _, src := range rules {
		var r Rule
		if err := yaml.Unmarshal([]byte(src), &r); err != nil {
			t.Fatalf("rule: %v\n%s", err, src)
		}
		c.Rules = append(c.Rules, &r)
		c.byID[r.ID] = &r
	}
	return c
}

func record(t *testing.T, src string) *Record {
	t.Helper()
	var r Record
	if err := yaml.Unmarshal([]byte(src), &r); err != nil {
		t.Fatalf("record: %v", err)
	}
	return &r
}

// evalOne evaluates the only rule of c and returns its evaluations and observed tags.
func evalOne(t *testing.T, c *Catalogue, rec, asOf string) ([]Evaluation, []string) {
	t.Helper()
	evs, traces := EvaluateRule(c, c.Rules[0], record(t, rec), MustDate(asOf))
	var tags []string
	for _, tr := range traces {
		tags = append(tags, ObservedTags(c.Rules[0], tr)...)
	}
	return evs, tags
}

func row(t *testing.T, ev Evaluation, id string) RequirementResult {
	t.Helper()
	for _, r := range ev.Requirements {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no row %s in %+v", id, ev.Requirements)
	return RequirementResult{}
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func wantStatus(t *testing.T, evs []Evaluation, i int, status, key string) {
	t.Helper()
	if len(evs) <= i {
		t.Fatalf("want at least %d evaluations, got %d", i+1, len(evs))
	}
	if evs[i].Status != status || evs[i].MessageKey != key {
		t.Fatalf("evaluation %d: got %s/%s, want %s/%s (rows %+v)", i, evs[i].Status, evs[i].MessageKey, status, key, evs[i].Requirements)
	}
}

func dedent(s string) string { return strings.TrimSpace(s) }
