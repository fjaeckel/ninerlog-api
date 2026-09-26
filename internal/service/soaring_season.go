package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// ErrInvalidSeasonYear is returned for a season year outside
// models.SoaringSeasonMinYear..next year.
var ErrInvalidSeasonYear = errors.New("invalid season year")

// SoaringSeasonService summarises a pilot's soaring season.
type SoaringSeasonService struct {
	repo repository.SoaringRepository
	now  func() time.Time
}

// NewSoaringSeasonService creates a soaring season service.
func NewSoaringSeasonService(repo repository.SoaringRepository) *SoaringSeasonService {
	return &SoaringSeasonService{repo: repo, now: time.Now}
}

// SetClock replaces the service clock.
func (s *SoaringSeasonService) SetClock(now func() time.Time) {
	s.now = now
}

// Season returns the user's soaring summary for year, or the current UTC
// year when year is nil.
func (s *SoaringSeasonService) Season(ctx context.Context, userID uuid.UUID, year *int) (*models.SoaringSeason, error) {
	now := s.now().UTC()
	y := now.Year()
	if year != nil {
		y = *year
	}
	if !models.ValidSoaringSeasonYear(y, now) {
		return nil, ErrInvalidSeasonYear
	}
	from := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC)
	stats, err := s.repo.SeasonStats(ctx, userID, from, to, models.SoaringSeasonTopSites)
	if err != nil {
		return nil, fmt.Errorf("soaring season stats: %w", err)
	}
	return buildSoaringSeason(y, stats), nil
}

func buildSoaringSeason(year int, st *repository.SoaringSeasonStats) *models.SoaringSeason {
	out := &models.SoaringSeason{
		Year:         year,
		Flights:      st.Flights,
		Launches:     st.Launches,
		TotalMinutes: st.TotalMinutes,
		Outlandings:  st.Outlandings,
		Sites:        []models.SoaringSeasonSite{},
	}
	for m, n := range st.LaunchesByMethod {
		out.LaunchesByMethod.Add(m, n)
	}
	if st.Flights > 0 {
		out.AverageFlightMinutes = (st.TotalMinutes + st.Flights/2) / st.Flights
	}
	if lf := st.LongestFlight; lf != nil {
		out.LongestFlight = &models.SoaringSeasonFlight{
			FlightID:    lf.FlightID,
			Date:        lf.Date.Format("2006-01-02"),
			Minutes:     lf.Minutes,
			AircraftReg: lf.AircraftReg,
		}
	}
	for _, site := range st.Sites {
		out.Sites = append(out.Sites, models.SoaringSeasonSite{Place: site.Place, Flights: site.Flights})
	}
	return out
}
