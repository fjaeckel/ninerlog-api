package importtemplate

import (
	"strings"
	"testing"
)

// Header rows as the respective applications write them.
//
// The ForeFlight, NinerLog and EASA/FAA rows are verbatim (they are produced
// by, or consumed by, code in this repository). FLYLOG.io, MyFlightbook and
// FLYLOG.io, MyFlightbook, LogTen Pro, Wader, SkyDemon and capzlog.aero are
// verbatim from real exports — every one of the six was wrong when written from
// vendor documentation instead. SkyDemon's came from an export of an empty
// logbook, so its column names are verified but its value formats are not.
// Only Vereinsflieger and mccPILOTLOG remain as documentation guesses. The remainder still
// reproduce the columns each vendor documents, which is what the best-effort
// templates target and what makes them provisional.
var (
	foreFlightHeaders = strings.Split("Date,AircraftID,From,To,Route,TimeOut,TimeOff,TimeOn,TimeIn,OnDuty,OffDuty,TotalTime,PIC,SIC,Night,Solo,CrossCountry,NVG,NVG Ops,Distance,DayTakeoffs,DayLandingsFullStop,NightTakeoffs,NightLandingsFullStop,AllLandings,ActualInstrument,SimulatedInstrument,HobbsStart,HobbsEnd,TachStart,TachEnd,Holds,Approach1,Approach2,Approach3,Approach4,Approach5,Approach6,DualGiven,DualReceived,SimulatedFlight,GroundTraining,InstructorName,InstructorComments,Person1,Person2,Person3,Person4,Person5,Person6,FlightReview,Checkride,IPC,NVG Proficiency,FAA6158,PilotComments", ",")

	// Written by writeStandardCSV in internal/api/handlers/export.go.
	ninerlogHeaders = strings.Split("Date,AircraftID,AircraftType,From,To,Route,TimeOut,TimeOff,TimeOn,TimeIn,TotalTime,PIC,SIC,Night,Solo,CrossCountry,Distance,DayTakeoffs,DayLandingsFullStop,NightTakeoffs,NightLandingsFullStop,AllLandings,ActualInstrument,SimulatedInstrument,Holds,ApproachesCount,DualGiven,DualReceived,SimulatedFlight,GroundTraining,InstructorName,InstructorComments,FlightReview,IPC,IFRTime,Remarks,PICName,MultiPilotTime,FSTDType,Endorsements", ",")

	// Written by writeEASACSV in internal/api/handlers/export.go.
	easaHeaders = strings.Split("Date,Dep Place,Dep Time,Arr Place,Arr Time,A/C Type,A/C Reg,SP-SE,SP-ME,Multi-Pilot,Total Time,PIC Name,Ldg Day,Ldg Night,Night,IFR,PIC,Co-Pilot,Dual,Instructor,FSTD Date,FSTD Type,FSTD Time,Remarks & Endorsements", ",")

	// Written by writeFAACSV in internal/api/handlers/export.go.
	faaHeaders = strings.Split("Date,A/C Type,A/C Ident,From,To,Solo,PIC,SIC,Dual Rcvd,Instr Given,Actual Inst,Sim Inst,XC,Night,Day Ldg,Night Ldg,Approaches,Holds,Total,Remarks/Endorsements", ",")

	logTenKeyHeaders = strings.Split("flight_flightDate,flight_selectedAircraftID,flight_from,flight_to,flight_route,flight_totalTime,flight_pic,flight_sic,flight_night,flight_actualInstrument,flight_simulatedInstrument,flight_dayLandings,flight_nightLandings,flight_holds,flight_dualGiven,flight_dualReceived,flight_remarks,flight_selectedCrewPIC,flight_selectedCrewSIC,flight_selectedCrewInstructor", ",")

	// The real LogTen Pro Dynamic Export header row (tab-delimited, with a
	// trailing empty column). It uses the FAA short spellings, which is why
	// FAA_CSV used to claim it.
	logTenHumanHeaders = strings.Split("Date\tAircraft ID\tAircraft Type\tFrom\tRoute\tTo\tOut\tIn\tTotal Time\tNight\tPIC\tDual Rcvd\tSolo\tXC\tSim Inst\tActual Inst\tSimulator\tGround\tPIC/P1 Crew\tStudent\tInstructor\tDay T/O\tDay Ldg\tNight T/O\tNight Ldg\tApproach 1\tApproach 2\tHolds\tRemarks\tFlight Review\t", "\t")

	// The real MyFlightbook export header row. Note both "Model" (a marketing
	// description) and "ICAO Model" (the type code), and "Aircraft ID" — which
	// is MyFlightbook's internal row ID, not a registration.
	myFlightbookHeaders = strings.Split("Date,Flight ID,Model,ICAO Model,Tail Number,Display Tail,Aircraft ID,Category/Class,Approaches,Hold,Landings,FS Night Landings,FS Day Landings,X-Country,Night,IMC,Simulated Instrument,Ground Simulator,Dual Received,CFI,SIC,PIC,Total Flight Time,CFI Time (HH:MM),SIC Time (HH:MM),PIC (HH:MM),Total Flight Time (HH:MM),Route,Flight Properties,Comments,Hobbs Start,Hobbs End,Engine Start,Engine End,Engine Time,Flight Start,Flight End,Flying Time,Complex,Controllable pitch prop,Flaps,Retract,Tailwheel,High Performance,Turbine,TAA,Signature State,Date of Signature,CFI Comment,CFI Certificate,CFI Name,CFI Email,CFI Expiration,Public", ",")

	// The real capzlog.aero export header row. No date column — the flight is
	// dated by the Off Block timestamp. "Departure"/"Arrival" are places,
	// not times.
	capzlogHeaders = strings.Split("Departure,Arrival,Off Block,On Block,Block,Takeoff,Landing,Airborne,Aircraft,Model,Single Engine,Multi Engine,Multi Pilot,PIC Name,Type of Flight,VFR,IFR,Day,Night,Pilot Function,PIC,Copi,Dual,Instructor,Landings,Day Landings,Night Landings,Remark,Mountain Landings,Mountain Takeoffs,Mountain Landings > 2000m,Mountain Landings > 2700m,Glacier Landings,Holding Patterns,Go Arounds,Touch and Goes,Number of PAX,Sea Takeoffs,Sea Landings,InstructionTime,HESLO1 Cycles,HESLO2 Cycles,HESLO3 Cycles,HESLO4 Cycles,HEC1 Cycles,HEC2 Cycles,HHO Cycles,HESLO1 Time,HESLO2 Time,HESLO3 Time,HESLO4 Time,HEC1 Time,HEC2 Time,HHO Time", ",")

	// The real FLYLOG.io export header row, from an export supplied by the
	// maintainer. The row this replaced was written from the vendor's prose
	// description and was wrong in every column but the date — see
	// testdata/importsamples/flylog.csv.
	flylogHeaders = strings.Split("DATE,DEPARTURE_AIRPORT,ARRIVAL_AIRPORT,AIRCRAFT_TYPE,AIRCRAFT_REGISTRATION,DURATION_BLOCK,LDGS_DAY,LDGS_NIGHT,TIME_BLOCK_START,TIME_BLOCK_END,DURATION_PIC,DURATION_PICUS,DURATION_SIC,DURATION_DUAL,DURATION_INSTRUCTOR,DURATION_EXAMINER,DURATION_NIGHT,DURATION_IFR,DURATION_IFR_ACTUAL,DURATION_IFR_SIMULATED,DURATION_XC,DURATION_MULTI_PILOT,DURATION_SIMULATOR,SIMULATOR_TYPE,REMARKS,PERSONAL_NOTE,APPROACH_TYPE,APPROACH_NR,TAGS,NAME_PIC,NAME_PICUS,NAME_COPILOT,NAME_STUDENT,NAME_INSTRUCTOR,NAME_EXAMINER,TAKEOFFS_DAY,TAKEOFFS_NIGHT,TIME_TAKEOFF,TIME_LANDING,DURATION_AIRBORNE,TIME_DUTY_START,TIME_DUTY_END,DURATION_DUTY,FLIGHT_NUMBER,ROUTE", ",")

	vereinsfliegerHeaders = strings.Split("Datum;Kennzeichen;Muster;Startort;Zielort;Startzeit;Landezeit;Flugzeit;Pilot;Begleiter;Startart;Landungen;Bemerkung", ";")

	mccPilotLogHeaders = strings.Split("flight_date,ac_reg,ac_model,af_dep,af_arr,time_dep,time_arr,time_total,time_night,time_ifr,time_pic,time_dual,time_instructor,pilot1_name,pilot2_name,ldg_day,ldg_night,to_day,to_night,remarks", ",")

	// The real Wader export header row: camelCase, with its own field
	// vocabulary. The row this replaced assumed EASA column names and
	// matched nothing, so Wader files were not detected at all.
	waderHeaders = strings.Split("isPreviousExperience,isSimulator,flightDate,startTime,takeoffTime,landingTime,parkingTime,flightNumber,depAirport,arrAirport,aircraftTailnumber,aircraftType,simType,function,pilotName1,pilotName2,pilotName3,pilotName4,totalTime,picTime,sicTime,soloTime,dualTime,picusTime,spicTime,examinerTime,instructorTime,simTraineeTime,simTrainerTime,crossCountryTime,actualInstrumentTime,simulatedInstrumentTime,reliefTime,ifrTime,nightTime,dayTakeoffs,nightTakeoffs,dayLandings,nightLandings,approachType,remarks,multiEngine,multiPilot,depNotes,depRunway,depProcedure,depTransition,depThreats,arrNotes,arrRunway,arrProcedure,arrTransition,arrThreats", ",")

	// The real SkyDemon export header row, from an export of an empty
	// logbook. Note the three unnamed columns, and that there is no date
	// column and no total-time column at all — both are derived from the
	// departure/arrival timestamps.
	skyDemonHeaders = strings.Split("Departure Time,Departure Place,Arrival Time,Arrival Place,Aircraft Reg,Aircraft Type,PIC Name,PIC Time,Dual Time,Night Time,IFR Time,Instructor Time,,,,Day Landings,Night Landings,Comments", ",")
)

func TestDetectIdentifiesEachSource(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
		want    string
	}{
		{"ForeFlight", foreFlightHeaders, "FOREFLIGHT_CSV"},
		{"NinerLog standard export", ninerlogHeaders, "NINERLOG_CSV"},
		{"NinerLog EASA export", easaHeaders, "EASA_CSV"},
		{"NinerLog FAA export", faaHeaders, "FAA_CSV"},
		{"LogTen Pro field keys", logTenKeyHeaders, "LOGTEN_CSV"},
		{"LogTen Pro Dynamic Export", logTenHumanHeaders, "LOGTEN_CSV"},
		{"MyFlightbook", myFlightbookHeaders, "MYFLIGHTBOOK_CSV"},
		{"capzlog.aero", capzlogHeaders, "CAPZLOG_CSV"},
		{"FLYLOG.io", flylogHeaders, "FLYLOG_CSV"},
		{"Vereinsflieger", vereinsfliegerHeaders, "VEREINSFLIEGER_CSV"},
		{"mccPILOTLOG", mccPilotLogHeaders, "MCC_PILOTLOG_CSV"},
		{"Wader", waderHeaders, "WADER_CSV"},
		{"SkyDemon", skyDemonHeaders, "SKYDEMON_CSV"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Detect(tc.headers)
			if got == nil {
				t.Fatalf("Detect() = nil, want %s", tc.want)
			}
			if got.ID != tc.want {
				t.Errorf("Detect() = %s, want %s", got.ID, tc.want)
			}
		})
	}
}

// Detection must survive the cosmetic damage real files pick up on the way:
// a spreadsheet round-trip that re-cases headers, a BOM from Excel, quoted
// header cells, and stray padding.
func TestDetectToleratesHeaderNoise(t *testing.T) {
	noisy := make([]string, len(foreFlightHeaders))
	for i, h := range foreFlightHeaders {
		noisy[i] = "  " + strings.ToUpper(h) + " "
	}
	noisy[0] = "\ufeff" + noisy[0]

	got := Detect(noisy)
	if got == nil || got.ID != "FOREFLIGHT_CSV" {
		t.Fatalf("Detect(noisy ForeFlight) = %v, want FOREFLIGHT_CSV", got)
	}
}

func TestDetectReturnsNilForUnknownFile(t *testing.T) {
	for _, headers := range [][]string{
		{"Name", "Email", "Phone", "Company"},
		{"col1", "col2", "col3"},
		{},
	} {
		if got := Detect(headers); got != nil {
			t.Errorf("Detect(%v) = %s, want nil", headers, got.ID)
		}
	}
}

// A ForeFlight file must not be claimed by the generic EASA layout just because
// both have a Date and a From/To pair, and vice versa. Cross-claiming is the
// failure mode that silently maps a column to the wrong field.
func TestDetectDoesNotCrossClaim(t *testing.T) {
	cases := map[string][]string{
		"FOREFLIGHT_CSV":     foreFlightHeaders,
		"EASA_CSV":           easaHeaders,
		"FAA_CSV":            faaHeaders,
		"MYFLIGHTBOOK_CSV":   myFlightbookHeaders,
		"VEREINSFLIEGER_CSV": vereinsfliegerHeaders,
	}
	for wantID, headers := range cases {
		for _, tpl := range All() {
			if tpl.ID == wantID || len(tpl.Signature) == 0 {
				continue
			}
			_, sig := tpl.Matches(headers)
			if sig >= tpl.MinSignatureHits {
				// Another template also clears its bar. That is allowed as long
				// as scoring still picks the right one — which Detect asserts —
				// but flag it so the overlap is a deliberate choice.
				if got := Detect(headers); got == nil || got.ID != wantID {
					t.Errorf("%s headers: %s claimed the file (sig=%d), Detect()=%v",
						wantID, tpl.ID, sig, got)
				}
			}
		}
	}
}

func TestSuggestMapsTheEssentialFields(t *testing.T) {
	// Every template must at minimum resolve the fields a flight cannot be
	// created without, otherwise the import is guaranteed to error out.
	required := []Field{FieldDate, FieldAircraftReg, FieldTotalTime}

	// Exceptions, each because the source genuinely has no such column and the
	// importer derives the value instead. These are the two derivations in
	// mapRowToFlight; adding a third exception here without a matching
	// derivation would be hiding a broken template.
	exempt := map[string]map[Field]string{
		"SKYDEMON_CSV": {
			FieldDate:      "no date column — derived from the departure timestamp",
			FieldTotalTime: "no total column — derived from the block times",
		},
		"CAPZLOG_CSV": {
			FieldDate: "no date column — derived from the off-block timestamp",
		},
		"MYFLIGHTBOOK_CSV": {
			// Present but not required here: MyFlightbook's own totals may be
			// blank, with the time coming from Engine Start/End.
		},
	}

	cases := map[string][]string{
		"FOREFLIGHT_CSV":     foreFlightHeaders,
		"NINERLOG_CSV":       ninerlogHeaders,
		"EASA_CSV":           easaHeaders,
		"FAA_CSV":            faaHeaders,
		"LOGTEN_CSV":         logTenKeyHeaders,
		"MYFLIGHTBOOK_CSV":   myFlightbookHeaders,
		"CAPZLOG_CSV":        capzlogHeaders,
		"FLYLOG_CSV":         flylogHeaders,
		"VEREINSFLIEGER_CSV": vereinsfliegerHeaders,
		"MCC_PILOTLOG_CSV":   mccPilotLogHeaders,
		"WADER_CSV":          waderHeaders,
		"SKYDEMON_CSV":       skyDemonHeaders,
	}

	for id, headers := range cases {
		t.Run(id, func(t *testing.T) {
			tpl := ByID(id)
			if tpl == nil {
				t.Fatalf("template %s not registered", id)
			}
			mapped := make(map[Field]string)
			for _, m := range tpl.Suggest(headers) {
				mapped[m.TargetField] = m.SourceColumn
			}
			for _, f := range required {
				if _, ok := mapped[f]; ok {
					continue
				}
				if why, excused := exempt[id][f]; excused {
					t.Logf("template %s does not map %s: %s", id, f, why)
					continue
				}
				t.Errorf("template %s does not map %s", id, f)
			}
		})
	}
}

// Departure and arrival are required by the importer. Every template except the
// ones whose source genuinely stores a single route string must resolve both.
func TestSuggestMapsAirports(t *testing.T) {
	cases := map[string][]string{
		"FOREFLIGHT_CSV":     foreFlightHeaders,
		"NINERLOG_CSV":       ninerlogHeaders,
		"EASA_CSV":           easaHeaders,
		"FAA_CSV":            faaHeaders,
		"LOGTEN_CSV":         logTenKeyHeaders,
		"CAPZLOG_CSV":        capzlogHeaders,
		"FLYLOG_CSV":         flylogHeaders,
		"VEREINSFLIEGER_CSV": vereinsfliegerHeaders,
		"MCC_PILOTLOG_CSV":   mccPilotLogHeaders,
		"WADER_CSV":          waderHeaders,
		"SKYDEMON_CSV":       skyDemonHeaders,
	}
	for id, headers := range cases {
		t.Run(id, func(t *testing.T) {
			mapped := make(map[Field]bool)
			for _, m := range ByID(id).Suggest(headers) {
				mapped[m.TargetField] = true
			}
			if !mapped[FieldDepartureIcao] || !mapped[FieldArrivalIcao] {
				t.Errorf("template %s does not map both airports (dep=%v arr=%v)",
					id, mapped[FieldDepartureIcao], mapped[FieldArrivalIcao])
			}
		})
	}

	// MyFlightbook records the route as one field; the importer derives the
	// airports from it, so the template only has to map the route itself.
	mapped := make(map[Field]bool)
	for _, m := range ByID("MYFLIGHTBOOK_CSV").Suggest(myFlightbookHeaders) {
		mapped[m.TargetField] = true
	}
	if !mapped[FieldRoute] {
		t.Error("MYFLIGHTBOOK_CSV must map the route column — airports are derived from it")
	}
}

func TestSuggestNeverEmitsIgnore(t *testing.T) {
	for _, tpl := range All() {
		for _, m := range tpl.Suggest(append(append([]string{}, foreFlightHeaders...), easaHeaders...)) {
			if m.TargetField == FieldIgnore {
				t.Errorf("%s suggested ignore for %q", tpl.ID, m.SourceColumn)
			}
		}
	}
}

func TestSuggestAttachesDateFormatHint(t *testing.T) {
	for _, m := range ByID("VEREINSFLIEGER_CSV").Suggest(vereinsfliegerHeaders) {
		if m.TargetField != FieldDate {
			continue
		}
		if m.DateFormat != "02.01.2006" {
			t.Errorf("date hint = %q, want 02.01.2006", m.DateFormat)
		}
		return
	}
	t.Fatal("no date mapping suggested")
}

// The fallback table is what an unrecognised file gets. It has to do better
// than nothing on a plain hand-kept spreadsheet.
func TestGenericFallbackMapsPlainSpreadsheet(t *testing.T) {
	headers := []string{"Date", "Registration", "Type", "From", "To", "Off Block", "On Block", "Total Time", "Remarks"}
	if got := Detect(headers); got != nil {
		t.Logf("note: plain spreadsheet detected as %s", got.ID)
	}
	mapped := make(map[Field]bool)
	for _, m := range Suggest(nil, headers) {
		mapped[m.TargetField] = true
	}
	for _, f := range []Field{FieldDate, FieldAircraftReg, FieldAircraftType, FieldDepartureIcao, FieldArrivalIcao, FieldOffBlockTime, FieldOnBlockTime, FieldTotalTime, FieldRemarks} {
		if !mapped[f] {
			t.Errorf("generic fallback did not map %s", f)
		}
	}
}

func TestDetectAndSuggestReportsGenericFormat(t *testing.T) {
	tpl, format, mappings := DetectAndSuggest([]string{"Name", "Email"})
	if tpl != nil {
		t.Errorf("template = %s, want nil", tpl.ID)
	}
	if format != FormatGenericCSV {
		t.Errorf("format = %s, want %s", format, FormatGenericCSV)
	}
	if len(mappings) != 0 {
		t.Errorf("mappings = %v, want none", mappings)
	}

	tpl, format, mappings = DetectAndSuggest(foreFlightHeaders)
	if tpl == nil || format != "FOREFLIGHT_CSV" {
		t.Fatalf("format = %s, want FOREFLIGHT_CSV", format)
	}
	if len(mappings) == 0 {
		t.Error("expected suggested mappings for a ForeFlight file")
	}
}

func TestNormalizeHeader(t *testing.T) {
	tests := map[string]string{
		"  Total  Time ":    "total time",
		"AircraftID":        "aircraftid",
		"Aircraft ID":       "aircraft ID",
		"\ufeffDate":        "date",
		`"Remarks"`:         "remarks",
		"Name(s) PIC":       "name(s) pic",
		"flight_flightDate": "flight_flightdate",
	}
	for in, want := range tests {
		want = strings.ToLower(want)
		if got := normalizeHeader(in); got != want {
			t.Errorf("normalizeHeader(%q) = %q, want %q", in, got, want)
		}
	}
}

// Catalogue hygiene: an unnormalised key is silently dead, and a template
// without export instructions is not much use on the import screen.
func TestCatalogueIsWellFormed(t *testing.T) {
	for _, tpl := range All() {
		if tpl.Name == "" || tpl.Description == "" || len(tpl.ExportSteps) == 0 {
			t.Errorf("%s: missing name, description or export steps", tpl.ID)
		}
		for key := range tpl.Columns {
			if normalizeHeader(key) != key {
				t.Errorf("%s: column key %q is not normalised (want %q)",
					tpl.ID, key, normalizeHeader(key))
			}
		}
		for _, sig := range tpl.Signature {
			if normalizeHeader(sig) != sig {
				t.Errorf("%s: signature %q is not normalised", tpl.ID, sig)
			}
			if _, ok := tpl.Columns[sig]; !ok {
				t.Errorf("%s: signature %q is not one of the template's columns", tpl.ID, sig)
			}
		}
		if len(tpl.Signature) > 0 && tpl.MinSignatureHits > len(tpl.Signature) {
			t.Errorf("%s: MinSignatureHits %d exceeds %d signature columns",
				tpl.ID, tpl.MinSignatureHits, len(tpl.Signature))
		}
	}
}
