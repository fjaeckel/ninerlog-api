package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/importtemplate"
)

type stubFlightImportRepo struct{ records []*models.FlightImport }

func (s *stubFlightImportRepo) Create(_ context.Context, rec *models.FlightImport) error {
	s.records = append(s.records, rec)
	return nil
}
func (s *stubFlightImportRepo) CountByUserID(context.Context, uuid.UUID) (int, error) {
	return len(s.records), nil
}
func (s *stubFlightImportRepo) ListByUserID(context.Context, uuid.UUID, int, int) ([]*models.FlightImport, error) {
	return s.records, nil
}
func (s *stubFlightImportRepo) GetByIDForUser(context.Context, uuid.UUID, uuid.UUID) (*models.FlightImport, error) {
	return nil, errors.New("not found")
}
func (s *stubFlightImportRepo) DeleteByUserID(context.Context, uuid.UUID) error {
	s.records = nil
	return nil
}

type stubFlightCrewRepo struct {
	saved map[uuid.UUID][]models.FlightCrewMember
	err   error
}

func (s *stubFlightCrewRepo) SetCrewMembers(_ context.Context, flightID uuid.UUID, members []models.FlightCrewMember) error {
	if s.err != nil {
		return s.err
	}
	s.saved[flightID] = members
	return nil
}
func (s *stubFlightCrewRepo) GetByFlightID(_ context.Context, flightID uuid.UUID) ([]models.FlightCrewMember, error) {
	return s.saved[flightID], nil
}
func (s *stubFlightCrewRepo) GetByFlightIDs(context.Context, []uuid.UUID) (map[uuid.UUID][]models.FlightCrewMember, error) {
	return s.saved, nil
}
func (s *stubFlightCrewRepo) DeleteByFlightID(_ context.Context, flightID uuid.UUID) error {
	delete(s.saved, flightID)
	return nil
}

type importConfirmEnv struct {
	h        *APIHandler
	userID   uuid.UUID
	flights  *mockFlightRepo
	aircraft *mockAircraftRepo
	crew     *stubFlightCrewRepo
}

func newImportConfirmEnv(t *testing.T, userName string) *importConfirmEnv {
	t.Helper()
	h, userRepo := setupTestHandler()
	user := &models.User{Email: uuid.NewString() + "@example.com", Name: userName}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("creating user: %v", err)
	}
	env := &importConfirmEnv{
		h:        h,
		userID:   user.ID,
		flights:  newMockFlightRepo(),
		aircraft: newMockAircraftRepo(),
		crew:     &stubFlightCrewRepo{saved: map[uuid.UUID][]models.FlightCrewMember{}},
	}
	h.flightService = service.NewFlightService(env.flights, nil)
	h.aircraftService = service.NewAircraftService(env.aircraft)
	h.flightImportRepo = &stubFlightImportRepo{}
	h.flightCrewRepo = env.crew
	env.flights.crew = env.crew
	return env
}

// confirm uploads data into a session with its suggested mappings and runs
// ConfirmImport, returning the result and the stored flights in date and
// off-block order.
func (e *importConfirmEnv) confirm(t *testing.T, data []byte) (generated.ImportResult, []*models.Flight) {
	t.Helper()
	columns, rows, aircraft, err := parseCSV(data)
	if err != nil {
		t.Fatalf("parseCSV: %v", err)
	}
	_, format, mappings := detectImportFormat(columns)
	token := uuid.NewString()
	sessionMu.Lock()
	uploadSessions[token] = &uploadSession{
		userID: e.userID, fileName: "test.csv", format: format, columns: columns,
		rows: rows, aircraft: aircraft, mappings: mappings, createdAt: time.Now(),
	}
	sessionMu.Unlock()

	w := httptest.NewRecorder()
	c := authenticatedContext(w, e.userID)
	body, _ := json.Marshal(map[string]string{"uploadToken": token})
	c.Request = httptest.NewRequest(http.MethodPost, "/imports/confirm", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	e.h.ConfirmImport(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("ConfirmImport status = %d: %s", w.Code, w.Body.String())
	}
	var result generated.ImportResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decoding result: %v", err)
	}

	var flights []*models.Flight
	for _, f := range e.flights.flights {
		flights = append(flights, f)
	}
	sort.Slice(flights, func(i, j int) bool {
		if !flights[i].Date.Equal(flights[j].Date) {
			return flights[i].Date.Before(flights[j].Date)
		}
		return safeStr(flights[i].OffBlockTime) < safeStr(flights[j].OffBlockTime)
	})
	return result, flights
}

func (e *importConfirmEnv) aircraftByReg(reg string) *models.Aircraft {
	for _, a := range e.aircraft.aircraft {
		if a.Registration == reg {
			return a
		}
	}
	return nil
}

func readImportFixture(t *testing.T, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", rel))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return data
}

func TestConfirmImport_ForeFlightSampleKeepsNightLandings(t *testing.T) {
	airports.SetTestDB(nil)
	env := newImportConfirmEnv(t, "Alex Rivera")
	result, flights := env.confirm(t, readImportFixture(t, "importsamples/foreflight.csv"))

	if result.ImportedCount != 2 || len(flights) != 2 {
		t.Fatalf("imported %d (stored %d), want 2; errors %+v", result.ImportedCount, len(flights), result.Errors)
	}

	day := flights[0]
	if day.LandingsDay != 2 || day.LandingsNight != 0 || day.AllLandings != 2 {
		t.Errorf("EDDF-EDDM landings day/night/all = %d/%d/%d, want 2/0/2",
			day.LandingsDay, day.LandingsNight, day.AllLandings)
	}
	if day.TakeoffsDay != 2 || day.TakeoffsNight != 0 {
		t.Errorf("EDDF-EDDM takeoffs day/night = %d/%d, want 2/0", day.TakeoffsDay, day.TakeoffsNight)
	}

	night := flights[1]
	if night.LandingsDay != 0 || night.LandingsNight != 3 || night.AllLandings != 3 {
		t.Errorf("KSFO-KOAK landings day/night/all = %d/%d/%d, want 0/3/3",
			night.LandingsDay, night.LandingsNight, night.AllLandings)
	}
	if !night.LandingsDayOverride || !night.LandingsNightOverride {
		t.Error("KSFO-KOAK landing split must be stored with its override flags")
	}
	if night.TakeoffsDay != 0 || night.TakeoffsNight != 3 {
		t.Errorf("KSFO-KOAK takeoffs day/night = %d/%d, want 0/3", night.TakeoffsDay, night.TakeoffsNight)
	}
	if !night.TakeoffsDayOverride || !night.TakeoffsNightOverride {
		t.Error("KSFO-KOAK takeoff split must be stored with its override flags")
	}

	for _, f := range flights {
		if f.IsPassenger || f.SICTime != 0 || f.SICTimeOverride {
			t.Errorf("%s: passenger=%v sic=%d sicOverride=%v, want a PIC flight with no SIC time",
				f.AircraftReg, f.IsPassenger, f.SICTime, f.SICTimeOverride)
		}
	}
}

func TestConfirmImport_AirlineFirstOfficerKeepsCoPilotTime(t *testing.T) {
	env := newImportConfirmEnv(t, "Mark Feld")
	result, flights := env.confirm(t, readImportFixture(t, "airline-fo-easa.csv"))

	if result.ImportedCount != 2 || len(flights) != 2 {
		t.Fatalf("imported %d (stored %d), want 2; errors %+v", result.ImportedCount, len(flights), result.Errors)
	}
	ac := env.aircraftByReg("D-AIZX")
	if ac == nil || !ac.IsMultiPilot {
		t.Fatalf("D-AIZX created as %+v, want a multi-pilot fleet entry", ac)
	}
	for _, f := range flights {
		if f.IsPassenger {
			t.Errorf("%s sector imported as a passenger flight", safeStr(f.DepartureICAO))
		}
		if f.TotalTime != 130 || f.SICTime != 130 || f.MultiPilotTime != 130 || f.PICTime != 0 {
			t.Errorf("%s total/sic/mp/pic = %d/%d/%d/%d, want 130/130/130/0", safeStr(f.DepartureICAO),
				f.TotalTime, f.SICTime, f.MultiPilotTime, f.PICTime)
		}
	}
	if flights[1].LandingsDay != 1 {
		t.Errorf("pilot-flying sector day landings = %d, want 1", flights[1].LandingsDay)
	}
}

func TestConfirmImport_ForeFlightFirstOfficerSICColumn(t *testing.T) {
	csv := "ForeFlight Logbook Import\n\nAircraft Table\n" +
		"AircraftID,TypeCode,Make,Model,Class\n" +
		"D-AIZY,A320,Airbus,A320-214,airplane_multi_engine_land\n" +
		"\nFlights Table\n" +
		"Date,AircraftID,From,To,TimeOut,TimeIn,TotalTime,PIC,SIC,DayLandingsFullStop,NightLandingsFullStop,AllLandings,Person1,Person2,PilotComments\n" +
		"2026-04-02,D-AIZY,EDDF,LEPA,06:10,08:20,2.2,0.0,2.2,0,0,0,Erika Sommer;PIC;,,SIC column\n" +
		"2026-04-02,D-AIZY,LEPA,EDDF,09:05,11:15,2.2,0.0,,1,0,1,Erika Sommer;PIC;,Self;SIC;,Self listed as SIC\n"
	env := newImportConfirmEnv(t, "Mark Feld")
	result, flights := env.confirm(t, []byte(csv))

	if result.ImportedCount != 2 || len(flights) != 2 {
		t.Fatalf("imported %d (stored %d), want 2; errors %+v", result.ImportedCount, len(flights), result.Errors)
	}
	if ac := env.aircraftByReg("D-AIZY"); ac == nil || ac.IsMultiPilot {
		t.Fatalf("D-AIZY created as %+v; the file carries no multi-pilot fact", ac)
	}
	for _, f := range flights {
		if f.IsPassenger || f.TotalTime != 130 || f.SICTime != 130 || !f.SICTimeOverride {
			t.Errorf("%s passenger=%v total=%d sic=%d sicOverride=%v, want 130 min SIC time",
				safeStr(f.DepartureICAO), f.IsPassenger, f.TotalTime, f.SICTime, f.SICTimeOverride)
		}
		if f.MultiPilotTime != 0 {
			t.Errorf("%s multi-pilot time = %d, want 0 on an aircraft not known to be multi-pilot",
				safeStr(f.DepartureICAO), f.MultiPilotTime)
		}
	}
}

func TestConfirmImport_KnownMultiPilotAircraftKeepsCoPilotTime(t *testing.T) {
	env := newImportConfirmEnv(t, "Mark Feld")
	if err := env.aircraft.Create(context.Background(), &models.Aircraft{
		UserID: env.userID, Registration: "D-AIZX", Type: "A320", Make: "Airbus", Model: "A320",
		IsActive: true, IsMultiPilot: true,
	}); err != nil {
		t.Fatal(err)
	}
	csv := "Date,Reg,Type,From,To,Off Block,On Block,PIC Name\n" +
		"2026-03-12,D-AIZX,A320,EDDF,LEPA,06:10,08:20,Erika Sommer\n"
	_, flights := env.confirm(t, []byte(csv))
	if len(flights) != 1 {
		t.Fatalf("stored %d flights, want 1", len(flights))
	}
	f := flights[0]
	if f.IsPassenger || f.TotalTime != 130 || f.SICTime != 130 || f.MultiPilotTime != 130 {
		t.Errorf("passenger=%v total=%d sic=%d mp=%d, want 130 min SIC and multi-pilot time",
			f.IsPassenger, f.TotalTime, f.SICTime, f.MultiPilotTime)
	}
}

func TestConfirmImport_UnknownAircraftWithoutCrewTimeIsNotInvented(t *testing.T) {
	env := newImportConfirmEnv(t, "Mark Feld")
	csv := "Date,Reg,Type,From,To,Off Block,On Block,PIC Name\n" +
		"2026-03-12,D-AIZX,A320,EDDF,LEPA,06:10,08:20,Erika Sommer\n"
	_, flights := env.confirm(t, []byte(csv))
	if len(flights) != 1 {
		t.Fatalf("stored %d flights, want 1", len(flights))
	}
	if ac := env.aircraftByReg("D-AIZX"); ac == nil || ac.IsMultiPilot {
		t.Errorf("D-AIZX created as %+v, want a fleet entry not marked multi-pilot", ac)
	}
	if f := flights[0]; !f.IsPassenger || f.SICTime != 0 {
		t.Errorf("passenger=%v sic=%d, want the third-party-PIC row on a single-pilot aircraft unchanged",
			f.IsPassenger, f.SICTime)
	}
}

func TestConfirmImport_SingleLandingsTotalKeepsSolarSplit(t *testing.T) {
	airports.SetTestDB(map[string]airports.AirportInfo{
		"KSFO": {ICAO: "KSFO", Latitude: 37.6189, Longitude: -122.375},
		"KOAK": {ICAO: "KOAK", Latitude: 37.7213, Longitude: -122.2208},
	})
	defer airports.SetTestDB(nil)

	env := newImportConfirmEnv(t, "Alex Rivera")
	csv := "Date,Reg,Type,From,To,Off Block,On Block,Landings\n" +
		"2025-11-03,N54321,PA28,KSFO,KOAK,04:40,05:25,3\n" +
		"2025-11-04,N54321,PA28,KOAK,KSFO,20:00,20:45,2\n"
	_, flights := env.confirm(t, []byte(csv))
	if len(flights) != 2 {
		t.Fatalf("stored %d flights, want 2", len(flights))
	}
	if f := flights[0]; f.LandingsNight != 3 || f.LandingsDay != 0 || f.LandingsNightOverride || f.LandingsDayOverride {
		t.Errorf("night arrival day/night = %d/%d overrides %v/%v, want 0/3 derived",
			f.LandingsDay, f.LandingsNight, f.LandingsDayOverride, f.LandingsNightOverride)
	}
	if f := flights[1]; f.LandingsDay != 2 || f.LandingsNight != 0 || f.LandingsDayOverride || f.LandingsNightOverride {
		t.Errorf("day arrival day/night = %d/%d overrides %v/%v, want 2/0 derived",
			f.LandingsDay, f.LandingsNight, f.LandingsDayOverride, f.LandingsNightOverride)
	}
}

func TestConfirmImport_CrewSaveFailureIsReported(t *testing.T) {
	env := newImportConfirmEnv(t, "Alex Rivera")
	env.crew.err = errors.New("crew table unavailable")
	csv := "Date,Reg,Type,From,To,Off Block,On Block,Landings,Instructor Name\n" +
		"2026-05-01,D-EABC,C172,EDDF,EDDM,08:00,09:00,1,Jordan Lake\n"
	result, flights := env.confirm(t, []byte(csv))
	if len(flights) != 1 || result.ImportedCount != 1 {
		t.Fatalf("imported %d (stored %d), want 1", result.ImportedCount, len(flights))
	}
	if result.Status != "partial" {
		t.Errorf("status = %q, want partial", result.Status)
	}
	if result.Errors == nil || len(*result.Errors) != 1 || (*result.Errors)[0].Field != "crewMembers" {
		t.Errorf("errors = %+v, want one crewMembers error", result.Errors)
	}
}

func TestBuildImportedFlight_LandingSplit(t *testing.T) {
	ip := func(n int) *int { return &n }
	tests := []struct {
		name               string
		all                int
		day, night         *int
		wantDay, wantNight int
		wantOverride       bool
	}{
		{"split only", 3, ip(0), ip(3), 0, 3, true},
		{"total above split adds to day", 4, ip(0), ip(2), 2, 2, true},
		{"night only column", 1, nil, ip(1), 0, 1, true},
		{"zero split falls back to derivation", 3, ip(0), ip(0), 0, 0, false},
		{"no split columns", 2, nil, nil, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := buildImportedFlight(uuid.New(), generated.FlightCreate{Landings: ip(tt.all)},
				importRowExtras{landingsDay: tt.day, landingsNight: tt.night})
			if f.LandingsDay != tt.wantDay || f.LandingsNight != tt.wantNight ||
				f.LandingsDayOverride != tt.wantOverride || f.LandingsNightOverride != tt.wantOverride {
				t.Errorf("day/night = %d/%d overrides %v/%v, want %d/%d %v",
					f.LandingsDay, f.LandingsNight, f.LandingsDayOverride, f.LandingsNightOverride,
					tt.wantDay, tt.wantNight, tt.wantOverride)
			}
			if f.AllLandings != tt.all {
				t.Errorf("allLandings = %d, want %d", f.AllLandings, tt.all)
			}
		})
	}
}

func TestMapImportRow_RemappedAuxColumnIsNotAuxiliary(t *testing.T) {
	headers := []string{"Date", "AircraftID", "From", "To", "TotalTime", "SIC", "PilotComments"}
	tpl, _, mappings := detectImportFormat(headers)
	aux := importtemplate.AuxColumns(tpl, headers)
	if aux["SIC"] != importtemplate.AuxSICTime {
		t.Fatalf("aux = %v, want SIC read as SIC time", aux)
	}
	row := map[string]string{"Date": "2026-04-02", "AircraftID": "D-AIZY", "From": "EDDF", "To": "LEPA", "TotalTime": "2.2", "SIC": "2.2"}

	lookup := toMappingLookup(mappings)
	flight, _, _ := mapImportRow(row, lookup, aux, nil)
	if flight.SicTime == nil || *flight.SicTime != 132 {
		t.Errorf("sicTime = %v, want 132", flight.SicTime)
	}

	lookup["SIC"] = generated.ImportColumnMapping{SourceColumn: "SIC", TargetField: generated.ImportFieldRemarks}
	flight, _, _ = mapImportRow(row, lookup, aux, nil)
	if flight.SicTime != nil {
		t.Errorf("sicTime = %d, want unset once the column is mapped elsewhere", *flight.SicTime)
	}
}
