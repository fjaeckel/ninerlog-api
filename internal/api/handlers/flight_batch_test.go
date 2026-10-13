package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func postFlightBatch(t *testing.T, h *APIHandler, userID *uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	var c *gin.Context
	if userID != nil {
		c = authenticatedContext(w, *userID)
	} else {
		c, _ = gin.CreateTestContext(w)
	}
	c.Request = httptest.NewRequest("POST", "/flights/batch", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.CreateFlightBatch(c)
	return w
}

// winchLegs returns n 8-minute legs starting at 10:00 UTC.
func winchLegs(n int) string {
	legs := make([]string, n)
	for i := range legs {
		start := 10*60 + i*10
		legs[i] = fmt.Sprintf(`{"departureTime":"%02d:%02d:00","arrivalTime":"%02d:%02d:00"}`,
			start/60, start%60, (start+8)/60, (start+8)%60)
	}
	return "[" + strings.Join(legs, ",") + "]"
}

const lenaTemplate = `{"date":"2026-05-09","aircraftReg":"D-1234","aircraftType":"ASK21",
	"departureIcao":"Hausen am Albis","arrivalIcao":"Hausen am Albis","launchMethod":"winch"}`

func TestCreateFlightBatch_Handler(t *testing.T) {
	userID := uuid.New()
	tests := []struct {
		name       string
		userID     *uuid.UUID
		body       string
		wantStatus int
		wantMsg    string
		wantLegs   int
	}{
		{
			name:       "L1 six winch circuits",
			userID:     &userID,
			body:       `{"template":` + lenaTemplate + `,"legs":` + winchLegs(6) + `}`,
			wantStatus: http.StatusCreated,
			wantLegs:   6,
		},
		{
			name:   "template block times do not reach legs with their own times",
			userID: &userID,
			body: `{"template":{"date":"2026-05-09","aircraftReg":"D-1234","aircraftType":"ASK21",
				"departureIcao":"Hausen am Albis","arrivalIcao":"Hausen am Albis","launchMethod":"winch",
				"offBlockTime":"09:00:00","onBlockTime":"12:00:00"},"legs":` + winchLegs(3) + `}`,
			wantStatus: http.StatusCreated,
			wantLegs:   3,
		},
		{
			name:       "unauthenticated",
			body:       `{"template":` + lenaTemplate + `,"legs":` + winchLegs(1) + `}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "malformed body",
			userID:     &userID,
			body:       `{bad`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "no legs",
			userID:     &userID,
			body:       `{"template":` + lenaTemplate + `,"legs":[]}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "legs must hold between 1 and 50 entries",
		},
		{
			name:       "51 legs",
			userID:     &userID,
			body:       `{"template":` + lenaTemplate + `,"legs":` + winchLegs(51) + `}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "legs must hold between 1 and 50 entries",
		},
		{
			name:   "leg without landing time names its index",
			userID: &userID,
			body: `{"template":` + lenaTemplate + `,"legs":[{"departureTime":"10:00:00","arrivalTime":"10:08:00"},` +
				`{"departureTime":"10:10:00"}]}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "Leg 1:",
		},
		{
			name:   "invalid release height on a leg's template names leg 0",
			userID: &userID,
			body: `{"template":{"date":"2026-05-09","aircraftReg":"D-1234","aircraftType":"ASK21",
				"departureIcao":"EDxx","arrivalIcao":"EDxx","releaseHeightM":20001},"legs":` + winchLegs(2) + `}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "Leg 0: release height must be between 0 and 20000 m",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := setupTestHandler()
			w := postFlightBatch(t, h, tt.userID, tt.body)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantMsg != "" && !strings.Contains(w.Body.String(), tt.wantMsg) {
				t.Errorf("body = %s, want it to contain %q", w.Body.String(), tt.wantMsg)
			}
			if tt.wantStatus != http.StatusCreated {
				if tt.userID != nil {
					n, _ := h.flightService.CountFlights(t.Context(), *tt.userID, nil)
					if n != 0 {
						t.Errorf("%d flights stored after a rejected batch, want 0", n)
					}
				}
				return
			}
			var res generated.FlightBatchResult
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
				t.Fatal(err)
			}
			if len(res.Flights) != tt.wantLegs {
				t.Fatalf("got %d flights, want %d", len(res.Flights), tt.wantLegs)
			}
			total, launches := 0, 0
			for _, f := range res.Flights {
				total += f.TotalTime
				launches += f.Launches
				if f.LaunchMethod == nil || *f.LaunchMethod != "winch" || f.AllLandings != 1 || f.LaunchesOverride {
					t.Errorf("leg = %+v, want a winch launch with one landing and derived launches", f)
				}
			}
			if total != 8*tt.wantLegs || launches != tt.wantLegs {
				t.Errorf("total %d min, %d launches; want %d min, %d launches", total, launches, 8*tt.wantLegs, tt.wantLegs)
			}
		})
	}
}

func TestBatchLegCreate(t *testing.T) {
	str := func(s string) *string { return &s }
	num := func(n int) *int { return &n }
	tests := []struct {
		name         string
		template     generated.FlightCreate
		leg          generated.FlightBatchLeg
		wantLandings *int
		wantLaunches *int
		wantRemarks  *string
		wantDep      *string
	}{
		{
			name:         "leg times replace the template's and landings default to 1",
			template:     generated.FlightCreate{DepartureTime: str("09:00:00"), Remarks: str("Thermik")},
			leg:          generated.FlightBatchLeg{DepartureTime: str("10:00:00"), ArrivalTime: str("10:08:00")},
			wantLandings: num(1),
			wantRemarks:  str("Thermik"),
			wantDep:      str("10:00:00"),
		},
		{
			name:         "template landings apply to every leg",
			template:     generated.FlightCreate{Landings: num(2)},
			leg:          generated.FlightBatchLeg{},
			wantLandings: num(2),
		},
		{
			name:         "leg landings, launches and remarks win",
			template:     generated.FlightCreate{Landings: num(2), Remarks: str("Thermik")},
			leg:          generated.FlightBatchLeg{Landings: num(3), Launches: num(4), Remarks: str("Seilriss")},
			wantLandings: num(3),
			wantLaunches: num(4),
			wantRemarks:  str("Seilriss"),
		},
	}
	eqInt := func(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
	eqStr := func(a, b *string) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := batchLegCreate(tt.template, tt.leg)
			if !eqInt(got.Landings, tt.wantLandings) || !eqInt(got.Launches, tt.wantLaunches) ||
				!eqStr(got.Remarks, tt.wantRemarks) || !eqStr(got.DepartureTime, tt.wantDep) {
				t.Errorf("merged = %+v", got)
			}
		})
	}
}

func TestCreateFlightBatch_CrewLinkedOnceAfterInsert(t *testing.T) {
	crewTemplate := `{"date":"2026-05-09","aircraftReg":"D-1234","aircraftType":"ASK21",
		"departureIcao":"Hausen am Albis","arrivalIcao":"Hausen am Albis","launchMethod":"winch",
		"crewMembers":[{"name":"Anna Lehrer","role":"Instructor"}]}`
	body := `{"template":` + crewTemplate + `,"legs":` + winchLegs(3) + `}`

	t.Run("failed insert creates no contact", func(t *testing.T) {
		h, _ := setupTestHandler()
		contacts := newMockContactRepo()
		flights := newMockFlightRepo()
		flights.createBatchErr = errors.New("tx aborted")
		h.contactService = service.NewContactService(contacts)
		h.flightService = service.NewFlightService(flights, nil)
		userID := uuid.New()

		w := postFlightBatch(t, h, &userID, body)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500: %s", w.Code, w.Body.String())
		}
		if len(contacts.contacts) != 0 {
			t.Errorf("%d contacts created by a failed batch, want 0", len(contacts.contacts))
		}
	})

	t.Run("template crew linked once for every leg", func(t *testing.T) {
		h, _ := setupTestHandler()
		contacts := newMockContactRepo()
		h.contactService = service.NewContactService(contacts)
		h.flightCrewRepo = newMockFlightCrewRepo()
		userID := uuid.New()

		w := postFlightBatch(t, h, &userID, body)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}
		if contacts.findOrCreateCalls != 1 || len(contacts.contacts) != 1 {
			t.Fatalf("%d lookups, %d contacts; want 1, 1", contacts.findOrCreateCalls, len(contacts.contacts))
		}
		var res generated.FlightBatchResult
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		for i, f := range res.Flights {
			if f.CrewMembers == nil || len(*f.CrewMembers) != 1 || (*f.CrewMembers)[0].ContactId == nil {
				t.Errorf("leg %d crew = %+v, want one linked member", i, f.CrewMembers)
			}
		}
	})
}

func TestCreateFlightBatch_ClientContactIDsWhenLinkingFails(t *testing.T) {
	h, _ := setupTestHandler()
	contacts := newMockContactRepo()
	contacts.findOrCreateErr = errors.New("db down")
	h.contactService = service.NewContactService(contacts)
	userID := uuid.New()
	owned := &models.Contact{ID: uuid.New(), UserID: userID, Name: "Anna Lehrer"}
	foreign := &models.Contact{ID: uuid.New(), UserID: uuid.New(), Name: "Carl Fremd"}
	contacts.contacts[owned.ID] = owned
	contacts.contacts[foreign.ID] = foreign

	template := `{"date":"2026-05-09","aircraftReg":"D-1234","aircraftType":"ASK21",
		"departureIcao":"Hausen am Albis","arrivalIcao":"Hausen am Albis","launchMethod":"winch",
		"crewMembers":[{"name":"Anna Lehrer","role":"Instructor","contactId":"` + owned.ID.String() + `"},
			{"name":"Carl Fremd","role":"Passenger","contactId":"` + foreign.ID.String() + `"},
			{"name":"Bea Neu","role":"Passenger"}]}`
	w := postFlightBatch(t, h, &userID, `{"template":`+template+`,"legs":`+winchLegs(2)+`}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
	var res generated.FlightBatchResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	for i, f := range res.Flights {
		if f.CrewMembers == nil || len(*f.CrewMembers) != 3 {
			t.Fatalf("leg %d crew = %+v", i, f.CrewMembers)
		}
		got := map[string]*uuid.UUID{}
		for _, m := range *f.CrewMembers {
			got[m.Name] = (*uuid.UUID)(m.ContactId)
		}
		if id := got["Anna Lehrer"]; id == nil || *id != owned.ID {
			t.Errorf("leg %d owned contactId = %v, want %s", i, id, owned.ID)
		}
		if id := got["Carl Fremd"]; id != nil {
			t.Errorf("leg %d foreign contactId = %s, want none", i, id)
		}
		if id := got["Bea Neu"]; id != nil {
			t.Errorf("leg %d unlinked contactId = %s, want none", i, id)
		}
	}
}

func TestCheckCrewContacts_NoContactService(t *testing.T) {
	h, _ := setupTestHandler()
	h.contactService = nil
	id := uuid.New()
	crew := []models.FlightCrewMember{{Name: "Anna", ContactID: &id}}
	if _, err := h.checkCrewContacts(context.Background(), uuid.New(), crew); err != nil {
		t.Fatal(err)
	}
	if crew[0].ContactID != nil {
		t.Errorf("contactId = %s, want cleared without a contact service", crew[0].ContactID)
	}
}

func TestCreateFlightBatch_FleetLoadedOnce(t *testing.T) {
	h, _ := setupTestHandler()
	aircraft := newMockAircraftRepo()
	h.aircraftService = service.NewAircraftService(aircraft)
	userID := uuid.New()
	ask := "GLIDER"
	aircraft.aircraft[uuid.New()] = &models.Aircraft{ID: uuid.New(), UserID: userID, Registration: "D-1234", AircraftClass: &ask}

	w := postFlightBatch(t, h, &userID, `{"template":`+lenaTemplate+`,"legs":`+winchLegs(6)+`}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
	if aircraft.getByUserIDCalls != 1 {
		t.Errorf("fleet loaded %d times for 6 legs, want 1", aircraft.getByUserIDCalls)
	}
}

type mockFlightCrewRepo struct {
	crew map[uuid.UUID][]models.FlightCrewMember
	// setErr, when set, fails a write that touches a flight it reports true for.
	setErr             func(flightID uuid.UUID) bool
	setForFlightsCalls int
}

func newMockFlightCrewRepo() *mockFlightCrewRepo {
	return &mockFlightCrewRepo{crew: map[uuid.UUID][]models.FlightCrewMember{}}
}

func (m *mockFlightCrewRepo) SetCrewMembers(_ context.Context, flightID uuid.UUID, members []models.FlightCrewMember) error {
	if m.setErr != nil && m.setErr(flightID) {
		return errors.New("db down")
	}
	m.crew[flightID] = append([]models.FlightCrewMember(nil), members...)
	return nil
}

func (m *mockFlightCrewRepo) SetCrewMembersForFlights(_ context.Context, flightIDs []uuid.UUID, members []models.FlightCrewMember) error {
	m.setForFlightsCalls++
	for _, id := range flightIDs {
		if m.setErr != nil && m.setErr(id) {
			return errors.New("db down")
		}
	}
	for _, id := range flightIDs {
		m.crew[id] = append([]models.FlightCrewMember(nil), members...)
	}
	return nil
}

func (m *mockFlightCrewRepo) GetByFlightID(_ context.Context, flightID uuid.UUID) ([]models.FlightCrewMember, error) {
	return m.crew[flightID], nil
}

func (m *mockFlightCrewRepo) GetByFlightIDs(_ context.Context, ids []uuid.UUID) (map[uuid.UUID][]models.FlightCrewMember, error) {
	out := map[uuid.UUID][]models.FlightCrewMember{}
	for _, id := range ids {
		if cm, ok := m.crew[id]; ok {
			out[id] = cm
		}
	}
	return out, nil
}

func (m *mockFlightCrewRepo) DeleteByFlightID(_ context.Context, flightID uuid.UUID) error {
	delete(m.crew, flightID)
	return nil
}

func TestCreateFlightBatch_FailedCrewWriteKeepsInsertedCrew(t *testing.T) {
	h, _ := setupTestHandler()
	h.contactService = service.NewContactService(newMockContactRepo())
	crewRepo := newMockFlightCrewRepo()
	calls := 0
	crewRepo.setErr = func(uuid.UUID) bool { calls++; return calls == 2 }
	crewRepo.crew = map[uuid.UUID][]models.FlightCrewMember{}
	h.flightCrewRepo = crewRepo
	userID := uuid.New()
	template := `{"date":"2026-05-09","aircraftReg":"D-1234","aircraftType":"ASK21",
		"departureIcao":"Hausen am Albis","arrivalIcao":"Hausen am Albis","launchMethod":"winch",
		"crewMembers":[{"name":"Anna Lehrer","role":"Instructor"}]}`

	w := postFlightBatch(t, h, &userID, `{"template":`+template+`,"legs":`+winchLegs(3)+`}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
	var res generated.FlightBatchResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	for i, f := range res.Flights {
		if f.CrewMembers == nil || len(*f.CrewMembers) != 1 {
			t.Fatalf("leg %d crew = %+v", i, f.CrewMembers)
		}
		if (*f.CrewMembers)[0].ContactId != nil {
			t.Errorf("leg %d linked after a failed write, want the crew as inserted", i)
		}
	}
	if len(crewRepo.crew) != 0 {
		t.Errorf("crew rewritten for %d legs, want none", len(crewRepo.crew))
	}
}

func TestCreateFlightBatch_LinkedCrewNotRewritten(t *testing.T) {
	h, _ := setupTestHandler()
	contacts := newMockContactRepo()
	h.contactService = service.NewContactService(contacts)
	crewRepo := newMockFlightCrewRepo()
	h.flightCrewRepo = crewRepo
	userID := uuid.New()
	owned := &models.Contact{ID: uuid.New(), UserID: userID, Name: "Anna Lehrer"}
	contacts.contacts[owned.ID] = owned
	template := `{"date":"2026-05-09","aircraftReg":"D-1234","aircraftType":"ASK21",
		"departureIcao":"Hausen am Albis","arrivalIcao":"Hausen am Albis","launchMethod":"winch",
		"crewMembers":[{"name":"Anna Lehrer","role":"Instructor","contactId":"` + owned.ID.String() + `"}]}`

	w := postFlightBatch(t, h, &userID, `{"template":`+template+`,"legs":`+winchLegs(4)+`}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
	if crewRepo.setForFlightsCalls != 0 {
		t.Errorf("crew rewritten %d times, want 0 when linking changed nothing", crewRepo.setForFlightsCalls)
	}
	if contacts.getByIDCalls != 1 {
		t.Errorf("contact looked up %d times, want 1", contacts.getByIDCalls)
	}
}

func TestCreateFlightBatch_ContactCheckErrorStoresNothing(t *testing.T) {
	h, _ := setupTestHandler()
	flights := newMockFlightRepo()
	h.flightService = service.NewFlightService(flights, nil)
	contacts := newMockContactRepo()
	contacts.getByIDErr = errors.New("db down")
	h.contactService = service.NewContactService(contacts)
	userID := uuid.New()
	template := `{"date":"2026-05-09","aircraftReg":"D-1234","aircraftType":"ASK21",
		"departureIcao":"Hausen am Albis","arrivalIcao":"Hausen am Albis","launchMethod":"winch",
		"crewMembers":[{"name":"Anna Lehrer","role":"Instructor","contactId":"` + uuid.NewString() + `"}]}`

	w := postFlightBatch(t, h, &userID, `{"template":`+template+`,"legs":`+winchLegs(2)+`}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", w.Code, w.Body.String())
	}
	if len(flights.flights) != 0 {
		t.Errorf("%d flights stored, want 0", len(flights.flights))
	}
}

func TestCreateFlight_ContactCheckErrorStoresNothing(t *testing.T) {
	h, _ := setupTestHandler()
	flights := newMockFlightRepo()
	h.flightService = service.NewFlightService(flights, nil)
	contacts := newMockContactRepo()
	contacts.getByIDErr = errors.New("db down")
	h.contactService = service.NewContactService(contacts)
	h.flightCrewRepo = newMockFlightCrewRepo()
	userID := uuid.New()
	body := `{"date":"2026-05-09","aircraftReg":"D-1234","aircraftType":"ASK21",
		"departureIcao":"Hausen am Albis","arrivalIcao":"Hausen am Albis",
		"departureTime":"10:00","arrivalTime":"10:08","landings":1,
		"crewMembers":[{"name":"Anna Lehrer","role":"Instructor","contactId":"` + uuid.NewString() + `"}]}`

	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("POST", "/flights", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.CreateFlight(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", w.Code, w.Body.String())
	}
	if len(flights.flights) != 0 {
		t.Errorf("%d flights stored, want 0", len(flights.flights))
	}
}

func TestPersistCrewMembers_LinkErrorKeepsCheckedContacts(t *testing.T) {
	h, _ := setupTestHandler()
	contacts := newMockContactRepo()
	contacts.findOrCreateErr = errors.New("db down")
	h.contactService = service.NewContactService(contacts)
	crewRepo := newMockFlightCrewRepo()
	h.flightCrewRepo = crewRepo
	userID := uuid.New()
	owned := &models.Contact{ID: uuid.New(), UserID: userID, Name: "Anna"}
	contacts.contacts[owned.ID] = owned

	flight := &models.Flight{ID: uuid.New(), UserID: userID,
		CrewMembers: []models.FlightCrewMember{{Name: "Anna", ContactID: &owned.ID}, {Name: "Bea"}}}
	linker, err := h.checkCrewContacts(context.Background(), userID, flight.CrewMembers)
	if err != nil {
		t.Fatal(err)
	}
	h.persistCrewMembers(context.Background(), linker, flight)

	got := crewRepo.crew[flight.ID]
	if len(got) != 2 || got[0].ContactID == nil || *got[0].ContactID != owned.ID || got[1].ContactID != nil {
		t.Errorf("stored crew = %+v, want Anna linked and Bea unlinked", got)
	}
}
