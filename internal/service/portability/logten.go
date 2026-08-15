package portability

import (
	"io"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/fjaeckel/ninerlog-api/pkg/csvsafe"
)

// LogTen Pro import layout.
//
// LogTen imports a flat CSV and maps columns by header name, presenting the
// mapping for confirmation. Using LogTen's own field names in the header row
// is what makes that mapping auto-detect instead of asking the pilot to wire
// up forty columns by hand.
//
// Unlike ForeFlight there is no separate aircraft table: aircraft attributes
// ride on each flight row, and LogTen builds its fleet from them.
//
// Layout source: LogTen Pro's documented import field names. Not yet
// round-tripped through a live LogTen import; see docs/PORTABILITY.md.

// logTenColumns is the header row, in order. Header names are LogTen's field
// names; changing one changes what LogTen maps the column to.
var logTenColumns = []string{
	"flight_flightDate",
	"flight_from",
	"flight_to",
	"flight_route",
	"aircraft_aircraftID",
	"aircraftType_type",
	"aircraftType_make",
	"aircraftType_model",
	"aircraftType_category",
	"aircraftType_aircraftClass",
	"flight_actualDepartureTime",
	"flight_actualArrivalTime",
	"flight_takeoffTime",
	"flight_landingTime",
	"flight_totalTime",
	"flight_pic",
	"flight_sic",
	"flight_solo",
	"flight_dualReceived",
	"flight_dualGiven",
	"flight_multiPilot",
	"flight_night",
	"flight_actualInstrument",
	"flight_simulatedInstrument",
	"flight_crossCountry",
	"flight_simulator",
	"flight_ground",
	"flight_distance",
	"flight_dayTakeoffs",
	"flight_dayLandings",
	"flight_nightTakeoffs",
	"flight_nightLandings",
	"flight_totalLandings",
	"flight_holds",
	"flight_approaches",
	"flight_approachType",
	"flight_flightReview",
	"flight_instrumentProficiencyCheck",
	"flight_proficiencyCheck",
	"flight_selectedCrewPIC",
	"flight_selectedCrewSIC",
	"flight_selectedCrewInstructor",
	"flight_selectedCrewStudent",
	"flight_remarks",
	"flight_instructorComments",
	"flight_endorsement",
}

func writeLogTenPro(out io.Writer, b *Bundle) error {
	w := csvsafe.NewWriter(out)
	if err := w.Write(logTenColumns); err != nil {
		return err
	}

	fleet := fleetIndex(b.Aircraft)
	simulators := simulatorRegistrations(b)
	for _, f := range b.Flights {
		if err := w.Write(logTenRow(f, b.PilotName, fleet, simulators)); err != nil {
			return err
		}
	}

	w.Flush()
	return w.Error()
}

func logTenRow(f *models.Flight, pilotName string, fleet map[string]*models.Aircraft, simulators map[string]bool) []string {
	key := strings.ToUpper(strings.TrimSpace(f.AircraftReg))

	make_, model := "", f.AircraftType
	class := ""
	if a, ok := fleet[key]; ok {
		if a.Make != "" {
			make_ = a.Make
		}
		if a.Model != "" {
			model = a.Model
		}
		class = str(a.AircraftClass)
	}

	// A training device gets no aircraft category or class — labelling an FNPT
	// as a single-engine aeroplane would let LogTen fold simulator hours into
	// aeroplane totals.
	category, ltClass := "", ""
	if !simulators[key] {
		category, ltClass = AircraftCategoryClass(class)
	}

	return []string{
		f.Date.Format("2006-01-02"),
		str(f.DepartureICAO),
		str(f.ArrivalICAO),
		str(f.Route),
		f.AircraftReg,
		f.AircraftType,
		make_,
		model,
		category,
		ltClass,
		clock(f.OffBlockTime),
		clock(f.OnBlockTime),
		clock(f.DepartureTime),
		clock(f.ArrivalTime),
		hours(f.TotalTime),
		hours(f.PICTime),
		hours(f.SICTime),
		hours(f.SoloTime),
		hours(f.DualTime),
		hours(f.DualGivenTime),
		hours(f.MultiPilotTime),
		hours(f.NightTime),
		hours(f.ActualInstrumentTime),
		hours(f.SimulatedInstrumentTime),
		hours(f.CrossCountryTime),
		hours(f.SimulatedFlightTime),
		hours(f.GroundTrainingTime),
		distanceCell(f.Distance),
		count(f.TakeoffsDay),
		count(f.LandingsDay),
		count(f.TakeoffsNight),
		count(f.LandingsNight),
		count(f.AllLandings),
		count(f.Holds),
		count(f.ApproachesCount),
		approachTypeSummary(f),
		boolCell(f.IsFlightReview),
		boolCell(f.IsIPC),
		boolCell(f.IsProficiencyCheck),
		resolvedPICName(f, pilotName),
		crewName(f, models.CrewRoleSIC, pilotName),
		flightrules.InstructorNameFromCrew(f, pilotName),
		crewName(f, models.CrewRoleStudent, pilotName),
		str(f.Remarks),
		str(f.InstructorComments),
		str(f.Endorsements),
	}
}

// approachTypeSummary joins the distinct structured approach types on a
// flight, e.g. "ILS, RNAV/GPS". LogTen carries one approach-type field per
// flight rather than a per-approach list.
func approachTypeSummary(f *models.Flight) string {
	if len(f.Approaches) == 0 {
		return ""
	}
	seen := make(map[string]bool, len(f.Approaches))
	var types []string
	for _, a := range f.Approaches {
		kind := strings.TrimSpace(a.Type)
		if kind == "" || seen[kind] {
			continue
		}
		seen[kind] = true
		types = append(types, kind)
	}
	return strings.Join(types, ", ")
}

// crewName returns the first crew member in the given role whose name is not
// the account holder's.
func crewName(f *models.Flight, role models.CrewRole, pilotName string) string {
	for _, m := range f.CrewMembers {
		if m.Role != role {
			continue
		}
		name := strings.TrimSpace(m.Name)
		if name == "" || flightrules.MatchesUser(name, pilotName) {
			continue
		}
		return name
	}
	return ""
}
