package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/pilotprofile"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type handlerPilotProfiles struct {
	rows map[uuid.UUID]*models.PilotProfile
}

func (m *handlerPilotProfiles) Get(_ context.Context, userID uuid.UUID) (*models.PilotProfile, error) {
	if p, ok := m.rows[userID]; ok {
		return p, nil
	}
	return nil, repository.ErrNotFound
}

func (m *handlerPilotProfiles) Upsert(_ context.Context, p *models.PilotProfile) error {
	m.rows[p.UserID] = p
	return nil
}

type handlerNoFlights struct{}

func (handlerNoFlights) GetDisciplineFlightGroups(context.Context, uuid.UUID) ([]models.DisciplineFlightGroup, error) {
	return nil, nil
}

type handlerNoLicences struct {
	repository.LicenseRepository
}

func (handlerNoLicences) GetByUserID(context.Context, uuid.UUID, *time.Time) ([]*models.License, error) {
	return nil, nil
}

type handlerNoRatings struct{}

func (handlerNoRatings) GetByLicenseID(context.Context, uuid.UUID) ([]*models.ClassRating, error) {
	return nil, nil
}

func addPilotProfileService(h *APIHandler) {
	h.pilotProfileService = pilotprofile.NewService(
		&handlerPilotProfiles{rows: map[uuid.UUID]*models.PilotProfile{}},
		handlerNoFlights{}, handlerNoLicences{}, handlerNoRatings{}, newMockAircraftRepo(),
	)
}

func patchPilotProfile(h *APIHandler, userID *uuid.UUID, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var c *gin.Context
	if userID != nil {
		c = authenticatedContext(w, *userID)
	} else {
		c, _ = gin.CreateTestContext(w)
	}
	c.Request = httptest.NewRequest("PATCH", "/users/me/pilot-profile", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdatePilotProfile(c)
	return w
}

func TestUpdatePilotProfile_Status(t *testing.T) {
	userID := uuid.New()
	tests := []struct {
		name   string
		userID *uuid.UUID
		body   string
		want   int
	}{
		{"unauthenticated", nil, `{}`, http.StatusUnauthorized},
		{"invalid JSON", &userID, `{bad`, http.StatusBadRequest},
		{"unknown discipline in intents", &userID, `{"intents":{"BALLOON":"on"}}`, http.StatusBadRequest},
		{"unknown intent", &userID, `{"intents":{"SAILPLANE":"maybe"}}`, http.StatusBadRequest},
		{"unknown mode", &userID, `{"mode":"sometimes"}`, http.StatusBadRequest},
		{"unknown discipline in acknowledge", &userID, `{"acknowledge":["BALLOON"]}`, http.StatusBadRequest},
		{"empty merge", &userID, `{}`, http.StatusOK},
		{"valid merge", &userID, `{"mode":"everything","intents":{"SAILPLANE":"goal","IFR":"off"},"acknowledge":["TMG"]}`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := setupTestHandler()
			addPilotProfileService(h)
			w := patchPilotProfile(h, tt.userID, tt.body)
			if w.Code != tt.want {
				t.Errorf("status = %d, want %d (%s)", w.Code, tt.want, w.Body.String())
			}
		})
	}
}

func TestUpdatePilotProfile_ReturnsMergedProfile(t *testing.T) {
	h, _ := setupTestHandler()
	addPilotProfileService(h)
	userID := uuid.New()

	w := patchPilotProfile(h, &userID, `{"mode":"everything","intents":{"SAILPLANE":"goal"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp struct {
		Mode        string `json:"mode"`
		Disciplines []struct {
			Discipline string `json:"discipline"`
			Status     string `json:"status"`
			Intent     string `json:"intent"`
		} `json:"disciplines"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Mode != "everything" || len(resp.Disciplines) != 10 {
		t.Fatalf("resp = %+v", resp)
	}
	if d := resp.Disciplines[2]; d.Discipline != "SAILPLANE" || d.Intent != "goal" || d.Status != "training" {
		t.Errorf("SAILPLANE = %+v", d)
	}
}

func TestGetPilotProfile(t *testing.T) {
	h, _ := setupTestHandler()
	addPilotProfileService(h)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/users/me/pilot-profile", nil)
	h.GetPilotProfile(c)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated status = %d, want 401", w.Code)
	}

	w = httptest.NewRecorder()
	c = authenticatedContext(w, uuid.New())
	c.Request = httptest.NewRequest("GET", "/users/me/pilot-profile", nil)
	h.GetPilotProfile(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["mode"] != "adaptive" {
		t.Errorf("mode = %v", resp["mode"])
	}
	pending, ok := resp["pendingAcknowledgement"].([]any)
	if !ok || len(pending) != 0 {
		t.Errorf("pendingAcknowledgement = %v", resp["pendingAcknowledgement"])
	}
	ds, _ := resp["disciplines"].([]any)
	if len(ds) != 10 {
		t.Fatalf("disciplines = %v", resp["disciplines"])
	}
	first, _ := ds[0].(map[string]any)
	if _, ok := first["evidence"].([]any); !ok {
		t.Errorf("evidence is not an array: %v", first["evidence"])
	}
	if _, ok := first["ulKinds"].([]any); !ok {
		t.Errorf("ulKinds is not an array: %v", first["ulKinds"])
	}
}
