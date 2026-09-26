package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// winchCircuit is one of Lena's 8-minute winch circuits in the ASK 21 D-1234.
func winchCircuit(userID uuid.UUID) *models.Flight {
	winch := "winch"
	return &models.Flight{
		UserID:       userID,
		Date:         time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC),
		AircraftReg:  "d-1234",
		AircraftType: "ASK21",
		TotalTime:    8,
		IsPIC:        true,
		PICTime:      8,
		LandingsDay:  1,
		AllLandings:  1,
		TakeoffsDay:  1,
		LaunchMethod: &winch,
	}
}

func TestCreateFlightBatch(t *testing.T) {
	userID := uuid.New()
	circuits := func(n int) []*models.Flight {
		out := make([]*models.Flight, n)
		for i := range out {
			out[i] = winchCircuit(userID)
		}
		return out
	}

	tests := []struct {
		name      string
		flights   func() []*models.Flight
		repoErr   error
		wantErr   error
		wantLeg   int
		wantSaved int
	}{
		{
			name:      "L1 six winch circuits are stored",
			flights:   func() []*models.Flight { return circuits(6) },
			wantSaved: 6,
		},
		{
			name:    "no legs",
			flights: func() []*models.Flight { return nil },
			wantErr: ErrInvalidFlightBatch,
		},
		{
			name:    "more than 50 legs",
			flights: func() []*models.Flight { return circuits(MaxFlightBatchLegs + 1) },
			wantErr: ErrInvalidFlightBatch,
		},
		{
			name: "invalid third leg names its index and stores nothing",
			flights: func() []*models.Flight {
				fs := circuits(4)
				fs[2].TotalTime = 0
				return fs
			},
			wantErr: ErrInvalidFlight,
			wantLeg: 2,
		},
		{
			name: "release height out of bounds on first leg",
			flights: func() []*models.Flight {
				fs := circuits(2)
				h := 20001
				fs[0].ReleaseHeightM = &h
				return fs
			},
			wantErr: models.ErrInvalidReleaseHeight,
			wantLeg: 0,
		},
		{
			name: "invalid launch method on last leg",
			flights: func() []*models.Flight {
				fs := circuits(3)
				bad := "catapult"
				fs[2].LaunchMethod = &bad
				return fs
			},
			wantErr: models.ErrInvalidLaunchMethod,
			wantLeg: 2,
		},
		{
			name:    "repository failure is returned",
			flights: func() []*models.Flight { return circuits(2) },
			repoErr: errors.New("tx aborted"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockFlightRepo()
			repo.createBatchErr = tt.repoErr
			svc := NewFlightService(repo, nil)
			flights := tt.flights()

			err := svc.CreateFlightBatch(context.Background(), flights)

			switch {
			case tt.repoErr != nil:
				if !errors.Is(err, tt.repoErr) {
					t.Fatalf("err = %v, want %v", err, tt.repoErr)
				}
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				var legErr *FlightBatchLegError
				if errors.As(err, &legErr) && legErr.Index != tt.wantLeg {
					t.Errorf("leg index = %d, want %d", legErr.Index, tt.wantLeg)
				}
			default:
				if err != nil {
					t.Fatalf("err = %v", err)
				}
			}
			if len(repo.flights) != tt.wantSaved {
				t.Errorf("stored %d flights, want %d", len(repo.flights), tt.wantSaved)
			}
			if tt.wantSaved > 0 {
				for _, f := range flights {
					if f.AircraftReg != "D-1234" || f.Launches != 1 {
						t.Errorf("leg reg=%q launches=%d, want D-1234 and 1", f.AircraftReg, f.Launches)
					}
				}
			}
		})
	}
}

func TestCreateFlight_DerivesLaunches(t *testing.T) {
	tests := []struct {
		name         string
		mutate       func(f *models.Flight)
		wantLaunches int
	}{
		{name: "take-off count", mutate: func(f *models.Flight) { f.TakeoffsDay = 2 }, wantLaunches: 2},
		{name: "backup row without take-offs has one launch", mutate: func(f *models.Flight) { f.TakeoffsDay = 0 }, wantLaunches: 1},
		{name: "override kept", mutate: func(f *models.Flight) { f.Launches, f.LaunchesOverride = 6, true }, wantLaunches: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewFlightService(newMockFlightRepo(), nil)
			f := winchCircuit(uuid.New())
			tt.mutate(f)
			if err := svc.CreateFlight(context.Background(), f); err != nil {
				t.Fatalf("CreateFlight: %v", err)
			}
			if f.Launches != tt.wantLaunches {
				t.Errorf("launches = %d, want %d", f.Launches, tt.wantLaunches)
			}
		})
	}
}
