package currency

import (
	"context"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func TestEASA_SPL_SupervisedSoloCountsTowardHours(t *testing.T) {
	tests := []struct {
		name     string
		rating   func() (*models.ClassRating, *models.License)
		progress map[models.ClassType]*Progress
		reqKey   string
		want     float64
		wantMet  bool
	}{
		{
			name:   "J3 SFCL.160(a)(1) counts SPIC with dual toward 5 h",
			rating: splGlider,
			progress: map[models.ClassType]*Progress{
				models.ClassTypeGlider: {InstructorMinutes: 180, SPICMinutes: 120, Launches: 15, TrainingFlights: 2},
			},
			reqKey: ReqKeyFlightTime, want: 300, wantMet: true,
		},
		{
			name:   "SFCL.160(a)(1) pools TMG SPIC time",
			rating: splGlider,
			progress: map[models.ClassType]*Progress{
				models.ClassTypeGlider: {PICMinutes: 200, Launches: 15, TrainingFlights: 2},
				models.ClassTypeTMG:    {SPICMinutes: 100},
			},
			reqKey: ReqKeyFlightTime, want: 300, wantMet: true,
		},
		{
			name:   "SFCL.160(b)(1) counts glider SPIC toward 12 h",
			rating: splTMG,
			progress: map[models.ClassType]*Progress{
				models.ClassTypeTMG:    {PICMinutes: 360, Landings: 12, LongestTrainingFlightMinutes: 60},
				models.ClassTypeGlider: {SPICMinutes: 360},
			},
			reqKey: ReqKeyFlightTime, want: 720, wantMet: true,
		},
		{
			name:   "SFCL.160(b)(1)(i) counts TMG SPIC toward 6 h on TMGs",
			rating: splTMG,
			progress: map[models.ClassType]*Progress{
				models.ClassTypeTMG: {PICMinutes: 300, SPICMinutes: 60, Landings: 12, LongestTrainingFlightMinutes: 60},
			},
			reqKey: ReqKeyTMGTime, want: 360, wantMet: true,
		},
		{
			name:   "SPIC alone short of 5 h stays lapsed",
			rating: splGlider,
			progress: map[models.ClassType]*Progress{
				models.ClassTypeGlider: {SPICMinutes: 299, Launches: 15, TrainingFlights: 2},
			},
			reqKey: ReqKeyFlightTime, want: 299, wantMet: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := newMockFlightDataProvider()
			for class, p := range tt.progress {
				dp.progressByClass[class] = p
			}
			rating, license := tt.rating()
			result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
			r := findReq(result.Requirements, tt.reqKey)
			if r == nil || r.Current != tt.want || r.Met != tt.wantMet {
				t.Errorf("%s = %+v, want current %v met %t", tt.reqKey, r, tt.want, tt.wantMet)
			}
		})
	}
}

func TestEASA_SPL_LaunchesCountStoredLaunches(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeGlider] = &Progress{
		Flights: 3, PICMinutes: 300, Launches: 15, TrainingFlights: 2,
	}
	rating, license := splGlider()
	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if r := findReq(result.Requirements, ReqKeyLaunches); r == nil || !r.Met || r.Current != 15 {
		t.Errorf("launches = %+v, want 15 from 3 series entries", r)
	}
	if result.Status != StatusCurrent {
		t.Errorf("status = %s, want current", result.Status)
	}
}
