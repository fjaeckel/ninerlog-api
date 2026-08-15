package importtemplate

import (
	"strings"
	"testing"
)

// Header rows as the respective applications write them. The ForeFlight,
// NinerLog and EASA/FAA rows are verbatim (they are produced by, or consumed
// by, code in this repository); the rest reproduce the columns each vendor
// documents, which is what the best-effort templates target.
var (
	foreFlightHeaders = strings.Split("Date,AircraftID,From,To,Route,TimeOut,TimeOff,TimeOn,TimeIn,OnDuty,OffDuty,TotalTime,PIC,SIC,Night,Solo,CrossCountry,NVG,NVG Ops,Distance,DayTakeoffs,DayLandingsFullStop,NightTakeoffs,NightLandingsFullStop,AllLandings,ActualInstrument,SimulatedInstrument,HobbsStart,HobbsEnd,TachStart,TachEnd,Holds,Approach1,Approach2,Approach3,Approach4,Approach5,Approach6,DualGiven,DualReceived,SimulatedFlight,GroundTraining,InstructorName,InstructorComments,Person1,Person2,Person3,Person4,Person5,Person6,FlightReview,Checkride,IPC,NVG Proficiency,FAA6158,PilotComments", ",")

	// Written by writeStandardCSV in internal/api/handlers/export.go.
	ninerlogHeaders = strings.Split("Date,AircraftID,AircraftType,From,To,Route,TimeOut,TimeOff,TimeOn,TimeIn,TotalTime,PIC,SIC,Night,Solo,CrossCountry,Distance,DayTakeoffs,DayLandingsFullStop,NightTakeoffs,NightLandingsFullStop,AllLandings,ActualInstrument,SimulatedInstrument,Holds,ApproachesCount,DualGiven,DualReceived,SimulatedFlight,GroundTraining,InstructorName,InstructorComments,FlightReview,IPC,IFRTime,Remarks,PICName,MultiPilotTime,FSTDType,Endorsements", ",")

	// Written by writeEASACSV in internal/api/handlers/export.go.
	easaHeaders = strings.Split("Date,Dep Place,Dep Time,Arr Place,Arr Time,A/C Type,A/C Reg,SP-SE,SP-ME,Multi-Pilot,Total Time,PIC Name,Ldg Day,Ldg Night,Night,IFR,PIC,Co-Pilot,Dual,Instructor,FSTD Date,FSTD Type,FSTD Time,Remarks & Endorsements", ",")

	// Written by writeFAACSV in internal/api/handlers/export.go.
	faaHeaders = strings.Split("Date,A/C Type,A/C Ident,From,To,Solo,PIC,SIC,Dual Rcvd,Instr Given,Actual Inst,Sim Inst,XC,Night,Day Ldg,Night Ldg,Approaches,Holds,Total,Remarks/Endorsements", ",")

	logTenKeyHeaders = strings.Split("flight_flightDate,flight_selectedAircraftID,flight_from,flight_to,flight_route,flight_totalTime,flight_pic,flight_sic,flight_night,flight_actualInstrument,flight_simulatedInstrument,flight_dayLandings,flight_nightLandings,flight_holds,flight_dualGiven,flight_dualReceived,flight_remarks,flight_selectedCrewPIC,flight_selectedCrewSIC,flight_selectedCrewInstructor", ",")

	logTenHumanHeaders = strings.Split("Flight Date,Aircraft ID,Aircraft Type,From,To,Route,Total Time,PIC,SIC,Solo,Night,Cross Country,Actual Instrument,Simulated Instrument,Day Landings,Night Landings,Holds,Approaches,Dual Given,Dual Received,Remarks,Out,In,Off,On,Selected Crew PIC,Selected Crew SIC,Selected Crew Instructor", ",")

	myFlightbookHeaders = strings.Split("Date,Tail Number,Model,Category/Class,Route,Comments,Approaches,Hold,Landings,FS Day Landings,FS Night Landings,X-Country,Night,Simulated Instrument,IMC,Ground Simulator,Dual Received,CFI,SIC,PIC,Total Flight Time,Hobbs Start,Hobbs End,Engine Start,Engine End,Flight Start,Flight End,Flight ID", ",")

	capzlogHeaders = strings.Split("Date,Departure Place,Departure Time,Arrival Place,Arrival Time,Aircraft Model,Aircraft Registration,Single Pilot SE,Single Pilot ME,Multi Pilot,Total Time of Flight,Name(s) PIC,Landings Day,Landings Night,Night,IFR,PIC,Co-Pilot,Dual,Instructor,FSTD Date,FSTD Type,FSTD Total Time,Remarks and Endorsements", ",")

	flylogHeaders = strings.Split("Date,Departure Airport,Arrival Airport,Aircraft Type,Aircraft Registration,Block Time,PIC Time,SIC Time,Multi Pilot Time,Night,IFR,Day Takeoffs,Night Takeoffs,Landings Day,Landings Night,PIC Name,Simulator Type,Remarks", ",")

	vereinsfliegerHeaders = strings.Split("Datum;Kennzeichen;Muster;Startort;Zielort;Startzeit;Landezeit;Flugzeit;Pilot;Begleiter;Startart;Landungen;Bemerkung", ";")

	mccPilotLogHeaders = strings.Split("flight_date,ac_reg,ac_model,af_dep,af_arr,time_dep,time_arr,time_total,time_night,time_ifr,time_pic,time_dual,time_instructor,pilot1_name,pilot2_name,ldg_day,ldg_night,to_day,to_night,remarks", ",")

	waderHeaders = strings.Split("Date,Sector,Aircraft,Aircraft Type,Departure,Arrival,Off Block,On Block,Total Time,Captain,Crew,Instructor Name,Night,IFR,Landings Day,Landings Night,Remarks", ",")

	skyDemonHeaders = strings.Split("Date,Aircraft,Aircraft Type,Pilot In Command,Departure,Departure Time,Arrival,Arrival Time,Duration,Distance", ",")
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
		{"LogTen Pro human headers", logTenHumanHeaders, "LOGTEN_CSV"},
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
				if _, ok := mapped[f]; !ok {
					t.Errorf("template %s does not map %s", id, f)
				}
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
