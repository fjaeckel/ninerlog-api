package handlers

import (
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
)

// webLogbookHeaders is the header row of Web Logbook's logbook CSV export
// (vsimakhin/web-logbook, app/ui/src/components/UIElements/CSVExportButton.jsx).
var webLogbookHeaders = []string{
	"Date",
	"Departure Place", "Departure Time",
	"Arrival Place", "Arrival Time",
	"Aircraft Model", "Aircraft Reg",
	"Time SE", "Time ME", "Time MCC", "Time Total",
	"Landings Day", "Landings Night",
	"Time Night", "Time IFR", "Time PIC", "Time CoPilot", "Time Dual", "Time Instructor",
	"SIM Type", "SIM Time",
	"PIC Name", "Remarks", "Tags",
}

// webLogbookSelf is the PIC name Web Logbook uses for the logbook owner.
const webLogbookSelf = "Self"

// writeWebLogbookCSV writes flights in Web Logbook's CSV layout and storage
// formats: DD/MM/YYYY dates, HHMM block times, H:MM durations. Columns follow
// the EASA layout; FSTD sessions are written as simulator rows with only the
// date, SIM Type, SIM Time, remarks and tags filled.
func writeWebLogbookCSV(w *csv.Writer, flights []*models.Flight, userName string) {
	csvWrite(w, webLogbookHeaders)

	for _, f := range flights {
		dep, depTime, arr, arrTime := "", "", "", ""
		acType, acReg := "", ""
		spSE, spME, mp, total := "", "", "", ""
		ldgDay, ldgNight := "0", "0"
		night, ifr, pic, cop, dual, instr := "", "", "", "", "", ""
		picName := ""

		simType, simTime := "", ""
		if f.IsSimulator {
			simType = "FSTD"
			if f.FSTDType != nil && *f.FSTDType != "" {
				simType = *f.FSTDType
			}
			simTime = fmtHM(f.SimulatedFlightTime)
		} else {
			_, simType, simTime = flightrules.FSTDFields(f, "02/01/2006", fmtHM)

			dep = safeStrCSV(f.DepartureICAO)
			arr = safeStrCSV(f.ArrivalICAO)
			depTime = webLogbookClock(f.OffBlockTime)
			arrTime = webLogbookClock(f.OnBlockTime)
			acType, acReg = f.AircraftType, f.AircraftReg

			seMin, meMin, mpMin := flightrules.RowTimes(f, "")
			spSE, spME, mp = fmtHM(seMin), fmtHM(meMin), fmtHM(mpMin)
			total = fmtHM(f.TotalTime)

			ldgDay = fmt.Sprintf("%d", f.LandingsDay)
			ldgNight = fmt.Sprintf("%d", f.LandingsNight)

			night = fmtHM(f.NightTime)
			ifr = fmtHM(flightrules.EffectiveIFRTime(f))
			pic = fmtHM(flightrules.PICColumnTime(f))
			cop = fmtHM(flightrules.CoPilotColumnTime(f))
			dual = fmtHM(f.DualTime)
			instr = fmtHM(f.DualGivenTime)

			picName = flightrules.DisplayPICName(f, userName)
			if strings.EqualFold(strings.TrimSpace(picName), "self") {
				picName = webLogbookSelf
			}
		}

		remarks := flightrules.CombinedRemarks(f,
			flightrules.FlagIPC, flightrules.FlagFlightReview, flightrules.FlagProficiencyCheck)

		csvWrite(w, []string{
			f.Date.Format("02/01/2006"),
			dep, depTime,
			arr, arrTime,
			acType, acReg,
			spSE, spME, mp, total,
			ldgDay, ldgNight,
			night, ifr, pic, cop, dual, instr,
			simType, simTime,
			picName, remarks, webLogbookTags(f),
		})
	}
}

// webLogbookClock converts a stored "HH:MM[:SS]" clock time to Web Logbook's
// "HHMM" form.
func webLogbookClock(s *string) string {
	return strings.Replace(fmtTimeCSV(s), ":", "", 1)
}

// webLogbookTags returns the flight's check and passenger flags as Web Logbook
// tags, comma-separated.
func webLogbookTags(f *models.Flight) string {
	var tags []string
	add := func(active bool, tag string) {
		if active {
			tags = append(tags, tag)
		}
	}
	add(f.IsIPC, "IPC")
	add(f.IsFlightReview, "Flight Review")
	add(f.IsProficiencyCheck, "Proficiency Check")
	add(f.IsPassenger, "Passenger")
	return strings.Join(tags, ",")
}
