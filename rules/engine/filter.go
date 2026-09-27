package engine

import "slices"

// match is a tri-state filter result.
type match int

const (
	matchNo match = iota
	matchYes
	matchUnknown
)

// matcher accumulates filter checks; any no wins, then any unknown.
type matcher struct {
	res     match
	unknown []string
}

func newMatcher() *matcher { return &matcher{res: matchYes} }

func (m *matcher) no() { m.res = matchNo }

func (m *matcher) check(ok bool) {
	if !ok {
		m.res = matchNo
	}
}

func (m *matcher) unknownBy(filter string) {
	if m.res == matchYes {
		m.res = matchUnknown
	}
	if !slices.Contains(m.unknown, filter) {
		m.unknown = append(m.unknown, filter)
	}
}

// resolvedFilter is a merged filter with $subject replaced.
type resolvedFilter struct {
	*Filter
	classes map[string]bool
	credit  map[string]ULCredit
	anyOf   []*resolvedFilter
}

const subjectToken = "$subject"

func (e *evalCtx) resolveList(l []string, subject string) []string {
	out := make([]string, 0, len(l))
	for _, v := range l {
		if v == subjectToken {
			if subject != "" {
				out = append(out, subject)
			}
			continue
		}
		out = append(out, v)
	}
	return out
}

// resolveFilter replaces $subject and expands class pools.
func (e *evalCtx) resolveFilter(f *Filter) *resolvedFilter {
	if f == nil {
		f = &Filter{}
	}
	c := *f
	c.Classes = e.resolveList(f.Classes, e.subj.Class)
	ul := e.subj.ULKind
	if ul == "" {
		ul = e.subj.Detail
	}
	c.ULKinds = e.resolveList(f.ULKinds, ul)
	c.LaunchMethods = e.resolveList(f.LaunchMethods, e.subj.Detail)
	c.TypeDesignators = e.resolveList(f.TypeDesignators, e.subj.typeDesignator)
	c.Variants = e.resolveList(f.Variants, e.subj.variant)
	r := &resolvedFilter{Filter: &c}
	if f.Has("classes") {
		r.classes = map[string]bool{}
		for _, cl := range c.Classes {
			r.classes[cl] = true
		}
		held := e.heldClassesOnLicence()
		for _, pool := range f.HeldClassPools {
			hit := false
			for _, cl := range pool {
				hit = hit || r.classes[cl]
			}
			if !hit {
				continue
			}
			for _, cl := range pool {
				if held[cl] {
					r.classes[cl] = true
				}
			}
		}
		r.credit = map[string]ULCredit{}
		for _, uc := range f.ULCredit {
			if r.classes[uc.Class] {
				r.credit[uc.Class] = uc
			}
		}
	}
	for _, a := range f.Any {
		r.anyOf = append(r.anyOf, e.resolveFilter(a))
	}
	return r
}

// heldClassesOnLicence returns the rating classes on the subject's licence.
func (e *evalCtx) heldClassesOnLicence() map[string]bool {
	out := map[string]bool{}
	if e.subj.licence == nil {
		return out
	}
	for _, r := range e.p.ratings {
		if r.LicenceID == e.subj.licence.ID {
			out[r.Class] = true
		}
	}
	return out
}

// matchFlight applies a resolved filter to a flight.
func (e *evalCtx) matchFlight(f *resolvedFilter, fl *Flight) *matcher {
	m := newMatcher()
	switch f.Simulator {
	case "only":
		m.check(fl.IsSimulator)
	case "include":
	default:
		m.check(!fl.IsSimulator)
	}
	if f.classes != nil {
		e.matchFlightClass(f, fl, m)
	}
	if len(f.ExcludeClasses) > 0 {
		m.check(!slices.Contains(f.ExcludeClasses, fl.Class))
	}
	if len(f.Categories) > 0 {
		m.check(slices.Contains(f.Categories, e.v.Category(fl.Class)))
	}
	if f.Has("ulKinds") {
		switch {
		case fl.Class != "ULTRALIGHT":
			m.no()
		case fl.ULKind == "":
			m.unknownBy("ulKinds")
		default:
			m.check(slices.Contains(f.ULKinds, fl.ULKind))
		}
	}
	if f.Has("launchMethods") {
		m.check(fl.LaunchMethod != "" && slices.Contains(f.LaunchMethods, fl.LaunchMethod))
	}
	if len(f.ExcludeLaunchMethods) > 0 {
		m.check(!slices.Contains(f.ExcludeLaunchMethods, fl.LaunchMethod))
	}
	stringFilter(m, f.Has("typeDesignators"), "typeDesignators", f.TypeDesignators, fl.TypeDesignator)
	stringFilter(m, f.Has("variants"), "variants", f.Variants, fl.Variant)
	if len(f.Roles) > 0 {
		m.check(slices.ContainsFunc(f.Roles, func(r string) bool { return roleMinutes(fl.Minutes, r) > 0 }))
	}
	for _, name := range f.WithMinutes {
		v, _ := fl.Minutes.Minute(name)
		m.check(v > 0)
	}
	for _, name := range f.WithoutMinutes {
		v, _ := fl.Minutes.Minute(name)
		m.check(v == 0)
	}
	if len(f.FSTDTypes) > 0 && fl.IsSimulator {
		if fl.FSTDType == "" {
			m.unknownBy("fstdTypes")
		} else {
			m.check(slices.Contains(f.FSTDTypes, fl.FSTDType))
		}
	}
	for name, want := range f.Flags {
		got, _ := fl.Flags.Get(name)
		m.check(got == want)
	}
	if f.MinMinutes != nil {
		m.check(fl.Minutes.Total >= *f.MinMinutes)
	}
	if f.MinLandings != nil {
		m.check(fl.Landings.Total() >= *f.MinLandings)
	}
	if f.MinDistanceKm != nil {
		if fl.DistanceKm == nil {
			m.unknownBy("minDistanceKm")
		} else {
			m.check(*fl.DistanceKm >= *f.MinDistanceKm)
		}
	}
	boolFilter(m, "tailwheel", f.Tailwheel, fl.Tailwheel)
	boolFilter(m, "soleManipulator", f.SoleManipulator, fl.SoleManipulator)
	boolFilter(m, "pilotFlying", f.PilotFlying, fl.PilotFlying)
	if len(f.TowKinds) > 0 {
		switch {
		case !fl.Flags.TowFlight:
			m.no()
		case fl.TowKind == "":
			m.unknownBy("towKinds")
		default:
			m.check(slices.Contains(f.TowKinds, fl.TowKind))
		}
	}
	if len(f.anyOf) > 0 {
		e.matchAny(m, f.anyOf, func(sub *resolvedFilter) *matcher { return e.matchFlight(sub, fl) })
	}
	return m
}

func (e *evalCtx) matchFlightClass(f *resolvedFilter, fl *Flight, m *matcher) {
	if f.classes[fl.Class] {
		return
	}
	if fl.Class != "ULTRALIGHT" || len(f.credit) == 0 {
		m.no()
		return
	}
	best := matchNo
	var unknownBy string
	for _, uc := range f.credit {
		switch {
		case fl.ULKind == "":
			best, unknownBy = matchUnknown, "ulCredit"
		case !slices.Contains(uc.ULKinds, fl.ULKind):
		case uc.MinMTOMKg != nil && fl.MTOMKg == nil:
			if best != matchYes {
				best, unknownBy = matchUnknown, "ulCredit"
			}
		case uc.MinMTOMKg != nil && *fl.MTOMKg < *uc.MinMTOMKg:
		default:
			best = matchYes
		}
		if best == matchYes {
			return
		}
	}
	if best == matchUnknown {
		m.unknownBy(unknownBy)
		return
	}
	m.no()
}

func (e *evalCtx) matchAny(m *matcher, subs []*resolvedFilter, fn func(*resolvedFilter) *matcher) {
	best := matchNo
	var unknown []string
	for _, s := range subs {
		r := fn(s)
		if r.res == matchYes {
			return
		}
		if r.res == matchUnknown {
			best = matchUnknown
			unknown = append(unknown, r.unknown...)
		}
	}
	if best == matchNo {
		m.no()
		return
	}
	for _, u := range unknown {
		m.unknownBy(u)
	}
}

func stringFilter(m *matcher, on bool, name string, want []string, got string) {
	if !on {
		return
	}
	if got == "" {
		m.unknownBy(name)
		return
	}
	m.check(slices.Contains(want, got))
}

func boolFilter(m *matcher, name string, want, got *bool) {
	if want == nil {
		return
	}
	if got == nil {
		m.unknownBy(name)
		return
	}
	m.check(*got == *want)
}

// matchEvent applies a resolved filter to an event.
func (e *evalCtx) matchEvent(f *resolvedFilter, ev *Event) *matcher {
	m := newMatcher()
	switch f.Simulator {
	case "only":
		m.check(ev.IsSimulator)
	case "include":
	default:
		m.check(!ev.IsSimulator)
	}
	if len(f.EventKinds) > 0 {
		m.check(slices.Contains(f.EventKinds, ev.Kind))
	}
	if len(f.EventRatings) > 0 {
		m.check(ev.Rating != "" && slices.Contains(f.EventRatings, ev.Rating))
	}
	if f.classes != nil {
		if ev.Class == "" {
			m.unknownBy("classes")
		} else {
			m.check(f.classes[ev.Class])
		}
	}
	if len(f.ExcludeClasses) > 0 {
		m.check(!slices.Contains(f.ExcludeClasses, ev.Class))
	}
	if len(f.Categories) > 0 {
		if ev.Class == "" {
			m.unknownBy("categories")
		} else {
			m.check(slices.Contains(f.Categories, e.v.Category(ev.Class)))
		}
	}
	if f.Has("ulKinds") {
		switch {
		case ev.Class != "ULTRALIGHT":
			m.no()
		case ev.ULKind == "":
			m.unknownBy("ulKinds")
		default:
			m.check(slices.Contains(f.ULKinds, ev.ULKind))
		}
	}
	stringFilter(m, f.Has("typeDesignators"), "typeDesignators", f.TypeDesignators, ev.TypeDesignator)
	stringFilter(m, f.Has("variants"), "variants", f.Variants, ev.Variant)
	if len(f.anyOf) > 0 {
		e.matchAny(m, f.anyOf, func(sub *resolvedFilter) *matcher { return e.matchEvent(sub, ev) })
	}
	return m
}
