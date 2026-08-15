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
	Description: "LogTen Pro flight export, in both the human-readable and the field-key (flight_…) dialect. Times come across as decimal hours.",
	Confidence:  ConfidenceBestEffort,
	Regions:     []string{"FAA", "EASA"},
	ExportSteps: []string{
		"Open LogTen Pro on Mac or iPad.",
		"Go to Reports → Exporters and choose Export Flights (Tab or CSV).",
		"Do not use Dynamic Export — it can omit columns.",
		"Save the file and upload it here.",
	},
	DateFormat: "2006-01-02",
	Columns: merge(coreColumns, map[string]Field{
		// Human-readable dialect.
		"aircraft id":              FieldAircraftReg,
		"aircraft type":            FieldAircraftType,
		"from":                     FieldDepartureIcao,
		"to":                       FieldArrivalIcao,
		"total time":               FieldTotalTime,
		"night":                    FieldNightTime,
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
	Signature: []string{
		"flight_flightdate", "flight_totaltime", "flight_selectedcrewpic",
		"flight_selectedaircraftid", "flight_dualreceived", "aircraft_aircraftid",
		"selected crew pic", "selected crew sic", "selected crew instructor",
		"aircraft id",
	},
	MinSignatureHits: 2,
	Priority:         10,
})

var myFlightbookTemplate = register(&Template{
	ID:          "MYFLIGHTBOOK_CSV",
	Name:        "MyFlightbook",
	Vendor:      "MyFlightbook",
	Website:     "https://myflightbook.com",
	Description: "MyFlightbook CSV export. It records the route as a single field rather than separate airports, so departure and arrival are taken from the first and last waypoint.",
	Confidence:  ConfidenceBestEffort,
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
		"model":                FieldAircraftType,
		"aircraft type":        FieldAircraftType,
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

		"flight id":        FieldIgnore,
		"pic":              FieldIsPic,
		"sic":              FieldIgnore,
		"x-country":        FieldIgnore,
		"ground simulator": FieldIgnore,
		"category/class":   FieldIgnore,
		"hobbs start":      FieldIgnore,
		"hobbs end":        FieldIgnore,
	}),
	Signature: []string{
		"tail number", "total flight time", "ground simulator", "x-country",
		"fs day landings", "fs night landings", "imc", "cfi", "flight id",
	},
	MinSignatureHits: 3,
	Priority:         10,
})

var capzlogTemplate = register(&Template{
	ID:          "CAPZLOG_CSV",
	Name:        "capzlog.aero",
	Vendor:      "Aviaso / capzlog.aero",
	Website:     "https://capzlog.aero",
	Description: "capzlog.aero flight export. Follows the EASA AMC1 FCL.050 column layout, so single-pilot, multi-pilot and FSTD columns come across as recorded.",
	Confidence:  ConfidenceBestEffort,
	Regions:     []string{"EASA"},
	ExportSteps: []string{
		"Sign in at capzlog.aero and open your Flights list.",
		"Apply any filter you want the export limited to (or none for everything).",
		"Choose Export and pick CSV rather than PDF.",
		"Upload the downloaded file here.",
	},
	DateFormat: "2006-01-02",
	Columns: merge(coreColumns, easaColumns, map[string]Field{
		"aircraft":       FieldAircraftReg,
		"flight time":    FieldTotalTime,
		"takeoffs day":   FieldIgnore,
		"takeoffs night": FieldIgnore,
		"function":       FieldIgnore,
		"flight rules":   FieldIgnore,
		"flight type":    FieldIgnore,
	}),
	Signature: []string{
		"total time of flight", "name(s) pic", "departure place", "arrival place",
		"remarks and endorsements", "fstd total time", "single pilot se",
		"single pilot me",
	},
	MinSignatureHits: 3,
	Priority:         8,
})

var flylogTemplate = register(&Template{
	ID:          "FLYLOG_CSV",
	Name:        "FLYLOG.io",
	Vendor:      "FLYLOG.io",
	Website:     "https://www.flylog.io",
	Description: "FLYLOG.io CSV export, including its custom bulk-import template layout.",
	Confidence:  ConfidenceBestEffort,
	Regions:     []string{"EASA"},
	ExportSteps: []string{
		"Sign in at flylog.io and open your Logbook.",
		"Choose Export and select the CSV format.",
		"Upload the downloaded file here.",
		"If FLYLOG gave you an XLSX, save it as CSV first.",
	},
	DateFormat: "2006-01-02",
	Columns: merge(coreColumns, easaColumns, map[string]Field{
		"departure":        FieldDepartureIcao,
		"arrival":          FieldArrivalIcao,
		"dep":              FieldDepartureIcao,
		"arr":              FieldArrivalIcao,
		"aircraft":         FieldAircraftReg,
		"sic time":         FieldIgnore,
		"multi pilot time": FieldIgnore,
		"takeoffs day":     FieldIgnore,
		"takeoffs night":   FieldIgnore,
		"night takeoffs":   FieldIgnore,
		"day takeoffs":     FieldIgnore,
		"simulator time":   FieldIgnore,
		"simulator type":   FieldIgnore,
	}),
	Signature: []string{
		"pic time", "sic time", "multi pilot time", "departure airport",
		"arrival airport", "simulator type", "block time",
	},
	MinSignatureHits: 3,
	Priority:         8,
})

var waderTemplate = register(&Template{
	ID:          "WADER_CSV",
	Name:        "Wader",
	Vendor:      "Wader Aviation",
	Website:     "https://www.waderaviation.com",
	Description: "Wader Pilot Logbook CSV export. Wader writes an EASA-shaped sector list; unrecognised columns land on the mapping screen.",
	Confidence:  ConfidenceBestEffort,
	Regions:     []string{"EASA", "FAA"},
	ExportSteps: []string{
		"Open Wader on your phone, or sign in at logbook.waderaviation.com.",
		"Go to Settings → Export (or Logbook → Export).",
		"Choose CSV rather than PDF.",
		"Upload the downloaded file here.",
	},
	DateFormat: "2006-01-02",
	Columns: merge(coreColumns, easaColumns, map[string]Field{
		"aircraft":        FieldAircraftReg,
		"departure":       FieldDepartureIcao,
		"arrival":         FieldArrivalIcao,
		"sector":          FieldIgnore,
		"crew":            FieldPerson2,
		"captain":         FieldPerson1,
		"instructor name": FieldInstructorName,
		"takeoffs":        FieldIgnore,
		"flight time":     FieldTotalTime,
	}),
	Signature: []string{
		"sector", "captain", "crew", "instructor name",
	},
	MinSignatureHits: 2,
	Priority:         12,
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

var skyDemonTemplate = register(&Template{
	ID:          "SKYDEMON_CSV",
	Name:        "SkyDemon",
	Vendor:      "Divelements / SkyDemon",
	Website:     "https://www.skydemon.aero",
	Description: "SkyDemon logbook export. SkyDemon logs the flown sector rather than a full logbook row, so instrument, night and landing columns are usually absent.",
	Confidence:  ConfidenceBestEffort,
	Regions:     []string{"EASA"},
	ExportSteps: []string{
		"Open SkyDemon on your tablet or PC and go to the Logbook.",
		"Choose Export and pick the CSV format.",
		"Upload the downloaded file here.",
		"Expect to fill in instrument and night columns afterwards — SkyDemon does not record them.",
	},
	DateFormat: "02/01/2006",
	Columns: merge(coreColumns, easaColumns, map[string]Field{
		"aircraft":         FieldAircraftReg,
		"aircraft type":    FieldAircraftType,
		"pilot in command": FieldPerson1,
		"departure":        FieldDepartureIcao,
		"arrival":          FieldArrivalIcao,
		"duration":         FieldTotalTime,
		"distance":         FieldIgnore,
		"engine time":      FieldIgnore,
	}),
	Signature: []string{
		"duration", "pilot in command", "departure time", "arrival time", "distance",
	},
	MinSignatureHits: 3,
	Priority:         14,
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
