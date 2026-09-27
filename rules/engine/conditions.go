package engine

import (
	"math"
	"slices"
)

// cond evaluates a stage or requirement condition on d; root is the tree state on asOf.
func (e *evalCtx) cond(c *Condition, d Date, root tri) bool {
	switch c.Op {
	case "always":
		return true
	case "all_met":
		return root == triMet
	case "undetermined":
		return root == triUnknown
	case "met", "unmet":
		st, ok := e.leafByID(c.Ref, d)
		if !ok {
			return false
		}
		if c.Op == "met" {
			return st.met
		}
		return st.tracked && !st.met
	case "expired":
		return e.expiry != nil && d.After(*e.expiry)
	case "no_expiry":
		return e.expiry == nil
	case "expires_within":
		return e.expiry != nil && !d.After(*e.expiry) && d.DaysUntil(*e.expiry) <= c.Days
	case "valid_until_within":
		return e.validUntil != nil && !d.After(*e.validUntil) && d.DaysUntil(*e.validUntil) <= c.Days
	case "before_window":
		s, ok := e.windowRange(e.rule.Window, d)
		return ok && !s.open && s.from.After(d)
	case "met_within":
		return e.metWithin(c.Span)
	case "holds":
		return e.holds(c.Holds, d)
	case "missing":
		return c.Ref == "date_of_birth" && e.p.rec.Holder.DateOfBirth == nil
	case "all":
		for _, s := range c.List {
			if !e.cond(s, d, root) {
				return false
			}
		}
		return true
	case "any":
		for _, s := range c.List {
			if e.cond(s, d, root) {
				return true
			}
		}
		return false
	case "not":
		return !e.cond(c.Not, d, root)
	}
	return false
}

// ImplementedConditions lists the stage conditions cond understands.
var ImplementedConditions = []string{"always", "all_met", "undetermined", "met", "unmet", "expired", "no_expiry", "expires_within", "valid_until_within", "before_window", "met_within", "holds", "missing", "all", "any", "not"}

func (e *evalCtx) leafByID(id string, d Date) (leafState, bool) {
	for n, lf := range e.leaves {
		if n.ID == id {
			return e.evalLeaf(lf, d), true
		}
	}
	return leafState{}, false
}

// holds tests what the holder holds on d.
func (e *evalCtx) holds(h *Holds, d Date) bool {
	valid := func(exp *Date) bool { return !h.Valid || exp == nil || !d.After(*exp) }
	onLicence := func(id string) bool {
		return !h.SameLicence || (e.subj.licence != nil && e.subj.licence.ID == id)
	}
	var found []string
	switch {
	case len(h.Classes) > 0:
		for _, r := range e.p.ratings {
			if !slices.Contains(h.Classes, r.Class) || !onLicence(r.LicenceID) || !valid(r.Expires) {
				continue
			}
			if len(h.ULKinds) > 0 && !slices.Contains(h.ULKinds, r.ULKind) {
				continue
			}
			if len(h.LicenceKinds) > 0 && !slices.Contains(h.LicenceKinds, e.p.licenceKind[r.LicenceID]) {
				continue
			}
			found = append(found, r.Class)
		}
		return holdsResult(h.Every, h.Classes, found)
	case len(h.LicenceKinds) > 0:
		for _, l := range e.p.rec.Licences {
			if slices.Contains(h.LicenceKinds, e.p.licenceKind[l.ID]) && onLicence(l.ID) && valid(l.Expires) {
				found = append(found, e.p.licenceKind[l.ID])
			}
		}
		return holdsResult(h.Every, h.LicenceKinds, found)
	case len(h.Privileges) > 0:
		for _, pv := range e.p.rec.Privileges {
			if slices.Contains(h.Privileges, pv.Kind) && onLicence(pv.LicenceID) && valid(pv.Expires) {
				found = append(found, pv.Kind)
			}
		}
		return holdsResult(h.Every, h.Privileges, found)
	case len(h.Credentials) > 0:
		for _, c := range e.p.rec.Credentials {
			if slices.Contains(h.Credentials, c.Type) && valid(c.Expires) {
				found = append(found, c.Type)
			}
		}
		return holdsResult(h.Every, h.Credentials, found)
	}
	return false
}

func holdsResult(every bool, want, found []string) bool {
	if !every {
		return len(found) > 0
	}
	for _, w := range want {
		if !slices.Contains(found, w) {
			return false
		}
	}
	return true
}

// params computes stage messageParams.
func (e *evalCtx) params(src map[string]ParamSource, ev *Evaluation) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := map[string]any{}
	for name, p := range src {
		switch p.Source {
		case "days_to_expiry":
			if e.expiry != nil {
				out[name] = e.asOf.DaysUntil(*e.expiry)
			}
		case "expiry_date":
			if e.expiry != nil {
				out[name] = e.expiry.String()
			}
		case "window_opens_at":
			if s, ok := e.windowRange(e.rule.Window, e.asOf); ok && !s.open {
				out[name] = s.from.String()
			}
		case "valid_until":
			if ev.ValidUntil != nil {
				out[name] = ev.ValidUntil.String()
			}
		case "days_to_valid_until":
			if ev.ValidUntil != nil {
				out[name] = e.asOf.DaysUntil(*ev.ValidUntil)
			}
		case "needed", "last_date":
			for _, r := range ev.Requirements {
				if r.ID != p.Ref {
					continue
				}
				if p.Source == "needed" {
					out[name] = int(math.Max(0, math.Ceil(r.Required-r.Current)))
				} else if r.LastDate != nil {
					out[name] = r.LastDate.String()
				}
			}
		}
	}
	return out
}

// ImplementedParamSources lists the param sources params understands.
var ImplementedParamSources = []string{"days_to_expiry", "expiry_date", "window_opens_at", "valid_until", "days_to_valid_until", "needed", "last_date"}

// derivedExpiry computes the validity end; nil when an input is missing.
func (e *evalCtx) derivedExpiry(v *Validity) *Date {
	anchor := e.subj.issued
	if v.From == "valid_from" && e.subj.validFrom != nil {
		anchor = e.subj.validFrom
	}
	if anchor == nil {
		return nil
	}
	dob := e.p.rec.Holder.DateOfBirth
	for _, p := range v.Periods {
		ok := true
		for c, n := range p.When {
			if dob == nil {
				return nil
			}
			age := dob.YearsBetween(*anchor)
			switch c {
			case "age_under":
				ok = ok && age < n
			case "age_at_least":
				ok = ok && age >= n
			}
		}
		if !ok {
			continue
		}
		end := anchor.AddMonths(p.Months)
		if p.EndOfMonth {
			end = end.LastOfMonth()
		}
		if p.CapAtAge != nil {
			if dob == nil {
				return nil
			}
			end = minDate(end, dob.AddMonths(12**p.CapAtAge))
		}
		return &end
	}
	return nil
}
