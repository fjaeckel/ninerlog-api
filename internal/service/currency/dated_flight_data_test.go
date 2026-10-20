package currency

import (
	"context"
	"slices"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// datedFlight is one flight in a datedFlightData log.
type datedFlight struct {
	date      time.Time
	class     models.ClassType
	progress  Progress
	profCheck bool
}

// datedFlightData is a FlightDataProvider over a flight log that honours since.
type datedFlightData struct {
	flights []datedFlight
}

func (d *datedFlightData) sum(classes []models.ClassType, since time.Time) *Progress {
	total := &Progress{}
	for _, f := range d.flights {
		if f.date.Before(since) || (classes != nil && !slices.Contains(classes, f.class)) {
			continue
		}
		p := f.progress
		p.Flights = 1
		p.LongestTrainingFlightMinutes = p.InstructorMinutes
		addProgress(total, &p, true)
	}
	return total
}

func (d *datedFlightData) GetProgressByAircraftClass(_ context.Context, _ uuid.UUID, classes []models.ClassType, _ bool, since time.Time) (*Progress, error) {
	return d.sum(classes, since), nil
}

func (d *datedFlightData) GetProgressAll(_ context.Context, _ uuid.UUID, since time.Time) (*Progress, error) {
	return d.sum(nil, since), nil
}

func (d *datedFlightData) GetLastFlightReview(context.Context, uuid.UUID) (*time.Time, error) {
	return nil, nil
}

func (d *datedFlightData) GetLastProficiencyCheck(_ context.Context, _ uuid.UUID, classes []models.ClassType, since time.Time) (*time.Time, error) {
	var latest *time.Time
	for _, f := range d.flights {
		if f.profCheck && !f.date.Before(since) && slices.Contains(classes, f.class) && (latest == nil || f.date.After(*latest)) {
			date := f.date
			latest = &date
		}
	}
	return latest, nil
}

func (d *datedFlightData) GetLaunchCounts(context.Context, uuid.UUID, models.ClassType, time.Time) (map[string]int, error) {
	return map[string]int{}, nil
}

func (d *datedFlightData) GetLandingDaysByAircraftClass(context.Context, uuid.UUID, models.ClassType, bool, bool, time.Time) ([]LandingDay, error) {
	return nil, nil
}

func (d *datedFlightData) GetProgressByULKind(context.Context, uuid.UUID, ULSelector, bool, time.Time) (*Progress, error) {
	return &Progress{}, nil
}

func (d *datedFlightData) GetLastProficiencyCheckByULKind(context.Context, uuid.UUID, ULSelector, time.Time) (*time.Time, error) {
	return nil, nil
}

func (d *datedFlightData) GetLandingDaysByULKind(context.Context, uuid.UUID, ULSelector, bool, time.Time) ([]LandingDay, error) {
	return nil, nil
}

// daysAgo returns midnight UTC n days before today.
func daysAgo(n int) time.Time {
	u := time.Now().UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -n)
}

// lastDayInWindow returns the last date whose window of the given length still contains flown.
// datedLaplLog returns twelve 1h/1-landing SEP flights on days 10, 20 … 120 ago
// and a 1h dual flight 300 days ago.
func datedLaplLog() *datedFlightData {
	d := &datedFlightData{}
	for i := 1; i <= 12; i++ {
		d.flights = append(d.flights, datedFlight{date: daysAgo(10 * i), class: models.ClassTypeSEPLand, progress: Progress{TotalMinutes: 60, PICMinutes: 60, Landings: 1}})
	}
	d.flights = append(d.flights, datedFlight{date: daysAgo(300), class: models.ClassTypeSEPLand, progress: Progress{TotalMinutes: 60, InstructorMinutes: 60}})
	return d
}

func datedLaplRating() (*models.ClassRating, *models.License) {
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "LAPL"}
	return rating, license
}
