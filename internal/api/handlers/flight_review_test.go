package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type strictLogbookRepo struct{ repository.LogbookRepository }

func (strictLogbookRepo) GetMode(context.Context, uuid.UUID) (*models.LogbookMode, error) {
	return &models.LogbookMode{Strict: true}, nil
}
func (strictLogbookRepo) LatestCertifiedThrough(context.Context, uuid.UUID) (*time.Time, error) {
	return nil, nil
}

type noLicenses struct{ repository.LicenseRepository }

func (noLicenses) GetByUserID(context.Context, uuid.UUID, *time.Time) ([]*models.License, error) {
	return nil, nil
}

type noSignatures struct {
	repository.FlightSignatureRepository
}

func (noSignatures) ListByFlightID(context.Context, uuid.UUID) ([]*models.FlightSignature, error) {
	return nil, nil
}

type strictFlightEnv struct {
	handler  *APIHandler
	contacts *mockContactRepo
	flights  *mockFlightRepo
	userID   uuid.UUID
}

func newStrictFlightEnv(t *testing.T) *strictFlightEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	e := &strictFlightEnv{contacts: newMockContactRepo(), flights: newMockFlightRepo(), userID: uuid.New()}
	flightSvc := service.NewFlightService(e.flights, nil)
	flightSvc.SetLogbookService(service.NewLogbookService(strictLogbookRepo{}, noLicenses{}, e.flights, noSignatures{}))
	userRepo := newHandlerMockUserRepo()
	userRepo.users[e.userID] = &models.User{ID: e.userID, Email: "owner@example.com", Name: "Owner", PreferredLocale: "en"}
	jwtMgr := jwt.NewManager("test-access", "test-refresh", 15*time.Minute, 7*24*time.Hour)
	e.handler = &APIHandler{
		authService: service.NewAuthService(userRepo, newHandlerMockRefreshTokenRepo(), &mockPasswordResetRepo{},
			&mockEmailVerificationRepo{}, jwtMgr, service.NewTwoFactorService(userRepo, jwtMgr, nil), service.SessionPolicy{}),
		flightService:  flightSvc,
		contactService: service.NewContactService(e.contacts),
	}
	return e
}

func (e *strictFlightEnv) seed(t *testing.T, age time.Duration) *models.Flight {
	t.Helper()
	f := &models.Flight{
		UserID: e.userID, Date: time.Now().Add(-age), AircraftReg: "D-EFGH", AircraftType: "C172",
		TotalTime: 90, PICTime: 90, IsPIC: true,
	}
	if err := e.handler.flightService.CreateFlight(context.Background(), f); err != nil {
		t.Fatalf("seed flight: %v", err)
	}
	e.flights.flights[f.ID].CreatedAt = time.Now().Add(-age)
	return f
}

func (e *strictFlightEnv) put(userID uuid.UUID, flightID uuid.UUID, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest(http.MethodPut, "/flights/"+flightID.String(), bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	e.handler.UpdateFlight(c, generated.FlightId(flightID))
	return w
}

func TestUpdateFlight_CreatesContactsOnlyForAcceptedUpdates(t *testing.T) {
	const crewBody = `"crewMembers":[{"name":"New Crew","role":"Instructor"}]`

	tests := []struct {
		name         string
		age          time.Duration
		body         string
		lock         bool
		foreign      bool
		wantStatus   int
		wantContacts int
	}{
		{"correction without a reason is rejected", 5 * 24 * time.Hour, `{"remarks":"x",` + crewBody + `}`, false, false, http.StatusUnprocessableEntity, 0},
		{"locked flight is rejected", time.Hour, `{"remarks":"x",` + crewBody + `}`, true, false, http.StatusConflict, 0},
		{"another user's flight is rejected", time.Hour, `{"remarks":"x",` + crewBody + `}`, false, true, http.StatusNotFound, 0},
		{"invalid flight is rejected", time.Hour, `{"aircraftType":"",` + crewBody + `}`, false, false, http.StatusBadRequest, 0},
		{"free edit creates the contact", time.Hour, `{"remarks":"x",` + crewBody + `}`, false, false, http.StatusOK, 1},
		{"correction with a reason creates the contact", 5 * 24 * time.Hour, `{"remarks":"x","correctionReason":"typo",` + crewBody + `}`, false, false, http.StatusOK, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newStrictFlightEnv(t)
			f := e.seed(t, tt.age)
			if tt.lock {
				sig := uuid.New()
				e.flights.flights[f.ID].SignatureID = &sig
			}
			userID := e.userID
			if tt.foreign {
				userID = uuid.New()
			}

			w := e.put(userID, f.ID, tt.body)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if got := len(e.contacts.contacts); got != tt.wantContacts {
				t.Errorf("contacts created = %d, want %d", got, tt.wantContacts)
			}
		})
	}
}

func TestFlightReasonLengthCountsCharacters(t *testing.T) {
	tests := []struct {
		name       string
		reason     string
		wantStatus int
	}{
		{"500 two-byte characters", strings.Repeat("ä", 500), http.StatusOK},
		{"501 characters", strings.Repeat("a", 501), http.StatusBadRequest},
		{"501 two-byte characters", strings.Repeat("ä", 501), http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run("correctionReason "+tt.name, func(t *testing.T) {
			e := newStrictFlightEnv(t)
			f := e.seed(t, 5*24*time.Hour)
			body, _ := json.Marshal(map[string]any{"remarks": "x", "correctionReason": tt.reason})
			if w := e.put(e.userID, f.ID, string(body)); w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})

		t.Run("delete reason "+tt.name, func(t *testing.T) {
			e := newStrictFlightEnv(t)
			f := e.seed(t, time.Hour)
			w := httptest.NewRecorder()
			c := authenticatedContext(w, e.userID)
			c.Request = httptest.NewRequest(http.MethodDelete, "/flights/"+f.ID.String(), nil)
			reason := tt.reason
			e.handler.DeleteFlight(c, generated.FlightId(f.ID), generated.DeleteFlightParams{Reason: &reason})
			c.Writer.WriteHeaderNow()
			want := tt.wantStatus
			if want == http.StatusOK {
				want = http.StatusNoContent
			}
			if w.Code != want {
				t.Fatalf("status = %d, want %d; body %s", w.Code, want, w.Body.String())
			}
		})
	}
}
