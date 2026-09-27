package engine

import (
	"math"
	"slices"
	"sort"

	"github.com/fjaeckel/ninerlog-rules/engine/hatches"
)

// tri is a three-valued requirement state.
type tri int

const (
	triUnmet tri = iota
	triMet
	triUnknown
)

// evalCtx evaluates one rule for one subject.
type evalCtx struct {
	cat     *Catalogue
	v       *Vocabulary
	p       *prepared
	rule    *Rule
	subj    subjectRef
	asOf    Date
	expiry  *Date
	leaves  map[*Node]*leaf
	restore []hookDate
	resets  []Date
	noReset bool
	trace   *Trace
}

// hookDate is one restored_by or resets event occurrence.
type hookDate struct {
	date Date
	kind string
}

// leaf is a requirement row with its matching items pre-computed.
type leaf struct {
	node   *Node
	window *Window
	metric MetricDef
	items  []leafItem
}

type leafItem struct {
	date    Date
	val     mv
	matched match
	by      []string
}

// leafState is a leaf evaluated on one date.
type leafState struct {
	current  float64
	tracked  bool
	met      bool
	inWindow bool
	span     span
	last     *Date
	unknown  []string
}

// Evaluate evaluates every applicable rule of the catalogue on asOf.
func Evaluate(c *Catalogue, rec *Record, asOf Date) []Evaluation {
	var out []Evaluation
	for _, r := range c.Rules {
		ev, _ := EvaluateRule(c, r, rec, asOf)
		out = append(out, ev...)
	}
	return out
}

// EvaluateRule evaluates one rule for every subject it applies to, with coverage traces.
func EvaluateRule(c *Catalogue, r *Rule, rec *Record, asOf Date) ([]Evaluation, []*Trace) {
	if r.Support == "not_supported" {
		return nil, nil
	}
	if r.EffectiveFrom != nil && asOf.Before(*r.EffectiveFrom) || r.EffectiveTo != nil && asOf.After(*r.EffectiveTo) {
		return nil, nil
	}
	p := prepare(rec, c.Vocabulary)
	var evs []Evaluation
	var traces []*Trace
	for _, s := range p.subjects(r, c.Vocabulary, asOf) {
		e := &evalCtx{cat: c, v: c.Vocabulary, p: p, rule: r, subj: s, asOf: asOf, trace: newTrace()}
		if r.AppliesTo.Holds != nil && !e.holds(r.AppliesTo.Holds, asOf) {
			continue
		}
		evs = append(evs, e.run())
		traces = append(traces, e.trace)
	}
	return evs, traces
}

func (e *evalCtx) run() Evaluation {
	r := e.rule
	e.expiry = e.effectiveExpiry()
	e.restore = e.hookDates(r.RestoredBy)
	for _, h := range e.hookDates(r.Resets) {
		e.resets = append(e.resets, h.date)
	}
	e.buildLeaves()
	e.traceResets()

	ev := Evaluation{
		RuleID:             r.ID,
		Subject:            e.subj.Subject,
		RuleDescriptionKey: r.RuleDescriptionKey,
		ExpiresOn:          e.expiry,
		Citations:          r.Citations,
		Requirements:       []RequirementResult{},
	}
	if len(ev.Citations) == 0 {
		ev.Citations = []string{r.Source.Cite}
	}
	if e.expiry != nil {
		switch e.asOf.Compare(*e.expiry) {
		case -1:
			e.trace.Expiry = "before"
		case 0:
			e.trace.Expiry = "on"
		default:
			e.trace.Expiry = "after"
		}
	}
	if r.Window != nil && !windowMoving(r.Window) && r.Window.Kind != "lifetime" {
		if s, ok := e.windowRange(r.Window, e.asOf); ok {
			from := s.from
			ev.WindowOpensAt = &from
		}
	}

	root := e.evalRoot(e.asOf, &ev.Requirements, true)
	if root == triMet && e.moving() {
		ev.ValidUntil = e.projectRoot()
	}
	for _, st := range r.Stages {
		if e.cond(&st.When, e.asOf, root) {
			ev.Status, ev.MessageKey = st.Status, st.MessageKey
			ev.MessageParams = e.params(st.Params, &ev)
			e.trace.Stage = st.Tag()
			return ev
		}
	}
	ev.Status = "unknown"
	return ev
}

// effectiveExpiry returns the earlier of the recorded and the derived expiry.
func (e *evalCtx) effectiveExpiry() *Date {
	exp := e.subj.expires
	if e.rule.Validity == nil {
		return exp
	}
	if d := e.derivedExpiry(e.rule.Validity); d != nil && (exp == nil || d.Before(*exp)) {
		exp = d
	}
	return exp
}

func (e *evalCtx) hookDates(hooks []EventHook) []hookDate {
	var out []hookDate
	for _, h := range hooks {
		f := e.resolveFilter(h.Filter)
		for i := range e.p.events {
			ev := &e.p.events[i]
			if ev.Kind != h.Event || ev.Date.After(e.asOf) {
				continue
			}
			if e.matchEvent(f, ev).res == matchYes {
				out = append(out, hookDate{ev.Date, h.Event})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].date.Before(out[j].date) })
	return out
}

// traceResets records resets events that moved a window start on asOf.
func (e *evalCtx) traceResets() {
	r, ok := e.latestReset(e.asOf)
	if !ok {
		return
	}
	e.noReset = true
	defer func() { e.noReset = false }()
	for _, lf := range e.leaves {
		if s, ok := e.windowRange(lf.window, e.asOf); ok && (s.open || r.After(s.from)) {
			for _, h := range e.rule.Resets {
				e.trace.Events = append(e.trace.Events, h.Event)
			}
			return
		}
	}
}

// buildLeaves resolves windows and filters and pre-filters items for every leaf.
func (e *evalCtx) buildLeaves() {
	e.leaves = map[*Node]*leaf{}
	var walk func(n *Node, w *Window, f *Filter)
	walk = func(n *Node, w *Window, f *Filter) {
		if n == nil {
			return
		}
		if n.Window != nil {
			w = n.Window
		}
		f = MergeFilter(f, n.Filter)
		if !n.IsLeaf() {
			for _, c := range n.Children() {
				walk(c, w, f)
			}
			return
		}
		lf := &leaf{node: n, window: w, metric: e.v.Metrics[n.Metric]}
		rf := e.resolveFilter(f)
		if lf.metric.Source == "events" {
			fn := eventMetricFuncs[n.Metric]
			for i := range e.p.events {
				ev := &e.p.events[i]
				m := e.matchEvent(rf, ev)
				if m.res != matchNo && fn != nil {
					lf.items = append(lf.items, leafItem{date: ev.Date, val: fn(ev), matched: m.res, by: m.unknown})
				}
			}
		} else {
			fn := flightMetricFuncs[n.Metric]
			for i := range e.p.flights {
				fl := &e.p.flights[i]
				m := e.matchFlight(rf, fl)
				if m.res != matchNo && fn != nil {
					lf.items = append(lf.items, leafItem{date: fl.Date, val: fn(fl), matched: m.res, by: m.unknown})
				}
			}
		}
		e.leaves[n] = lf
	}
	walk(e.rule.Requirements, e.rule.Window, e.rule.Filter)
}

// evalLeaf computes a leaf on date d.
func (e *evalCtx) evalLeaf(lf *leaf, d Date) leafState {
	var st leafState
	s, ok := e.windowRange(lf.window, d)
	if !ok {
		return st
	}
	st.inWindow, st.span = true, s
	var sum, aux, best float64
	knownItems, items := 0, 0
	for _, it := range lf.items {
		if !s.contains(it.date) {
			continue
		}
		items++
		if it.matched == matchUnknown {
			st.unknown = append(st.unknown, it.by...)
			continue
		}
		if !it.val.known {
			st.unknown = append(st.unknown, lf.node.Metric)
			continue
		}
		knownItems++
		sum += it.val.v
		aux += it.val.aux
		best = math.Max(best, it.val.v)
		if it.val.v > 0 || it.val.aux > 0 {
			dd := it.date
			st.last = &dd
		}
	}
	switch lf.metric.Aggregate {
	case "max":
		st.current = best
	case "min_of_sums":
		st.current = math.Min(sum, aux)
	default:
		st.current = sum
	}
	st.tracked = (items == 0 || knownItems > 0) && lf.metric.Aggregate != "none"
	if lf.node.EscapeHatch != "" {
		if h, ok := hatches.Registry[lf.node.EscapeHatch]; ok {
			out := h(hatches.Input{Value: st.current, Tracked: st.tracked})
			st.current, st.tracked = out.Value, out.Tracked
		} else {
			st.tracked = false
		}
	}
	st.met = st.tracked && st.current >= required(lf.node)
	return st
}

func required(n *Node) float64 {
	if n.Min == nil {
		return 0
	}
	return *n.Min
}

// evalRoot evaluates the tree (plus restoration) on d; rows are filled when record is set.
func (e *evalCtx) evalRoot(d Date, rows *[]RequirementResult, record bool) tri {
	t := triMet
	if e.rule.Requirements != nil {
		t = e.evalNode(e.rule.Requirements, d, rows, record)
	}
	if t != triMet {
		if kind, ok := e.restoredAt(d); ok {
			if record {
				e.trace.Events = append(e.trace.Events, kind)
			}
			return triMet
		}
	}
	return t
}

func (e *evalCtx) evalNode(n *Node, d Date, rows *[]RequirementResult, record bool) tri {
	if n.IsLeaf() {
		lf := e.leaves[n]
		st := e.evalLeaf(lf, d)
		if record {
			*rows = append(*rows, e.row(lf, st))
			e.trace.leaf(lf, st, e.v)
		}
		switch {
		case st.met:
			return triMet
		case !st.tracked:
			return triUnknown
		}
		return triUnmet
	}
	var states []tri
	var ids []string
	for _, c := range n.Children() {
		if c.When != nil && !e.cond(c.When, d, triUnknown) {
			continue
		}
		s := e.evalNode(c, d, rows, record)
		if c.Informational {
			continue
		}
		states = append(states, s)
		ids = append(ids, c.ID)
	}
	met, unknown := 0, 0
	for _, s := range states {
		switch s {
		case triMet:
			met++
		case triUnknown:
			unknown++
		}
	}
	var out tri
	switch n.Combinator() {
	case "any_of":
		out = combine(met > 0, met+unknown > 0)
		if record {
			for i, s := range states {
				if s == triMet && met == 1 {
					e.trace.AnyOf = append(e.trace.AnyOf, n.ID+":"+ids[i])
				}
			}
		}
	case "n_of":
		out = combine(met >= n.NOf.N, met+unknown >= n.NOf.N)
		if record {
			e.trace.NOf[n.ID] = out == triMet
		}
	default:
		out = combine(met == len(states), met+unknown == len(states))
	}
	return out
}

func combine(met, possible bool) tri {
	switch {
	case met:
		return triMet
	case possible:
		return triUnknown
	}
	return triUnmet
}

func (e *evalCtx) row(lf *leaf, st leafState) RequirementResult {
	n := lf.node
	r := RequirementResult{
		ID: n.ID, NameKey: n.NameKey, Metric: n.Metric, Unit: n.Unit,
		Current: st.current, Required: required(n), Met: st.met, Tracked: st.tracked,
		LastDate: st.last,
	}
	if st.met && windowMoving(lf.window) {
		r.ValidUntil = e.projectLeaf(lf)
	}
	if n.Messages != nil {
		switch {
		case !st.tracked:
			r.MessageKey = n.Messages.Untracked
		case st.met:
			r.MessageKey = n.Messages.Met
		default:
			r.MessageKey = n.Messages.Unmet
		}
	}
	if st.tracked && !st.met && n.RemedyKey != "" {
		r.RemedyKey = n.RemedyKey
		r.RemedyParams = e.remedyParams(n, st)
	}
	return r
}

func (e *evalCtx) remedyParams(n *Node, st leafState) map[string]any {
	def, ok := e.cat.Keys.Get(n.RemedyKey)
	if !ok || len(def.Params) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, p := range def.Params {
		switch trimOptional(p) {
		case "missing":
			out["missing"] = int(math.Ceil(required(n) - st.current))
		case "unit":
			out["unit"] = n.Unit
		case "method":
			out["method"] = e.subj.Detail
		}
	}
	return out
}

func trimOptional(p string) string {
	if len(p) > 0 && p[len(p)-1] == '?' {
		return p[:len(p)-1]
	}
	return p
}

// restoredAt returns the restored_by event that keeps the tree met on d.
func (e *evalCtx) restoredAt(d Date) (string, bool) {
	for i := len(e.restore) - 1; i >= 0; i-- {
		r := e.restore[i]
		if r.date.After(d) {
			continue
		}
		if windowMoving(e.rule.Window) {
			if d.Before(windowExit(e.rule.Window, r.date)) {
				return r.kind, true
			}
			continue
		}
		if s, ok := e.windowRange(e.rule.Window, d); ok && s.contains(r.date) {
			return r.kind, true
		}
	}
	return "", false
}

// moving reports whether any leaf window or the rule window follows asOf.
func (e *evalCtx) moving() bool {
	if windowMoving(e.rule.Window) && len(e.restore) > 0 {
		return true
	}
	for _, lf := range e.leaves {
		if windowMoving(lf.window) {
			return true
		}
	}
	return false
}

// changeDates returns the dates after which the tree can turn unmet: item exits and resets.
func (e *evalCtx) changeDates(only *leaf) []Date {
	var out []Date
	add := func(w *Window, x Date) {
		if windowMoving(w) && !x.After(e.asOf) {
			out = append(out, windowExit(w, x))
		}
	}
	for _, lf := range e.leaves {
		if only != nil && lf != only {
			continue
		}
		for _, it := range lf.items {
			add(lf.window, it.date)
		}
	}
	if only == nil {
		for _, r := range e.restore {
			add(e.rule.Window, r.date)
		}
	}
	for _, r := range e.resets {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return slices.CompactFunc(out, func(a, b Date) bool { return a.Equal(b) })
}

// projectRoot returns the last date the tree stays met without new items.
func (e *evalCtx) projectRoot() *Date {
	for _, c := range e.changeDates(nil) {
		if !c.After(e.asOf) {
			continue
		}
		if e.evalRoot(c, nil, false) != triMet {
			d := c.AddDays(-1)
			return &d
		}
	}
	return nil
}

func (e *evalCtx) projectLeaf(lf *leaf) *Date {
	for _, c := range e.changeDates(lf) {
		if !c.After(e.asOf) {
			continue
		}
		if !e.evalLeaf(lf, c).met {
			d := c.AddDays(-1)
			return &d
		}
	}
	return nil
}

// metWithin reports whether the tree was met on some date in [cutoff, asOf].
func (e *evalCtx) metWithin(w *Window) bool {
	var cutoff Date
	switch w.Kind {
	case "calendar_months":
		cutoff = e.asOf.FirstOfMonth().AddMonths(-w.N)
	case "rolling_months":
		cutoff = e.asOf.AddMonths(-w.N)
	default:
		cutoff = e.asOf.AddDays(-w.N)
	}
	cands := []Date{e.asOf}
	for _, c := range e.changeDates(nil) {
		last := c.AddDays(-1)
		if !last.Before(cutoff) && last.Before(e.asOf) {
			cands = append(cands, last)
		}
	}
	for _, d := range cands {
		if e.evalRoot(d, nil, false) == triMet {
			return true
		}
	}
	return false
}
