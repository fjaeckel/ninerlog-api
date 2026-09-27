package engine

import (
	"slices"
	"sort"
)

// Trace records what one evaluation exercised, for the coverage gate (DESIGN.md section 7).
type Trace struct {
	Stage        string
	Requirements map[string]string
	AnyOf        []string
	NOf          map[string]bool
	Edges        map[string]bool
	Events       []string
	Unknown      []string
	Expiry       string
}

func newTrace() *Trace {
	return &Trace{Requirements: map[string]string{}, NOf: map[string]bool{}, Edges: map[string]bool{}}
}

func (t *Trace) leaf(lf *leaf, st leafState, v *Vocabulary) {
	switch {
	case st.met:
		t.Requirements[lf.node.ID] = "met"
	case st.tracked:
		t.Requirements[lf.node.ID] = "unmet"
	default:
		t.Requirements[lf.node.ID] = "untracked"
	}
	for _, u := range st.unknown {
		if u == lf.node.Metric && st.tracked {
			continue
		}
		if !slices.Contains(t.Unknown, u) {
			t.Unknown = append(t.Unknown, u)
		}
	}
	if !st.inWindow || st.span.open || lf.window == nil {
		return
	}
	for _, it := range lf.items {
		switch {
		case it.date.Equal(st.span.from):
			t.Edges[lf.window.String()+":edge-in"] = true
		case it.date.Equal(st.span.from.AddDays(-1)):
			t.Edges[lf.window.String()+":edge-out"] = true
		}
	}
}

// edgeWindows returns the distinct bounded windows used by a rule's leaves.
func edgeWindows(r *Rule) []string {
	var out []string
	var walk func(n *Node, w *Window)
	walk = func(n *Node, w *Window) {
		if n == nil {
			return
		}
		if n.Window != nil {
			w = n.Window
		}
		if n.IsLeaf() {
			if w != nil && w.Kind != "lifetime" && !slices.Contains(out, w.String()) {
				out = append(out, w.String())
			}
			return
		}
		for _, c := range n.Children() {
			walk(c, w)
		}
	}
	walk(r.Requirements, r.Window)
	sort.Strings(out)
	return out
}

func windowTag(windows []string, w, edge string) string {
	if len(windows) == 1 {
		return "window:" + edge
	}
	return "window:" + w + ":" + edge
}

// UsesExpiry reports whether a rule depends on the subject's expiry date.
func UsesExpiry(r *Rule) bool {
	uses := r.Validity != nil
	check := func(w *Window) {
		if w != nil && (w.Kind == "before_expiry_months" || w.Kind == "validity_period") {
			uses = true
		}
	}
	check(r.Window)
	r.Requirements.Walk(func(n *Node) { check(n.Window) })
	for i := range r.Stages {
		r.Stages[i].When.Walk(func(c *Condition) {
			switch c.Op {
			case "expired", "no_expiry", "expires_within":
				uses = true
			}
		})
	}
	return uses
}

// RequiredTags derives the coverage tags a supported or partial rule's cases must hit.
func RequiredTags(r *Rule, v *Vocabulary) []string {
	set := map[string]bool{}
	for _, s := range r.Stages {
		set["stage:"+s.Tag()] = true
	}
	optionalFilters := map[string]bool{}
	var walk func(n *Node, f *Filter)
	walk = func(n *Node, f *Filter) {
		if n == nil {
			return
		}
		f = MergeFilter(f, n.Filter)
		if n.IsLeaf() {
			set["requirement:"+n.ID+":met"] = true
			set["requirement:"+n.ID+":unmet"] = true
			if v.Metrics[n.Metric].Optional {
				set["unknown:"+n.Metric] = true
			}
			for _, k := range filterKeysDeep(f) {
				if v.Filters[k].Absent == "unknown" && !(k == "fstdTypes" && f.Simulator == "") {
					optionalFilters[k] = true
				}
			}
			return
		}
		switch n.Combinator() {
		case "any_of":
			for _, c := range n.Children() {
				set["any_of:"+n.ID+":"+c.ID] = true
			}
		case "n_of":
			set["n_of:"+n.ID+":met"] = true
			set["n_of:"+n.ID+":unmet"] = true
		}
		for _, c := range n.Children() {
			walk(c, f)
		}
	}
	walk(r.Requirements, r.Filter)
	for k := range optionalFilters {
		set["unknown:"+k] = true
	}
	ws := edgeWindows(r)
	for _, w := range ws {
		set[windowTag(ws, w, "edge-in")] = true
		set[windowTag(ws, w, "edge-out")] = true
	}
	for _, h := range r.RestoredBy {
		set["event:"+h.Event] = true
	}
	for _, h := range r.Resets {
		set["event:"+h.Event] = true
	}
	if UsesExpiry(r) {
		set["expiry:before"], set["expiry:on"], set["expiry:after"] = true, true, true
	}
	return sortedKeys(set)
}

func filterKeysDeep(f *Filter) []string {
	if f == nil {
		return nil
	}
	out := f.Keys()
	for _, a := range f.Any {
		out = append(out, filterKeysDeep(a)...)
	}
	return out
}

// ObservedTags returns the coverage tags one evaluation trace exercised.
func ObservedTags(r *Rule, t *Trace) []string {
	set := map[string]bool{}
	if t.Stage != "" {
		set["stage:"+t.Stage] = true
	}
	for id, st := range t.Requirements {
		if st != "untracked" {
			set["requirement:"+id+":"+st] = true
		}
	}
	for _, a := range t.AnyOf {
		set["any_of:"+a] = true
	}
	for id, met := range t.NOf {
		if met {
			set["n_of:"+id+":met"] = true
		} else {
			set["n_of:"+id+":unmet"] = true
		}
	}
	ws := edgeWindows(r)
	for e := range t.Edges {
		for _, w := range ws {
			if e == w+":edge-in" {
				set[windowTag(ws, w, "edge-in")] = true
			}
			if e == w+":edge-out" {
				set[windowTag(ws, w, "edge-out")] = true
			}
		}
	}
	for _, e := range t.Events {
		set["event:"+e] = true
	}
	for _, u := range t.Unknown {
		set["unknown:"+u] = true
	}
	if t.Expiry != "" {
		set["expiry:"+t.Expiry] = true
	}
	return sortedKeys(set)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
