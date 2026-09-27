package engine

//go:generate go run ../cmd/rulesgen -root ..

// mv is one item's contribution to a metric; aux is the second sum of min_of_sums.
type mv struct {
	v, aux float64
	known  bool
}

func known(v int) mv { return mv{v: float64(v), known: true} }

func optInt(p *int) mv {
	if p == nil {
		return mv{}
	}
	return known(*p)
}

func boolCount(b bool) mv {
	if b {
		return known(1)
	}
	return known(0)
}

// metrics implements every metric the vocabulary declares (see zz_metrics_gen.go).
type metrics struct{}

func (metrics) metricFlights(*Flight) mv { return known(1) }

func (metrics) metricRouteSectors(f *Flight) mv {
	if f.CruiseMinutes == nil {
		return mv{}
	}
	return boolCount(*f.CruiseMinutes >= 15)
}

func (metrics) metricMinutesTotal(f *Flight) mv     { return known(f.Minutes.Total) }
func (metrics) metricMinutesPic(f *Flight) mv       { return known(f.Minutes.PIC) }
func (metrics) metricMinutesDual(f *Flight) mv      { return known(f.Minutes.Dual) }
func (metrics) metricMinutesSpic(f *Flight) mv      { return known(f.Minutes.SPIC) }
func (metrics) metricMinutesDualGiven(f *Flight) mv { return known(f.Minutes.DualGiven) }
func (metrics) metricMinutesIfr(f *Flight) mv       { return known(f.Minutes.IFR) }
func (metrics) metricMinutesPicOrDualOrSpic(f *Flight) mv {
	return known(f.Minutes.PIC + f.Minutes.Dual + f.Minutes.SPIC)
}

func (metrics) metricMinutesDualOrSpic(f *Flight) mv {
	return known(f.Minutes.Dual + f.Minutes.SPIC)
}

func (metrics) metricMinutesPicOrSpic(f *Flight) mv {
	return known(f.Minutes.PIC + f.Minutes.SPIC)
}

func (metrics) metricMinutesInstructorOrExaminer(f *Flight) mv {
	return known(f.Minutes.DualGiven + f.Minutes.Examiner)
}

func (metrics) metricTakeoffsTotal(f *Flight) mv { return known(f.Takeoffs.Total()) }
func (metrics) metricTakeoffsNight(f *Flight) mv { return known(f.Takeoffs.Night) }
func (metrics) metricLandingsTotal(f *Flight) mv { return known(f.Landings.Total()) }
func (metrics) metricLandingsNight(f *Flight) mv { return known(f.Landings.Night) }

func (metrics) metricTakeoffsAndLandings(f *Flight) mv {
	return mv{v: float64(f.Takeoffs.Total()), aux: float64(f.Landings.Total()), known: true}
}

func (metrics) metricFullStopLandings(f *Flight) mv      { return optInt(f.FullStopLandings) }
func (metrics) metricFullStopNightLandings(f *Flight) mv { return optInt(f.FullStopNightLandings) }
func (metrics) metricMountainLandings(f *Flight) mv      { return optInt(f.MountainLandings) }
func (metrics) metricLaunches(f *Flight) mv              { return known(f.Launches) }
func (metrics) metricApproaches(f *Flight) mv            { return known(f.Approaches) }
func (metrics) metricHolds(f *Flight) mv                 { return known(f.Holds) }

func (metrics) metricInterceptAndTrack(f *Flight) mv {
	if f.InterceptAndTrack == nil {
		return mv{}
	}
	return boolCount(*f.InterceptAndTrack)
}

func (metrics) metricTrainingFlights(f *Flight) mv { return boolCount(f.Minutes.Dual > 0) }

func (metrics) metricLongestTrainingFlightMinutes(f *Flight) mv {
	if f.Minutes.Dual <= 0 {
		return known(0)
	}
	return known(f.Minutes.Total)
}

func (metrics) metricTows(f *Flight) mv {
	if !f.Flags.TowFlight {
		return known(0)
	}
	if f.TowedGliders != nil {
		return known(*f.TowedGliders)
	}
	return known(1)
}

func (metrics) metricNotRecorded(*Flight) mv { return mv{} }

func (metrics) metricEvents(*Event) mv { return known(1) }
