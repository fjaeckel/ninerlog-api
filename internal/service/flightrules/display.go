package flightrules

import (
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// DisplayPICName returns the value to render in the "PIC Name" column of a
// logbook row, applying the canonical fallback chain:
//
//  1. flight.PICName, if set
//  2. flight.InstructorName, if set (dual flights typically only have the
//     instructor name populated; the instructor *is* PIC of record)
//  3. "SELF"
//
// All exporters (CSV, PDF, future formats) MUST use this helper instead of
// re-implementing the fallback locally.
func DisplayPICName(f *models.Flight) string {
	if f == nil {
		return "SELF"
	}
	if f.PICName != nil && strings.TrimSpace(*f.PICName) != "" {
		return *f.PICName
	}
	if f.InstructorName != nil && strings.TrimSpace(*f.InstructorName) != "" {
		return *f.InstructorName
	}
	return "SELF"
}

// ResolvePICNameForSave returns the value that should be persisted into
// `flight.PICName` when a flight is saved. The write-path equivalent of
// DisplayPICName:
//
//   - existing PICName wins (user explicitly set it),
//   - if the user is PIC and no instructor is involved → "Self",
//   - if the user is Dual and an InstructorName is set → that instructor,
//   - otherwise nil (column stays empty; exporter will fall through to
//     DisplayPICName's "SELF" default at render time).
//
// The CRUD handler must call this once at save time so the persisted
// column is canonical.
func ResolvePICNameForSave(f *models.Flight) *string {
	if f == nil {
		return nil
	}
	if f.PICName != nil && strings.TrimSpace(*f.PICName) != "" {
		return f.PICName
	}
	if f.IsPIC && !f.IsDual {
		s := "Self"
		return &s
	}
	if f.IsDual && f.InstructorName != nil && strings.TrimSpace(*f.InstructorName) != "" {
		return f.InstructorName
	}
	return nil
}

// PilotingCategory is the per-row bucket used for the EASA AMC1 FCL.050
// columns "SP-SE" / "SP-ME" / "Multi-Pilot".
type PilotingCategory int

const (
	// CategorySPSE: single-pilot single-engine.
	CategorySPSE PilotingCategory = iota
	// CategorySPME: single-pilot multi-engine.
	CategorySPME
	// CategoryMP: multi-pilot.
	CategoryMP
)

// PilotingCategoryFor returns the bucket for `flight`. acClass is the user's
// stored AircraftClass for this registration (e.g. "SEP", "MEP", "SET",
// "MET", "SES"), passed in by callers that have aircraft data. When acClass
// is empty (no fleet entry available) the rule defaults to SP-SE.
//
// Rule:
//   - MultiPilotTime > 0 ⇒ MP (user-declared)
//   - acClass starts with "ME" or "MET" or "SET" ⇒ SP-ME
//   - otherwise ⇒ SP-SE
func PilotingCategoryFor(f *models.Flight, acClass string) PilotingCategory {
	if f != nil && f.MultiPilotTime > 0 {
		return CategoryMP
	}
	c := strings.ToUpper(strings.TrimSpace(acClass))
	if strings.HasPrefix(c, "MEP") || strings.HasPrefix(c, "MET") || strings.HasPrefix(c, "SET") {
		return CategorySPME
	}
	return CategorySPSE
}

// RowTimes returns the minutes that should be filled into the SP-SE / SP-ME
// / MP columns for `flight` given its piloting category. Exactly one of the
// three returned values is non-zero (the others are 0).
func RowTimes(f *models.Flight, acClass string) (spSE, spME, mp int) {
	if f == nil {
		return 0, 0, 0
	}
	switch PilotingCategoryFor(f, acClass) {
	case CategoryMP:
		return 0, 0, f.MultiPilotTime
	case CategorySPME:
		return 0, f.TotalTime, 0
	default:
		return f.TotalTime, 0, 0
	}
}
