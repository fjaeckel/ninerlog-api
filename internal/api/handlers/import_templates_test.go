package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/service/importtemplate"
)

// mappingsFor builds the lookup mapRowToFlight expects from a template's
// suggestions for a given header row.
func mappingsFor(templateID string, headers []string) map[string]generated.ImportColumnMapping {
	tpl := importtemplate.ByID(templateID)
	lookup := make(map[string]generated.ImportColumnMapping)
	for _, m := range toGeneratedMappings(importtemplate.Suggest(tpl, headers)) {
		lookup[m.SourceColumn] = m
	}
	return lookup
}

func headersOf(row map[string]string) []string {
	out := make([]string, 0, len(row))
	for k := range row {
		out = append(out, k)
	}
	return out
}

// A MyFlightbook row has no departure or arrival column at all — the sector
// lives in the route string. Before route-derived airports, every row of a
// MyFlightbook export failed validation on two required fields the file
// actually contained.
func TestMapRowToFlight_MyFlightbookDerivesAirportsFromRoute(t *testing.T) {
	row := map[string]string{
		"Date":        "2026-03-07",
		"Tail Number": "N12345",
		// The type code lives in "ICAO Model"; "Model" is the marketing
		// description and is deliberately not mapped.
		"ICAO Model":        "C172",
		"Model":             "C-172 S, Cessna Skyhawk SP",
		"Route":             "KSFO KSJC KOAK",
		"Comments":          "Bay tour",
		"Total Flight Time": "1.5",
		"Landings":          "3",
		"FS Day Landings":   "1",
		"FS Night Landings": "0",
		"IMC":               "0.2",
		"Approaches":        "1",
	}

	flight, errs := mapRowToFlight(row, mappingsFor("MYFLIGHTBOOK_CSV", headersOf(row)), nil)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if flight.DepartureIcao != "KSFO" {
		t.Errorf("DepartureIcao = %q, want KSFO", flight.DepartureIcao)
	}
	if flight.ArrivalIcao != "KOAK" {
		t.Errorf("ArrivalIcao = %q, want KOAK", flight.ArrivalIcao)
	}
	if flight.AircraftReg != "N12345" {
		t.Errorf("AircraftReg = %q, want N12345", flight.AircraftReg)
	}
	if flight.AircraftType != "C172" {
		t.Errorf("AircraftType = %q, want C172", flight.AircraftType)
	}
	if flight.TotalTime == nil || *flight.TotalTime != 90 {
		t.Errorf("TotalTime = %v, want 90 minutes", flight.TotalTime)
	}
	// 3 landings total, of which 1 was a full-stop day landing: the two
	// touch-and-goes only appear in the total column.
	if flight.Landings != 3 {
		t.Errorf("Landings = %d, want 3", flight.Landings)
	}
}

// A route naming one airport is how a local flight is logged; both endpoints
// are that airport rather than the row failing on a missing arrival.
func TestMapRowToFlight_SingleWaypointRoute(t *testing.T) {
	row := map[string]string{
		"Date":              "2026-03-07",
		"Tail Number":       "N12345",
		"ICAO Model":        "C172",
		"Route":             "KSFO",
		"Total Flight Time": "0.8",
	}
	flight, errs := mapRowToFlight(row, mappingsFor("MYFLIGHTBOOK_CSV", headersOf(row)), nil)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if flight.DepartureIcao != "KSFO" || flight.ArrivalIcao != "KSFO" {
		t.Errorf("got %q → %q, want KSFO → KSFO", flight.DepartureIcao, flight.ArrivalIcao)
	}
}

// An explicit departure/arrival column always wins over the route string.
func TestMapRowToFlight_ExplicitAirportsBeatRoute(t *testing.T) {
	row := map[string]string{
		"Date":       "2026-03-07",
		"AircraftID": "D-EABC",
		"From":       "EDDF",
		"To":         "EDDM",
		"Route":      "EDDS EDDN",
		"TotalTime":  "1.0",
	}
	flight, errs := mapRowToFlight(row, mappingsFor("FOREFLIGHT_CSV", headersOf(row)), nil)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if flight.DepartureIcao != "EDDF" || flight.ArrivalIcao != "EDDM" {
		t.Errorf("got %q → %q, want EDDF → EDDM", flight.DepartureIcao, flight.ArrivalIcao)
	}
}

func TestSplitRouteEndpoints(t *testing.T) {
	tests := []struct {
		in       string
		dep, arr string
	}{
		{"KSFO KOAK", "KSFO", "KOAK"},
		{"KSFO-KSJC-KOAK", "KSFO", "KOAK"},
		{"EDDF -> EDDM", "EDDF", "EDDM"},
		{"EDDF, EDDM", "EDDF", "EDDM"},
		{"EDDF/EDDM", "EDDF", "EDDM"},
		{"  KSFO  ", "KSFO", "KSFO"},
		{"", "", ""},
		{"   ", "", ""},
	}
	for _, tc := range tests {
		dep, arr := splitRouteEndpoints(tc.in)
		if dep != tc.dep || arr != tc.arr {
			t.Errorf("splitRouteEndpoints(%q) = (%q, %q), want (%q, %q)", tc.in, dep, arr, tc.dep, tc.arr)
		}
	}
}

// The day/night split and a total-landings column must not double-count, and
// the total must not erase a larger split.
func TestMapRowToFlight_LandingsReconciliation(t *testing.T) {
	tests := []struct {
		name            string
		all, day, night string
		want            int
	}{
		{"total exceeds split (touch-and-goes)", "5", "1", "1", 5},
		{"split equals total", "2", "1", "1", 2},
		{"split exceeds total", "1", "2", "1", 3},
		{"only total", "4", "", "", 4},
		{"only split", "", "2", "3", 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			row := map[string]string{
				"Date":                  "2026-03-07",
				"AircraftID":            "D-EABC",
				"From":                  "EDDF",
				"To":                    "EDDM",
				"TotalTime":             "1.0",
				"AllLandings":           tc.all,
				"DayLandingsFullStop":   tc.day,
				"NightLandingsFullStop": tc.night,
			}
			flight, errs := mapRowToFlight(row, mappingsFor("FOREFLIGHT_CSV", headersOf(row)), nil)
			if len(errs) > 0 {
				t.Fatalf("unexpected errors: %+v", errs)
			}
			if flight.Landings != tc.want {
				t.Errorf("Landings = %d, want %d", flight.Landings, tc.want)
			}
		})
	}
}

// A Vereinsflieger standard export is German-language and semicolon-separated,
// its dates are DD.MM.YYYY, its durations are bare whole minutes, and its places
// are written "Name ICAO". All of that has to work together or the rows error
// out — which is what the previous version of this template did: written from
// documentation, it missed "Lfz.", "Start" and "Landung" entirely, so every row
// failed on a registration the file was carrying all along.
func TestMapRowToFlight_VereinsfliegerStandard(t *testing.T) {
	row := map[string]string{
		"Datum":        "14.03.2026",
		"Lfz.":         "D-EABC",
		"Pilot":        "Rivera, Alex",
		"Begleiter/FI": "Okafor, Sam",
		"Start":        "09:12",
		"Landung":      "10:47",
		"Flugzeit":     "95",
		"Startort":     "Uetersen EDHE",
		"Landeort":     "Stade EDHS",
		"Landungen":    "1",
		"S.-Art":       "E",
		"Flugart":      "N",
		"Abr.":         "K",
		"Verein":       "Aero-Club Musterstadt e.V.",
		"Bemerkung":    "Überlandflug",
	}

	flight, errs := mapRowToFlight(row, mappingsFor("VEREINSFLIEGER_CSV", headersOf(row)), nil)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if got := flight.Date.String(); got != "2026-03-14" {
		t.Errorf("Date = %q, want 2026-03-14", got)
	}
	if flight.AircraftReg != "D-EABC" {
		t.Errorf("AircraftReg = %q, want D-EABC", flight.AircraftReg)
	}
	if flight.DepartureIcao != "EDHE" || flight.ArrivalIcao != "EDHS" {
		t.Errorf("got %q → %q, want EDHE → EDHS", flight.DepartureIcao, flight.ArrivalIcao)
	}
	// "Start"/"Landung" are wheels-off and wheels-on, not chocks. The standard
	// export has no block pair at all, so they must not be filed as one: block
	// times override the file's own total downstream, and taxi time would be
	// lost from every flight in the logbook.
	if flight.DepartureTime == nil || *flight.DepartureTime != "09:12:00" {
		t.Errorf("DepartureTime = %v, want 09:12:00", flight.DepartureTime)
	}
	if flight.ArrivalTime == nil || *flight.ArrivalTime != "10:47:00" {
		t.Errorf("ArrivalTime = %v, want 10:47:00", flight.ArrivalTime)
	}
	if flight.OffBlockTime != "" || flight.OnBlockTime != "" {
		t.Errorf("block times = %q/%q, want empty — the standard export has none",
			flight.OffBlockTime, flight.OnBlockTime)
	}
	// A bare integer is minutes, which is what Vereinsflieger writes. Read as
	// decimal hours it would be 95 hours.
	if flight.TotalTime == nil || *flight.TotalTime != 95 {
		t.Errorf("TotalTime = %v, want 95 minutes", flight.TotalTime)
	}
	if flight.Landings != 1 {
		t.Errorf("Landings = %d, want 1", flight.Landings)
	}
	if flight.Remarks == nil || *flight.Remarks != "Überlandflug" {
		t.Errorf("Remarks = %v, want Überlandflug", flight.Remarks)
	}

	// The club record names the pilot, and Vereinsflieger writes every name
	// inverted. Both names come through in reading order so they match the same
	// person arriving from any other logbook.
	if flight.CrewMembers == nil {
		t.Fatal("expected crew members for the Pilot and Begleiter/FI columns")
	}
	crew := map[string]string{}
	for _, c := range *flight.CrewMembers {
		crew[c.Name] = string(c.Role)
	}
	if role, ok := crew["Alex Rivera"]; !ok || role != "PIC" {
		t.Errorf("crew = %v, want Alex Rivera as PIC", crew)
	}
	// "Begleiter/FI" is companion-or-instructor with nothing in the file to say
	// which. Assuming instructor would mark the flight as training and change
	// what the logbook counts, so it is recorded as an ordinary second person.
	if role, ok := crew["Sam Okafor"]; !ok || role != "Passenger" {
		t.Errorf("crew = %v, want Sam Okafor as Passenger", crew)
	}
	if flight.InstructorName != nil {
		t.Errorf("InstructorName = %v, want nil — the column does not say it is an instructor",
			flight.InstructorName)
	}
}

// The extended export is the same list plus the block columns, and it carries
// two whole-minute durations that disagree: Flugzeit 95 airborne against
// Blockzeit 112 block. A logbook totals block time.
func TestMapRowToFlight_VereinsfliegerExtended(t *testing.T) {
	row := map[string]string{
		"Datum":                "14.03.2026",
		"Lfz.":                 "D-EABC",
		"Pilot":                "Rivera, Alex",
		"Begleiter/FI":         "",
		"Start":                "09:12",
		"Landung":              "10:47",
		"Flugzeit":             "95",
		"Startort":             "Uetersen EDHE",
		"Landeort":             "Stade EDHS",
		"Landungen":            "1",
		"Off-Block":            "09:04",
		"On-Block":             "10:56",
		"Blockzeit in Minuten": "112",
		"Flugart":              "N",
		"Bemerkung":            "",
		"Abr.":                 "K",
	}

	flight, errs := mapRowToFlight(row, mappingsFor("VEREINSFLIEGER_EXTENDED_CSV", headersOf(row)), nil)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if flight.OffBlockTime != "09:04:00" || flight.OnBlockTime != "10:56:00" {
		t.Errorf("block times = %q/%q, want 09:04:00/10:56:00",
			flight.OffBlockTime, flight.OnBlockTime)
	}
	if flight.DepartureTime == nil || *flight.DepartureTime != "09:12:00" {
		t.Errorf("DepartureTime = %v, want 09:12:00", flight.DepartureTime)
	}
	if flight.TotalTime == nil || *flight.TotalTime != 112 {
		t.Errorf("TotalTime = %v, want 112 (block) — 95 would be the airborne Flugzeit",
			flight.TotalTime)
	}
	if got := effectiveTotalMinutes(t, flight); got != 112 {
		t.Errorf("effective total = %d, want 112", got)
	}
}

// A LogTen Pro field-key export must map through the same path.
func TestMapRowToFlight_LogTenFieldKeys(t *testing.T) {
	row := map[string]string{
		"flight_flightDate":             "2026-03-07",
		"flight_selectedAircraftID":     "N778LT",
		"flight_from":                   "KSFO",
		"flight_to":                     "KLAX",
		"flight_totalTime":              "1.4",
		"flight_dualReceived":           "1.4",
		"flight_selectedCrewInstructor": "Sam Carter",
		"flight_dayLandings":            "1",
		"flight_remarks":                "Checkout",
	}
	flight, errs := mapRowToFlight(row, mappingsFor("LOGTEN_CSV", headersOf(row)), nil)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if flight.AircraftReg != "N778LT" {
		t.Errorf("AircraftReg = %q, want N778LT", flight.AircraftReg)
	}
	if flight.TotalTime == nil || *flight.TotalTime != 84 {
		t.Errorf("TotalTime = %v, want 84 minutes", flight.TotalTime)
	}
	if flight.InstructorName == nil || *flight.InstructorName != "Sam Carter" {
		t.Errorf("InstructorName = %v, want Sam Carter", flight.InstructorName)
	}
}

// The catalogue is served to the import screen, so every entry needs the fields
// the UI renders — and the IDs must be values the ImportFormat enum accepts,
// since the same string is written to the flight_imports row.
func TestImportTemplateCatalogueIsServable(t *testing.T) {
	valid := make(map[generated.ImportFormat]bool)
	for _, f := range []generated.ImportFormat{
		generated.CSV, generated.FOREFLIGHTCSV, generated.NINERLOGCSV, generated.LOGTENCSV,
		generated.MYFLIGHTBOOKCSV, generated.CAPZLOGCSV, generated.FLYLOGCSV, generated.WADERCSV,
		generated.VEREINSFLIEGERCSV, generated.VEREINSFLIEGEREXTENDEDCSV,
		generated.MCCPILOTLOGCSV, generated.SKYDEMONCSV,
		generated.EASACSV, generated.FAACSV, generated.XLS, generated.XLSX,
	} {
		valid[f] = true
	}

	templates := importtemplate.All()
	if len(templates) < 10 {
		t.Fatalf("catalogue has only %d templates", len(templates))
	}

	for _, tpl := range templates {
		got := toGeneratedTemplate(tpl)
		if !valid[got.Id] {
			t.Errorf("template %s: ID is not an ImportFormat enum value", tpl.ID)
		}
		if got.Name == "" || got.Description == "" {
			t.Errorf("template %s: missing name or description", tpl.ID)
		}
		if len(got.ExportSteps) == 0 {
			t.Errorf("template %s: no export instructions", tpl.ID)
		}
		if len(got.Regions) == 0 {
			t.Errorf("template %s: no regions", tpl.ID)
		}
		switch got.Confidence {
		case generated.Exact, generated.BestEffort:
		default:
			t.Errorf("template %s: bad confidence %q", tpl.ID, got.Confidence)
		}
		for _, step := range got.ExportSteps {
			if strings.TrimSpace(step) == "" {
				t.Errorf("template %s: blank export step", tpl.ID)
			}
		}
	}

	// The generic entry is catalogue-only: it must never be auto-detected, or
	// it would shadow every real template.
	for _, tpl := range templates {
		if tpl.ID == importtemplate.FormatGenericCSV && toGeneratedTemplate(tpl).AutoDetected {
			t.Error("generic CSV entry must not be marked auto-detected")
		}
	}
}

// SkyDemon has no date column: a flight is dated by its departure and arrival
// timestamps. Whether those carry a date is the open question — the export we
// have is of an empty logbook — so both shapes are pinned here. The first is
// what makes SkyDemon importable; the second documents exactly what breaks if
// SkyDemon writes bare clock times instead.
func TestMapRowToFlight_SkyDemonDatedByDepartureTimestamp(t *testing.T) {
	base := map[string]string{
		"Departure Place": "EGKA",
		"Arrival Place":   "EGHR",
		"Aircraft Reg":    "G-ABCD",
		"Aircraft Type":   "PA28",
		"Day Landings":    "1",
		"PIC Name":        "Alex Rivera",
		"Comments":        "Coastal hop",
	}

	t.Run("timestamped times date the flight", func(t *testing.T) {
		for _, stamp := range []struct{ name, dep, arr string }{
			{"ISO with seconds", "2026-08-11 11:03:00", "2026-08-11 12:04:00"},
			{"ISO without seconds", "2026-08-11 11:03", "2026-08-11 12:04"},
			{"ISO T separator", "2026-08-11T11:03:00Z", "2026-08-11T12:04:00Z"},
			// Unambiguous day-first (25 > 12, so it cannot be a month).
			// An ambiguous slash date like "11/08/2026" resolves MM/DD-first
			// by the repo's existing convention (see dateLayouts) — asserted
			// separately below, because for a genuinely day-first source that
			// is a silent off-by-months and we have no populated SkyDemon
			// export to say which convention it uses.
			{"day-first unambiguous", "25/08/2026 11:03", "25/08/2026 12:04"},
		} {
			t.Run(stamp.name, func(t *testing.T) {
				row := map[string]string{"Departure Time": stamp.dep, "Arrival Time": stamp.arr}
				for k, v := range base {
					row[k] = v
				}
				flight, errs := mapRowToFlight(row, mappingsFor("SKYDEMON_CSV", headersOf(row)), nil)
				if len(errs) > 0 {
					t.Fatalf("unexpected errors: %+v", errs)
				}
				wantDate := "2026-08-11"
				if strings.HasPrefix(stamp.dep, "25/") {
					wantDate = "2026-08-25"
				}
				if got := flight.Date.String(); got != wantDate {
					t.Errorf("date = %q, want %s derived from the departure timestamp", got, wantDate)
				}
				// The clock half must still land in the block times, or the
				// total cannot be derived — SkyDemon has no total column.
				if flight.OffBlockTime != "11:03:00" || flight.OnBlockTime != "12:04:00" {
					t.Errorf("block times = %q/%q, want 11:03:00/12:04:00",
						flight.OffBlockTime, flight.OnBlockTime)
				}
				if mins := effectiveTotalMinutes(t, flight); mins != 61 {
					t.Errorf("derived total = %d min, want 61", mins)
				}
			})
		}
	})

	// If SkyDemon writes a bare clock time, the file carries no date anywhere
	// and the row must fail on it rather than invent one.
	t.Run("bare clock times cannot date the flight", func(t *testing.T) {
		row := map[string]string{"Departure Time": "11:03", "Arrival Time": "12:04"}
		for k, v := range base {
			row[k] = v
		}
		_, errs := mapRowToFlight(row, mappingsFor("SKYDEMON_CSV", headersOf(row)), nil)
		var dateErr bool
		for _, e := range errs {
			if e.field == "date" {
				dateErr = true
			}
		}
		if !dateErr {
			t.Errorf("expected a date error, got %+v — a bare clock time carries no "+
				"date and must not be invented", errs)
		}
	})
}

// An ambiguous slash-separated date in a derived timestamp follows the same
// MM/DD-first convention as a mapped date column (dateLayouts). For a day-first
// source that is an off-by-months no validation can catch, so it is pinned here
// rather than left to be discovered in someone's logbook.
func TestCaptureDateFromTimestamp_AmbiguousSlashDateIsMonthFirst(t *testing.T) {
	var got time.Time
	captureDateFromTimestamp("11/08/2026 11:03", &got)
	if s := got.Format("2006-01-02"); s != "2026-11-08" {
		t.Errorf("11/08/2026 derived as %s, want 2026-11-08 (month-first, per dateLayouts)", s)
	}
}

// The derivation must never override a real date column, and must ignore
// values that are not timestamps.
func TestCaptureDateFromTimestamp(t *testing.T) {
	var got time.Time

	captureDateFromTimestamp("11:03", &got)
	if !got.IsZero() {
		t.Errorf("a bare clock time yielded %v, want nothing", got)
	}
	captureDateFromTimestamp("", &got)
	if !got.IsZero() {
		t.Errorf("an empty cell yielded %v, want nothing", got)
	}

	captureDateFromTimestamp("2026-08-11 11:03", &got)
	if got.Format("2006-01-02") != "2026-08-11" {
		t.Fatalf("got %v, want 2026-08-11", got)
	}
	// First usable value wins: a later column must not move the flight.
	captureDateFromTimestamp("2020-01-01 09:00", &got)
	if got.Format("2006-01-02") != "2026-08-11" {
		t.Errorf("a later timestamp overwrote the date: %v", got)
	}
}

// A mapped date column always wins — the derivation is a fallback, not a
// competitor.
func TestMapRowToFlight_ExplicitDateBeatsTimestampDerivation(t *testing.T) {
	row := map[string]string{
		"Date":       "2026-03-07",
		"AircraftID": "D-EABC",
		"From":       "EDDF",
		"To":         "EDDM",
		"TimeOut":    "2026-08-11 10:00",
		"TimeIn":     "2026-08-11 11:00",
	}
	flight, errs := mapRowToFlight(row, mappingsFor("FOREFLIGHT_CSV", headersOf(row)), nil)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if got := flight.Date.String(); got != "2026-03-07" {
		t.Errorf("date = %q, want the Date column's 2026-03-07", got)
	}
}

// SkyDemon writes its places as "ICAO Name". The code has to be extracted
// without consulting the airport database, because that data is fetched at
// startup and refreshed in the background: relying on it would make the same
// file import as "EDOI" on one instance and "EDOI Bienenfarm" on another, and
// the long form silently breaks night/solar, distance, cross-country and
// airport statistics, all of which need an exact match.
func TestNormalizeLocation_LeadingICAOCode(t *testing.T) {
	tests := map[string]string{
		"EDOI Bienenfarm":         "EDOI",
		"KSFO San Francisco Intl": "KSFO",
		"EDDF":                    "EDDF",
		"eddf":                    "EDDF",
		"  EGKA  ":                "EGKA",
		// Not an ICAO code: a named site whose first word happens to be four
		// letters. Capitalisation is what tells them apart.
		"Golf Course Strip": "Golf Course Strip",
		"Home Field":        "Home Field",
		// A bare name with no code stays intact rather than being truncated.
		"Bienenfarm": "Bienenfarm",
	}
	for in, want := range tests {
		if got := normalizeLocation(in); got != want {
			t.Errorf("normalizeLocation(%q) = %q, want %q", in, got, want)
		}
	}
}

// Vereinsflieger writes the same two tokens the other way round —
// "Uetersen EDHE" where SkyDemon writes "EDOI Bienenfarm" — so the reduction
// has to work from either end, and for the same reason: without it the value
// depends on whether the airport database happened to resolve the code, so the
// same file imports differently on two instances.
func TestNormalizeLocation_TrailingICAOCode(t *testing.T) {
	tests := map[string]string{
		"Uetersen EDHE":     "EDHE",
		"Stade EDHS":        "EDHS",
		"Bad Neuenahr EDRA": "EDRA",
		"Hartenholm EDHM":   "EDHM",
		// Lower case is a name, not a code — the same guard the leading form uses.
		"Landewiese hinten": "Landewiese hinten",
		"Feld beim Hof":     "Feld beim Hof",
		// One token only: neither pattern applies, and it is too long to be a code.
		"Bienenfarm": "Bienenfarm",
	}
	for in, want := range tests {
		if got := normalizeLocation(in); got != want {
			t.Errorf("normalizeLocation(%q) = %q, want %q", in, got, want)
		}
	}
}

// Contacts are deduplicated by name, so a logbook that writes "Rivera, Alex"
// where every other one writes "Alex Rivera" would give the same person two
// contact records and split the flights they flew together between them.
//
// The rewrite is narrow on purpose: a false positive mangles a name on every
// flight it appears on, and the crew column of an unknown format can hold
// anything.
func TestNormalizePersonName(t *testing.T) {
	tests := map[string]string{
		"Rivera, Alex":        "Alex Rivera",
		"van der Berg, Anna":  "Anna van der Berg",
		"Okafor, Sam Michael": "Sam Michael Okafor",
		"  Rivera ,  Alex  ":  "Alex Rivera",

		// Already in reading order, or a single name: untouched.
		"Alex Rivera": "Alex Rivera",
		"Alex":        "Alex",
		"":            "",

		// Not a name in inverted order. Each of these would be actively damaged
		// by the swap, which is why they are pinned rather than merely allowed.
		"Rivera, Jr.":                 "Rivera, Jr.",
		"Rivera, Alex, Okafor, Sam":   "Rivera, Alex, Okafor, Sam",
		"Cessna 172, right seat":      "Cessna 172, right seat",
		"Rivera, standing in for Sam": "Rivera, standing in for Sam",
		", Alex":                      ", Alex",
		"Rivera,":                     "Rivera,",
	}
	for in, want := range tests {
		if got := normalizePersonName(in); got != want {
			t.Errorf("normalizePersonName(%q) = %q, want %q", in, got, want)
		}
	}
}
