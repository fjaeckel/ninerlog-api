package training

import (
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
)

// Message keys of training items.
const (
	MsgMet                 = "training.met"
	MsgNotMet              = "training.not_met"
	MsgCrossCountryUnknown = "training.cross_country_distance_unknown"
	MsgCreditAvailable     = "training.credit_available"
	MsgCreditNone          = "training.credit_none"
)

// Syllabus thresholds.
const (
	SPLInstructionMinutes    = 15 * 60
	SPLDualMinutes           = 10 * 60
	SPLSupervisedSoloMinutes = 2 * 60
	SPLLaunches              = 45
	SPLSoloCrossCountryKM    = 50
	SPLDualCrossCountryKM    = 100
	SPLCreditPercent         = 10
	SPLCreditMaxMinutes      = 7 * 60

	TMGInstructionMinutes = 6 * 60
	TMGDualMinutes        = 4 * 60
	TMGSoloCrossCountryKM = 150

	ULThreeAxisTotalMinutes   = 30 * 60
	ULThreeAxisSoloMinutes    = 5 * 60
	ULWeightShiftTotalMinutes = 25 * 60
	ULWeightShiftDualMinutes  = 10 * 60
	ULWeightShiftSoloMinutes  = 5 * 60
)

const kmPerNM = 1.852

// Credit carries the SFCL.130(b) input: PIC minutes on aircraft of another category.
type Credit struct {
	PICMinutes int
}

// Minutes is 10 % of the PIC minutes, capped at 7 hours.
func (c Credit) Minutes() int {
	m := c.PICMinutes * SPLCreditPercent / 100
	if m > SPLCreditMaxMinutes {
		return SPLCreditMaxMinutes
	}
	return m
}

type programmeMeta struct {
	discipline models.Discipline
	titleKey   string
	legalBasis string
	match      func(repository.TrainingFlight) bool
}

func classIs(class string) func(repository.TrainingFlight) bool {
	return func(f repository.TrainingFlight) bool { return f.AircraftClass == class }
}

func ulKindIn(kinds ...models.ULKind) func(repository.TrainingFlight) bool {
	return func(f repository.TrainingFlight) bool {
		if f.AircraftClass != string(models.ClassTypeUL) || f.ULKind == nil {
			return false
		}
		for _, k := range kinds {
			if *f.ULKind == k {
				return true
			}
		}
		return false
	}
}

var programmes = map[models.TrainingProgrammeID]programmeMeta{
	models.TrainingSPL: {
		discipline: models.DisciplineSailplane, titleKey: "training.programme.spl",
		legalBasis: "SFCL.130", match: classIs(string(models.ClassTypeGlider)),
	},
	models.TrainingSPLTMGExtension: {
		discipline: models.DisciplineTMG, titleKey: "training.programme.spl_tmg_extension",
		legalBasis: "SFCL.150(b)", match: classIs(string(models.ClassTypeTMG)),
	},
	models.TrainingULThreeAxis: {
		discipline: models.DisciplineUltralight, titleKey: "training.programme.ul_three_axis",
		legalBasis: "LuftPersV §42", match: ulKindIn(models.AircraftKindsForRating(models.ULKindThreeAxis)...),
	},
	models.TrainingULWeightShift: {
		discipline: models.DisciplineUltralight, titleKey: "training.programme.ul_weight_shift",
		legalBasis: "LuftPersV §42", match: ulKindIn(models.ULKindWeightShift),
	},
}

// totals sums a programme's matching flights.
type totals struct {
	dual, pic, spic int
	launches        int
	signed          int
	soloXC, dualXC  xcResult
}

// xcResult records whether a qualifying cross-country flight was found and whether its
// distance was known.
type xcResult struct {
	found    bool
	verified bool
}

func (r *xcResult) consider(f repository.TrainingFlight, minKM float64) {
	if f.CrossCountryMinutes <= 0 {
		return
	}
	switch {
	case f.DistanceNM <= 0:
		r.found = true
	case f.DistanceNM*kmPerNM >= minKM:
		r.found = true
		r.verified = true
	}
}

// isSolo reports whether a flight has no dual time and some PIC or SPIC time.
func isSolo(f repository.TrainingFlight) bool {
	return f.DualMinutes == 0 && f.PICMinutes+f.SPICMinutes > 0
}

func sum(flights []repository.TrainingFlight, match func(repository.TrainingFlight) bool, soloKM, dualKM float64) totals {
	var t totals
	for _, f := range flights {
		if !match(f) {
			continue
		}
		t.dual += f.DualMinutes
		t.pic += f.PICMinutes
		t.spic += f.SPICMinutes
		if f.DualMinutes > 0 || f.SPICMinutes > 0 {
			t.launches += f.Launches
		}
		if f.Signed {
			t.signed++
		}
		if soloKM > 0 && isSolo(f) {
			t.soloXC.consider(f, soloKM)
		}
		if dualKM > 0 && f.DualMinutes > 0 {
			t.dualXC.consider(f, dualKM)
		}
	}
	return t
}

func item(key string, required, current int, unit models.TrainingUnit) models.TrainingItem {
	it := models.TrainingItem{Key: key, Required: required, Current: current, Unit: unit, Met: current >= required, MessageKey: MsgNotMet}
	if it.Met {
		it.MessageKey = MsgMet
	}
	return it
}

// crossCountryItem is met by any qualifying result, preferring one with a known distance.
func crossCountryItem(key string, results ...xcResult) models.TrainingItem {
	var found, verified bool
	for _, r := range results {
		found = found || r.found
		verified = verified || r.verified
	}
	it := models.TrainingItem{Key: key, Required: 1, Unit: models.TrainingUnitFlights, MessageKey: MsgNotMet}
	if found {
		it.Current, it.Met, it.MessageKey = 1, true, MsgMet
		if !verified {
			it.MessageKey = MsgCrossCountryUnknown
		}
	}
	return it
}

// Evaluate computes one programme over the pilot's training flights. credit is the
// SFCL.130(b) input for SPL, nil when the pilot holds no licence for another category.
func Evaluate(id models.TrainingProgrammeID, flights []repository.TrainingFlight, credit *Credit) models.TrainingProgramme {
	meta := programmes[id]
	p := models.TrainingProgramme{
		ID: id, Discipline: meta.discipline, TitleKey: meta.titleKey, LegalBasis: meta.legalBasis,
	}
	switch id {
	case models.TrainingSPL:
		t := sum(flights, meta.match, SPLSoloCrossCountryKM, SPLDualCrossCountryKM)
		p.SignedFlights = t.signed
		p.Items = []models.TrainingItem{
			item("training.spl.instruction_time", SPLInstructionMinutes, t.dual+t.spic, models.TrainingUnitMinutes),
			item("training.spl.dual_time", SPLDualMinutes, t.dual, models.TrainingUnitMinutes),
			item("training.spl.supervised_solo_time", SPLSupervisedSoloMinutes, t.spic, models.TrainingUnitMinutes),
			item("training.spl.launches", SPLLaunches, t.launches, models.TrainingUnitLaunches),
			crossCountryItem("training.spl.cross_country", t.soloXC, t.dualXC),
		}
		if credit != nil {
			c := credit.Minutes()
			it := models.TrainingItem{
				Key: "training.spl.credit_sfcl130b", Required: SPLCreditMaxMinutes, Current: c,
				Unit: models.TrainingUnitMinutes, Met: c > 0, Informational: true, MessageKey: MsgCreditNone,
			}
			if c > 0 {
				it.MessageKey = MsgCreditAvailable
			}
			p.Items = append(p.Items, it)
		}
	case models.TrainingSPLTMGExtension:
		t := sum(flights, meta.match, TMGSoloCrossCountryKM, 0)
		p.SignedFlights = t.signed
		p.Items = []models.TrainingItem{
			item("training.tmg.instruction_time", TMGInstructionMinutes, t.dual+t.spic, models.TrainingUnitMinutes),
			item("training.tmg.dual_time", TMGDualMinutes, t.dual, models.TrainingUnitMinutes),
			crossCountryItem("training.tmg.solo_cross_country", t.soloXC),
		}
	case models.TrainingULThreeAxis:
		t := sum(flights, meta.match, 0, 0)
		p.SignedFlights = t.signed
		p.Items = []models.TrainingItem{
			item("training.ul.total_time", ULThreeAxisTotalMinutes, t.dual+t.pic+t.spic, models.TrainingUnitMinutes),
			item("training.ul.solo_time", ULThreeAxisSoloMinutes, t.pic+t.spic, models.TrainingUnitMinutes),
		}
	case models.TrainingULWeightShift:
		t := sum(flights, meta.match, 0, 0)
		p.SignedFlights = t.signed
		p.Items = []models.TrainingItem{
			item("training.ul.total_time", ULWeightShiftTotalMinutes, t.dual+t.pic+t.spic, models.TrainingUnitMinutes),
			item("training.ul.dual_time", ULWeightShiftDualMinutes, t.dual, models.TrainingUnitMinutes),
			item("training.ul.solo_time", ULWeightShiftSoloMinutes, t.pic+t.spic, models.TrainingUnitMinutes),
		}
	}
	p.AllMet = true
	for _, it := range p.Items {
		if !it.Informational && !it.Met {
			p.AllMet = false
		}
	}
	return p
}
