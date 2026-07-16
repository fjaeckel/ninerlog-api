// Package flightsearch implements the flight logbook search query language.
//
// A query is a boolean expression over tagged field conditions and free-text
// terms, e.g.:
//
//	departure:EDDF AND nightTime>0
//	(aircraft:D-EFGH OR aircraft:D-KXYZ) NOT remarks:checkride
//	isPic:true date:2024-05 crew:"John Doe"
//
// Every flight field is exposed as a tag. Queries are parsed into an AST
// (Parse) and compiled into a parameterized SQL fragment (Query.Compile)
// executed by PostgreSQL — no external search service is involved.
package flightsearch

// FieldType describes how a tag's values are parsed and compared.
type FieldType string

const (
	// FieldText matches strings. ':' is case-insensitive contains (with '*'
	// wildcard), '=' is case-insensitive equals, '!=' is its negation.
	FieldText FieldType = "text"
	// FieldDuration matches flight times stored as integer minutes. Values
	// accept minutes ("90"), H:MM ("1:30"), or decimal hours ("1.5h").
	FieldDuration FieldType = "duration"
	// FieldInt matches integer counters (landings, holds, ...).
	FieldInt FieldType = "int"
	// FieldNumber matches decimal values (distance).
	FieldNumber FieldType = "number"
	// FieldBool matches true/false (also yes/no/1/0).
	FieldBool FieldType = "bool"
	// FieldDate matches dates. Values may be YYYY, YYYY-MM, or YYYY-MM-DD;
	// partial dates expand to the containing range.
	FieldDate FieldType = "date"
	// FieldClock matches time-of-day columns using HH:MM or HH:MM:SS (UTC).
	FieldClock FieldType = "clock"
)

// Field describes one searchable tag.
type Field struct {
	// Name is the canonical tag, camelCased to match the API's JSON fields.
	Name string
	// Aliases are alternative tag spellings (lower-cased lookup).
	Aliases []string
	// Column is the SQL expression the tag compares against. Empty for
	// fields with fully custom SQL (crew, approachType, signed).
	Column string
	Type   FieldType
	// Description documents the tag for the OpenAPI spec and error messages.
	Description string
}

// Fields lists every searchable tag, in display order. All columns are on the
// flights table; text columns are wrapped in COALESCE so negations behave
// sensibly for NULLs.
var Fields = []Field{
	{Name: "date", Type: FieldDate, Column: "date", Description: "Flight date (YYYY, YYYY-MM or YYYY-MM-DD)"},
	{Name: "aircraftReg", Aliases: []string{"reg", "registration", "aircraft"}, Type: FieldText, Column: "COALESCE(aircraft_reg, '')", Description: "Aircraft registration"},
	{Name: "aircraftType", Aliases: []string{"type", "model"}, Type: FieldText, Column: "COALESCE(aircraft_type, '')", Description: "Aircraft type designator"},
	{Name: "departureIcao", Aliases: []string{"departure", "from", "dep"}, Type: FieldText, Column: "COALESCE(departure_icao, '')", Description: "Departure airport ICAO code"},
	{Name: "arrivalIcao", Aliases: []string{"arrival", "to", "arr"}, Type: FieldText, Column: "COALESCE(arrival_icao, '')", Description: "Arrival airport ICAO code"},
	{Name: "route", Type: FieldText, Column: "COALESCE(route, '')", Description: "Route waypoints"},
	{Name: "remarks", Aliases: []string{"comments"}, Type: FieldText, Column: "COALESCE(remarks, '')", Description: "Remarks"},
	{Name: "offBlockTime", Aliases: []string{"offblock"}, Type: FieldClock, Column: "off_block_time", Description: "Off-block time (UTC, HH:MM)"},
	{Name: "onBlockTime", Aliases: []string{"onblock"}, Type: FieldClock, Column: "on_block_time", Description: "On-block time (UTC, HH:MM)"},
	{Name: "departureTime", Aliases: []string{"takeofftime"}, Type: FieldClock, Column: "departure_time", Description: "Takeoff time (UTC, HH:MM)"},
	{Name: "arrivalTime", Aliases: []string{"landingtime"}, Type: FieldClock, Column: "arrival_time", Description: "Landing time (UTC, HH:MM)"},
	{Name: "totalTime", Aliases: []string{"total"}, Type: FieldDuration, Column: "total_time", Description: "Total block time"},
	{Name: "picTime", Type: FieldDuration, Column: "pic_time", Description: "PIC time"},
	{Name: "dualTime", Type: FieldDuration, Column: "dual_time", Description: "Dual received time"},
	{Name: "nightTime", Aliases: []string{"night"}, Type: FieldDuration, Column: "night_time", Description: "Night time"},
	{Name: "ifrTime", Aliases: []string{"ifr"}, Type: FieldDuration, Column: "ifr_time", Description: "IFR time"},
	{Name: "soloTime", Aliases: []string{"solo"}, Type: FieldDuration, Column: "solo_time", Description: "Solo time"},
	{Name: "crossCountryTime", Aliases: []string{"xc", "crosscountry"}, Type: FieldDuration, Column: "cross_country_time", Description: "Cross-country time"},
	{Name: "sicTime", Type: FieldDuration, Column: "sic_time", Description: "SIC time"},
	{Name: "dualGivenTime", Aliases: []string{"dualgiven"}, Type: FieldDuration, Column: "dual_given_time", Description: "Dual given (instruction) time"},
	{Name: "simulatedFlightTime", Aliases: []string{"simtime"}, Type: FieldDuration, Column: "simulated_flight_time", Description: "FSTD / simulator time"},
	{Name: "groundTrainingTime", Type: FieldDuration, Column: "ground_training_time", Description: "Ground training time"},
	{Name: "actualInstrumentTime", Type: FieldDuration, Column: "actual_instrument_time", Description: "Actual instrument time"},
	{Name: "simulatedInstrumentTime", Aliases: []string{"hoodtime"}, Type: FieldDuration, Column: "simulated_instrument_time", Description: "Simulated instrument (hood) time"},
	{Name: "multiPilotTime", Type: FieldDuration, Column: "multi_pilot_time", Description: "Multi-pilot time"},
	{Name: "landings", Type: FieldInt, Column: "all_landings", Description: "Total landings"},
	{Name: "landingsDay", Type: FieldInt, Column: "landings_day", Description: "Day landings"},
	{Name: "landingsNight", Type: FieldInt, Column: "landings_night", Description: "Night landings"},
	{Name: "takeoffsDay", Type: FieldInt, Column: "takeoffs_day", Description: "Day takeoffs"},
	{Name: "takeoffsNight", Type: FieldInt, Column: "takeoffs_night", Description: "Night takeoffs"},
	{Name: "holds", Type: FieldInt, Column: "holds", Description: "Holding procedures"},
	{Name: "approaches", Type: FieldInt, Column: "approaches_count", Description: "Instrument approach count"},
	{Name: "approachType", Type: FieldText, Description: "Instrument approach type (ILS, RNAV/GPS, VOR, ...)"},
	{Name: "distance", Type: FieldNumber, Column: "distance", Description: "Great-circle distance (NM)"},
	{Name: "isPic", Type: FieldBool, Column: "is_pic", Description: "Logged as PIC"},
	{Name: "isDual", Type: FieldBool, Column: "is_dual", Description: "Logged as dual received"},
	{Name: "isIpc", Aliases: []string{"ipc"}, Type: FieldBool, Column: "is_ipc", Description: "Instrument proficiency check"},
	{Name: "isFlightReview", Aliases: []string{"flightreview", "bfr"}, Type: FieldBool, Column: "is_flight_review", Description: "Flight review (BFR)"},
	{Name: "isProficiencyCheck", Aliases: []string{"proficiencycheck"}, Type: FieldBool, Column: "is_proficiency_check", Description: "Proficiency check"},
	{Name: "signed", Type: FieldBool, Description: "Locked by a completed instructor signature"},
	{Name: "instructorName", Aliases: []string{"instructor"}, Type: FieldText, Column: "COALESCE(instructor_name, '')", Description: "Instructor name"},
	{Name: "instructorComments", Type: FieldText, Column: "COALESCE(instructor_comments, '')", Description: "Instructor comments"},
	{Name: "picName", Type: FieldText, Column: "COALESCE(pic_name, '')", Description: "PIC name (logbook column 12)"},
	{Name: "crew", Type: FieldText, Description: "Crew member name"},
	{Name: "fstdType", Aliases: []string{"fstd"}, Type: FieldText, Column: "COALESCE(fstd_type, '')", Description: "FSTD type designation"},
	{Name: "endorsements", Type: FieldText, Column: "COALESCE(endorsements, '')", Description: "Endorsements"},
	{Name: "launchMethod", Aliases: []string{"launch"}, Type: FieldText, Column: "COALESCE(launch_method, '')", Description: "Glider launch method (winch, aerotow, self-launch)"},
	{Name: "createdAt", Type: FieldDate, Column: "created_at", Description: "Entry creation date"},
	{Name: "updatedAt", Type: FieldDate, Column: "updated_at", Description: "Entry last-modified date"},
}

// freeTextColumns are searched by bare (untagged) terms. Superset of the
// legacy `search` parameter's columns, so `q=EDDF` finds at least everything
// `search=EDDF` found.
var freeTextColumns = []string{
	"COALESCE(aircraft_reg, '')",
	"COALESCE(aircraft_type, '')",
	"COALESCE(departure_icao, '')",
	"COALESCE(arrival_icao, '')",
	"COALESCE(route, '')",
	"COALESCE(remarks, '')",
	"COALESCE(instructor_name, '')",
	"COALESCE(pic_name, '')",
}

// fieldIndex maps lower-cased names and aliases to their Field.
var fieldIndex = func() map[string]*Field {
	idx := make(map[string]*Field)
	for i := range Fields {
		f := &Fields[i]
		idx[lower(f.Name)] = f
		for _, a := range f.Aliases {
			idx[lower(a)] = f
		}
	}
	return idx
}()

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// LookupField resolves a tag name (case-insensitive, alias-aware).
func LookupField(name string) (*Field, bool) {
	f, ok := fieldIndex[lower(name)]
	return f, ok
}
