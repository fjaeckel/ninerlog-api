package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// deltaSyncHandler wires an APIHandler over mocks the test keeps a handle on,
// which setupTestHandler deliberately does not expose.
type deltaSyncHandler struct {
	handler     *APIHandler
	aircraft    *mockAircraftRepo
	contacts    *mockContactRepo
	credentials *mockCredentialRepo
	flights     *mockFlightRepo
}

func setupDeltaSyncHandler() *deltaSyncHandler {
	gin.SetMode(gin.TestMode)
	d := &deltaSyncHandler{
		aircraft:    newMockAircraftRepo(),
		contacts:    newMockContactRepo(),
		credentials: newMockCredentialRepo(),
		flights:     newMockFlightRepo(),
	}
	d.handler = &APIHandler{
		aircraftService:   service.NewAircraftService(d.aircraft),
		contactService:    service.NewContactService(d.contacts),
		credentialService: service.NewCredentialService(d.credentials),
		flightService:     service.NewFlightService(d.flights, nil),
	}
	return d
}

// deltaSyncContext is authenticatedContext with a request attached, which the
// list handlers need in order to reach the request context.
func deltaSyncContext(w *httptest.ResponseRecorder, userID uuid.UUID) *gin.Context {
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return c
}

// An explicitly empty ?updatedSince= binds to a pointer to the zero time. It
// must mean "no filter" rather than a year-1 watermark, which would reach the
// database as a bogus timestamp and read as a delta pull that is really a full
// pull.
func TestDeltaWatermarkNormalisation(t *testing.T) {
	if got := deltaWatermark(nil); got != nil {
		t.Errorf("absent parameter = %v, want nil", got)
	}
	var zero time.Time
	if got := deltaWatermark(&zero); got != nil {
		t.Errorf("zero time = %v, want nil", got)
	}
	set := time.Date(2026, 8, 5, 10, 8, 45, 0, time.UTC)
	got := deltaWatermark(&set)
	if got == nil || !got.Equal(set) {
		t.Errorf("real watermark = %v, want %v", got, set)
	}
}

// Asserts the list handlers hand updatedSince down to the service rather
// than filtering the page they already fetched.
func TestListHandlersApplyUpdatedSince(t *testing.T) {
	userID := uuid.New()
	stale := time.Now().Add(-2 * time.Hour)
	fresh := time.Now()
	watermark := time.Now().Add(-1 * time.Hour)

	t.Run("aircraft pagination counts only the delta", func(t *testing.T) {
		d := setupDeltaSyncHandler()
		for i, updatedAt := range []time.Time{stale, stale, fresh} {
			id := uuid.New()
			d.aircraft.aircraft[id] = &models.Aircraft{
				ID: id, UserID: userID, Registration: []string{"D-EAAA", "D-EBBB", "D-ECCC"}[i],
				Type: "C172", IsActive: true, CreatedAt: updatedAt, UpdatedAt: updatedAt,
			}
		}

		w := httptest.NewRecorder()
		c := deltaSyncContext(w, userID)
		d.handler.ListAircraft(c, generated.ListAircraftParams{UpdatedSince: &watermark})

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
		}
		var page struct {
			Data       []map[string]any `json:"data"`
			Pagination struct {
				Total      int `json:"total"`
				TotalPages int `json:"totalPages"`
			} `json:"pagination"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(page.Data) != 1 {
			t.Errorf("returned %d aircraft, want 1", len(page.Data))
		}
		if page.Pagination.Total != 1 {
			t.Errorf("pagination.total = %d, want 1 (the delta, not the fleet)", page.Pagination.Total)
		}
		if page.Pagination.TotalPages != 1 {
			t.Errorf("pagination.totalPages = %d, want 1", page.Pagination.TotalPages)
		}
	})

	t.Run("aircraft without the parameter returns everything", func(t *testing.T) {
		d := setupDeltaSyncHandler()
		for _, updatedAt := range []time.Time{stale, fresh} {
			id := uuid.New()
			d.aircraft.aircraft[id] = &models.Aircraft{
				ID: id, UserID: userID, Registration: "D-E" + id.String()[:3],
				Type: "C172", IsActive: true, CreatedAt: updatedAt, UpdatedAt: updatedAt,
			}
		}

		w := httptest.NewRecorder()
		c := deltaSyncContext(w, userID)
		d.handler.ListAircraft(c, generated.ListAircraftParams{})

		var page struct {
			Pagination struct {
				Total int `json:"total"`
			} `json:"pagination"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if page.Pagination.Total != 2 {
			t.Errorf("pagination.total = %d, want 2", page.Pagination.Total)
		}
	})

	t.Run("contacts", func(t *testing.T) {
		d := setupDeltaSyncHandler()
		staleID, freshID := uuid.New(), uuid.New()
		d.contacts.contacts[staleID] = &models.Contact{ID: staleID, UserID: userID, Name: "Stale", UpdatedAt: stale}
		d.contacts.contacts[freshID] = &models.Contact{ID: freshID, UserID: userID, Name: "Fresh", UpdatedAt: fresh}

		w := httptest.NewRecorder()
		c := deltaSyncContext(w, userID)
		d.handler.ListContacts(c, generated.ListContactsParams{UpdatedSince: &watermark})

		var got []map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got) != 1 || got[0]["name"] != "Fresh" {
			t.Fatalf("returned %d contacts %v, want only Fresh", len(got), got)
		}
	})

	t.Run("credentials", func(t *testing.T) {
		d := setupDeltaSyncHandler()
		staleID, freshID := uuid.New(), uuid.New()
		d.credentials.credentials[staleID] = &models.Credential{
			ID: staleID, UserID: userID, CredentialType: models.CredentialTypeEASAClass2Medical, UpdatedAt: stale,
		}
		d.credentials.credentials[freshID] = &models.Credential{
			ID: freshID, UserID: userID, CredentialType: models.CredentialTypeLangICAOLevel4, UpdatedAt: fresh,
		}

		w := httptest.NewRecorder()
		c := deltaSyncContext(w, userID)
		d.handler.ListCredentials(c, generated.ListCredentialsParams{UpdatedSince: &watermark})

		var got []map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got) != 1 || got[0]["credentialType"] != string(models.CredentialTypeLangICAOLevel4) {
			t.Fatalf("returned %d credentials %v, want only the fresh one", len(got), got)
		}
	})

	t.Run("flights push the watermark into the query options", func(t *testing.T) {
		d := setupDeltaSyncHandler()
		staleID, freshID := uuid.New(), uuid.New()
		d.flights.flights[staleID] = &models.Flight{ID: staleID, UserID: userID, AircraftReg: "D-EAAA", UpdatedAt: stale}
		d.flights.flights[freshID] = &models.Flight{ID: freshID, UserID: userID, AircraftReg: "D-EBBB", UpdatedAt: fresh}

		w := httptest.NewRecorder()
		c := deltaSyncContext(w, userID)
		d.handler.ListFlights(c, generated.ListFlightsParams{UpdatedSince: &watermark})

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
		}
		if d.flights.lastOpts == nil || d.flights.lastOpts.UpdatedSince == nil {
			t.Fatal("updatedSince never reached FlightQueryOptions")
		}
		if !d.flights.lastOpts.UpdatedSince.Equal(watermark) {
			t.Errorf("opts.UpdatedSince = %v, want %v", *d.flights.lastOpts.UpdatedSince, watermark)
		}

		var page struct {
			Data       []map[string]any `json:"data"`
			Pagination struct {
				Total int `json:"total"`
			} `json:"pagination"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(page.Data) != 1 {
			t.Errorf("returned %d flights, want 1", len(page.Data))
		}
		if page.Pagination.Total != 1 {
			t.Errorf("pagination.total = %d, want 1", page.Pagination.Total)
		}
	})
}
