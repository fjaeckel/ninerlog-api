package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fjaeckel/ninerlog-rules/engine"
)

// semantics checks rule contents against the vocabulary, keys and sources.
type semantics struct {
	cat       *engine.Catalogue
	root      string
	changelog string
	used      map[string]map[string]bool
	errs      []string
}

func (s *semantics) use(section, name string) {
	if s.used[section] == nil {
		s.used[section] = map[string]bool{}
	}
	s.used[section][name] = true
}

func (s *semantics) errorf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if !slices.Contains(s.errs, msg) {
		s.errs = append(s.errs, msg)
	}
}

// collectUsage records vocabulary use without reporting errors.
func (s *semantics) collectUsage(r *engine.Rule) {
	saved := s.errs
	s.checkRule(r)
	s.errs = saved
}

func (s *semantics) checkRule(r *engine.Rule) []string {
	s.errs = nil
	v := s.cat.Vocabulary
	s.checkIdentity(r)
	s.checkSource("source", r.Source)
	for i, rs := range r.RelatedSources {
		s.checkSource(fmt.Sprintf("related_sources[%d]", i), rs)
	}
	s.checkAppliesTo(r)
	if r.Window != nil {
		s.checkWindow("window", r.Window)
	}
	if r.Filter != nil {
		s.checkFilter("filter", r.Filter, "")
	}
	ids := map[string]bool{}
	r.Requirements.Walk(func(n *engine.Node) {
		if n.ID != "" {
			if ids[n.ID] {
				s.errorf("requirement id %q is used twice", n.ID)
			}
			ids[n.ID] = true
		}
	})
	s.checkNode(r, r.Requirements, r.Filter, true)
	if r.Validity != nil {
		if !slices.Contains(v.Validity.Anchors, r.Validity.From) {
			s.errorf("validity.from %q is not one of %v", r.Validity.From, v.Validity.Anchors)
		}
		for _, p := range r.Validity.Periods {
			for c := range p.When {
				if !slices.Contains(v.Validity.PeriodConditions, c) {
					s.errorf("validity period condition %q is not one of %v", c, v.Validity.PeriodConditions)
				}
			}
		}
	}
	tags := map[string]bool{}
	for i, st := range r.Stages {
		where := fmt.Sprintf("stages[%d]", i)
		if tags[st.Tag()] {
			s.errorf("%s: stage tag %q is used twice; give the stages distinct ids", where, st.Tag())
		}
		tags[st.Tag()] = true
		if _, ok := v.Statuses[st.Status]; !ok {
			s.errorf("%s: status %q is not in the vocabulary", where, st.Status)
		}
		s.checkCondition(where, &st.When, ids)
		def := s.checkKey(where+".messageKey", st.MessageKey, "message")
		for name, p := range st.Params {
			s.use("param_sources", p.Source)
			if _, ok := v.ParamSources[p.Source]; !ok {
				s.errorf("%s: param %s: source %q is not in the vocabulary", where, name, p.Source)
			}
			if (p.Source == "needed" || p.Source == "last_date") && !ids[p.Ref] {
				s.errorf("%s: param %s: requirement %q does not exist", where, name, p.Ref)
			}
			if def != nil && !slices.ContainsFunc(def.Params, func(x string) bool { return strings.TrimSuffix(x, "?") == name }) {
				s.errorf("%s: %s takes no param %q (params: %v)", where, st.MessageKey, name, def.Params)
			}
		}
		if def != nil {
			for _, p := range def.Params {
				if !strings.HasSuffix(p, "?") && st.Params[p].Source == "" {
					s.errorf("%s: %s needs param %q", where, st.MessageKey, p)
				}
			}
		}
	}
	if r.Support != "not_supported" && (len(r.Stages) == 0 || r.Stages[len(r.Stages)-1].When.Op != "always") {
		s.errorf("the last stage must be `when: always`")
	}
	if r.Support == "not_supported" && strings.TrimSpace(r.Notes) == "" {
		s.errorf("a not_supported rule needs notes saying what is missing")
	}
	for _, h := range r.RestoredBy {
		s.checkHook("restored_by", h)
	}
	for _, h := range r.Resets {
		s.checkHook("resets", h)
	}
	if r.RuleDescriptionKey != "" {
		s.checkKey("ruleDescriptionKey", r.RuleDescriptionKey, "rule_description")
	}
	seen := map[string]bool{}
	for _, d := range r.Divergences {
		if seen[d.ID] {
			s.errorf("divergence %q is listed twice", d.ID)
		}
		seen[d.ID] = true
		if ref := r.ID + "#" + d.ID; !strings.Contains(s.changelog, ref) {
			s.errorf("divergence %s has no CHANGELOG.md line (mention %s)", d.ID, ref)
		}
	}
	return s.errs
}

func (s *semantics) checkIdentity(r *engine.Rule) {
	v := s.cat.Vocabulary
	parts := strings.Split(r.ID, ".")
	rel, _ := filepath.Rel(s.root, r.File)
	if len(parts) >= 3 {
		want := filepath.Join("catalogue", parts[0], parts[1], r.ID+".yaml")
		if rel != want {
			s.errorf("file must be %s (is %s)", want, rel)
		}
		if parts[0] != r.Authority {
			s.errorf("authority %q differs from the id prefix %q", r.Authority, parts[0])
		}
	}
	if !slices.Contains(v.CatalogueAuthorities, r.Authority) {
		s.errorf("authority %q is not one of %v", r.Authority, v.CatalogueAuthorities)
	}
	if _, ok := v.SourceKinds[r.SourceKind]; !ok {
		s.errorf("source_kind %q is not in the vocabulary", r.SourceKind)
	}
}

func (s *semantics) checkSource(where string, src engine.Source) {
	b, err := os.ReadFile(filepath.Join(s.root, src.File))
	if err != nil {
		s.errorf("%s.file: %v", where, err)
		return
	}
	text := normQuote(string(b))
	for _, q := range src.Quote {
		if strings.HasPrefix(strings.TrimSpace(q), "TODO: verify") {
			continue
		}
		if !strings.Contains(text, normQuote(q)) {
			s.errorf("%s.quote is not verbatim in %s: %q", where, src.File, abbreviate(q))
		}
	}
}

func abbreviate(s string) string {
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}

func (s *semantics) checkAppliesTo(r *engine.Rule) {
	v := s.cat.Vocabulary
	a := r.AppliesTo
	s.use("subjects", a.Subject)
	if _, ok := v.Subjects[a.Subject]; !ok {
		s.errorf("applies_to.subject %q is not in the vocabulary", a.Subject)
	}
	for _, x := range append(slices.Clone(a.Authorities), a.ExcludeAuthorities...) {
		if !slices.ContainsFunc(v.RecordAuthorities, func(y string) bool { return strings.EqualFold(x, y) }) {
			s.errorf("applies_to: authority %q is not one of %v", x, v.RecordAuthorities)
		}
	}
	for _, x := range append(slices.Clone(a.LicenceKinds), a.ExcludeLicenceKinds...) {
		if _, ok := v.LicenceKinds[x]; !ok {
			s.errorf("applies_to: licence kind %q is not in the vocabulary", x)
		}
	}
	s.values("applies_to.classes", append(slices.Clone(a.Classes), a.ExcludeClasses...), keys(v.Classes), false)
	s.values("applies_to.ulKinds", a.ULKinds, append(slices.Clone(v.ULKinds), "none"), false)
	s.values("applies_to.privilegeKinds", a.PrivilegeKinds, v.PrivilegeKinds, false)
	s.values("applies_to.credentialTypes", a.CredentialTypes, v.CredentialTypes, false)
	s.values("applies_to.launchMethods", a.LaunchMethods, v.LaunchMethods, false)
	if a.Holds != nil {
		s.checkHolds("applies_to.holds", a.Holds)
	}
}

func (s *semantics) values(where string, got, allowed []string, subject bool) {
	for _, x := range got {
		if x == "$subject" && subject {
			continue
		}
		if !slices.Contains(allowed, x) {
			s.errorf("%s: %q is not in the vocabulary", where, x)
		}
	}
}

func (s *semantics) checkWindow(where string, w *engine.Window) {
	s.use("windows", w.Kind)
	if _, ok := s.cat.Vocabulary.Windows[w.Kind]; !ok {
		s.errorf("%s: window %q is not in the vocabulary", where, w.Kind)
	}
}

// checkFilter validates filter keys and values; source is flights, events or "" (any).
func (s *semantics) checkFilter(where string, f *engine.Filter, source string) {
	v := s.cat.Vocabulary
	for _, k := range f.Keys() {
		s.use("filters", k)
		def, ok := v.Filters[k]
		if !ok {
			s.errorf("%s: filter %q is not in the vocabulary", where, k)
			continue
		}
		if source != "" && !slices.Contains(def.AppliesTo, source) {
			s.errorf("%s: filter %q does not apply to %s", where, k, source)
		}
	}
	s.values(where+".classes", f.Classes, keys(v.Classes), true)
	s.values(where+".excludeClasses", f.ExcludeClasses, keys(v.Classes), false)
	for _, pool := range f.HeldClassPools {
		s.values(where+".heldClassPools", pool, keys(v.Classes), false)
	}
	s.values(where+".categories", f.Categories, v.Categories, false)
	s.values(where+".ulKinds", f.ULKinds, v.ULKinds, true)
	for _, c := range f.ULCredit {
		s.values(where+".ulCredit.class", []string{c.Class}, keys(v.Classes), false)
		s.values(where+".ulCredit.ulKinds", c.ULKinds, v.ULKinds, false)
	}
	s.values(where+".launchMethods", f.LaunchMethods, v.LaunchMethods, true)
	s.values(where+".excludeLaunchMethods", f.ExcludeLaunchMethods, v.LaunchMethods, false)
	s.values(where+".roles", f.Roles, v.Roles, false)
	s.values(where+".withMinutes", f.WithMinutes, v.MinuteFields, false)
	s.values(where+".withoutMinutes", f.WithoutMinutes, v.MinuteFields, false)
	s.values(where+".fstdTypes", f.FSTDTypes, v.FSTDTypes, false)
	s.values(where+".towKinds", f.TowKinds, v.TowKinds, false)
	s.values(where+".eventKinds", f.EventKinds, keys(v.EventKinds), false)
	for flag := range f.Flags {
		s.values(where+".flags", []string{flag}, v.FlightFlags, false)
	}
	for _, x := range [][]string{f.TypeDesignators, f.Variants} {
		for _, y := range x {
			if strings.HasPrefix(y, "$") && y != "$subject" {
				s.errorf("%s: %q is not a valid reference (only $subject)", where, y)
			}
		}
	}
	for i, a := range f.Any {
		s.checkFilter(fmt.Sprintf("%s.any[%d]", where, i), a, source)
	}
}

func (s *semantics) checkNode(r *engine.Rule, n *engine.Node, inherited *engine.Filter, root bool) {
	if n == nil {
		return
	}
	v := s.cat.Vocabulary
	where := "requirement " + n.ID
	if n.ID == "" {
		where = "requirements"
	}
	if n.Window != nil {
		s.checkWindow(where, n.Window)
	}
	if n.When != nil {
		s.checkCondition(where+".when", n.When, nil)
	}
	f := inherited
	if n.Filter != nil {
		f = mergeForCheck(inherited, n.Filter)
	}
	if !n.IsLeaf() {
		s.use("combinators", n.Combinator())
		if n.Combinator() == "n_of" && n.NOf.N > len(n.NOf.Of) {
			s.errorf("%s: n_of needs %d of only %d children", where, n.NOf.N, len(n.NOf.Of))
		}
		for _, c := range n.Children() {
			if n.Combinator() != "all_of" && c.ID == "" {
				s.errorf("%s: every %s branch needs an id (coverage tags name it)", where, n.Combinator())
			}
			s.checkNode(r, c, f, false)
		}
		return
	}
	s.use("metrics", n.Metric)
	m, ok := v.Metrics[n.Metric]
	if !ok {
		s.errorf("%s: metric %q is not in the vocabulary", where, n.Metric)
	}
	s.use("units", n.Unit)
	if !v.HasUnit(n.Unit) {
		s.errorf("%s: unit %q is not in the vocabulary", where, n.Unit)
	} else if ok && !slices.Contains(m.Units, n.Unit) {
		s.errorf("%s: unit %q does not fit metric %s (one of %v)", where, n.Unit, n.Metric, m.Units)
	}
	if n.Window == nil && r.Window == nil && !windowInTree(r.Requirements, n) {
		s.errorf("%s: no window (set one on the rule, a parent or the requirement)", where)
	}
	if f != nil {
		s.checkFilter(where+".filter", f, m.Source)
	}
	if m.Source == "events" && (f == nil || len(f.EventKinds) == 0) {
		s.errorf("%s: metric events needs an eventKinds filter", where)
	}
	s.checkKey(where+".nameKey", n.NameKey, "requirement_name")
	if n.RemedyKey != "" {
		s.checkKey(where+".remedyKey", n.RemedyKey, "remedy")
	}
	if n.Messages != nil {
		for _, k := range []string{n.Messages.Met, n.Messages.Unmet, n.Messages.Untracked} {
			if k != "" {
				s.checkKey(where+".messages", k, "")
			}
		}
	}
	if n.EscapeHatch != "" {
		s.use("hatches", n.EscapeHatch)
		if _, ok := v.Hatches[n.EscapeHatch]; !ok {
			s.errorf("%s: escape hatch %q is not in the vocabulary", where, n.EscapeHatch)
		}
	}
}

// windowInTree reports whether some ancestor of target sets a window.
func windowInTree(n, target *engine.Node) bool {
	var found bool
	var walk func(x *engine.Node, has bool) bool
	walk = func(x *engine.Node, has bool) bool {
		has = has || x.Window != nil
		if x == target {
			found = has
			return true
		}
		for _, c := range x.Children() {
			if walk(c, has) {
				return true
			}
		}
		return false
	}
	if n != nil {
		walk(n, false)
	}
	return found
}

func mergeForCheck(base, over *engine.Filter) *engine.Filter {
	if base == nil {
		return over
	}
	return engine.MergeFilter(base, over)
}

func (s *semantics) checkCondition(where string, c *engine.Condition, ids map[string]bool) {
	v := s.cat.Vocabulary
	c.Walk(func(x *engine.Condition) {
		s.use("stage_conditions", x.Op)
		if _, ok := v.StageConditions[x.Op]; !ok {
			s.errorf("%s: condition %q is not in the vocabulary", where, x.Op)
		}
		switch x.Op {
		case "met", "unmet", "untracked":
			if ids != nil && !ids[x.Ref] {
				s.errorf("%s: %s names requirement %q, which does not exist", where, x.Op, x.Ref)
			}
		case "missing":
			if !slices.Contains(v.MissingInputs, x.Ref) {
				s.errorf("%s: missing %q is not one of %v", where, x.Ref, v.MissingInputs)
			}
		case "met_within":
			s.checkWindow(where+".met_within", x.Span)
			if !slices.Contains([]string{"calendar_months", "rolling_months", "rolling_days"}, x.Span.Kind) {
				s.errorf("%s: met_within takes calendar_months, rolling_months or rolling_days", where)
			}
		case "holds":
			s.checkHolds(where+".holds", x.Holds)
		}
	})
}

func (s *semantics) checkHolds(where string, h *engine.Holds) {
	v := s.cat.Vocabulary
	s.values(where+".classes", h.Classes, keys(v.Classes), false)
	s.values(where+".ulKinds", h.ULKinds, v.ULKinds, false)
	s.values(where+".licenceKinds", h.LicenceKinds, keys(v.LicenceKinds), false)
	s.values(where+".privileges", h.Privileges, v.PrivilegeKinds, false)
	s.values(where+".credentials", h.Credentials, v.CredentialTypes, false)
	if len(h.Classes)+len(h.LicenceKinds)+len(h.Privileges)+len(h.Credentials) == 0 {
		s.errorf("%s: name classes, licenceKinds, privileges or credentials", where)
	}
}

func (s *semantics) checkHook(section string, h engine.EventHook) {
	s.use("rule_events", section)
	if _, ok := s.cat.Vocabulary.EventKinds[h.Event]; !ok {
		s.errorf("%s: event %q is not in the vocabulary", section, h.Event)
	}
	if h.Filter != nil {
		s.checkFilter(section+".filter", h.Filter, "events")
	}
}

// checkKey verifies a key exists, is not deprecated and has the expected kind.
func (s *semantics) checkKey(where, key, kind string) *engine.KeyDef {
	if key == "" {
		s.errorf("%s: key is empty", where)
		return nil
	}
	def, ok := s.cat.Keys.Get(key)
	if !ok {
		s.errorf("%s: key %q is missing from messages/keys.yaml", where, key)
		return nil
	}
	if def.Deprecated {
		s.errorf("%s: key %q is deprecated", where, key)
	}
	if kind != "" && def.Kind != kind {
		s.errorf("%s: key %q is a %s key, want %s", where, key, def.Kind, kind)
	}
	return def
}
