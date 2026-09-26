package handlers

import (
	"fmt"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/fjaeckel/ninerlog-api/pkg/duration"
)

// sumFlights sums v over flights.
func sumFlights(flights []*models.Flight, v func(*models.Flight) int) int {
	total := 0
	for _, f := range flights {
		total += v(f)
	}
	return total
}

// csvTotalsLabel is the first cell of a totals row.
func csvTotalsLabel(flights []*models.Flight) string {
	return fmt.Sprintf("Total (%d flights)", len(flights))
}

// standardCSVTotals is the totals row for writeStandardCSV.
func standardCSVTotals(flights []*models.Flight, prefs exportPrefs) []string {
	dec := func(v func(*models.Flight) int) string { return prefs.formatDecimal(sumFlights(flights, v)) }
	cnt := func(v func(*models.Flight) int) string { return fmt.Sprintf("%d", sumFlights(flights, v)) }
	distance := 0.0
	for _, f := range flights {
		distance += f.Distance
	}
	return []string{
		csvTotalsLabel(flights), "", "", "", "", "",
		"", "", "", "",
		dec(func(f *models.Flight) int { return f.TotalTime }),
		dec(func(f *models.Flight) int { return f.PICTime }),
		dec(func(f *models.Flight) int { return f.SICTime }),
		dec(func(f *models.Flight) int { return f.NightTime }),
		dec(func(f *models.Flight) int { return f.SoloTime }),
		dec(func(f *models.Flight) int { return f.CrossCountryTime }),
		fmt.Sprintf("%.1f", distance),
		cnt(func(f *models.Flight) int { return f.TakeoffsDay }),
		cnt(func(f *models.Flight) int { return f.LandingsDay }),
		cnt(func(f *models.Flight) int { return f.TakeoffsNight }),
		cnt(func(f *models.Flight) int { return f.LandingsNight }),
		cnt(func(f *models.Flight) int { return f.AllLandings }),
		dec(func(f *models.Flight) int { return f.ActualInstrumentTime }),
		dec(func(f *models.Flight) int { return f.SimulatedInstrumentTime }),
		cnt(func(f *models.Flight) int { return f.Holds }),
		cnt(func(f *models.Flight) int { return f.ApproachesCount }),
		dec(func(f *models.Flight) int { return f.DualGivenTime }),
		dec(func(f *models.Flight) int { return f.DualTime }),
		dec(func(f *models.Flight) int { return f.SimulatedFlightTime }),
		dec(func(f *models.Flight) int { return f.GroundTrainingTime }),
		"", "",
		"", "",
		dec(func(f *models.Flight) int { return f.IFRTime }),
		"",
		"",
		dec(func(f *models.Flight) int { return f.MultiPilotTime }),
		"", "",
		dec(func(f *models.Flight) int { return f.PICUSTime }),
		dec(func(f *models.Flight) int { return f.SPICTime }),
		dec(func(f *models.Flight) int { return f.ExaminerTime }),
		dec(func(f *models.Flight) int { return f.ReliefTime }),
		"",
		cnt(func(f *models.Flight) int { return f.Launches }),
		cnt(func(f *models.Flight) int { return boolCount(f.IsOutlanding) }),
		cnt(func(f *models.Flight) int { return boolCount(f.IsTowFlight) }),
		"",
		"", "",
	}
}

// boolCount returns 1 for true and 0 for false.
func boolCount(b bool) int {
	if b {
		return 1
	}
	return 0
}

// fstdMinutes returns the FSTD Time minutes of f as FSTDFields fills them.
func fstdMinutes(f *models.Flight) int {
	if !flightrules.IsFSTDRow(f) {
		return 0
	}
	return f.SimulatedFlightTime
}

func spSEMinutes(f *models.Flight) int { se, _, _ := flightrules.RowTimes(f, ""); return se }
func spMEMinutes(f *models.Flight) int { _, me, _ := flightrules.RowTimes(f, ""); return me }
func mpMinutes(f *models.Flight) int   { _, _, mp := flightrules.RowTimes(f, ""); return mp }

// easaCSVTotals is the totals row for writeEASACSV.
func easaCSVTotals(flights []*models.Flight) []string {
	hm := func(v func(*models.Flight) int) string { return fmtHM(sumFlights(flights, v)) }
	cnt := func(v func(*models.Flight) int) string { return fmt.Sprintf("%d", sumFlights(flights, v)) }
	return []string{
		csvTotalsLabel(flights),
		"", "", "", "",
		"", "",
		hm(spSEMinutes),
		hm(spMEMinutes),
		hm(mpMinutes),
		hm(func(f *models.Flight) int { return f.TotalTime }),
		"",
		cnt(func(f *models.Flight) int { return f.LandingsDay }),
		cnt(func(f *models.Flight) int { return f.LandingsNight }),
		hm(func(f *models.Flight) int { return f.NightTime }),
		hm(flightrules.EffectiveIFRTime),
		hm(flightrules.PICColumnTime),
		hm(flightrules.CoPilotColumnTime),
		hm(func(f *models.Flight) int { return f.DualTime }),
		hm(func(f *models.Flight) int { return f.DualGivenTime }),
		"", "",
		hm(fstdMinutes),
		"",
	}
}

// faaCSVTotals is the totals row for writeFAACSV.
func faaCSVTotals(flights []*models.Flight) []string {
	dec := func(v func(*models.Flight) int) string { return duration.FormatDecimal(sumFlights(flights, v)) }
	cnt := func(v func(*models.Flight) int) string { return fmt.Sprintf("%d", sumFlights(flights, v)) }
	return []string{
		csvTotalsLabel(flights),
		"", "",
		"", "",
		dec(func(f *models.Flight) int { return f.SoloTime }),
		dec(func(f *models.Flight) int { return f.PICTime }),
		dec(flightrules.FAASICColumnTime),
		dec(flightrules.FAADualColumnTime),
		dec(func(f *models.Flight) int { return f.DualGivenTime }),
		dec(func(f *models.Flight) int { return f.ActualInstrumentTime }),
		dec(func(f *models.Flight) int { return f.SimulatedInstrumentTime }),
		dec(func(f *models.Flight) int { return f.CrossCountryTime }),
		dec(func(f *models.Flight) int { return f.NightTime }),
		cnt(func(f *models.Flight) int { return f.LandingsDay }),
		cnt(func(f *models.Flight) int { return f.LandingsNight }),
		cnt(func(f *models.Flight) int { return f.ApproachesCount }),
		cnt(func(f *models.Flight) int { return f.Holds }),
		dec(func(f *models.Flight) int { return f.TotalTime }),
		"",
	}
}

// webLogbookSimMinutes returns the SIM Time minutes of f as writeWebLogbookCSV fills them.
func webLogbookSimMinutes(f *models.Flight) int {
	if f.IsSimulator {
		return f.SimulatedFlightTime
	}
	return fstdMinutes(f)
}

// webLogbookCSVTotals is the totals row for writeWebLogbookCSV.
func webLogbookCSVTotals(flights []*models.Flight) []string {
	flown := make([]*models.Flight, 0, len(flights))
	for _, f := range flights {
		if !f.IsSimulator {
			flown = append(flown, f)
		}
	}
	hm := func(v func(*models.Flight) int) string { return fmtHM(sumFlights(flown, v)) }
	cnt := func(v func(*models.Flight) int) string { return fmt.Sprintf("%d", sumFlights(flown, v)) }
	return []string{
		csvTotalsLabel(flights),
		"", "",
		"", "",
		"", "",
		hm(spSEMinutes),
		hm(spMEMinutes),
		hm(mpMinutes),
		hm(func(f *models.Flight) int { return f.TotalTime }),
		cnt(func(f *models.Flight) int { return f.LandingsDay }),
		cnt(func(f *models.Flight) int { return f.LandingsNight }),
		hm(func(f *models.Flight) int { return f.NightTime }),
		hm(flightrules.EffectiveIFRTime),
		hm(flightrules.PICColumnTime),
		hm(flightrules.CoPilotColumnTime),
		hm(func(f *models.Flight) int { return f.DualTime }),
		hm(func(f *models.Flight) int { return f.DualGivenTime }),
		"",
		fmtHM(sumFlights(flights, webLogbookSimMinutes)),
		"", "", "",
	}
}
