package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/training"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type handlerNoTrainingFlights struct{}

func (handlerNoTrainingFlights) ListTrainingFlights(context.Context, uuid.UUID) ([]repository.TrainingFlight, error) {
	return nil, nil
}

func (handlerNoTrainingFlights) OtherCategoryPICMinutes(context.Context, uuid.UUID) (int, error) {
	return 0, nil
}

func TestGetTrainingProgress_Status(t *testing.T) {
	userID := uuid.New()
	tests := []struct {
		name      string
		userID    *uuid.UUID
		programme []generated.TrainingProgrammeId
		want      int
		wantCount int
	}{
		{"unauthenticated", nil, nil, http.StatusUnauthorized, 0},
		{"R1 no training and no request", &userID, nil, http.StatusOK, 0},
		{"explicit SPL", &userID, []generated.TrainingProgrammeId{generated.SPL}, http.StatusOK, 1},
		{"unknown programme", &userID, []generated.TrainingProgrammeId{"PPL"}, http.StatusBadRequest, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := setupTestHandler()
			addPilotProfileService(h)
			h.SetTrainingService(training.NewService(h.pilotProfileService, handlerNoLicences{}, handlerNoTrainingFlights{}))
			w := httptest.NewRecorder()
			var c *gin.Context
			if tt.userID != nil {
				c = authenticatedContext(w, *tt.userID)
			} else {
				c, _ = gin.CreateTestContext(w)
			}
			c.Request = httptest.NewRequest("GET", "/training/progress", nil)
			params := generated.GetTrainingProgressParams{}
			if tt.programme != nil {
				params.Programme = &tt.programme
			}
			h.GetTrainingProgress(c, params)
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", w.Code, tt.want, w.Body.String())
			}
			if w.Code != http.StatusOK {
				return
			}
			var resp struct {
				Programmes []json.RawMessage `json:"programmes"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if resp.Programmes == nil || len(resp.Programmes) != tt.wantCount {
				t.Errorf("programmes = %s, want %d", w.Body.String(), tt.wantCount)
			}
		})
	}
}
