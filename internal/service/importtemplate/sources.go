package importtemplate

// This file is the catalogue. Everything here is data — adding support for
// another logbook means adding a Template below, an ImportFormat enum member in
// api-spec/openapi.yaml, and the matching value in the import_format database
// enum. No handler or service code changes.
//
// Two conventions matter when adding one:
//
//   - Column keys must already be normalised (lower case, single spaces). They
//     are looked up verbatim, and a stray capital silently disables the alias.
//   - Signature entries must be spellings the source is close to alone in
//     using. "date" or "total time" as a signature would make every template
//     match every file.
//
// Column tables list known-but-unmappable columns as FieldIgnore rather than
// omitting them: they still count towards recognition, but are skipped when
// suggesting mappings.

// FormatGenericCSV is the import_format recorded when no template matched.
const FormatGenericCSV = "CSV"

// ─── Shared column tables ────────────────────────────────────────────────────

// coreColumns are spellings that mean the same thing in every logbook we know
// of. They are safe to merge into any template.
var coreColumns = map[string]Field{
	"date":           FieldDate,
	"flight date":    FieldDate,
	"date of flight": FieldDate,

	"route":   FieldRoute,
	"remarks": FieldRemarks,
	"notes":   FieldRemarks,
	"comment": FieldRemarks,

	"holds": FieldHolds,
	"hold":  FieldHolds,
}

// easaColumns is the AMC1 FCL.050 logbook layout. It is the lingua franca of
// European logbook apps, and the exact header row NinerLog's own EASA CSV
// export writes.
//
// Note the two senses of "instructor" across formats: in the EASA table it is a
// duration (instructor time given), whereas US-oriented apps use it for the
// instructor's name. Here it is a duration — which is why the alias tables are
// per-template rather than one global guess table.
var easaColumns = map[string]Field{
	"dep place":           FieldDepartureIcao,
	"departure place":     FieldDepartureIcao,
	"place of departure":  FieldDepartureIcao,
	"departure airport":   FieldDepartureIcao,
	"departure aerodrome": FieldDepartureIcao,
	"from":                FieldDepartureIcao,

	"arr place":         FieldArrivalIcao,
	"arrival place":     FieldArrivalIcao,
	"place of arrival":  FieldArrivalIcao,
	"arrival airport":   FieldArrivalIcao,
	"arrival aerodrome": FieldArrivalIcao,
	"destination":       FieldArrivalIcao,
	"to":                FieldArrivalIcao,

	"dep time":          FieldOffBlockTime,
	"departure time":    FieldOffBlockTime,
	"time of departure": FieldOffBlockTime,
	"off block":         FieldOffBlockTime,
	"off-block":         FieldOffBlockTime,
	"off block time":    FieldOffBlockTime,
	"out":               FieldOffBlockTime,

	"arr time":        FieldOnBlockTime,
	"arrival time":    FieldOnBlockTime,
	"time of arrival": FieldOnBlockTime,
	"on block":        FieldOnBlockTime,
	"on-block":        FieldOnBlockTime,
	"on block time":   FieldOnBlockTime,
	"in":              FieldOnBlockTime,

	"a/c type":       FieldAircraftType,
	"aircraft type":  FieldAircraftType,
	"aircraft model": FieldAircraftType,
	"type":           FieldAircraftType,
	"model":          FieldAircraftType,

	"a/c reg":               FieldAircraftReg,
	"aircraft reg":          FieldAircraftReg,
	"aircraft registration": FieldAircraftReg,
	"registration":          FieldAircraftReg,
	"reg":                   FieldAircraftReg,
	"call sign":             FieldAircraftReg,
	"callsign":              FieldAircraftReg,

	"total time":           FieldTotalTime,
	"total time of flight": FieldTotalTime,
	"total flight time":    FieldTotalTime,
	"block time":           FieldTotalTime,
	"total":                FieldTotalTime,

	"pic name":         FieldPerson1,
	"name pic":         FieldPerson1,
	"name(s) pic":      FieldPerson1,
	"pilot in command": FieldPerson1,
	"commander":        FieldPerson1,

	"ldg day":      FieldLandingsDay,
	"landings day": FieldLandingsDay,
	"day landings": FieldLandingsDay,
	"landing day":  FieldLandingsDay,

	"ldg night":      FieldLandingsNight,
	"landings night": FieldLandingsNight,
	"night landings": FieldLandingsNight,
	"landing night":  FieldLandingsNight,

	"landings": FieldLandingsTotal,

	"night":      FieldNightTime,
	"night time": FieldNightTime,
	"ifr":        FieldIFRTime,
	"ifr time":   FieldIFRTime,
	"instrument": FieldIFRTime,

	"pic":       FieldIsPic,
	"pic time":  FieldIsPic,
	"dual":      FieldIsDual,
	"dual time": FieldIsDual,

	"instructor":      FieldDualGivenTime,
	"instructor time": FieldDualGivenTime,
	"fi":              FieldDualGivenTime,

	"remarks & endorsements":   FieldRemarks,
	"remarks and endorsements": FieldRemarks,

	// Present in the EASA table, but with no NinerLog import field: recognised
	// for scoring, skipped when mapping.
	"sp-se":           FieldIgnore,
	"sp se":           FieldIgnore,
	"single pilot se": FieldIgnore,
	"sp-me":           FieldIgnore,
	"sp me":           FieldIgnore,
	"single pilot me": FieldIgnore,
	"multi-pilot":     FieldIgnore,
	"multi pilot":     FieldIgnore,
	"multipilottime":  FieldIgnore,
	"co-pilot":        FieldIgnore,
	"co pilot":        FieldIgnore,
	"copilot":         FieldIgnore,
	"sic":             FieldIgnore,
	"fstd date":       FieldIgnore,
	"fstd type":       FieldIgnore,
	"fstd time":       FieldIgnore,
	"fstd total time": FieldIgnore,
}

// faaColumns is the ASA/Jeppesen-style US layout, and the header row NinerLog's
// own FAA CSV export writes.
var faaColumns = map[string]Field{
	"a/c type":       FieldAircraftType,
	"a/c ident":      FieldAircraftReg,
	"aircraft ident": FieldAircraftReg,
	"ident":          FieldAircraftReg,
	"tail":           FieldAircraftReg,
	"tail number":    FieldAircraftReg,

	"from": FieldDepartureIcao,
	"to":   FieldArrivalIcao,

	"solo":          FieldIgnore,
	"pic":           FieldIsPic,
	"sic":           FieldIgnore,
	"xc":            FieldIgnore,
	"cross country": FieldIgnore,

	"dual rcvd":         FieldIsDual,
	"dual received":     FieldIsDual,
	"instr given":       FieldDualGivenTime,
	"dual given":        FieldDualGivenTime,
	"instruction given": FieldDualGivenTime,

	"actual inst":          FieldActualInstrumentTime,
	"actual instrument":    FieldActualInstrumentTime,
	"sim inst":             FieldSimulatedInstrumentTime,
	"simulated instrument": FieldSimulatedInstrumentTime,
	"hood":                 FieldSimulatedInstrumentTime,

	"night":      FieldNightTime,
	"day ldg":    FieldLandingsDay,
	"night ldg":  FieldLandingsNight,
	"approaches": FieldApproachesCount,
	"total":      FieldTotalTime,

	"remarks/endorsements":     FieldRemarks,
	"remarks & endorsements":   FieldRemarks,
	"remarks and endorsements": FieldRemarks,
}

// germanColumns covers German-language exports. Club software in the
// German-speaking countries writes these headers, and no English-language
// template shares them, which makes them strong signatures.
var germanColumns = map[string]Field{
	"datum":        FieldDate,
	"flugdatum":    FieldDate,
	"kennzeichen":  FieldAircraftReg,
	"luftfahrzeug": FieldAircraftReg,
	"muster":       FieldAircraftType,
	"flugzeugtyp":  FieldAircraftType,
	"flugzeug":     FieldAircraftType,
	"typ":          FieldAircraftType,

	"startort":    FieldDepartureIcao,
	"abflugort":   FieldDepartureIcao,
	"startplatz":  FieldDepartureIcao,
	"von":         FieldDepartureIcao,
	"zielort":     FieldArrivalIcao,
	"landeort":    FieldArrivalIcao,
	"ankunftsort": FieldArrivalIcao,
	"landeplatz":  FieldArrivalIcao,
	"nach":        FieldArrivalIcao,

	"startzeit":    FieldOffBlockTime,
	"abflugzeit":   FieldOffBlockTime,
	"landezeit":    FieldOnBlockTime,
	"ankunftszeit": FieldOnBlockTime,

	"flugzeit":   FieldTotalTime,
	"flugdauer":  FieldTotalTime,
	"blockzeit":  FieldTotalTime,
	"gesamtzeit": FieldTotalTime,

	"pilot":          FieldPerson1,
	"pilot 1":        FieldPerson1,
	"pilot1":         FieldPerson1,
	"flugzeugführer": FieldPerson1,
	"begleiter":      FieldPerson2,
	"pilot 2":        FieldPerson2,
	"pilot2":         FieldPerson2,
	"copilot":        FieldPerson2,
	"fluglehrer":     FieldInstructorName,
	"lehrer":         FieldInstructorName,

	"landungen":   FieldLandingsTotal,
	"nachtflug":   FieldNightTime,
	"nachtzeit":   FieldNightTime,
	"bemerkung":   FieldRemarks,
	"bemerkungen": FieldRemarks,
	"notizen":     FieldRemarks,

	"startart":      FieldIgnore,
	"flugart":       FieldIgnore,
	"motorzeit":     FieldIgnore,
	"motorlaufzeit": FieldIgnore,
	"gastflug":      FieldIgnore,
	"verein":        FieldIgnore,
}

// ─── Templates ───────────────────────────────────────────────────────────────

var foreFlightTemplate = register(&Template{
	ID:          "FOREFLIGHT_CSV",
	Name:        "ForeFlight",
	Vendor:      "ForeFlight (Boeing)",
	Website:     "https://foreflight.com",
	Description: "ForeFlight Logbook export. Carries a separate Aircraft Table, so your fleet — make, model and class — is created alongside the flights.",
	Confidence:  ConfidenceExact,
	Regions:     []string{"FAA", "EASA"},
	ExportSteps: []string{
		"Open ForeFlight on iPad or iPhone and go to Logbook.",
		"Tap the gear icon, then Export Logbook.",
		"Mail the file to yourself and save the attached .csv.",
		"Upload that file here — the Aircraft Table is imported too.",
	},
	DateFormat: "2006-01-02",
	Columns: merge(coreColumns, map[string]Field{
		"date":       FieldDate,
		"aircraftid": FieldAircraftReg,
		"from":       FieldDepartureIcao,
		"to":         FieldArrivalIcao,
		"route":      FieldRoute,
		"timeout":    FieldOffBlockTime,
		"timein":     FieldOnBlockTime,
		"timeoff":    FieldDepartureTime,
		"timeon":     FieldArrivalTime,
		"totaltime":  FieldTotalTime,
		"pic":        FieldIsPic,
		"night":      FieldNightTime,

		"dualreceived":          FieldIsDual,
		"dualgiven":             FieldDualGivenTime,
		"actualinstrument":      FieldActualInstrumentTime,
		"simulatedinstrument":   FieldSimulatedInstrumentTime,
		"daylandingsfullstop":   FieldLandingsDay,
		"nightlandingsfullstop": FieldLandingsNight,
		"alllandings":           FieldLandingsTotal,
		"holds":                 FieldHolds,
		"flightreview":          FieldIsFlightReview,
		"ipc":                   FieldIsIpc,
		"pilotcomments":         FieldRemarks,
		"instructorname":        FieldInstructorName,
		"instructorcomments":    FieldInstructorComments,

		"person1": FieldPerson1,
		"person2": FieldPerson2,
		"person3": FieldPerson3,
		"person4": FieldPerson4,
		"person5": FieldPerson5,
		"person6": FieldPerson6,

		"sic":             FieldIgnore,
		"solo":            FieldIgnore,
		"crosscountry":    FieldIgnore,
		"distance":        FieldIgnore,
		"daytakeoffs":     FieldIgnore,
		"nighttakeoffs":   FieldIgnore,
		"hobbsstart":      FieldIgnore,
		"hobbsend":        FieldIgnore,
		"tachstart":       FieldIgnore,
		"tachend":         FieldIgnore,
		"onduty":          FieldIgnore,
		"offduty":         FieldIgnore,
		"nvg":             FieldIgnore,
		"nvg ops":         FieldIgnore,
		"simulatedflight": FieldIgnore,
		"groundtraining":  FieldIgnore,
		"checkride":       FieldIgnore,
		"faa6158":         FieldIgnore,
		// Approach1–6 are parsed structurally by the importer, not via a mapping.
		"approach1": FieldIgnore,
		"approach2": FieldIgnore,
		"approach3": FieldIgnore,
		"approach4": FieldIgnore,
		"approach5": FieldIgnore,
		"approach6": FieldIgnore,
	}),
	Signature: []string{
		"aircraftid", "timeout", "timein", "daylandingsfullstop",
		"nightlandingsfullstop", "pilotcomments", "alllandings", "person1",
	},
	MinSignatureHits: 3,
	Priority:         10,
})

var ninerlogTemplate = register(&Template{
	ID:          "NINERLOG_CSV",
	Name:        "NinerLog",
	Vendor:      "NinerLog",
	Website:     "https://ninerlog.com",
	Description: "A CSV written by NinerLog's own Export screen. Re-importing one round-trips cleanly, including instructor, approach and endorsement columns.",
	Confidence:  ConfidenceExact,
	Regions:     []string{"EASA", "FAA"},
	ExportSteps: []string{
		"Open Export in another NinerLog account or installation.",
		"Choose CSV and the Standard column layout.",
		"Upload the downloaded file here.",
		"To move an entire account — aircraft, licences and credentials as well as flights — use Restore JSON Backup instead.",
	},
	DateFormat: "2006-01-02",
	Columns: merge(coreColumns, foreFlightTemplate.Columns, map[string]Field{
		"aircrafttype":    FieldAircraftType,
		"ifrtime":         FieldIFRTime,
		"approachescount": FieldApproachesCount,
		"picname":         FieldPerson1,
		"endorsements":    FieldRemarks,
		"multipilottime":  FieldIgnore,
		"fstdtype":        FieldIgnore,
	}),
	Signature: []string{
		"aircrafttype", "ifrtime", "approachescount", "picname",
		"multipilottime", "fstdtype", "endorsements",
	},
	MinSignatureHits: 3,
	Priority:         5,
})

var logTenTemplate = register(&Template{
	ID:          "LOGTEN_CSV",
	Name:        "LogTen Pro",
	Vendor:      "Coradine Aviation",
	Website:     "https://logtenpro.com",
	Description: "LogTen Pro flight export — the Dynamic Export column set and the field-key (flight_…) dialect. Times are H:MM or bare four-digit clock times.",
	Confidence:  ConfidenceExact,
	Regions:     []string{"FAA", "EASA"},
	ExportSteps: []string{
		"Open LogTen Pro on Mac or iPad.",
		"Go to Reports → Exporters and export your flights (Dynamic Export or Export Flights, tab or CSV).",
		"Save the file — a .txt from a tab export is fine.",
		"Upload it here.",
	},
	DateFormat: "2006-01-02",
	Columns: merge(coreColumns, map[string]Field{
		// Dynamic Export dialect, from a real export.
		//
		// LogTen writes the FAA short spellings — "Dual Rcvd", "Sim Inst",
		// "Actual Inst", "Day Ldg", "Night Ldg", "XC" — which are exactly
		// FAA_CSV's signature columns. Detection therefore cannot lean on
		// those; see this template's Signature for the columns LogTen is
		// alone in writing.
		"aircraft id":   FieldAircraftReg,
		"aircraft type": FieldAircraftType,
		"from":          FieldDepartureIcao,
		"to":            FieldArrivalIcao,
		"total time":    FieldTotalTime,
		"night":         FieldNightTime,
		"actual inst":   FieldActualInstrumentTime,
		"sim inst":      FieldSimulatedInstrumentTime,
		"day ldg":       FieldLandingsDay,
		"night ldg":     FieldLandingsNight,
		"dual rcvd":     FieldIsDual,
		"flight review": FieldIsFlightReview,

		// These hold names, unlike the EASA layout where "Instructor" is a
		// duration.
		"pic/p1 crew": FieldPerson1,
		"student":     FieldPerson2,
		"instructor":  FieldInstructorName,

		"day t/o":    FieldIgnore,
		"night t/o":  FieldIgnore,
		"approach 1": FieldIgnore,
		"approach 2": FieldIgnore,
		"approach 3": FieldIgnore,
		"approach 4": FieldIgnore,
		"approach 5": FieldIgnore,
		"approach 6": FieldIgnore,
		"simulator":  FieldIgnore,
		"ground":     FieldIgnore,
		"xc":         FieldIgnore,

		// Longer spellings LogTen's other exporters emit.
		"actual instrument":        FieldActualInstrumentTime,
		"simulated instrument":     FieldSimulatedInstrumentTime,
		"day landings":             FieldLandingsDay,
		"night landings":           FieldLandingsNight,
		"dual given":               FieldDualGivenTime,
		"dual received":            FieldIsDual,
		"approaches":               FieldApproachesCount,
		"out":                      FieldOffBlockTime,
		"in":                       FieldOnBlockTime,
		"off":                      FieldDepartureTime,
		"on":                       FieldArrivalTime,
		"selected crew pic":        FieldPerson1,
		"selected crew sic":        FieldPerson2,
		"selected crew instructor": FieldInstructorName,
		"selected crew student":    FieldPerson2,
		"pic":                      FieldIsPic,
		"sic":                      FieldIgnore,
		"solo":                     FieldIgnore,
		"cross country":            FieldIgnore,
		"day takeoffs":             FieldIgnore,
		"night takeoffs":           FieldIgnore,

		// Field-key dialect.
		"flight_flightdate":                 FieldDate,
		"flight_from":                       FieldDepartureIcao,
		"flight_to":                         FieldArrivalIcao,
		"flight_route":                      FieldRoute,
		"flight_totaltime":                  FieldTotalTime,
		"flight_night":                      FieldNightTime,
		"flight_actualinstrument":           FieldActualInstrumentTime,
		"flight_simulatedinstrument":        FieldSimulatedInstrumentTime,
		"flight_daylandings":                FieldLandingsDay,
		"flight_nightlandings":              FieldLandingsNight,
		"flight_holds":                      FieldHolds,
		"flight_dualgiven":                  FieldDualGivenTime,
		"flight_dualreceived":               FieldIsDual,
		"flight_remarks":                    FieldRemarks,
		"flight_actualdeparturetime":        FieldOffBlockTime,
		"flight_actualarrivaltime":          FieldOnBlockTime,
		"flight_takeofftime":                FieldDepartureTime,
		"flight_landingtime":                FieldArrivalTime,
		"flight_selectedcrewpic":            FieldPerson1,
		"flight_selectedcrewsic":            FieldPerson2,
		"flight_selectedcrewinstructor":     FieldInstructorName,
		"flight_selectedcrewstudent":        FieldPerson2,
		"flight_selectedaircraftid":         FieldAircraftReg,
		"aircraft_aircraftid":               FieldAircraftReg,
		"aircraft_type":                     FieldAircraftType,
		"flight_flightreview":               FieldIsFlightReview,
		"flight_instrumentproficiencycheck": FieldIsIpc,
		"flight_pic":                        FieldIsPic,
		"flight_sic":                        FieldIgnore,
		"flight_solo":                       FieldIgnore,
		"flight_crosscountry":               FieldIgnore,
	}),
	// Only spellings LogTen is alone in using. The obvious candidates —
	// "dual rcvd", "sim inst", "day ldg" — are shared verbatim with the FAA
	// layout, and leaning on those let FAA_CSV claim a LogTen file and import
	// it with no registration, no aircraft type, no times and no crew.
	Signature: []string{
		"flight_flightdate", "flight_totaltime", "flight_selectedcrewpic",
		"flight_selectedaircraftid", "flight_dualreceived", "aircraft_aircraftid",
		"selected crew pic", "selected crew sic", "selected crew instructor",
		"pic/p1 crew", "day t/o", "night t/o", "approach 1", "approach 2",
	},
	MinSignatureHits: 2,
	Priority:         10,
})

// myFlightbookTemplate is built from a real MyFlightbook export.
//
// Two columns need care. "Model" is a marketing description
// ("C-172 S, Cessna Skyhawk SP") while "ICAO Model" is the type code ("C172"),
// so the type must come from the latter — mapping "Model" put a sentence in the
// aircraft type and created a fleet entry to match. And "Aircraft ID" is
// MyFlightbook's internal numeric row ID, not a registration, despite being the
// spelling LogTen Pro uses for exactly that.
//
// MyFlightbook also has no departure/arrival columns: the sector lives in
// "Route", and the importer derives the airports from its first and last
// waypoint. A row with an empty Route therefore cannot become a flight, because
// FlightCreate requires both airports.
var myFlightbookTemplate = register(&Template{
	ID:          "MYFLIGHTBOOK_CSV",
	Name:        "MyFlightbook",
	Vendor:      "MyFlightbook",
	Website:     "https://myflightbook.com",
	Description: "MyFlightbook CSV export. It records the route as a single field rather than separate airports, so departure and arrival are taken from the first and last waypoint — a flight logged with an empty Route cannot be imported.",
	Confidence:  ConfidenceExact,
	Regions:     []string{"FAA"},
	ExportSteps: []string{
		"Sign in at myflightbook.com.",
		"Go to Logbook → Import/Export (or Profile → Download your logbook).",
		"Download the CSV of all flights.",
		"Upload it here.",
	},
	DateFormat: "2006-01-02",
	Columns: merge(coreColumns, map[string]Field{
		"tail number":          FieldAircraftReg,
		"icao model":           FieldAircraftType,
		"route":                FieldRoute,
		"comments":             FieldRemarks,
		"approaches":           FieldApproachesCount,
		"hold":                 FieldHolds,
		"landings":             FieldLandingsTotal,
		"fs day landings":      FieldLandingsDay,
		"fs night landings":    FieldLandingsNight,
		"night":                FieldNightTime,
		"imc":                  FieldActualInstrumentTime,
		"simulated instrument": FieldSimulatedInstrumentTime,
		"dual received":        FieldIsDual,
		"cfi":                  FieldDualGivenTime,
		"total flight time":    FieldTotalTime,
		"engine start":         FieldOffBlockTime,
		"engine end":           FieldOnBlockTime,
		"flight start":         FieldDepartureTime,
		"flight end":           FieldArrivalTime,
		"cfi name":             FieldInstructorName,
		"cfi comment":          FieldInstructorComments,

		// "Model" is the marketing description, not the type code — see above.
		"model":        FieldIgnore,
		"display tail": FieldIgnore,
		"aircraft id":  FieldIgnore,

		// HH:MM duplicates of the decimal-hour columns above.
		"cfi time (hh:mm)":          FieldIgnore,
		"sic time (hh:mm)":          FieldIgnore,
		"pic (hh:mm)":               FieldIgnore,
		"total flight time (hh:mm)": FieldIgnore,

		"flight id":               FieldIgnore,
		"pic":                     FieldIsPic,
		"sic":                     FieldIgnore,
		"x-country":               FieldIgnore,
		"ground simulator":        FieldIgnore,
		"category/class":          FieldIgnore,
		"hobbs start":             FieldIgnore,
		"hobbs end":               FieldIgnore,
		"engine time":             FieldIgnore,
		"flying time":             FieldIgnore,
		"flight properties":       FieldIgnore,
		"complex":                 FieldIgnore,
		"controllable pitch prop": FieldIgnore,
		"flaps":                   FieldIgnore,
		"retract":                 FieldIgnore,
		"tailwheel":               FieldIgnore,
		"high performance":        FieldIgnore,
		"turbine":                 FieldIgnore,
		"taa":                     FieldIgnore,
		"signature state":         FieldIgnore,
		"date of signature":       FieldIgnore,
		"cfi certificate":         FieldIgnore,
		"cfi email":               FieldIgnore,
		"cfi expiration":          FieldIgnore,
		"public":                  FieldIgnore,
	}),
	Signature: []string{
		"tail number", "total flight time", "ground simulator", "x-country",
		"fs day landings", "fs night landings", "imc", "cfi", "flight id",
		"icao model", "display tail", "signature state", "flight properties",
	},
	MinSignatureHits: 3,
	Priority:         10,
})

// capzlogTemplate is built from a real capzlog.aero export.
//
// It shares SkyDemon's structural surprise — there is no date column, and the
// flight is dated by the "Off Block" timestamp — and adds its own vocabulary:
// "Departure"/"Arrival" are places rather than times, "Copi" is the co-pilot,
// "Remark" is singular, and the sheet carries Swiss and rotary-specific columns
// (mountain and glacier landings, HESLO/HEC/HHO external-load and hoist cycles)
// that no other logbook in the catalogue writes. Those make it unmistakable.
//
// One caveat worth knowing. The export seen here dates its timestamps
// month-first ("8/15/2026 04:00"), which is only provable because 15 cannot be
// a month. If capzlog follows the user's locale, a European export would be
// day-first and an ambiguous date like "8/9/2026" would resolve to the wrong
// month — silently. See TestCaptureDateFromTimestamp_AmbiguousSlashDateIsMonthFirst.
var capzlogTemplate = register(&Template{
	ID:          "CAPZLOG_CSV",
	Name:        "capzlog.aero",
	Vendor:      "Aviaso / capzlog.aero",
	Website:     "https://capzlog.aero",
	Description: "capzlog.aero flights report. Dates each flight by its off-block timestamp rather than a date column, and carries the Swiss mountain/glacier and rotary external-load columns alongside the standard EASA breakdown.",
	Confidence:  ConfidenceExact,
	Regions:     []string{"EASA"},
	ExportSteps: []string{
		"Sign in at capzlog.aero and open your Flights list.",
		"Apply any filter you want the export limited to (or none for everything).",
		"Choose Export and pick CSV rather than PDF.",
		"Upload the downloaded file here.",
	},
	DateFormat: "1/2/2006",
	Columns: map[string]Field{
		"departure": FieldDepartureIcao,
		"arrival":   FieldArrivalIcao,
		"off block": FieldOffBlockTime,
		"on block":  FieldOnBlockTime,
		"takeoff":   FieldDepartureTime,
		"landing":   FieldArrivalTime,
		"block":     FieldTotalTime,
		"aircraft":  FieldAircraftReg,
		"model":     FieldAircraftType,
		"pic name":  FieldPerson1,

		"ifr":              FieldIFRTime,
		"night":            FieldNightTime,
		"pic":              FieldIsPic,
		"dual":             FieldIsDual,
		"instructor":       FieldDualGivenTime,
		"landings":         FieldLandingsTotal,
		"day landings":     FieldLandingsDay,
		"night landings":   FieldLandingsNight,
		"holding patterns": FieldHolds,
		"remark":           FieldRemarks,

		// Recognised for scoring, not mapped.
		"airborne":       FieldIgnore,
		"single engine":  FieldIgnore,
		"multi engine":   FieldIgnore,
		"multi pilot":    FieldIgnore,
		"type of flight": FieldIgnore,
		"vfr":            FieldIgnore,
		"day":            FieldIgnore,
		"pilot function": FieldIgnore,
		"copi":           FieldIgnore,
		// InstructionTime duplicates the "Instructor" duration above.
		"instructiontime":           FieldIgnore,
		"mountain landings":         FieldIgnore,
		"mountain takeoffs":         FieldIgnore,
		"mountain landings > 2000m": FieldIgnore,
		"mountain landings > 2700m": FieldIgnore,
		"glacier landings":          FieldIgnore,
		"go arounds":                FieldIgnore,
		"touch and goes":            FieldIgnore,
		"number of pax":             FieldIgnore,
		"sea takeoffs":              FieldIgnore,
		"sea landings":              FieldIgnore,
		"heslo1 cycles":             FieldIgnore,
		"heslo2 cycles":             FieldIgnore,
		"heslo3 cycles":             FieldIgnore,
		"heslo4 cycles":             FieldIgnore,
		"hec1 cycles":               FieldIgnore,
		"hec2 cycles":               FieldIgnore,
		"hho cycles":                FieldIgnore,
		"heslo1 time":               FieldIgnore,
		"heslo2 time":               FieldIgnore,
		"heslo3 time":               FieldIgnore,
		"heslo4 time":               FieldIgnore,
		"hec1 time":                 FieldIgnore,
		"hec2 time":                 FieldIgnore,
		"hho time":                  FieldIgnore,
	},
	Signature: []string{
		"copi", "pilot function", "type of flight", "airborne", "block",
		"mountain landings", "glacier landings", "heslo1 cycles", "hho time",
		"go arounds", "touch and goes", "number of pax", "instructiontime",
		"sea takeoffs", "sea landings", "holding patterns",
	},
	MinSignatureHits: 4,
	Priority:         8,
})

// flylogTemplate is built from a real FLYLOG.io export, not from documentation.
//
// The header row is UPPER_SNAKE_CASE with its own vocabulary — DURATION_*,
// TIME_*, LDGS_*, NAME_* — and shares almost nothing with the EASA column names
// an EASA-region logbook might be expected to use. Durations are H:MM. Crew is
// recorded in role-named columns rather than positionally, and the logbook
// owner appears in them as the literal string SELF (see selfCrewSentinel in the
// importer, which drops it rather than filing the owner as their own crew).
var flylogTemplate = register(&Template{
	ID:          "FLYLOG_CSV",
	Name:        "FLYLOG.io",
	Vendor:      "FLYLOG.io",
	Website:     "https://www.flylog.io",
	Description: "FLYLOG.io CSV export. Carries block and airborne times, the full EASA duration breakdown, and named crew per role.",
	Confidence:  ConfidenceExact,
	Regions:     []string{"EASA"},
	ExportSteps: []string{
		"Sign in at flylog.io and open your Logbook.",
		"Choose Export and select the CSV format.",
		"Upload the downloaded file here.",
		"If FLYLOG gave you an XLSX, save it as CSV first.",
	},
	DateFormat: "2006-01-02",
	Columns: map[string]Field{
		"date":                  FieldDate,
		"departure_airport":     FieldDepartureIcao,
		"arrival_airport":       FieldArrivalIcao,
		"aircraft_type":         FieldAircraftType,
		"aircraft_registration": FieldAircraftReg,
		"route":                 FieldRoute,

		"duration_block":   FieldTotalTime,
		"time_block_start": FieldOffBlockTime,
		"time_block_end":   FieldOnBlockTime,
		"time_takeoff":     FieldDepartureTime,
		"time_landing":     FieldArrivalTime,

		"ldgs_day":   FieldLandingsDay,
		"ldgs_night": FieldLandingsNight,

		"duration_pic":           FieldIsPic,
		"duration_dual":          FieldIsDual,
		"duration_instructor":    FieldDualGivenTime,
		"duration_night":         FieldNightTime,
		"duration_ifr":           FieldIFRTime,
		"duration_ifr_actual":    FieldActualInstrumentTime,
		"duration_ifr_simulated": FieldSimulatedInstrumentTime,

		"approach_nr": FieldApproachesCount,
		"remarks":     FieldRemarks,

		"name_pic":        FieldPerson1,
		"name_student":    FieldPerson2,
		"name_instructor": FieldInstructorName,

		// Recognised for scoring, deliberately not mapped.
		//
		// NAME_COPILOT and NAME_PICUS name a role the import fields cannot
		// express: person2–person6 carry a position, not a role, and
		// InferLegacyCrew files anything past person2 as a Passenger. Filing a
		// co-pilot as a passenger is worse than leaving the column for the
		// pilot to assign, and the mapping screen now counts unmapped columns
		// so it is visible rather than silent. Role-typed crew import fields
		// would fix this properly for FLYLOG, LogTen and mccPILOTLOG alike.
		"name_copilot":  FieldIgnore,
		"name_picus":    FieldIgnore,
		"name_examiner": FieldIgnore,

		"duration_picus":       FieldIgnore,
		"duration_sic":         FieldIgnore,
		"duration_examiner":    FieldIgnore,
		"duration_xc":          FieldIgnore,
		"duration_multi_pilot": FieldIgnore,
		"duration_simulator":   FieldIgnore,
		"duration_airborne":    FieldIgnore,
		"duration_duty":        FieldIgnore,
		"simulator_type":       FieldIgnore,
		"personal_note":        FieldIgnore,
		"approach_type":        FieldIgnore,
		"tags":                 FieldIgnore,
		"takeoffs_day":         FieldIgnore,
		"takeoffs_night":       FieldIgnore,
		"time_duty_start":      FieldIgnore,
		"time_duty_end":        FieldIgnore,
		"flight_number":        FieldIgnore,
	},
	Signature: []string{
		"duration_block", "time_block_start", "time_block_end",
		"ldgs_day", "ldgs_night", "duration_picus", "name_picus",
		"duration_ifr_actual", "duration_airborne", "aircraft_registration",
	},
	MinSignatureHits: 3,
	Priority:         8,
})

// waderTemplate is built from a real Wader export.
//
// Wader writes camelCase headers and its own field vocabulary — nothing in
// common with the EASA column names this template previously assumed. Two
// behaviours matter beyond the column names:
//
//   - Unrecorded times are written as 00:00 rather than left blank. The
//     importer drops placeholder midnights before deriving a block time; see
//     mapRowToFlight.
//   - The logbook's owner appears in pilotName1 as the literal string SELF,
//     the same convention FLYLOG.io uses.
//
// isPreviousExperience and isSimulator mark rows that are not ordinary flights
// — a carried-forward totals line and an FSTD session. Both import as flights
// today, because the mapping layer works column by column and has no way to
// reject a row. Anyone importing a Wader logbook that uses either should
// expect to clean those rows up afterwards.
var waderTemplate = register(&Template{
	ID:          "WADER_CSV",
	Name:        "Wader",
	Vendor:      "Wader Aviation",
	Website:     "https://www.waderaviation.com",
	Description: "Wader Pilot Logbook CSV export. Carries block, takeoff and landing times, the full EASA duration breakdown and up to four named crew. Rows Wader marks as previous experience or simulator sessions are imported as ordinary flights and are worth reviewing afterwards.",
	Confidence:  ConfidenceExact,
	Regions:     []string{"EASA", "FAA"},
	ExportSteps: []string{
		"Open Wader on your phone, or sign in at logbook.waderaviation.com.",
		"Open your logbook and choose Export.",
		"Pick CSV rather than PDF.",
		"Upload the downloaded file here.",
	},
	DateFormat: "2006-01-02",
	Columns: map[string]Field{
		"flightdate":         FieldDate,
		"depairport":         FieldDepartureIcao,
		"arrairport":         FieldArrivalIcao,
		"aircrafttailnumber": FieldAircraftReg,
		"aircrafttype":       FieldAircraftType,

		"starttime":   FieldOffBlockTime,
		"parkingtime": FieldOnBlockTime,
		"takeofftime": FieldDepartureTime,
		"landingtime": FieldArrivalTime,

		"totaltime":               FieldTotalTime,
		"nighttime":               FieldNightTime,
		"ifrtime":                 FieldIFRTime,
		"actualinstrumenttime":    FieldActualInstrumentTime,
		"simulatedinstrumenttime": FieldSimulatedInstrumentTime,
		"pictime":                 FieldIsPic,
		"dualtime":                FieldIsDual,
		"instructortime":          FieldDualGivenTime,

		"daylandings":   FieldLandingsDay,
		"nightlandings": FieldLandingsNight,
		"remarks":       FieldRemarks,

		"pilotname1": FieldPerson1,
		"pilotname2": FieldPerson2,
		"pilotname3": FieldPerson3,
		"pilotname4": FieldPerson4,

		// Recognised for scoring, not mapped.
		"ispreviousexperience": FieldIgnore,
		"issimulator":          FieldIgnore,
		"simtype":              FieldIgnore,
		"function":             FieldIgnore,
		"flightnumber":         FieldIgnore,
		"sictime":              FieldIgnore,
		"solotime":             FieldIgnore,
		"picustime":            FieldIgnore,
		"spictime":             FieldIgnore,
		"examinertime":         FieldIgnore,
		"simtraineetime":       FieldIgnore,
		"simtrainertime":       FieldIgnore,
		"crosscountrytime":     FieldIgnore,
		"relieftime":           FieldIgnore,
		"daytakeoffs":          FieldIgnore,
		"nighttakeoffs":        FieldIgnore,
		"approachtype":         FieldIgnore,
		"multiengine":          FieldIgnore,
		"multipilot":           FieldIgnore,
		"depnotes":             FieldIgnore,
		"deprunway":            FieldIgnore,
		"depprocedure":         FieldIgnore,
		"deptransition":        FieldIgnore,
		"depthreats":           FieldIgnore,
		"arrnotes":             FieldIgnore,
		"arrrunway":            FieldIgnore,
		"arrprocedure":         FieldIgnore,
		"arrtransition":        FieldIgnore,
		"arrthreats":           FieldIgnore,
	},
	Signature: []string{
		"aircrafttailnumber", "ispreviousexperience", "issimulator",
		"depairport", "arrairport", "pilotname1", "picustime", "spictime",
		"relieftime", "depthreats", "arrthreats", "parkingtime",
		"simtraineetime",
	},
	MinSignatureHits: 3,
	Priority:         10,
})

var vereinsfliegerTemplate = register(&Template{
	ID:          "VEREINSFLIEGER_CSV",
	Name:        "Vereinsflieger",
	Vendor:      "Vereinsflieger.de",
	Website:     "https://vereinsflieger.de",
	Description: "Vereinsflieger club flight list (German column headers). Club records log the aircraft, times and crew; instrument and night columns are usually absent and stay empty.",
	Confidence:  ConfidenceBestEffort,
	Regions:     []string{"EASA"},
	ExportSteps: []string{
		"Sign in at vereinsflieger.de.",
		"Open Flugbetrieb → Flüge and filter to your own flights.",
		"Use the CSV export button above the list.",
		"Upload the downloaded file here — German headers are recognised.",
	},
	DateFormat: "02.01.2006",
	Columns:    merge(coreColumns, germanColumns),
	Signature: []string{
		"kennzeichen", "startort", "landezeit", "startzeit", "flugzeit",
		"startart", "bemerkung", "begleiter", "zielort",
	},
	MinSignatureHits: 3,
	Priority:         10,
})

var mccPilotLogTemplate = register(&Template{
	ID:          "MCC_PILOTLOG_CSV",
	Name:        "mccPILOTLOG",
	Vendor:      "CrewLounge AERO",
	Website:     "https://crewlounge.aero",
	Description: "mccPILOTLOG / CrewLounge PILOTLOG export. Its column names carry the mcc_ and flight_ prefixes used by the desktop database.",
	Confidence:  ConfidenceBestEffort,
	Regions:     []string{"EASA", "FAA"},
	ExportSteps: []string{
		"Open mccPILOTLOG or CrewLounge PILOTLOG on your computer.",
		"Go to File → Export and choose the CSV / text export.",
		"Select all flights and export.",
		"Upload the downloaded file here.",
	},
	DateFormat: "2006-01-02",
	Columns: merge(coreColumns, easaColumns, map[string]Field{
		"flight_date":     FieldDate,
		"ac_reg":          FieldAircraftReg,
		"ac_model":        FieldAircraftType,
		"af_dep":          FieldDepartureIcao,
		"af_arr":          FieldArrivalIcao,
		"time_dep":        FieldOffBlockTime,
		"time_arr":        FieldOnBlockTime,
		"time_total":      FieldTotalTime,
		"time_night":      FieldNightTime,
		"time_ifr":        FieldIFRTime,
		"time_instructor": FieldDualGivenTime,
		"time_dual":       FieldIsDual,
		"time_pic":        FieldIsPic,
		"pilot1_name":     FieldPerson1,
		"pilot2_name":     FieldPerson2,
		"pilot3_name":     FieldPerson3,
		"pilot4_name":     FieldPerson4,
		"ldg_day":         FieldLandingsDay,
		"ldg_night":       FieldLandingsNight,
		"to_day":          FieldIgnore,
		"to_night":        FieldIgnore,
	}),
	Signature: []string{
		"time_total", "af_dep", "af_arr", "ac_reg", "pilot1_name",
		"ldg_day", "ldg_night", "flight_date",
	},
	MinSignatureHits: 3,
	Priority:         10,
})

// skyDemonTemplate is built from a real SkyDemon export header.
//
// Two structural things set it apart from every other template here. There is
// no date column: the flight is dated by the "Departure Time"/"Arrival Time"
// pair, which the importer falls back on when nothing else supplies a date.
// And there is no total-time column either, so the total is derived from those
// same two values.
//
// A populated export settled the question the header row could not: SkyDemon's
// time columns carry full timestamps ("2025-10-11 14:46"), so a SkyDemon
// logbook can be dated and is importable. Its durations are integer minutes
// rather than decimal hours, and its places are written "ICAO Name"
// ("EDOI Bienenfarm"), from which normalizeLocation takes the leading code.
//
// One thing it does that nothing else here does: the registration has its
// hyphen stripped ("DEROQ" for D-EROQ). It is imported as written, so a pilot
// whose fleet already holds D-EROQ gets a second aircraft rather than a match.
var skyDemonTemplate = register(&Template{
	ID:          "SKYDEMON_CSV",
	Name:        "SkyDemon",
	Vendor:      "Divelements / SkyDemon",
	Website:     "https://www.skydemon.aero",
	Description: "SkyDemon logbook export. It dates each flight by its departure and arrival timestamps rather than a date column, and records no total time — the total is derived from those two. Durations are whole minutes, and registrations are exported without their hyphen. Approach and hold detail is not exported at all.",
	Confidence:  ConfidenceExact,
	Regions:     []string{"EASA"},
	ExportSteps: []string{
		"Open SkyDemon on your tablet or PC and go to the Logbook.",
		"Choose Export and pick the CSV format.",
		"Upload the downloaded file here.",
		"Expect to fill in approach and hold detail afterwards — SkyDemon does not record it.",
	},
	DateFormat: "2006-01-02",
	Columns: map[string]Field{
		"departure time":  FieldOffBlockTime,
		"arrival time":    FieldOnBlockTime,
		"departure place": FieldDepartureIcao,
		"arrival place":   FieldArrivalIcao,
		"aircraft reg":    FieldAircraftReg,
		"aircraft type":   FieldAircraftType,
		"pic name":        FieldPerson1,
		"pic time":        FieldIsPic,
		"dual time":       FieldIsDual,
		"night time":      FieldNightTime,
		"ifr time":        FieldIFRTime,
		"instructor time": FieldDualGivenTime,
		"day landings":    FieldLandingsDay,
		"night landings":  FieldLandingsNight,
		"comments":        FieldRemarks,
	},
	// The individual spellings are shared with the EASA layout; it is the
	// combination that identifies SkyDemon, so the bar is set high enough that
	// a file merely using EASA names cannot clear it.
	Signature: []string{
		"departure time", "arrival time", "departure place", "arrival place",
		"aircraft reg", "pic name", "pic time", "dual time", "instructor time",
		"day landings", "night landings",
	},
	MinSignatureHits: 5,
	Priority:         12,
})

var easaTemplate = register(&Template{
	ID:          "EASA_CSV",
	Name:        "EASA logbook (AMC1 FCL.050)",
	Vendor:      "Any EASA-format logbook",
	Website:     "",
	Description: "The standard European logbook column layout. Use this for any EU logbook app or spreadsheet whose columns follow AMC1 FCL.050, including NinerLog's own EASA CSV export.",
	Confidence:  ConfidenceExact,
	Regions:     []string{"EASA"},
	ExportSteps: []string{
		"Export from your current logbook in the EASA / FCL.050 column layout.",
		"If you keep a spreadsheet, save it as CSV with the EASA headings in row 1.",
		"Upload the file here.",
	},
	DateFormat: "2006-01-02",
	Columns:    merge(coreColumns, easaColumns),
	Signature: []string{
		"dep place", "arr place", "a/c reg", "a/c type", "sp-se", "sp-me",
		"multi-pilot", "ldg day", "ldg night", "remarks & endorsements",
		"fstd date", "pic name",
	},
	MinSignatureHits: 4,
	Priority:         20,
})

var faaTemplate = register(&Template{
	ID:          "FAA_CSV",
	Name:        "FAA logbook layout",
	Vendor:      "Any FAA-format logbook",
	Website:     "",
	Description: "The classic US paper-logbook column layout used by ASA and Jeppesen books and by NinerLog's own FAA CSV export.",
	Confidence:  ConfidenceExact,
	Regions:     []string{"FAA"},
	ExportSteps: []string{
		"Export from your current logbook in the FAA / ASA column layout.",
		"If you keep a spreadsheet, save it as CSV with the FAA headings in row 1.",
		"Upload the file here.",
	},
	DateFormat: "01/02/2006",
	Columns:    merge(coreColumns, faaColumns),
	Signature: []string{
		"a/c ident", "dual rcvd", "instr given", "actual inst", "sim inst",
		"day ldg", "night ldg", "remarks/endorsements", "xc",
	},
	MinSignatureHits: 4,
	Priority:         20,
})

// genericTemplate is the cross-vendor fallback used when nothing else matches.
// It merges every unambiguous alias in the catalogue, so an unrecognised file
// still arrives at the mapping screen mostly filled in.
//
// It is registered last and carries no Signature, which keeps it out of
// detection: it is a mapping table, not a format.
var genericTemplate = register(&Template{
	ID:          FormatGenericCSV,
	Name:        "Other CSV / spreadsheet",
	Vendor:      "",
	Website:     "",
	Description: "Any other CSV, tab- or semicolon-separated file. Columns are matched by name where possible and the rest is mapped by hand — nothing is imported until you have seen the preview.",
	Confidence:  ConfidenceBestEffort,
	Regions:     []string{"EASA", "FAA"},
	ExportSteps: []string{
		"Export or save your logbook as CSV, with the column headings in row 1.",
		"Upload it here.",
		"Match each column to a NinerLog field on the next screen.",
	},
	DateFormat: "",
	Columns: merge(
		coreColumns,
		easaColumns,
		faaColumns,
		germanColumns,
		genericExtraColumns,
	),
	Priority: 100,
})

// genericExtraColumns are spellings that belong to no single template but show
// up in hand-kept spreadsheets and one-off exports.
var genericExtraColumns = map[string]Field{
	"aircraft":     FieldAircraftReg,
	"aircraft id":  FieldAircraftReg,
	"aircraftid":   FieldAircraftReg,
	"aircraftreg":  FieldAircraftReg,
	"tail number":  FieldAircraftReg,
	"aircrafttype": FieldAircraftType,
	"typecode":     FieldAircraftType,
	"type code":    FieldAircraftType,

	"departure":      FieldDepartureIcao,
	"departure icao": FieldDepartureIcao,
	"departureicao":  FieldDepartureIcao,
	"dep":            FieldDepartureIcao,
	"origin":         FieldDepartureIcao,
	"arrival":        FieldArrivalIcao,
	"arrival icao":   FieldArrivalIcao,
	"arrivalicao":    FieldArrivalIcao,
	"arr":            FieldArrivalIcao,
	"dest":           FieldArrivalIcao,

	"offblock":     FieldOffBlockTime,
	"offblocktime": FieldOffBlockTime,
	"out block":    FieldOffBlockTime,
	"out-block":    FieldOffBlockTime,
	"chocks off":   FieldOffBlockTime,
	"timeout":      FieldOffBlockTime,
	"onblock":      FieldOnBlockTime,
	"onblocktime":  FieldOnBlockTime,
	"in block":     FieldOnBlockTime,
	"in-block":     FieldOnBlockTime,
	"chocks on":    FieldOnBlockTime,
	"timein":       FieldOnBlockTime,
	"takeoff":      FieldDepartureTime,
	"take off":     FieldDepartureTime,
	"takeoff time": FieldDepartureTime,
	"timeoff":      FieldDepartureTime,
	"landing":      FieldArrivalTime,
	"landing time": FieldArrivalTime,
	"timeon":       FieldArrivalTime,

	"totaltime":   FieldTotalTime,
	"flight time": FieldTotalTime,
	"duration":    FieldTotalTime,
	"nighttime":   FieldNightTime,
	"ifrtime":     FieldIFRTime,
	"ifr time":    FieldIFRTime,

	"actualinstrument":          FieldActualInstrumentTime,
	"actual instrument time":    FieldActualInstrumentTime,
	"simulatedinstrument":       FieldSimulatedInstrumentTime,
	"simulated instrument time": FieldSimulatedInstrumentTime,

	"landingsday":           FieldLandingsDay,
	"daylandingsfullstop":   FieldLandingsDay,
	"landingsnight":         FieldLandingsNight,
	"nightlandingsfullstop": FieldLandingsNight,
	"alllandings":           FieldLandingsTotal,
	"total landings":        FieldLandingsTotal,

	"approachescount": FieldApproachesCount,
	"approach count":  FieldApproachesCount,

	"pilotcomments":       FieldRemarks,
	"comments":            FieldRemarks,
	"instructorname":      FieldInstructorName,
	"instructor name":     FieldInstructorName,
	"instructorcomments":  FieldInstructorComments,
	"instructor comments": FieldInstructorComments,
	"dualgiven":           FieldDualGivenTime,
	"dualreceived":        FieldIsDual,
	"dual rcvd":           FieldIsDual,

	"person1": FieldPerson1,
	"person2": FieldPerson2,
	"person3": FieldPerson3,
	"person4": FieldPerson4,
	"person5": FieldPerson5,
	"person6": FieldPerson6,

	"flightreview":  FieldIsFlightReview,
	"flight review": FieldIsFlightReview,
	"ipc":           FieldIsIpc,
}
