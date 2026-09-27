package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReport runs the gate over the module in -report mode; it passes while porting is
// still under way and prints the progress report with -v.
func TestReport(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"-report", "-coverage=false", "-root", "../.."}, &out); code != 0 {
		t.Fatalf("exit %d\n%s", code, out.String())
	}
	for _, want := range []string{"== Schema validation: ok", "== Rules", "Inventory: code rules", "Inventory: articles"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report misses %q", want)
		}
	}
	t.Log(out.String())
}

// TestReferenceRulesGreen keeps the two reference rules passing the strict gate.
func TestReferenceRulesGreen(t *testing.T) {
	for _, id := range []string{"easa.part-fcl.fcl-740-a.sep", "faa.14cfr61.61-57-c.instrument"} {
		var out bytes.Buffer
		if code := run([]string{"-rule", id, "-root", "../.."}, &out); code != 0 {
			t.Errorf("%s: exit %d\n%s", id, code, out.String())
		}
	}
}

func TestFlags(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"-strict", "-report"}, &out); code != 2 {
		t.Errorf("-strict -report: exit %d", code)
	}
	if code := run([]string{"-bogus"}, &out); code != 2 {
		t.Errorf("unknown flag: exit %d", code)
	}
	if code := run([]string{"-rule", "no.such.rule", "-root", "../.."}, &out); code != 2 {
		t.Errorf("unknown rule: exit %d", code)
	}
}

// TestBrokenRule checks that the gate reports the problems a porting agent is likely to make.
func TestBrokenRule(t *testing.T) {
	root := t.TempDir()
	copyFile(t, "../../vocabulary.yaml", filepath.Join(root, "vocabulary.yaml"))
	copyFile(t, "../../messages/keys.yaml", filepath.Join(root, "messages/keys.yaml"))
	copyFile(t, "../../sources/faa/61.57.md", filepath.Join(root, "sources/faa/61.57.md"))
	for _, s := range []string{"vocabulary", "messages", "rule", "record", "case", "pack"} {
		copyFile(t, "../../schema/"+s+".schema.json", filepath.Join(root, "schema", s+".schema.json"))
	}
	writeFile(t, filepath.Join(root, "inventory/code-rules.yaml"), "rules:\n  - { id: a, maps_to: faa.14cfr61.61-57-a.x }\n  - { id: b, scope: consumer }\n  - { id: c, maps_to: [nope.x.y] }\n")
	writeFile(t, filepath.Join(root, "inventory/articles-faa.yaml"), "articles:\n  - { cite: 61.57, maps_to: [faa.14cfr61.61-57-a.x] }\n  - { cite: 61.58, maps_to: [] }\n  - { cite: 61.59, maps_to: [gone.x.y] }\n")
	writeFile(t, filepath.Join(root, "packs/faa.yaml"), "authority: faa\n")
	writeFile(t, filepath.Join(root, "catalogue/faa/14cfr61/faa.14cfr61.61-57-a.x.yaml"), `
id: faa.14cfr61.61-57-a.x
title: Broken on purpose
authority: easa
instrument: 14 CFR Part 61
article: 61.57(a)
support: supported
source_kind: statute
source:
  cite: 14 CFR 61.57(a)
  url: https://www.ecfr.gov/current/title-14/part-61/section-61.57
  file: sources/faa/61.57.md
  quote: [three takeoffs and three landings within the preceding 90 days, "made up text", "TODO: verify"]
related_sources:
  - { cite: x, url: "https://x", file: sources/faa/missing.md, quote: x }
applies_to:
  subject: passengerz
  authorities: [MARS]
  licenceKinds: [PILOT]
  classes: [JET]
  holds: { valid: true }
filter: { classes: [$subject], wings: 2 }
requirements:
  id: root
  any_of:
    - { id: landings, metric: landingz, min: 3, unit: landings, nameKey: requirement.nope, remedyKey: remedy.nope, escape_hatch: nope }
    - { id: landings, metric: minutes.total, min: 3, unit: landings, nameKey: rating.expired, remedyKey: rating.expired, messages: { met: nope.key } }
    - all_of:
        - { id: ev, metric: events, min: 1, unit: check, nameKey: requirement.ipc, filter: { roles: [pilot] } }
    - id: few
      n_of:
        n: 3
        of:
          - { id: a1, metric: flights, min: 1, unit: flights, nameKey: requirement.launches_and_landings, filter: { flags: { wet: true }, ulCredit: [{ class: JET, ulKinds: [BLIMP] }] }, window: { rolling_days: 3 } }
          - { id: a2, metric: flights, min: 1, unit: flights, nameKey: requirement.launches, window: { rolling_days: 3 }, when: { holds: { classes: [JET] } } }
validity: { from: birthday, periods: [{ when: { height: 2 }, months: 1 }] }
stages:
  - { when: { met: ghost }, status: sleeping, messageKey: rating.expiring, params: { days: dayz, date: { needed: ghost } } }
  - { when: { missing: shoe_size }, status: current, messageKey: rating.window_not_open }
  - { when: { met_within: { lifetime: true } }, status: current, messageKey: rating.glider_current }
  - { when: all_met, status: current, messageKey: nope.key }
restored_by: [{ event: seance, filter: { roles: [pic] } }]
resets: [{ event: ipc }]
ruleDescriptionKey: requirement.holds
divergences:
  - { id: dup, api: a, article: b, cite: c }
  - { id: dup, api: a, article: b, cite: c }
`)
	writeFile(t, filepath.Join(root, "catalogue/faa/other/faa.14cfr61.61-58.y.yaml"), `
id: faa.14cfr61.61-58.y
title: Not supported without notes
authority: faa
instrument: 14 CFR Part 61
article: 61.58
support: not_supported
source_kind: regulation
source: { cite: x, url: "https://x", file: sources/faa/61.57.md, quote: "TODO: verify" }
applies_to: { subject: type }
`)
	writeFile(t, filepath.Join(root, "catalogue/faa/14cfr61/faa.14cfr61.61-57-b.z.yaml"), "id: faa.14cfr61.61-57-b.z\nstages: [{ when: { bogus: 1 } }]\n")
	writeFile(t, filepath.Join(root, "cases/faa.14cfr61.61-57-a.x/one.yaml"), "rule: faa.14cfr61.61-57-a.x\nname: one\nasOf: 2026-01-01\nrecord: {}\nexpect: []\n")
	writeFile(t, filepath.Join(root, "cases/faa.14cfr61.61-57-a.x/two.yaml"), "rule: other.x.y\nname: two\nasOf: 2026-01-01\nrecord: {}\nexpect: []\n")
	writeFile(t, filepath.Join(root, "cases/faa.14cfr61.61-57-a.x/three.yaml"), "rule: x\n")
	writeFile(t, filepath.Join(root, "cases/orphan.x.y/one.yaml"), "rule: orphan.x.y\n")
	var out bytes.Buffer
	if code := run([]string{"-root", root, "-coverage=false"}, &out); code != 1 {
		t.Fatalf("exit %d\n%s", code, out.String())
	}
	report := out.String()
	for _, want := range []string{
		"authority \"easa\" differs", "source_kind \"statute\"", "quote is not verbatim", "missing.md",
		"applies_to.subject \"passengerz\"", "authority \"MARS\"", "licence kind \"PILOT\"", "applies_to.classes", "name classes, licenceKinds",
		"filter \"wings\"", "requirement id \"landings\" is used twice", "metric \"landingz\"", "requirement.nope\" is missing",
		"is a requirement_name key, want rule_description", "escape hatch \"nope\"", "does not fit metric", "every any_of branch needs an id", "n_of needs 3 of only 2",
		"metric events needs an eventKinds filter", "filter \"roles\" does not apply to events", "\"pilot\" is not in the vocabulary",
		"flags: \"wet\"", "ulCredit.class", "validity.from", "period condition \"height\"", "status \"sleeping\"",
		"names requirement \"ghost\"", "source \"dayz\"", "rating.window_not_open needs param \"date\"", "missing \"shoe_size\"",
		"met_within takes", "deprecated", "load: ", "unknown condition \"bogus\"", "last stage must be", "event \"seance\"", "rule_description",
		"divergence \"dup\" is listed twice", "no CHANGELOG.md line", "needs notes", "file must be", "rule is \"other.x.y\"",
		"schema validation failed", "cases without a rule", "maps_to names no catalogue rule: c -> nope.x.y",
		"61.59 -> gone.x.y", "61.58", "(0 of 3; 1 scope: consumer excluded)", "not measured", "FAIL:",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report misses %q", want)
		}
	}
	t.Log(report)
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, to, string(b))
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProfileCoverage(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.out")
	writeFile(t, p, "mode: set\na.go:1.1,2.2 3 1\na.go:1.1,2.2 3 0\nb.go:1.1,2.2 1 0\nbad line\n")
	pct, err := profileCoverage(p)
	if err != nil || pct != 75 {
		t.Errorf("coverage %v %v", pct, err)
	}
	writeFile(t, p, "mode: set\n")
	if _, err := profileCoverage(p); err == nil {
		t.Error("empty profile must fail")
	}
	if _, err := profileCoverage(filepath.Join(t.TempDir(), "none")); err == nil {
		t.Error("missing profile must fail")
	}
}

// TestContractChecks covers the checks added at integration: overlaps, groups, supersedes,
// source kinds, reservations and CHANGELOG references (DESIGN.md section 13).
func TestContractChecks(t *testing.T) {
	root := t.TempDir()
	vocab, err := os.ReadFile("../../vocabulary.yaml")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "vocabulary.yaml"), string(vocab)+`
reserved_for:
  subjects: { privilege: "used by a rule, so the reservation is stale" }
  units: { parsecs: "not declared" }
`)
	copyFile(t, "../../messages/keys.yaml", filepath.Join(root, "messages/keys.yaml"))
	copyFile(t, "../../sources/faa/61.57.md", filepath.Join(root, "sources/faa/61.57.md"))
	for _, s := range []string{"vocabulary", "messages", "rule", "record", "case", "pack"} {
		copyFile(t, "../../schema/"+s+".schema.json", filepath.Join(root, "schema", s+".schema.json"))
	}
	writeFile(t, filepath.Join(root, "inventory/code-rules.yaml"), "rules: []\n")
	rule := func(id, extra string) {
		parts := strings.Split(id, ".")
		writeFile(t, filepath.Join(root, "catalogue", parts[0], parts[1], id+".yaml"), `
id: `+id+`
title: Test
authority: `+parts[0]+`
instrument: Test
article: Test
support: supported
`+extra+`
stages:
  - { when: always, status: current, messageKey: privilege.valid }
`)
	}
	rule("faa.14cfr61.a", `source_kind: regulation
source: { cite: x, url: "https://x", file: sources/faa/61.57.md, quote: "three takeoffs and three landings" }
applies_to: { subject: privilege, privilegeKinds: [CFI, TOW_X] }
group: nosuch_group
supersedes: [faa.14cfr61.a, faa.14cfr61.gone]`)
	rule("faa.14cfr61.b", `source_kind: regulation
source: { cite: x, url: "https://x", file: sources/faa/61.57.md, quote: "three takeoffs and three landings" }
applies_to: { subject: privilege, privilegeKinds: [CFI] }`)
	rule("faa.14cfr61.c", `source_kind: regulation
source: { cite: x, url: "https://x", file: sources/faa/61.57.md, quote: "three takeoffs and three landings" }
applies_to: { subject: privilege, privilegeKinds: [CFI] }
supersedes: [faa.14cfr61.b]
group: faa_cfi`)
	rule("de.dulv.d", `source_kind: association
source: { cite: DULV rule, file: sources/faa/61.57.md, quote: "`+strings.Repeat("long ", 70)+`" }
applies_to: { subject: licence, authorities: [DULV] }`)
	rule("other.ninerlog.e", `source_kind: app_policy
source: { cite: policy, quote: "no quotes for policy" }
applies_to: { subject: licence, authorities: [FAA], licenceKinds: [FAA_SPORT] }`)
	rule("faa.14cfr61.f", `source_kind: regulation
source: { cite: x }
applies_to: { subject: credential, credentialTypes: [FAA_BASICMED] }`)
	writeFile(t, filepath.Join(root, "CHANGELOG.md"), "- `faa.14cfr61.b#nope` and `gone.x.y#z`\n- `faa.14cfr61.b#nope`\n")
	writeFile(t, filepath.Join(root, "sources/README.md"), "# Sources\n")
	writeFile(t, filepath.Join(root, "sources/faa/raw.xml"), "<xml/>\n")
	writeFile(t, filepath.Join(root, "sources/faa/no-origin.md"), "# X\n\n- Source URL: https://www.ecfr.gov/x\n\n---\n\ntext\n")
	writeFile(t, filepath.Join(root, "sources/de/amc.md"), "# AMC1 FCL.060\n\n- Origin: eu-legal-act\n- Attribution: none\n- URL: https://www.easa.europa.eu/document-library/easy-access-rules\n\n## Text\n\ncopied\n")
	writeFile(t, filepath.Join(root, "sources/de/dulv.md"), "# Rule\n\n- Origin: de-amtliches-werk\n- Attribution: § 5(1) UrhG\n\n## DULV Ausbildungsrichtlinie\n\ntext\n")
	var out bytes.Buffer
	if code := run([]string{"-root", root, "-coverage=false"}, &out); code != 1 {
		t.Fatalf("exit %d\n%s", code, out.String())
	}
	report := out.String()
	for _, want := range []string{
		"faa.14cfr61.a and faa.14cfr61.b can both evaluate the same privilege subject (privileges CFI)",
		"group \"nosuch_group\" is not declared", "group \"faa_cfi\" has only this rule", "a rule cannot supersede itself",
		"supersedes: rule \"faa.14cfr61.gone\" does not exist",
		"association documents are never stored", "an association quote may have at most 300 characters",
		"an app_policy rule has no source file and no quote", "schema validation failed",
		"faa.14cfr61.b#nope: the rule has no such divergence", "gone.x.y#z: no such rule", "faa.14cfr61.b#nope is listed on more than one line",
		"subjects.privilege (used by a rule; drop the reservation)", "units.parsecs (not declared)",
		"sources/faa/raw.xml: only Markdown texts", "sources/faa/no-origin.md: header has no allowed origin",
		"sources/de/amc.md: origin eu-legal-act belongs in sources/easa/", "sources/de/amc.md: header needs an \"- Attribution:\" line containing \"© European Union",
		"which is not a host of origin eu-legal-act", "names \"AMC\" in its header or a heading", "names \"DULV\" in its header or a heading",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report misses %q", want)
		}
	}
	if strings.Contains(report, "faa.14cfr61.b and faa.14cfr61.c can both") {
		t.Error("supersedes must resolve the overlap of b and c")
	}
	t.Log(report)
}

func TestSelection(t *testing.T) {
	s := selectValues([]string{"a", "b", "c"}, nil, []string{"b"}, nil)
	if !s["a"] || s["b"] || !s["c"] {
		t.Errorf("exclude: %v", s)
	}
	if got := sortedSel(selection{"1": true, "2": true, "3": true, "4": true, "5": true, "6": true, "7": true}); len(got) != 7 || got[6] != "... (7)" {
		t.Errorf("sortedSel: %v", got)
	}
}
