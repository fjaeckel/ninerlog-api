package portability

import (
	"io"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/fjaeckel/ninerlog-api/pkg/csvsafe"
)

// ForeFlight Logbook import template.
//
// The file is not a plain CSV: it is a title line, then two labelled tables
// (aircraft, then flights), each preceded by a row declaring the data type of
// every column. ForeFlight matches columns by position within each table, so
// the order below is load-bearing — a column inserted in the wrong place
// silently shifts every value after it into the wrong field.
//
// Layout source: ForeFlight's published Logbook import template. It has not
// been round-tripped through a live ForeFlight import from this codebase; see
// docs/PORTABILITY.md for what still needs validating.

// foreFlightAircraftColumns is the aircraft table header, in order.
var foreFlightAircraftColumns = []string{
	"AircraftID", "EquipmentType", "TypeCode", "Year", "Make", "Model",
	"Category", "Class", "GearType", "EngineType",
	"Complex", "HighPerformance", "Pressurized", "TAA",
}

// foreFlightAircraftTypes declares each aircraft column's data type.
var foreFlightAircraftTypes = []string{
	"text", "text", "text", "text", "text", "text",
	"category", "class", "gearType", "engineType",
	"boolean", "boolean", "boolean", "boolean",
}

// foreFlightFlightColumns is the flights table header, in order.
var foreFlightFlightColumns = []string{
	"Date", "AircraftID", "From", "To", "Route",
	"TimeOut", "TimeOff", "TimeOn", "TimeIn", "OnDuty", "OffDuty",
	"TotalTime", "PIC", "SIC", "Night", "Solo", "CrossCountry",
	"Distance",
	"DayTakeoffs", "DayLandingsFullStop", "NightTakeoffs", "NightLandingsFullStop", "AllLandings",
	"ActualInstrument", "SimulatedInstrument",
	"HobbsStart", "HobbsEnd", "TachStart", "TachEnd",
	"Holds",
	"Approach1", "Approach2", "Approach3", "Approach4", "Approach5", "Approach6",
	"DualGiven", "DualReceived", "SimulatedFlight", "GroundTraining",
	"InstructorName", "InstructorComments",
	"Person1", "Person2", "Person3", "Person4", "Person5", "Person6",
	"FlightReview", "Checkride", "IPC", "NVGProficiency", "FAA6158",
	"PilotComments",
}

// foreFlightFlightTypes declares each flight column's data type.
var foreFlightFlightTypes = []string{
	"date", "text", "text", "text", "text",
	"hhmm", "hhmm", "hhmm", "hhmm", "hhmm", "hhmm",
	"decimal", "decimal", "decimal", "decimal", "decimal", "decimal",
	"decimal",
	"number", "number", "number", "number", "number",
	"decimal", "decimal",
	"decimal", "decimal", "decimal", "decimal",
	"number",
	"text", "text", "text", "text", "text", "text",
	"decimal", "decimal", "decimal", "decimal",
	"text", "text",
	"text", "text", "text", "text", "text", "text",
	"boolean", "boolean", "boolean", "boolean", "boolean",
	"text",
}

// foreFlightMaxPersons is how many Person columns the template carries.
const foreFlightMaxPersons = 6

// foreFlightMaxApproaches is how many Approach columns the template carries.
const foreFlightMaxApproaches = 6

func writeForeFlight(out io.Writer, b *Bundle) error {
	w := csvsafe.NewWriter(out)

	// Title line. ForeFlight uses it to recognise the file as its own template.
	if err := w.Write([]string{"ForeFlight Logbook Import"}); err != nil {
		return err
	}
	if err := w.Write([]string{""}); err != nil {
		return err
	}

	// --- Aircraft table ---
	if err := w.Write([]string{"Aircraft Table"}); err != nil {
		return err
	}
	if err := w.Write(foreFlightAircraftTypes); err != nil {
		return err
	}
	if err := w.Write(foreFlightAircraftColumns); err != nil {
		return err
	}
	for _, a := range resolveAircraft(b) {
		// A training device is declared as a simulator, not an aeroplane.
		// Getting this wrong would let ForeFlight count FNPT hours as aircraft
		// flight time — an error a pilot would only discover at a licence
		// renewal, long after the migration.
		equipment, category, class, gear, engine := "aircraft", "", "", "", ""
		if a.IsSimulator {
			equipment = "sim"
		} else {
			category, class = AircraftCategoryClass(a.Class)
			gear = "fixed_tricycle"
			if a.Tailwheel {
				gear = "fixed_tailwheel"
			}
			engine = "Piston"
		}

		if err := w.Write([]string{
			a.Registration,
			equipment,
			a.TypeCode,
			"", // Year — NinerLog does not record it
			a.Make,
			a.Model,
			category,
			class,
			gear,
			engine,
			boolCell(a.Complex),
			boolCell(a.HighPerf),
			"", // Pressurized — not recorded
			"", // TAA — not recorded
		}); err != nil {
			return err
		}
	}

	if err := w.Write([]string{""}); err != nil {
		return err
	}

	// --- Flights table ---
	if err := w.Write([]string{"Flights Table"}); err != nil {
		return err
	}
	if err := w.Write(foreFlightFlightTypes); err != nil {
		return err
	}
	if err := w.Write(foreFlightFlightColumns); err != nil {
		return err
	}
	for _, f := range b.Flights {
		if err := w.Write(foreFlightRow(f, b.PilotName)); err != nil {
			return err
		}
	}

	w.Flush()
	return w.Error()
}

func foreFlightRow(f *models.Flight, pilotName string) []string {
	row := []string{
		f.Date.Format("2006-01-02"),
		f.AircraftReg,
		str(f.DepartureICAO),
		str(f.ArrivalICAO),
		str(f.Route),
		clockCompact(f.OffBlockTime),
		clockCompact(f.DepartureTime),
		clockCompact(f.ArrivalTime),
		clockCompact(f.OnBlockTime),
		"", // OnDuty — NinerLog does not track duty periods
		"", // OffDuty
		hours(f.TotalTime),
		hours(f.PICTime),
		hours(f.SICTime),
		hours(f.NightTime),
		hours(f.SoloTime),
		hours(f.CrossCountryTime),
		distanceCell(f.Distance),
		count(f.TakeoffsDay),
		count(f.LandingsDay),
		count(f.TakeoffsNight),
		count(f.LandingsNight),
		count(f.AllLandings),
		hours(f.ActualInstrumentTime),
		hours(f.SimulatedInstrumentTime),
		"", "", "", "", // Hobbs/Tach — not recorded
		count(f.Holds),
	}

	row = append(row, foreFlightApproaches(f)...)

	row = append(row,
		hours(f.DualGivenTime),
		hours(f.DualTime),
		hours(f.SimulatedFlightTime),
		hours(f.GroundTrainingTime),
		flightrules.InstructorNameFromCrew(f, pilotName),
		str(f.InstructorComments),
	)

	row = append(row, foreFlightPersons(f, pilotName)...)

	row = append(row,
		boolCell(f.IsFlightReview),
		boolCell(f.IsProficiencyCheck),
		boolCell(f.IsIPC),
		"", // NVGProficiency — not recorded
		"", // FAA6158 — not recorded
		str(f.Remarks),
	)

	return row
}

// foreFlightApproaches renders approaches in ForeFlight's
// "count;type;runway;airport" notation across the six Approach columns.
//
// Two gaps are closed here deliberately:
//
//   - A pilot who logged only a total count, with no structured detail, still
//     gets those approaches out — as one generic entry rather than nothing.
//   - When ApproachesCount exceeds the structured entries, the remainder is
//     emitted as a generic entry so the total the pilot flew still matches.
//
// ForeFlight has only six columns; a flight with more distinct approaches than
// fit is truncated here. The open archive carries the full structured list, so
// the detail is never lost from the export as a whole.
func foreFlightApproaches(f *models.Flight) []string {
	cells := make([]string, foreFlightMaxApproaches)

	next := 0
	for _, a := range f.Approaches {
		if next >= foreFlightMaxApproaches {
			return cells
		}
		cells[next] = foreFlightApproachCell(a)
		next++
	}

	if remainder := f.ApproachesCount - len(f.Approaches); remainder > 0 && next < foreFlightMaxApproaches {
		cells[next] = strings.Join([]string{
			countZero(remainder),
			"APCH",
			"",
			str(f.ArrivalICAO),
		}, ";")
	}
	return cells
}

// foreFlightApproachCell renders one structured approach. Each ApproachEntry
// is a single approach, so the leading count is always 1.
func foreFlightApproachCell(a models.ApproachEntry) string {
	kind := strings.TrimSpace(a.Type)
	if kind == "" {
		kind = "APCH"
	}
	return strings.Join([]string{
		"1",
		kind,
		strings.TrimSpace(str(a.Runway)),
		strings.TrimSpace(str(a.Airport)),
	}, ";")
}

// foreFlightPersons fills the Person columns from the flight's crew.
//
// Two people are left out on purpose:
//
//   - the account holder, because ForeFlight records the other people on
//     board, not the pilot whose logbook it is; and
//   - the instructor, who already has the dedicated InstructorName column —
//     listing them twice makes ForeFlight show the same person as both the
//     instructor and a passenger on every training flight.
func foreFlightPersons(f *models.Flight, pilotName string) []string {
	cells := make([]string, foreFlightMaxPersons)
	instructor := strings.TrimSpace(flightrules.InstructorNameFromCrew(f, pilotName))

	i := 0
	for _, m := range f.CrewMembers {
		if i >= foreFlightMaxPersons {
			break
		}
		name := strings.TrimSpace(m.Name)
		if name == "" || flightrules.MatchesUser(name, pilotName) {
			continue
		}
		if instructor != "" && strings.EqualFold(name, instructor) {
			continue
		}
		cells[i] = name
		i++
	}
	return cells
}
