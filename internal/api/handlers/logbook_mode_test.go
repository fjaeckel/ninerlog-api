package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

type modeSwitchRepo struct {
	repository.LogbookRepository
	strict   bool
	switches int
}

func (r *modeSwitchRepo) SetStrict(_ context.Context, _ uuid.UUID, strict bool, _ string, _ *uuid.UUID) (bool, error) {
	r.switches++
	r.strict = strict
	return true, nil
}

func (r *modeSwitchRepo) GetOverview(context.Context, uuid.UUID) (*models.LogbookOverview, error) {
	return &models.LogbookOverview{LogbookMode: models.LogbookMode{Strict: r.strict}}, nil
}

func (r *modeSwitchRepo) ListModeEvents(context.Context, uuid.UUID) ([]models.LogbookModeEvent, error) {
	return nil, nil
}

func TestUpdateLogbookMode_Body(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantStatus   int
		wantSwitches int
		wantStrict   bool
	}{
		{"switches on", `{"strict":true}`, http.StatusOK, 1, true},
		{"switches off", `{"strict":false}`, http.StatusOK, 1, false},
		{"missing key is refused instead of switching off", `{}`, http.StatusBadRequest, 0, true},
		{"wrong type is refused", `{"strict":"yes"}`, http.StatusBadRequest, 0, true},
		{"malformed body is refused", `{`, http.StatusBadRequest, 0, true},
		{"array is refused", `[]`, http.StatusBadRequest, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &modeSwitchRepo{strict: true}
			h := &APIHandler{logbookService: service.NewLogbookService(repo, noLicenses{}, nil, nil)}

			w := httptest.NewRecorder()
			c := authenticatedContext(w, uuid.New())
			c.Request = httptest.NewRequest(http.MethodPut, "/logbook/mode", bytes.NewBufferString(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			h.UpdateLogbookMode(c)

			assert.Equal(t, tt.wantStatus, w.Code, w.Body.String())
			assert.Equal(t, tt.wantSwitches, repo.switches)
			assert.Equal(t, tt.wantStrict, repo.strict)
		})
	}
}
