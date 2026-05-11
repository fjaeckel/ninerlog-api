package flightrules

import (
	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// IsFSTDRow reports whether a flight row should populate the FSTD-block
// columns (EASA AMC1 FCL.050 Cols 20-22, FAA §61.51(b)(1)(iv)).
//
// Both: a non-empty FSTDType identifier AND non-zero simulated flight time.
func IsFSTDRow(f *models.Flight) bool {
	if f == nil {
		return false
	}
	if f.FSTDType == nil || *f.FSTDType == "" {
		return false
	}
	return f.SimulatedFlightTime > 0
}

// FSTDFields returns the (date, type, time) triple to fill into the FSTD
// columns. dateLayout is the Go time layout used to format the date (e.g.
// "02.01.2006" for full EASA CSV, "02.01" for the compact PDF column).
// timeFmt formats the minutes value the way the caller wants (HH:MM, decimal
// hours, etc.).
//
// Returns ("", "", "") when IsFSTDRow is false.
func FSTDFields(f *models.Flight, dateLayout string, timeFmt func(int) string) (date, kind, dur string) {
	if !IsFSTDRow(f) {
		return "", "", ""
	}
	return f.Date.Format(dateLayout), *f.FSTDType, timeFmt(f.SimulatedFlightTime)
}
