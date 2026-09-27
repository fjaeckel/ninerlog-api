package engine

// span is an inclusive date range; an unbounded start has open set.
type span struct {
	from Date
	to   Date
	open bool
}

func (s span) contains(d Date) bool {
	return !d.After(s.to) && (s.open || !d.Before(s.from))
}

// windowRange returns the window w as seen on date d, or false when an anchor is unknown.
func (e *evalCtx) windowRange(w *Window, d Date) (span, bool) {
	var s span
	if w == nil {
		return span{to: d, open: true}, true
	}
	switch w.Kind {
	case "lifetime":
		s = span{to: d, open: true}
	case "rolling_days":
		s = span{from: d.AddDays(-w.N), to: d}
	case "rolling_months":
		s = span{from: d.AddMonths(-w.N), to: d}
	case "calendar_months":
		s = span{from: d.FirstOfMonth().AddMonths(-w.N), to: d}
	case "before_expiry_months":
		if e.expiry == nil {
			return span{}, false
		}
		s = span{from: e.expiry.AddMonths(-w.N), to: minDate(*e.expiry, d)}
	case "validity_period":
		from := e.subj.validFrom
		if from == nil {
			from = e.subj.issued
		}
		if from == nil {
			return span{}, false
		}
		s = span{from: *from, to: d}
		if e.expiry != nil {
			s.to = minDate(*e.expiry, d)
		}
	case "since_issue":
		var from *Date
		if w.Anchor == "licence" {
			if e.subj.licence != nil {
				from = e.subj.licence.Issued
			}
		} else {
			from = e.subj.issued
		}
		if from == nil {
			return span{}, false
		}
		s = span{from: *from, to: d}
	default:
		return span{}, false
	}
	return s, true
}

// windowMoving reports whether w follows the evaluation date.
func windowMoving(w *Window) bool {
	if w == nil {
		return false
	}
	switch w.Kind {
	case "rolling_days", "rolling_months", "calendar_months":
		return true
	}
	return false
}

// windowExit returns the first date on which an item dated x is outside moving window w.
func windowExit(w *Window, x Date) Date {
	switch w.Kind {
	case "rolling_days":
		return x.AddDays(w.N + 1)
	case "calendar_months":
		return x.FirstOfMonth().AddMonths(w.N + 1)
	}
	c := x.AddMonths(w.N).AddDays(1)
	for c.AddDays(-1).After(x) && c.AddDays(-1).AddMonths(-w.N).After(x) {
		c = c.AddDays(-1)
	}
	for !c.AddMonths(-w.N).After(x) {
		c = c.AddDays(1)
	}
	return c
}
