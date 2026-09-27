package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/fjaeckel/ninerlog-rules/engine"
)

// check runs every validation and gate step and returns the report.
func check(o options) (*report, error) {
	r := newReport()
	sch, err := loadSchemas(o.root)
	if err != nil {
		return nil, err
	}
	validate := func(name, path string) {
		r.files++
		r.schema = append(r.schema, sch.validate(name, path)...)
	}
	validate("vocabulary", filepath.Join(o.root, "vocabulary.yaml"))
	validate("messages", filepath.Join(o.root, "messages", "keys.yaml"))
	if len(r.schema) > 0 {
		return r, nil
	}
	cat, err := engine.LoadCatalogue(o.root)
	if err != nil {
		return nil, err
	}
	for _, e := range cat.Errors {
		r.schema = append(r.schema, "load: "+e.Error())
	}
	packs, err := engine.YAMLFiles(filepath.Join(o.root, "packs"))
	if err != nil {
		return nil, err
	}
	for _, p := range packs {
		validate("pack", p)
	}
	changelog, _ := os.ReadFile(filepath.Join(o.root, "CHANGELOG.md"))
	sem := &semantics{cat: cat, root: o.root, changelog: string(changelog), used: map[string]map[string]bool{}}

	for _, rule := range cat.Rules {
		if o.rule != "" && rule.ID != o.rule {
			sem.collectUsage(rule)
			continue
		}
		rr := r.rule(rule.ID)
		rr.support = rule.Support
		before := len(r.schema)
		validate("rule", rule.File)
		if len(r.schema) > before {
			rr.errors = append(rr.errors, "schema validation failed (see above)")
		}
		rr.errors = append(rr.errors, sem.checkRule(rule)...)
		checkCases(cat, rule, rr, validate, &r.schema)
	}
	if o.rule != "" {
		if _, ok := cat.Rule(o.rule); !ok {
			return nil, fmt.Errorf("no rule %q in the catalogue", o.rule)
		}
		return r, nil
	}
	caseDirs, _ := filepath.Glob(filepath.Join(o.root, "cases", "*"))
	for _, d := range caseDirs {
		if _, ok := cat.Rule(filepath.Base(d)); !ok {
			r.orphanCases = append(r.orphanCases, d)
		}
	}
	checkVocabulary(cat.Vocabulary, sem.used, r)
	r.codeRules, err = checkCodeRules(o.root, cat)
	if err != nil {
		return nil, err
	}
	r.articles, err = checkArticles(o.root, cat)
	if err != nil {
		return nil, err
	}
	if o.coverage {
		pct, err := engineCoverage(o.root)
		switch {
		case err != nil:
			r.coverage, r.coverageFail = "could not measure: "+err.Error(), true
		case pct < o.minCoverage:
			r.coverage, r.coverageFail = fmt.Sprintf("%.1f%% (minimum %.0f%%)", pct, o.minCoverage), true
		default:
			r.coverage = fmt.Sprintf("%.1f%% (minimum %.0f%%)", pct, o.minCoverage)
		}
	} else {
		r.coverage = "not measured (-coverage=false)"
	}
	return r, nil
}

// checkCases runs a rule's cases and derives the missing coverage tags.
func checkCases(cat *engine.Catalogue, rule *engine.Rule, rr *ruleReport, validate func(string, string), schemaErrs *[]string) {
	files, err := engine.YAMLFiles(filepath.Join(cat.Root, "cases", rule.ID))
	if err != nil {
		rr.errors = append(rr.errors, err.Error())
		return
	}
	observed := map[string]bool{}
	for _, f := range files {
		before := len(*schemaErrs)
		validate("case", f)
		if len(*schemaErrs) > before {
			rr.failures = append(rr.failures, filepath.Base(f)+": schema validation failed (see above)")
			continue
		}
		c, err := engine.LoadCase(f)
		if err != nil {
			rr.failures = append(rr.failures, err.Error())
			continue
		}
		rr.cases++
		if c.Rule != rule.ID {
			rr.failures = append(rr.failures, fmt.Sprintf("%s: rule is %q, but the case lives under cases/%s", c.Name, c.Rule, rule.ID))
			continue
		}
		res := engine.RunCase(cat, c)
		for _, d := range res.Diffs {
			rr.failures = append(rr.failures, c.Name+": "+d)
		}
		for _, t := range res.Observed {
			observed[t] = true
		}
	}
	if rule.Support == "not_supported" {
		return
	}
	required := engine.RequiredTags(rule, cat.Vocabulary)
	rr.required = len(required)
	for _, t := range required {
		if !observed[t] {
			rr.missing = append(rr.missing, t)
		}
	}
}

// checkVocabulary reports entries no rule uses and entries the engine lacks.
func checkVocabulary(v *engine.Vocabulary, used map[string]map[string]bool, r *report) {
	declared := map[string][]string{
		"subjects":         keys(v.Subjects),
		"metrics":          keys(v.Metrics),
		"filters":          keys(v.Filters),
		"windows":          keys(v.Windows),
		"combinators":      keys(v.Combinators),
		"stage_conditions": keys(v.StageConditions),
		"rule_events":      keys(v.RuleEvents),
		"param_sources":    keys(v.ParamSources),
		"hatches":          keys(v.Hatches),
		"units":            v.Units,
	}
	impl := engine.Implemented()
	for section, names := range declared {
		for _, n := range names {
			if !used[section][n] {
				r.vocabUnused[section] = append(r.vocabUnused[section], n)
			}
			if list, ok := impl[section]; ok && !slices.Contains(list, n) {
				r.vocabUnimpl[section] = append(r.vocabUnimpl[section], n)
			}
		}
		sort.Strings(r.vocabUnused[section])
		sort.Strings(r.vocabUnimpl[section])
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// normQuote collapses whitespace and drops Markdown emphasis for quote matching.
func normQuote(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "*", "")), " ")
}
