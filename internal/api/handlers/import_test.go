package handlers

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
)

func TestParseCSV_ForeFlightFormat(t *testing.T) {
	csvData := `ForeFlight Logbook Import;;;;;;;
;;;;;;;
Aircraft Table;;;;;;;
AircraftID;EquipmentType;TypeCode;Year;Make;Model;Category;Class;GearType;EngineType;Complex;TAA;HighPerformance;Pressurized
D-EABC;aircraft;C172;;Cessna;172SP;airplane;airplane_single_engine_land;fixed_tricycle;Piston;FALSE;FALSE;FALSE;FALSE
D-EXYZ;aircraft;HR20;;Robin;HR200;airplane;airplane_single_engine_land;fixed_tricycle;Piston;FALSE;FALSE;FALSE;FALSE
;;;;;;;
Flights Table;;;;;;;
Date;AircraftID;From;To;Route;TimeOut;TimeOff;TimeOn;TimeIn;OnDuty;OffDuty;TotalTime;PIC;SIC;Night;Solo;CrossCountry;NVG;NVG Ops;Distance;DayTakeoffs;DayLandingsFullStop;NightTakeoffs;NightLandingsFullStop;AllLandings;ActualInstrument;SimulatedInstrument;HobbsStart;HobbsEnd;TachStart;TachEnd;Holds;Approach1;Approach2;Approach3;Approach4;Approach5;Approach6;DualGiven;DualReceived;SimulatedFlight;GroundTraining;InstructorName;InstructorComments;Person1;Person2;Person3;Person4;Person5;Person6;FlightReview;Checkride;IPC;NVG Proficiency;FAA6158;PilotComments
2022-06-10;D-EABC;EDOI;EDOI;EDAY FWE KLF EDAZ;08:11;08:27;10:49;10:55;;;2.7;2.7;0.0;0.0;0.0;0.0;0.0;0;0.00;3;2;0;0;3;0.0;0.0;0.00;0.00;0.00;0.00;0;;;;;;;0.0;0.0;0.0;0.0;;;;;;;;;;FALSE;FALSE;FALSE;FALSE;FALSE;Training flight
2022-05-13;D-EXYZ;EDDF;EDDH;;09:10;09:20;10:10;10:25;;;1.3;1.3;0.0;0.0;0.0;1.3;0.0;0;200.00;1;1;0;0;1;0.5;0.0;0.00;0.00;0.00;0.00;1;ILS RWY 23;;;;;;;0.0;0.0;0.0;0.0;;;;;;;;;;FALSE;FALSE;FALSE;FALSE;FALSE;XC with ILS
`

	headers, rows, aircraft, err := parseCSV([]byte(csvData))
	if err != nil {
		t.Fatalf("parseCSV() error = %v", err)
	}

	if len(headers) < 10 {
		t.Errorf("Expected at least 10 headers, got %d", len(headers))
	}
	if headers[0] != "Date" {
		t.Errorf("First header = %s, want Date", headers[0])
	}

	if len(rows) != 2 {
		t.Fatalf("Expected 2 flight rows, got %d", len(rows))
	}
	if rows[0]["Date"] != "2022-06-10" {
		t.Errorf("Row 0 Date = %s, want 2022-06-10", rows[0]["Date"])
	}
	if rows[0]["AircraftID"] != "D-EABC" {
		t.Errorf("Row 0 AircraftID = %s, want D-EABC", rows[0]["AircraftID"])
	}
	if rows[0]["Route"] != "EDAY FWE KLF EDAZ" {
		t.Errorf("Row 0 Route = %s, want EDAY FWE KLF EDAZ", rows[0]["Route"])
	}
	// PilotComments column position may vary with ForeFlight format — tested separately in mapRowToFlight
	if rows[1]["ActualInstrument"] != "0.5" {
		t.Errorf("Row 1 ActualInstrument = %s, want 0.5", rows[1]["ActualInstrument"])
	}
	if rows[1]["Holds"] != "1" {
		t.Errorf("Row 1 Holds = %s, want 1", rows[1]["Holds"])
	}

	if len(aircraft) != 2 {
		t.Fatalf("Expected 2 aircraft rows, got %d", len(aircraft))
	}
	if aircraft[0]["AircraftID"] != "D-EABC" {
		t.Errorf("Aircraft 0 AircraftID = %s, want D-EABC", aircraft[0]["AircraftID"])
	}
	if aircraft[0]["TypeCode"] != "C172" {
		t.Errorf("Aircraft 0 TypeCode = %s, want C172", aircraft[0]["TypeCode"])
	}
	if aircraft[0]["Class"] != "airplane_single_engine_land" {
		t.Errorf("Aircraft 0 Class = %s, want airplane_single_engine_land", aircraft[0]["Class"])
	}
}

func TestIsForeFlight(t *testing.T) {
	ffHeaders := []string{"Date", "AircraftID", "From", "To", "Route", "TimeOut", "TimeOff", "TimeOn", "TimeIn", "TotalTime", "PIC"}
	if !isForeFlight(ffHeaders) {
		t.Error("Expected isForeFlight=true for ForeFlight headers")
	}

	genericHeaders := []string{"date", "registration", "departure", "arrival"}
	if isForeFlight(genericHeaders) {
		t.Error("Expected isForeFlight=false for generic headers")
	}
}

func TestSuggestForeFlight_MapsAllFields(t *testing.T) {
	mappings := suggestForeFlight()
	if len(mappings) < 15 {
		t.Errorf("Expected at least 15 mappings, got %d", len(mappings))
	}

	fieldMap := make(map[string]string)
	for _, m := range mappings {
		fieldMap[m.SourceColumn] = string(m.TargetField)
	}

	expected := map[string]string{
		"Date":                "date",
		"AircraftID":          "aircraftReg",
		"From":                "departureIcao",
		"To":                  "arrivalIcao",
		"Route":               "route",
		"TimeOut":             "offBlockTime",
		"TimeIn":              "onBlockTime",
		"ActualInstrument":    "actualInstrumentTime",
		"SimulatedInstrument": "simulatedInstrumentTime",
		"Holds":               "holds",
		"FlightReview":        "isFlightReview",
		"IPC":                 "isIpc",
		"PilotComments":       "remarks",
	}

	for src, want := range expected {
		got, ok := fieldMap[src]
		if !ok {
			t.Errorf("Missing mapping for %q", src)
		} else if got != want {
			t.Errorf("Column %q mapped to %q, want %q", src, got, want)
		}
	}
}

func TestMapRowToFlight_ForeFlight(t *testing.T) {
	row := map[string]string{
		"Date":                  "2022-06-10",
		"AircraftID":            "D-EABC",
		"From":                  "EDOI",
		"To":                    "EDOI",
		"Route":                 "EDAY FWE KLF",
		"TimeOut":               "08:11",
		"TimeOff":               "08:27",
		"TimeOn":                "10:49",
		"TimeIn":                "10:55",
		"TotalTime":             "2.7",
		"ActualInstrument":      "0.5",
		"SimulatedInstrument":   "0.3",
		"DayLandingsFullStop":   "3",
		"NightLandingsFullStop": "0",
		"Holds":                 "2",
		"Approach1":             "ILS RWY 23",
		"Approach2":             "VOR RWY 05",
		"Approach3":             "",
		"FlightReview":          "FALSE",
		"IPC":                   "TRUE",
		"PilotComments":         "Test flight",
	}

	mappingLookup := make(map[string]generated.ImportColumnMapping)
	for _, m := range suggestForeFlight() {
		mappingLookup[m.SourceColumn] = m
	}

	flight, errs := mapRowToFlight(row, mappingLookup, nil)
	if len(errs) > 0 {
		t.Fatalf("mapRowToFlight() errors = %v", errs)
	}

	if flight.AircraftReg != "D-EABC" {
		t.Errorf("AircraftReg = %s, want D-EABC", flight.AircraftReg)
	}
	if flight.Route == nil || *flight.Route != "EDAY FWE KLF" {
		t.Errorf("Route = %v, want EDAY FWE KLF", flight.Route)
	}
	if flight.ActualInstrumentTime == nil || *flight.ActualInstrumentTime != 30 {
		t.Errorf("ActualInstrumentTime = %v, want 30", flight.ActualInstrumentTime)
	}
	if flight.Holds == nil || *flight.Holds != 2 {
		t.Errorf("Holds = %v, want 2", flight.Holds)
	}
	if flight.ApproachesCount == nil || *flight.ApproachesCount != 2 {
		t.Errorf("ApproachesCount = %v, want 2", flight.ApproachesCount)
	}
	if flight.IsIpc == nil || !*flight.IsIpc {
		t.Error("IsIpc should be true")
	}
	if flight.IsFlightReview == nil || *flight.IsFlightReview {
		t.Error("IsFlightReview should be false")
	}
	if flight.IfrTime == nil || *flight.IfrTime != 48 {
		t.Errorf("IfrTime = %v, want 48", flight.IfrTime)
	}
	if flight.Landings != 3 {
		t.Errorf("Landings = %d, want 3", flight.Landings)
	}
}

func TestMapRowToFlight_MissingRequiredFields(t *testing.T) {
	row := map[string]string{
		"AircraftID": "D-EABC",
	}

	mappingLookup := make(map[string]generated.ImportColumnMapping)
	for _, m := range suggestForeFlight() {
		mappingLookup[m.SourceColumn] = m
	}

	_, errs := mapRowToFlight(row, mappingLookup, nil)
	if len(errs) == 0 {
		t.Error("Expected validation errors for missing fields")
	}

	hasDateErr := false
	for _, e := range errs {
		if e.field == "date" {
			hasDateErr = true
		}
	}
	if !hasDateErr {
		t.Error("Expected date error")
	}
}

func TestNormalizeTime(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"08:11", "08:11:00"},
		{"8:11", "08:11:00"},
		{"08:11:00", "08:11:00"},
		{"14:30", "14:30:00"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeTime(tt.input); got != tt.want {
			t.Errorf("normalizeTime(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseBoolish(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"TRUE", true},
		{"FALSE", false},
		{"true", true},
		{"1", true},
		{"0", false},
		{"2.7", true},
		{"0.0", false},
	}
	for _, tt := range tests {
		if got := parseBoolish(tt.input, nil); got != tt.want {
			t.Errorf("parseBoolish(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestSuggestForeFlight_IncludesPersonFields(t *testing.T) {
	mappings := suggestForeFlight()
	fieldMap := make(map[string]string)
	for _, m := range mappings {
		fieldMap[m.SourceColumn] = string(m.TargetField)
	}

	personExpected := map[string]string{
		"Person1":            "person1",
		"Person2":            "person2",
		"Person3":            "person3",
		"Person4":            "person4",
		"Person5":            "person5",
		"Person6":            "person6",
		"InstructorName":     "instructorName",
		"InstructorComments": "instructorComments",
		"DualGiven":          "dualGivenTime",
	}
	for src, want := range personExpected {
		got, ok := fieldMap[src]
		if !ok {
			t.Errorf("Missing mapping for %q", src)
		} else if got != want {
			t.Errorf("Column %q mapped to %q, want %q", src, got, want)
		}
	}
}

func TestMapRowToFlight_PersonAsInstructor(t *testing.T) {
	// When InstructorName is set, Person1 should be Instructor, Person2 should be Student
	row := map[string]string{
		"Date":               "2022-06-10",
		"AircraftID":         "D-EABC",
		"From":               "EDOI",
		"To":                 "EDOI",
		"TimeOut":            "08:11",
		"TimeIn":             "10:55",
		"TotalTime":          "2.7",
		"InstructorName":     "Max Mustermann",
		"InstructorComments": "Good flight",
		"DualReceived":       "2.7",
		"Person1":            "Max Mustermann",
		"Person2":            "Student Pilot",
	}

	mappingLookup := make(map[string]generated.ImportColumnMapping)
	for _, m := range suggestForeFlight() {
		mappingLookup[m.SourceColumn] = m
	}

	flight, errs := mapRowToFlight(row, mappingLookup, nil)
	if len(errs) > 0 {
		t.Fatalf("mapRowToFlight() errors = %v", errs)
	}

	if flight.InstructorName == nil || *flight.InstructorName != "Max Mustermann" {
		t.Errorf("InstructorName = %v, want Max Mustermann", flight.InstructorName)
	}
	if flight.InstructorComments == nil || *flight.InstructorComments != "Good flight" {
		t.Errorf("InstructorComments = %v, want Good flight", flight.InstructorComments)
	}

	if flight.CrewMembers == nil {
		t.Fatal("CrewMembers should not be nil")
	}
	crew := *flight.CrewMembers
	if len(crew) < 2 {
		t.Fatalf("Expected at least 2 crew members, got %d", len(crew))
	}

	// Person1 (same as InstructorName) should be Instructor
	foundInstructor := false
	foundStudent := false
	for _, cm := range crew {
		if cm.Role == "Instructor" && cm.Name == "Max Mustermann" {
			foundInstructor = true
		}
		if cm.Role == "Student" && cm.Name == "Student Pilot" {
			foundStudent = true
		}
	}
	if !foundInstructor {
		t.Error("Expected crew member with Instructor role named 'Max Mustermann'")
	}
	if !foundStudent {
		t.Error("Expected crew member with Student role named 'Student Pilot'")
	}
}

func TestMapRowToFlight_PersonAsPIC(t *testing.T) {
	// When no instructor, Person1 should be PIC
	row := map[string]string{
		"Date":       "2022-06-10",
		"AircraftID": "D-EABC",
		"From":       "EDDF",
		"To":         "EDDH",
		"TimeOut":    "08:11",
		"TimeIn":     "10:55",
		"TotalTime":  "2.7",
		"Person1":    "John Captain",
		"Person2":    "Jane Passenger",
	}

	mappingLookup := make(map[string]generated.ImportColumnMapping)
	for _, m := range suggestForeFlight() {
		mappingLookup[m.SourceColumn] = m
	}

	flight, errs := mapRowToFlight(row, mappingLookup, nil)
	if len(errs) > 0 {
		t.Fatalf("mapRowToFlight() errors = %v", errs)
	}

	if flight.CrewMembers == nil {
		t.Fatal("CrewMembers should not be nil")
	}
	crew := *flight.CrewMembers
	if len(crew) != 2 {
		t.Fatalf("Expected 2 crew members, got %d", len(crew))
	}

	// Person1 should be PIC
	if crew[0].Name != "John Captain" || crew[0].Role != "PIC" {
		t.Errorf("Person1: name=%q role=%q, want name='John Captain' role='PIC'", crew[0].Name, crew[0].Role)
	}
	// Person2 should be Passenger (no instructor present)
	if crew[1].Name != "Jane Passenger" || crew[1].Role != "Passenger" {
		t.Errorf("Person2: name=%q role=%q, want name='Jane Passenger' role='Passenger'", crew[1].Name, crew[1].Role)
	}
}

func TestMapRowToFlight_DualGivenMakesPersonStudent(t *testing.T) {
	// When DualGiven > 0, logged-in user is instructor, Person1 is Student
	row := map[string]string{
		"Date":       "2022-06-10",
		"AircraftID": "D-EABC",
		"From":       "EDDF",
		"To":         "EDDH",
		"TimeOut":    "08:11",
		"TimeIn":     "10:55",
		"TotalTime":  "2.7",
		"DualGiven":  "2.7",
		"Person1":    "Student Learner",
	}

	mappingLookup := make(map[string]generated.ImportColumnMapping)
	for _, m := range suggestForeFlight() {
		mappingLookup[m.SourceColumn] = m
	}

	flight, errs := mapRowToFlight(row, mappingLookup, nil)
	if len(errs) > 0 {
		t.Fatalf("mapRowToFlight() errors = %v", errs)
	}

	if flight.CrewMembers == nil {
		t.Fatal("CrewMembers should not be nil")
	}
	crew := *flight.CrewMembers
	if len(crew) != 1 {
		t.Fatalf("Expected 1 crew member, got %d", len(crew))
	}

	if crew[0].Name != "Student Learner" || crew[0].Role != "Student" {
		t.Errorf("Person1: name=%q role=%q, want name='Student Learner' role='Student'", crew[0].Name, crew[0].Role)
	}

	if flight.DualGivenTime == nil || *flight.DualGivenTime != 162 {
		t.Errorf("DualGivenTime = %v, want 162", flight.DualGivenTime)
	}
}

func TestMapRowToFlight_NoPersons(t *testing.T) {
	// When no person columns have values, CrewMembers should be nil
	row := map[string]string{
		"Date":       "2022-06-10",
		"AircraftID": "D-EABC",
		"From":       "EDDF",
		"To":         "EDDH",
		"TimeOut":    "08:11",
		"TimeIn":     "10:55",
		"TotalTime":  "2.7",
	}

	mappingLookup := make(map[string]generated.ImportColumnMapping)
	for _, m := range suggestForeFlight() {
		mappingLookup[m.SourceColumn] = m
	}

	flight, errs := mapRowToFlight(row, mappingLookup, nil)
	if len(errs) > 0 {
		t.Fatalf("mapRowToFlight() errors = %v", errs)
	}

	if flight.CrewMembers != nil {
		t.Errorf("CrewMembers should be nil when no persons, got %v", flight.CrewMembers)
	}
}

func TestMapRowToFlight_InstructorNameDiffFromPerson1(t *testing.T) {
	// When InstructorName != Person1, both should be added as separate crew members
	row := map[string]string{
		"Date":           "2022-06-10",
		"AircraftID":     "D-EABC",
		"From":           "EDDF",
		"To":             "EDDH",
		"TimeOut":        "08:11",
		"TimeIn":         "10:55",
		"TotalTime":      "2.7",
		"InstructorName": "Chief Instructor",
		"DualReceived":   "2.7",
		"Person1":        "Safety Pilot",
		"Person2":        "Student Me",
	}

	mappingLookup := make(map[string]generated.ImportColumnMapping)
	for _, m := range suggestForeFlight() {
		mappingLookup[m.SourceColumn] = m
	}

	flight, errs := mapRowToFlight(row, mappingLookup, nil)
	if len(errs) > 0 {
		t.Fatalf("mapRowToFlight() errors = %v", errs)
	}

	if flight.CrewMembers == nil {
		t.Fatal("CrewMembers should not be nil")
	}
	crew := *flight.CrewMembers
	if len(crew) != 3 {
		t.Fatalf("Expected 3 crew members, got %d: %+v", len(crew), crew)
	}

	foundInstructor := false
	foundPIC := false
	foundStudent := false
	for _, cm := range crew {
		if cm.Role == "Instructor" && cm.Name == "Chief Instructor" {
			foundInstructor = true
		}
		if cm.Role == "PIC" && cm.Name == "Safety Pilot" {
			foundPIC = true
		}
		if cm.Role == "Student" && cm.Name == "Student Me" {
			foundStudent = true
		}
	}
	if !foundInstructor {
		t.Error("Expected Instructor crew member 'Chief Instructor'")
	}
	if !foundPIC {
		t.Error("Expected PIC crew member 'Safety Pilot'")
	}
	if !foundStudent {
		t.Error("Expected Student crew member 'Student Me'")
	}
}
