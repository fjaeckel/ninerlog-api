package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

// seedPDFExportHandler builds a fully wired APIHandler (via setupTestHandler)
// with a registered user and a flight repo pre-populated with n flights
// owned by that user.
func seedPDFExportHandler(t *testing.T, n int) (*APIHandler, uuid.UUID) {
	t.Helper()
	h, userRepo := setupTestHandler()

	userID := uuid.New()
	userRepo.users[userID] = &models.User{ID: userID, Email: "pdf-export-test@example.com"}

	repo := newMockFlightRepo()
	for _, f := range buildSamplePDFFlights(n) {
		f.ID = uuid.New()
		f.UserID = userID
		repo.flights[f.ID] = f
	}
	h.flightService = service.NewFlightService(repo, nil)

	return h, userID
}

func TestExportFlightsPDF_RejectsWhenOverCap(t *testing.T) {
	h, userID := seedPDFExportHandler(t, maxPDFExportFlights+1)

	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("GET", "/exports/pdf", nil)

	h.ExportFlightsPDF(c, generated.ExportFlightsPDFParams{})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when flight count exceeds cap, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct == "application/pdf" {
		t.Error("should not have started rendering a PDF when over the cap")
	}
}

func TestExportFlightsPDF_AllowsWhenUnderCap(t *testing.T) {
	h, userID := seedPDFExportHandler(t, 5)

	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("GET", "/exports/pdf", nil)

	h.ExportFlightsPDF(c, generated.ExportFlightsPDFParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when under the cap, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("expected non-empty PDF body")
	}
}
