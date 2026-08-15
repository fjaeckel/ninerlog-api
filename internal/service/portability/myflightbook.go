package portability

import (
	"io"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/fjaeckel/ninerlog-api/pkg/csvsafe"
)

// MyFlightbook import layout.
//
// MyFlightbook matches columns by header name and ignores column order, but
// the names must match exactly — whole name, no surrounding whitespace. Its
// importer is also strict about two value conventions:
//
//   - booleans are the literal English strings "Yes" and "No", whatever the
//     pilot's locale; and
//   - anything beyond its core columns must be named as a flight property,
//     which is how the EASA-specific times below survive the move.
//
// Layout source: MyFlightbook's published import table. Not yet round-tripped
// through a live MyFlightbook import; see docs/PORTABILITY.md.

// myFlightbookColumns is the header row. The first block is MyFlightbook's
// core flight fields; the second is named properties carrying the times its
// core schema has no column for.
var myFlightbookColumns = []string{
	"Date",
	"Tail Number",
	"Model",
	"Route",
	"Comments",
	"Total Flight Time",
	"PIC",
	"SIC",
	"Solo",
	"Dual Received",
	"CFI",
	"Night",
	"IMC",
	"Simulated Instrument",
	"Ground Simulator",
	"Cross-Country",
	"Landings",
	"FS Day Landings",
	"FS Night Landings",
	"Approaches",
	"Hold",
	"Flight Review",
	"Instrument Proficiency Check",
	"Engine Start",
	"Engine End",
	"Flight Start",
	"Flight End",

	// Named properties. These are how the EASA side of the logbook survives an
	// import into a product built around FAA columns.
	"Multi-Pilot Time",
	"IFR Time",
	"Ground Instruction Given",
	"Night Takeoffs",
	"Day Takeoffs",
	"Name of PIC",
	"Instructor Name",
	"Student Name",
	"Simulator Type",
	"Endorsements",
	"Launch Method",
}

func writeMyFlightbook(out io.Writer, b *Bundle) error {
	w := csvsafe.NewWriter(out)
	if err := w.Write(myFlightbookColumns); err != nil {
		return err
	}

	fleet := fleetIndex(b.Aircraft)
	for _, f := range b.Flights {
		if err := w.Write(myFlightbookRow(f, b.PilotName, fleet)); err != nil {
			return err
		}
	}

	w.Flush()
	return w.Error()
}

func myFlightbookRow(f *models.Flight, pilotName string, fleet map[string]*models.Aircraft) []string {
	// MyFlightbook keys aircraft on tail number and model. Prefer the fleet's
	// model when there is one; the ICAO type designator is a usable fallback.
	model := f.AircraftType
	if a, ok := fleet[strings.ToUpper(strings.TrimSpace(f.AircraftReg))]; ok && a.Model != "" {
		model = a.Model
	}

	return []string{
		f.Date.Format("2006-01-02"),
		f.AircraftReg,
		model,
		// MyFlightbook has no departure/arrival columns — the route field is
		// where airports live. Passing only an explicit route would strand
		// every flight logged without one with no airports at all.
		routeOrEndpoints(f),
		str(f.Remarks),
		hoursZero(f.TotalTime),
		hours(f.PICTime),
		hours(f.SICTime),
		hours(f.SoloTime),
		hours(f.DualTime),
		hours(f.DualGivenTime),
		hours(f.NightTime),
		hours(f.ActualInstrumentTime),
		hours(f.SimulatedInstrumentTime),
		hours(f.SimulatedFlightTime),
		hours(f.CrossCountryTime),
		count(f.AllLandings),
		count(f.LandingsDay),
		count(f.LandingsNight),
		count(f.ApproachesCount),
		yesNo(f.Holds > 0),
		yesNo(f.IsFlightReview),
		yesNo(f.IsIPC),
		clock(f.OffBlockTime),
		clock(f.OnBlockTime),
		clock(f.DepartureTime),
		clock(f.ArrivalTime),

		// Named properties.
		hours(f.MultiPilotTime),
		hours(flightrules.EffectiveIFRTime(f)),
		hours(f.GroundTrainingTime),
		count(f.TakeoffsNight),
		count(f.TakeoffsDay),
		resolvedPICName(f, pilotName),
		flightrules.InstructorNameFromCrew(f, pilotName),
		crewName(f, models.CrewRoleStudent, pilotName),
		str(f.FSTDType),
		str(f.Endorsements),
		str(f.LaunchMethod),
	}
}
