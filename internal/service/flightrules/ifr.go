package flightrules

import (
	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// EffectiveIFRTime returns the IFR/Instrument time minutes that should be
// reported for a flight. If the user explicitly set IFRTime it is returned
// as-is. Otherwise the sum of ActualInstrumentTime + SimulatedInstrumentTime
// is used (capped at TotalTime so a logging mistake cannot produce >100%).
//
// This is the single source of truth for IFR derivation: imports, the
// auto-calc pipeline and exporters all flow through it.
func EffectiveIFRTime(f *models.Flight) int {
	if f == nil {
		return 0
	}
	if f.IFRTime > 0 {
		return f.IFRTime
	}
	derived := f.ActualInstrumentTime + f.SimulatedInstrumentTime
	if derived <= 0 {
		return 0
	}
	if f.TotalTime > 0 && derived > f.TotalTime {
		return f.TotalTime
	}
	return derived
}
