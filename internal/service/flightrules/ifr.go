package flightrules

import (
	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// EffectiveIFRTime returns the IFR/Instrument time minutes that should be
// reported for a flight. Explicit and derived values alike are capped at
// TotalTime; an unset IFRTime is derived from ActualInstrumentTime +
// SimulatedInstrumentTime.
//
// IFR is an operational condition of a flight (AMC1 FCL.050 Col 14), so an
// FSTD session always reports zero — its instrument work stays in
// SimulatedInstrumentTime and the FSTD columns.
//
// This is the single source of truth for IFR derivation: imports, the
// auto-calc pipeline and exporters all flow through it.
func EffectiveIFRTime(f *models.Flight) int {
	if f == nil || f.IsSimulator {
		return 0
	}
	derived := f.IFRTime
	if derived <= 0 {
		derived = f.ActualInstrumentTime + f.SimulatedInstrumentTime
	}
	if derived <= 0 {
		return 0
	}
	if derived > f.TotalTime {
		return f.TotalTime
	}
	return derived
}
