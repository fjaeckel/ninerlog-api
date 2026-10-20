package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countryLicenses struct {
	repository.LicenseRepository
	country string
}

func (l countryLicenses) GetByUserID(_ context.Context, userID uuid.UUID, _ *time.Time) ([]*models.License, error) {
	return []*models.License{{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL", IssuingCountry: &l.country}}, nil
}

func TestConfirmImport_StrictCHRowsAreCheckedForDownwardOverrides(t *testing.T) {
	tests := []struct {
		name         string
		strict       bool
		country      string
		wantImported int
		wantErrored  int
		wantFields   []string
	}{
		{"CH strict rejects the row with raised night values", true, "CH", 1, 1, []string{"nightTime", "takeoffsNight", "landingsNight"}},
		{"other licences import both rows", true, "DE", 2, 0, nil},
		{"standard mode imports both rows", false, "CH", 2, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			airports.SetTestDB(nil)
			env := newImportConfirmEnv(t, "Alex Rivera")
			repo := strictLogbookRepo{}
			var logbookRepo repository.LogbookRepository = repo
			if !tt.strict {
				logbookRepo = standardLogbookRepo{}
			}
			env.h.logbookService = service.NewLogbookService(logbookRepo, countryLicenses{country: tt.country}, env.flights, noSignatures{})

			result, flights := env.confirm(t, readImportFixture(t, "importsamples/foreflight.csv"))

			assert.Equal(t, tt.wantImported, result.ImportedCount)
			assert.Equal(t, tt.wantErrored, result.ErrorCount)
			assert.Len(t, flights, tt.wantImported)
			var fields []string
			if result.Errors != nil {
				for _, e := range *result.Errors {
					fields = append(fields, e.Field)
					assert.Positive(t, e.RowIndex)
				}
			}
			require.Equal(t, tt.wantFields, fields)
		})
	}
}

type standardLogbookRepo struct{ repository.LogbookRepository }

func (standardLogbookRepo) GetMode(context.Context, uuid.UUID) (*models.LogbookMode, error) {
	return &models.LogbookMode{}, nil
}
