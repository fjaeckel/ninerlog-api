package portability

import (
	"io"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/fjaeckel/ninerlog-api/pkg/csvsafe"
)

// CrewLounge PILOTLOG (formerly mccPILOTLOG) import layout.
//
// PILOTLOG is the EASA-side destination: it is the only target here that
// models multi-pilot time, the single-/multi-engine split and FSTD sessions as
// first-class columns, so an EASA logbook survives this route intact where the
// FAA-oriented formats have to flatten it.
//
// Its importer maps by header name and reads times as HH:MM rather than
// decimal hours, matching how an EASA logbook is kept.
//
// Layout source: CrewLounge's documented import columns. Not yet round-tripped
// through a live PILOTLOG import; see docs/PORTABILITY.md.

// crewLoungeColumns is the header row, in order.
var crewLoungeColumns = []string{
	"Date",
	"AircraftReg",
	"AircraftType",
	"AircraftMake",
	"AircraftModel",
	"AircraftClass",
	"DepartureICAO",
	"DepartureTime",
	"ArrivalICAO",
	"ArrivalTime",
	"TakeoffTime",
	"LandingTime",
	"TotalTime",
	"SingleEngine",
	"MultiEngine",
	"MultiPilot",
	"PICTime",
	"CopilotTime",
	"DualTime",
	"InstructorTime",
	"NightTime",
	"IFRTime",
	"ActualInstrument",
	"SimulatedInstrument",
	"CrossCountry",
	"SoloTime",
	"TakeoffsDay",
	"TakeoffsNight",
	"LandingsDay",
	"LandingsNight",
	"Approaches",
	"Holdings",
	"SimType",
	"SimTime",
	"PICName",
	"CopilotName",
	"InstructorName",
	"StudentName",
	"LaunchMethod",
	"Remarks",
	"Endorsement",
	"Signed",
}

func writeCrewLounge(out io.Writer, b *Bundle) error {
	w := csvsafe.NewWriter(out)
	if err := w.Write(crewLoungeColumns); err != nil {
		return err
	}

	fleet := fleetIndex(b.Aircraft)
	for _, f := range b.Flights {
		if err := w.Write(crewLoungeRow(f, b.PilotName, fleet)); err != nil {
			return err
		}
	}

	w.Flush()
	return w.Error()
}

func crewLoungeRow(f *models.Flight, pilotName string, fleet map[string]*models.Aircraft) []string {
	acMake, acModel := "", f.AircraftType
	class := aircraftClassFor(fleet, f.AircraftReg)
	if a, ok := fleet[strings.ToUpper(strings.TrimSpace(f.AircraftReg))]; ok {
		acMake = a.Make
		if a.Model != "" {
			acModel = a.Model
		}
	}

	// RowTimes is the single place that decides how a row splits across
	// single-pilot single-engine, single-pilot multi-engine and multi-pilot.
	// Reusing it here keeps this export consistent with the EASA PDF and CSV
	// the same pilot can download alongside it.
	spSE, spME, mp := flightrules.RowTimes(f, class)

	// An FSTD session is not flight time, and PILOTLOG models that properly:
	// it has dedicated simulator columns that stay out of the flight totals.
	//
	// NinerLog stores a session's duration in TotalTime (a flight row requires
	// it), so writing both TotalTime and SimTime would count the session twice
	// in the destination and inflate the pilot's aeroplane hours. The session
	// therefore goes into the simulator columns only.
	simType, simTime, totalTime := "", "", hhmm(f.TotalTime)
	if flightrules.IsFSTDRow(f) {
		simType = str(f.FSTDType)
		simTime = hhmm(f.SimulatedFlightTime)
		totalTime = ""
		spSE, spME, mp = 0, 0, 0
	}

	return []string{
		f.Date.Format("2006-01-02"),
		f.AircraftReg,
		f.AircraftType,
		acMake,
		acModel,
		class,
		str(f.DepartureICAO),
		clock(f.OffBlockTime),
		str(f.ArrivalICAO),
		clock(f.OnBlockTime),
		clock(f.DepartureTime),
		clock(f.ArrivalTime),
		totalTime,
		hhmm(spSE),
		hhmm(spME),
		hhmm(mp),
		hhmm(f.PICTime),
		hhmm(f.SICTime),
		hhmm(f.DualTime),
		hhmm(f.DualGivenTime),
		hhmm(f.NightTime),
		hhmm(flightrules.EffectiveIFRTime(f)),
		hhmm(f.ActualInstrumentTime),
		hhmm(f.SimulatedInstrumentTime),
		hhmm(f.CrossCountryTime),
		hhmm(f.SoloTime),
		count(f.TakeoffsDay),
		count(f.TakeoffsNight),
		count(f.LandingsDay),
		count(f.LandingsNight),
		count(f.ApproachesCount),
		count(f.Holds),
		simType,
		simTime,
		flightrules.DisplayPICName(f, pilotName),
		crewName(f, models.CrewRoleSIC, pilotName),
		flightrules.InstructorNameFromCrew(f, pilotName),
		crewName(f, models.CrewRoleStudent, pilotName),
		str(f.LaunchMethod),
		str(f.Remarks),
		str(f.Endorsements),
		yesNo(f.SignatureID != nil),
	}
}
