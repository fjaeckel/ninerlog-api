package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type mockBaselineRepo struct {
	stored *models.FlightBaseline
	getErr error
}

func (m *mockBaselineRepo) Get(ctx context.Context, userID uuid.UUID) (*models.FlightBaseline, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.stored == nil || m.stored.UserID != userID {
		return nil, repository.ErrNotFound
	}
	clone := *m.stored
	return &clone, nil
}

func (m *mockBaselineRepo) Upsert(ctx context.Context, b *models.FlightBaseline) error {
	clone := *b
	now := time.Now()
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = now
	}
	clone.UpdatedAt = now
	m.stored = &clone
	*b = clone
	return nil
}

func (m *mockBaselineRepo) Delete(ctx context.Context, userID uuid.UUID) error {
	if m.stored == nil || m.stored.UserID != userID {
		return repository.ErrNotFound
	}
	m.stored = nil
	return nil
}

// TestGetStatsByUserID_WithBaseline verifies the baseline is added on top of
// flight totals when applyBaseline=true and the requested window covers the
// baseline cutoff.
func TestGetStatsByUserID_WithBaseline(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	flightRepo := newMockFlightRepo()
	baselineRepo := &mockBaselineRepo{}
	svc := NewFlightService(flightRepo, baselineRepo)

	// Seed a flight: 60 minutes PIC + 1 day landing.
	flight := &models.Flight{
		UserID:       userID,
		Date:         time.Now().UTC(),
		AircraftReg:  "DEABC",
		AircraftType: "C172",
		TotalTime:    60,
		IsPIC:        true,
		PICTime:      60,
		LandingsDay:  1,
	}
	if err := svc.CreateFlight(ctx, flight); err != nil {
		t.Fatalf("CreateFlight: %v", err)
	}

	// Seed a baseline: 100h total, 80h PIC, 200 day landings, 10 night.
	baselineDate := time.Now().UTC().AddDate(-1, 0, 0)
	notes := "from paper logbook"
	baseline := &models.FlightBaseline{
		UserID:        userID,
		BaselineDate:  baselineDate,
		TotalFlights:  150,
		TotalMinutes:  6000,
		PICMinutes:    4800,
		LandingsDay:   200,
		LandingsNight: 10,
		Notes:         &notes,
	}
	if err := svc.UpsertBaseline(ctx, baseline); err != nil {
		t.Fatalf("UpsertBaseline: %v", err)
	}

	stats, applied, err := svc.GetStatsByUserID(ctx, userID, nil, nil, true)
	if err != nil {
		t.Fatalf("GetStatsByUserID: %v", err)
	}
	if applied == nil {
		t.Fatalf("expected baseline to be applied")
	}
	if got, want := stats.TotalFlights, 1+150; got != want {
		t.Errorf("TotalFlights = %d, want %d", got, want)
	}
	if got, want := stats.TotalMinutes, 60+6000; got != want {
		t.Errorf("TotalMinutes = %d, want %d", got, want)
	}
	if got, want := stats.PICMinutes, 60+4800; got != want {
		t.Errorf("PICMinutes = %d, want %d", got, want)
	}
	if got, want := stats.LandingsDay, 1+200; got != want {
		t.Errorf("LandingsDay = %d, want %d", got, want)
	}
	if got, want := stats.LandingsNight, 10; got != want {
		t.Errorf("LandingsNight = %d, want %d", got, want)
	}

	// applyBaseline=false → no baseline contribution.
	stats2, applied2, err := svc.GetStatsByUserID(ctx, userID, nil, nil, false)
	if err != nil {
		t.Fatalf("GetStatsByUserID (no baseline): %v", err)
	}
	if applied2 != nil {
		t.Errorf("expected baseline NOT to be applied when applyBaseline=false")
	}
	if stats2.TotalFlights != 1 {
		t.Errorf("TotalFlights without baseline = %d, want 1", stats2.TotalFlights)
	}
}

// TestGetStatsByUserID_BaselineExcludedWhenStartAfterBaselineDate verifies the
// baseline is skipped when the requested startDate is after the baseline cutoff.
func TestGetStatsByUserID_BaselineExcludedWhenStartAfterBaselineDate(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	flightRepo := newMockFlightRepo()
	baselineRepo := &mockBaselineRepo{}
	svc := NewFlightService(flightRepo, baselineRepo)

	baselineDate := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := svc.UpsertBaseline(ctx, &models.FlightBaseline{
		UserID:       userID,
		BaselineDate: baselineDate,
		TotalMinutes: 6000,
		PICMinutes:   4800,
	}); err != nil {
		t.Fatalf("UpsertBaseline: %v", err)
	}

	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, applied, err := svc.GetStatsByUserID(ctx, userID, &startDate, nil, true)
	if err != nil {
		t.Fatalf("GetStatsByUserID: %v", err)
	}
	if applied != nil {
		t.Errorf("expected baseline NOT to be applied when startDate > baselineDate")
	}
	if stats.TotalMinutes != 0 {
		t.Errorf("TotalMinutes = %d, want 0", stats.TotalMinutes)
	}
}

// TestUpsertBaseline_Validation rejects invalid input.
func TestUpsertBaseline_Validation(t *testing.T) {
	ctx := context.Background()
	svc := NewFlightService(newMockFlightRepo(), &mockBaselineRepo{})

	err := svc.UpsertBaseline(ctx, &models.FlightBaseline{
		UserID:       uuid.New(),
		BaselineDate: time.Time{}, // missing
	})
	if !errors.Is(err, models.ErrInvalidFlightBaseline) {
		t.Errorf("expected ErrInvalidFlightBaseline for missing date, got %v", err)
	}

	err = svc.UpsertBaseline(ctx, &models.FlightBaseline{
		UserID:       uuid.New(),
		BaselineDate: time.Now().AddDate(1, 0, 0),
	})
	if !errors.Is(err, models.ErrInvalidFlightBaseline) {
		t.Errorf("expected ErrInvalidFlightBaseline for future date, got %v", err)
	}

	err = svc.UpsertBaseline(ctx, &models.FlightBaseline{
		UserID:       uuid.New(),
		BaselineDate: time.Now().AddDate(0, 0, -1),
		PICMinutes:   -5,
	})
	if !errors.Is(err, models.ErrInvalidFlightBaseline) {
		t.Errorf("expected ErrInvalidFlightBaseline for negative value, got %v", err)
	}
}

// TestGetBaseline_NotConfigured returns ErrNotFound when no baseline exists.
func TestGetBaseline_NotConfigured(t *testing.T) {
	svc := NewFlightService(newMockFlightRepo(), &mockBaselineRepo{})
	_, err := svc.GetBaseline(context.Background(), uuid.New())
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestLogbookTotals_UnfilteredAddsBaseline adds the snapshot to unfiltered list
// totals and leaves the columns it does not record alone.
func TestLogbookTotals_UnfilteredAddsBaseline(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	svc := NewFlightService(newMockFlightRepo(), &mockBaselineRepo{})
	if err := svc.UpsertBaseline(ctx, &models.FlightBaseline{
		UserID:        userID,
		BaselineDate:  time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC),
		TotalFlights:  12,
		TotalMinutes:  2000,
		PICMinutes:    1800,
		NightMinutes:  30,
		LandingsDay:   36,
		LandingsNight: 2,
	}); err != nil {
		t.Fatalf("UpsertBaseline: %v", err)
	}

	logged := &models.FlightTotals{Flights: 84, TotalTime: 7154, PICTime: 5180, LandingsDay: 124, LandingsNight: 19, Approaches: 7}
	sum, applied, err := svc.LogbookTotals(ctx, userID, &repository.FlightQueryOptions{Page: 1, PageSize: 20}, logged)
	if err != nil {
		t.Fatalf("LogbookTotals: %v", err)
	}
	if applied == nil || sum == nil {
		t.Fatalf("expected baseline to be applied")
	}
	want := models.FlightTotals{Flights: 96, TotalTime: 9154, PICTime: 6980, NightTime: 30, LandingsDay: 160, LandingsNight: 21, Approaches: 7}
	if *sum != want {
		t.Errorf("sum = %+v, want %+v", *sum, want)
	}
	if logged.Flights != 84 || logged.TotalTime != 7154 {
		t.Errorf("logged totals were modified: %+v", *logged)
	}
}

// TestLogbookTotals_NotApplied returns nothing for filtered lists, windows
// after the snapshot date and users without a snapshot.
func TestLogbookTotals_NotApplied(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	svc := NewFlightService(newMockFlightRepo(), &mockBaselineRepo{})
	if err := svc.UpsertBaseline(ctx, &models.FlightBaseline{
		UserID:       userID,
		BaselineDate: time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC),
		TotalMinutes: 2000,
	}); err != nil {
		t.Fatalf("UpsertBaseline: %v", err)
	}
	logged := &models.FlightTotals{Flights: 1, TotalTime: 60}

	isPIC := true
	reg := "DEABC"
	search := "EDDM"
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
	since := time.Now()
	cases := map[string]*repository.FlightQueryOptions{
		"isPic":         {IsPIC: &isPIC},
		"aircraftReg":   {AircraftReg: &reg},
		"search":        {Search: &search},
		"updatedSince":  {UpdatedSince: &since},
		"registrations": {FilterByRegistrations: true},
		"startAfter":    {StartDate: &after},
	}
	for name, opts := range cases {
		sum, applied, err := svc.LogbookTotals(ctx, userID, opts, logged)
		if err != nil || sum != nil || applied != nil {
			t.Errorf("%s: got (%v, %v, %v), want nothing", name, sum, applied, err)
		}
	}

	sum, applied, err := svc.LogbookTotals(ctx, userID, &repository.FlightQueryOptions{StartDate: &before}, logged)
	if err != nil || sum == nil || applied == nil {
		t.Errorf("startBefore: got (%v, %v, %v), want the baseline applied", sum, applied, err)
	}

	sum, applied, err = svc.LogbookTotals(ctx, uuid.New(), &repository.FlightQueryOptions{}, logged)
	if err != nil || sum != nil || applied != nil {
		t.Errorf("no snapshot: got (%v, %v, %v), want nothing", sum, applied, err)
	}
}
