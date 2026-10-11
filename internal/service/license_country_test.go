package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func TestLicenseService_IssuingCountry(t *testing.T) {
	ctx := context.Background()
	str := func(s string) *string { return &s }

	newLicense := func(country *string) *models.License {
		return &models.License{
			UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL", LicenseNumber: "1",
			IssueDate: time.Now(), IssuingAuthority: "LBA", IssuingCountry: country,
		}
	}

	tests := []struct {
		name    string
		input   *string
		want    *string
		wantErr error
	}{
		{"absent stays absent", nil, nil, nil},
		{"upper-cased", str("de"), str("DE"), nil},
		{"trimmed", str(" ch "), str("CH"), nil},
		{"blank clears", str("  "), nil, nil},
		{"three letters rejected", str("DEU"), nil, ErrInvalidIssuingCountry},
		{"digits rejected", str("D1"), nil, ErrInvalidIssuingCountry},
		{"one letter rejected", str("D"), nil, ErrInvalidIssuingCountry},
	}
	for _, tt := range tests {
		t.Run("create "+tt.name, func(t *testing.T) {
			svc := NewLicenseService(newMockLicenseRepo())
			lic := newLicense(tt.input)
			err := svc.CreateLicense(ctx, lic)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && !equalStrPtr(lic.IssuingCountry, tt.want) {
				t.Errorf("issuingCountry = %v, want %v", lic.IssuingCountry, tt.want)
			}
		})
		t.Run("update "+tt.name, func(t *testing.T) {
			repo := newMockLicenseRepo()
			svc := NewLicenseService(repo)
			lic := newLicense(str("AT"))
			if err := svc.CreateLicense(ctx, lic); err != nil {
				t.Fatal(err)
			}
			patch := *lic
			patch.IssuingCountry = tt.input
			err := svc.UpdateLicense(ctx, &patch, lic.UserID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil {
				stored, _ := repo.GetByID(ctx, lic.ID)
				if !equalStrPtr(stored.IssuingCountry, tt.want) {
					t.Errorf("stored issuingCountry = %v, want %v", stored.IssuingCountry, tt.want)
				}
			}
		})
	}
}

func equalStrPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
