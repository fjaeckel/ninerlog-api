package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type singleLicenseRepo struct {
	repository.LicenseRepository
	license *models.License
}

func (r *singleLicenseRepo) GetByID(context.Context, uuid.UUID) (*models.License, error) {
	cp := *r.license
	return &cp, nil
}

func (r *singleLicenseRepo) Update(_ context.Context, l *models.License) error {
	cp := *l
	r.license = &cp
	return nil
}

func TestUpdateLicense_ResponseCarriesTheStoredIssuingCountry(t *testing.T) {
	tests := []struct {
		name string
		body string
		want any
	}{
		{"lower case is upper-cased", `{"issuingCountry":"ch"}`, "CH"},
		{"padded value is trimmed", `{"issuingCountry":" de "}`, "DE"},
		{"null clears the country", `{"issuingCountry":null}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := uuid.New()
			country := "AT"
			repo := &singleLicenseRepo{license: &models.License{
				ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL",
				LicenseNumber: "1", IssueDate: time.Now(), IssuingAuthority: "LBA", IssuingCountry: &country,
			}}
			h := &APIHandler{licenseService: service.NewLicenseService(repo)}

			w := httptest.NewRecorder()
			c := authenticatedContext(w, userID)
			c.Request = httptest.NewRequest(http.MethodPatch, "/licenses/"+repo.license.ID.String(), bytes.NewBufferString(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			h.UpdateLicense(c, generated.LicenseId(repo.license.ID))

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var resp map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, tt.want, resp["issuingCountry"])
			if tt.want == nil {
				assert.Nil(t, repo.license.IssuingCountry)
			} else {
				assert.Equal(t, tt.want, *repo.license.IssuingCountry)
			}
		})
	}
}
