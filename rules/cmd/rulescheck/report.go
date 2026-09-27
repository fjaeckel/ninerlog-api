package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// report collects every finding, grouped for printing.
type report struct {
	files        int
	schema       []string
	rules        map[string]*ruleReport
	vocabUnused  map[string][]string
	vocabUnimpl  map[string][]string
	orphanCases  []string
	codeRules    inventoryResult
	articles     map[string]inventoryResult
	coverage     string
	coverageFail bool
}

type ruleReport struct {
	id       string
	support  string
	cases    int
	required int
	errors   []string
	missing  []string
	failures []string
}

type inventoryResult struct {
	total    int
	excluded int
	unmapped []string
	unknown  []string
}

func newReport() *report {
	return &report{rules: map[string]*ruleReport{}, vocabUnused: map[string][]string{}, vocabUnimpl: map[string][]string{}, articles: map[string]inventoryResult{}}
}

func (r *report) rule(id string) *ruleReport {
	if rr, ok := r.rules[id]; ok {
		return rr
	}
	rr := &ruleReport{id: id}
	r.rules[id] = rr
	return rr
}

func (rr *ruleReport) problems() int { return len(rr.errors) + len(rr.missing) + len(rr.failures) }

func (r *report) problems() int {
	n := len(r.schema) + len(r.orphanCases) + len(r.codeRules.unmapped) + len(r.codeRules.unknown)
	for _, rr := range r.rules {
		n += rr.problems()
	}
	for _, l := range r.vocabUnused {
		n += len(l)
	}
	for _, l := range r.vocabUnimpl {
		n += len(l)
	}
	for _, a := range r.articles {
		n += len(a.unmapped) + len(a.unknown)
	}
	if r.coverageFail {
		n++
	}
	return n
}

func (r *report) print(w io.Writer, o options) {
	ids := make([]string, 0, len(r.rules))
	supported := 0
	cases := 0
	for id, rr := range r.rules {
		ids = append(ids, id)
		if rr.support != "not_supported" {
			supported++
		}
		cases += rr.cases
	}
	sort.Strings(ids)
	fmt.Fprintf(w, "rulescheck: %d rules (%d supported or partial), %d cases, %d files validated\n", len(ids), supported, cases, r.files)

	section(w, "Schema validation", len(r.schema))
	list(w, r.schema)

	bad := 0
	for _, id := range ids {
		if r.rules[id].problems() > 0 {
			bad++
		}
	}
	section(w, "Rules", bad)
	for _, id := range ids {
		rr := r.rules[id]
		state := "ok"
		if rr.problems() > 0 {
			state = "FAIL"
		}
		detail := rr.support
		if rr.support != "not_supported" {
			detail = fmt.Sprintf("%s, %d cases, %d/%d tags", rr.support, rr.cases, rr.required-len(rr.missing), rr.required)
		}
		fmt.Fprintf(w, "  %-4s %s (%s)\n", state, id, detail)
		for _, e := range rr.errors {
			fmt.Fprintf(w, "         error: %s\n", e)
		}
		if len(rr.missing) > 0 {
			fmt.Fprintf(w, "         missing tags (%d): %s\n", len(rr.missing), strings.Join(rr.missing, ", "))
		}
		for _, f := range rr.failures {
			fmt.Fprintf(w, "         case: %s\n", f)
		}
	}
	if len(r.orphanCases) > 0 {
		fmt.Fprintf(w, "  cases without a rule:\n")
		list(w, r.orphanCases)
	}
	if o.rule != "" {
		summary(w, r, o)
		return
	}

	section(w, "Vocabulary entries no rule uses", count(r.vocabUnused))
	groups(w, r.vocabUnused)
	section(w, "Vocabulary entries the engine does not implement", count(r.vocabUnimpl))
	groups(w, r.vocabUnimpl)

	c := r.codeRules
	section(w, fmt.Sprintf("Inventory: code rules without a catalogue rule (%d of %d; %d scope: consumer excluded)", len(c.unmapped), c.total, c.excluded), len(c.unmapped)+len(c.unknown))
	list(w, c.unmapped)
	for _, u := range c.unknown {
		fmt.Fprintf(w, "  - maps_to names no catalogue rule: %s\n", u)
	}

	total, missing := 0, 0
	auths := make([]string, 0, len(r.articles))
	for a, res := range r.articles {
		auths = append(auths, a)
		total += res.total
		missing += len(res.unmapped) + len(res.unknown)
	}
	sort.Strings(auths)
	section(w, fmt.Sprintf("Inventory: articles without a catalogue entry (%d of %d)", missing, total), missing)
	for _, a := range auths {
		res := r.articles[a]
		if len(res.unmapped) > 0 {
			fmt.Fprintf(w, "  %s (%d of %d): %s\n", a, len(res.unmapped), res.total, strings.Join(res.unmapped, "; "))
		}
		for _, u := range res.unknown {
			fmt.Fprintf(w, "  %s: maps_to names no catalogue rule: %s\n", a, u)
		}
	}

	fail := 0
	if r.coverageFail {
		fail = 1
	}
	section(w, "Engine statement coverage", fail)
	fmt.Fprintf(w, "  %s\n", r.coverage)
	summary(w, r, o)
}

func summary(w io.Writer, r *report, o options) {
	if n := r.problems(); n > 0 {
		mode := ""
		if o.report {
			mode = " (-report: exit 0)"
		}
		fmt.Fprintf(w, "\nFAIL: %d problems%s\n", n, mode)
		return
	}
	fmt.Fprintln(w, "\nOK")
}

func section(w io.Writer, title string, problems int) {
	state := "ok"
	if problems > 0 {
		state = fmt.Sprintf("%d problems", problems)
	}
	fmt.Fprintf(w, "\n== %s: %s\n", title, state)
}

func list(w io.Writer, l []string) {
	for _, s := range l {
		fmt.Fprintf(w, "  - %s\n", s)
	}
}

func groups(w io.Writer, m map[string][]string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if len(m[k]) > 0 {
			fmt.Fprintf(w, "  %s (%d): %s\n", k, len(m[k]), strings.Join(m[k], ", "))
		}
	}
}

func count(m map[string][]string) int {
	n := 0
	for _, l := range m {
		n += len(l)
	}
	return n
}
