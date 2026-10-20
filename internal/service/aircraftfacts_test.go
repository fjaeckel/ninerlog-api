package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

type countingAircraftRepo struct {
	*mockAircraftRepo
	fleetLoads int
}

func (m *countingAircraftRepo) GetByUserID(ctx context.Context, userID uuid.UUID, updatedSince *time.Time) ([]*models.Aircraft, error) {
	m.fleetLoads++
	return m.mockAircraftRepo.GetByUserID(ctx, userID, updatedSince)
}

func TestAircraftFactsLookup(t *testing.T) {
	userID := uuid.New()
	repo := &countingAircraftRepo{mockAircraftRepo: newMockAircraftRepo()}
	id := uuid.New()
	repo.aircraft[id] = &models.Aircraft{ID: id, UserID: userID, Registration: "D-EABC", IsMultiPilot: true}
	lookup := service.NewAircraftService(repo).AircraftFactsLookup(context.Background(), userID)

	if lookup("  ") != nil || repo.fleetLoads != 0 {
		t.Fatalf("blank registration: fleet loads = %d, want 0 and nil facts", repo.fleetLoads)
	}
	if f := lookup("d-eabc"); f == nil || !f.IsMultiPilot {
		t.Errorf("lookup(d-eabc) = %+v, want the multi-pilot fleet entry", f)
	}
	if f := lookup("D-EXYZ"); f != nil {
		t.Errorf("lookup(D-EXYZ) = %+v, want nil", f)
	}
	if repo.fleetLoads != 1 {
		t.Errorf("fleet loads = %d, want 1", repo.fleetLoads)
	}
}

func TestAircraftFactsLookup_NilService(t *testing.T) {
	var s *service.AircraftService
	if f := s.AircraftFactsLookup(context.Background(), uuid.New())("D-EABC"); f != nil {
		t.Errorf("nil service lookup = %+v, want nil", f)
	}
}
