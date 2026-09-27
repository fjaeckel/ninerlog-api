package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogueCases(t *testing.T) {
	cat, err := LoadCatalogue("..")
	if err != nil {
		t.Fatal(err)
	}
	if len(Evaluate(cat, &Record{}, MustDate("2026-01-01"))) != 0 {
		t.Error("an empty record yields no evaluations")
	}
	for _, r := range cat.Rules {
		cases, err := LoadCases("..", r.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cases {
			if res := RunCase(cat, c); len(res.Diffs) > 0 {
				t.Errorf("%s: %s", c.File, strings.Join(res.Diffs, "\n"))
			}
		}
	}
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if _, err := LoadCatalogue(dir); err == nil {
		t.Error("missing vocabulary must fail")
	}
	if _, err := LoadVocabulary(write("bad.yaml", "version: [")); err == nil {
		t.Error("bad vocabulary yaml must fail")
	}
	vocab, _ := os.ReadFile("../vocabulary.yaml")
	write("vocabulary.yaml", string(vocab))
	if _, err := LoadCatalogue(dir); err == nil {
		t.Error("missing keys must fail")
	}
	if _, err := LoadKeys(write("k1.yaml", "keys: [")); err == nil {
		t.Error("bad keys yaml must fail")
	}
	if _, err := LoadKeys(write("k2.yaml", "keys:\n  - {key: a}\n  - {key: a}\n")); err == nil {
		t.Error("duplicate keys must fail")
	}
	write("messages/keys.yaml", "keys:\n  - {key: a}\n")
	if c, err := LoadCatalogue(dir); err != nil || len(c.Rules) != 0 {
		t.Errorf("empty catalogue: %v", err)
	}
	write("catalogue/x/y/a.yaml", "id: a.b.c\n")
	write("catalogue/x/y/b.yaml", "id: a.b.c\n")
	if c, err := LoadCatalogue(dir); err != nil || len(c.Errors) != 1 || len(c.Rules) != 1 {
		t.Errorf("duplicate rule ids are a load error: %v %v", err, c.Errors)
	}
	write("catalogue/x/y/b.yaml", "id: [")
	if c, err := LoadCatalogue(dir); err != nil || len(c.Errors) != 1 {
		t.Errorf("bad rule yaml is a load error: %v", err)
	}
	if _, err := LoadRule(filepath.Join(dir, "nope.yaml")); err == nil {
		t.Error("missing rule must fail")
	}
	if _, err := LoadCase(filepath.Join(dir, "nope.yaml")); err == nil {
		t.Error("missing case must fail")
	}
	if _, err := LoadCase(write("c.yaml", "asOf: 2026-13-45\n")); err == nil {
		t.Error("bad case date must fail")
	}
	write("cases/r.x.y/bad.yaml", "asOf: nope\n")
	if _, err := LoadCases(dir, "r.x.y"); err == nil {
		t.Error("bad case in a set must fail")
	}
	if _, err := LoadVocabulary(filepath.Join(dir, "nope.yaml")); err == nil {
		t.Error("missing vocabulary must fail")
	}
	if _, err := LoadKeys(filepath.Join(dir, "nope.yaml")); err == nil {
		t.Error("missing keys must fail")
	}
}
