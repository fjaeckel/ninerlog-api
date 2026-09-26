package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type mockSoaringRepo struct {
	stats    *repository.SoaringSeasonStats
	err      error
	userID   uuid.UUID
	from, to time.Time
	topSites int
	calls    int
}

func (m *mockSoaringRepo) SeasonStats(_ context.Context, userID uuid.UUID, from, to time.Time, topSites int) (*repository.SoaringSeasonStats, error) {
	m.calls++
	m.userID, m.from, m.to, m.topSites = userID, from, to, topSites
	if m.err != nil {
		return nil, m.err
	}
	if m.stats == nil {
		return &repository.SoaringSeasonStats{LaunchesByMethod: map[string]int{}}, nil
	}
	return m.stats, nil
}

func seasonClock() time.Time { return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC) }

func intPtr(v int) *int { return &v }

func TestSoaringSeasonYear(t *testing.T) {
	tests := []struct {
		name     string
		year     *int
		wantErr  bool
		wantYear int
	}{
		{"default is current year", nil, false, 2026},
		{"past season", intPtr(2024), false, 2024},
		{"next year allowed", intPtr(2027), false, 2027},
		{"1900 allowed", intPtr(1900), false, 1900},
		{"1899 rejected", intPtr(1899), true, 0},
		{"two years ahead rejected", intPtr(2028), true, 0},
		{"negative rejected", intPtr(-1), true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockSoaringRepo{}
			svc := NewSoaringSeasonService(repo)
			svc.SetClock(seasonClock)
			user := uuid.New()
			got, err := svc.Season(context.Background(), user, tt.year)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidSeasonYear) {
					t.Fatalf("err = %v, want ErrInvalidSeasonYear", err)
				}
				if repo.calls != 0 {
					t.Fatal("repository queried for an invalid year")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Year != tt.wantYear {
				t.Errorf("year = %d, want %d", got.Year, tt.wantYear)
			}
			if repo.userID != user {
				t.Error("stats not scoped to the caller")
			}
			wantFrom := time.Date(tt.wantYear, 1, 1, 0, 0, 0, 0, time.UTC)
			wantTo := time.Date(tt.wantYear, 12, 31, 0, 0, 0, 0, time.UTC)
			if !repo.from.Equal(wantFrom) || !repo.to.Equal(wantTo) {
				t.Errorf("window = %v..%v", repo.from, repo.to)
			}
			if repo.topSites != 5 {
				t.Errorf("topSites = %d, want 5", repo.topSites)
			}
		})
	}
}

func TestSoaringSeasonBuild(t *testing.T) {
	reg := "D-5678"
	flightID := uuid.New()

	t.Run("L-job 4 season with launches by method, longest flight and sites", func(t *testing.T) {
		repo := &mockSoaringRepo{stats: &repository.SoaringSeasonStats{
			Flights: 8, Launches: 9, TotalMinutes: 243, Outlandings: 1,
			LaunchesByMethod: map[string]int{"winch": 7, "aerotow": 1, "": 1},
			LongestFlight: &repository.SoaringLongestFlight{
				FlightID: flightID, Date: time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC), Minutes: 185, AircraftReg: &reg,
			},
			Sites: []repository.SoaringSite{{Place: "EDVM", Flights: 7}, {Place: "EDVK", Flights: 1}},
		}}
		svc := NewSoaringSeasonService(repo)
		svc.SetClock(seasonClock)
		got, err := svc.Season(context.Background(), uuid.New(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Flights != 8 || got.Launches != 9 || got.TotalMinutes != 243 || got.Outlandings != 1 {
			t.Errorf("totals = %+v", got)
		}
		lbm := got.LaunchesByMethod
		if lbm.Winch != 7 || lbm.Aerotow != 1 || lbm.Unspecified != 1 || lbm.SelfLaunch != 0 {
			t.Errorf("launchesByMethod = %+v", lbm)
		}
		if got.AverageFlightMinutes != 30 {
			t.Errorf("average = %d, want 30 (243/8 rounded)", got.AverageFlightMinutes)
		}
		if got.LongestFlight == nil || got.LongestFlight.FlightID != flightID || got.LongestFlight.Date != "2026-06-14" ||
			got.LongestFlight.Minutes != 185 || *got.LongestFlight.AircraftReg != reg {
			t.Errorf("longest = %+v", got.LongestFlight)
		}
		if len(got.Sites) != 2 || got.Sites[0].Place != "EDVM" || got.Sites[0].Flights != 7 {
			t.Errorf("sites = %+v", got.Sites)
		}
	})

	t.Run("A1 empty season is zeros, not an error", func(t *testing.T) {
		svc := NewSoaringSeasonService(&mockSoaringRepo{})
		svc.SetClock(seasonClock)
		got, err := svc.Season(context.Background(), uuid.New(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Flights != 0 || got.Launches != 0 || got.AverageFlightMinutes != 0 || got.LongestFlight != nil {
			t.Errorf("season = %+v", got)
		}
		if got.Sites == nil || len(got.Sites) != 0 {
			t.Errorf("sites = %#v, want empty non-nil", got.Sites)
		}
	})

	t.Run("repository error is wrapped", func(t *testing.T) {
		boom := errors.New("boom")
		svc := NewSoaringSeasonService(&mockSoaringRepo{err: boom})
		svc.SetClock(seasonClock)
		if _, err := svc.Season(context.Background(), uuid.New(), nil); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
	})
}
