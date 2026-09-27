package currency

import (
	"context"
	"slices"
	"testing"
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
func lastDayInWindow(flown time.Time, years, months, days int) string {
	d := flown.AddDate(years, months, days)
	for d.AddDate(-years, -months, -days).After(flown) {
		d = d.AddDate(0, 0, -1)
	}
	return d.Format("2006-01-02")
}

// laplLog returns twelve 1h/1-landing SEP flights on days 10, 20 … 120 ago
// and a 1h dual flight 300 days ago.
func laplLog() *datedFlightData {
	d := &datedFlightData{}
	for i := 1; i <= 12; i++ {
		d.flights = append(d.flights, datedFlight{date: daysAgo(10 * i), class: models.ClassTypeSEPLand, progress: Progress{TotalMinutes: 60, PICMinutes: 60, Landings: 1}})
	}
	d.flights = append(d.flights, datedFlight{date: daysAgo(300), class: models.ClassTypeSEPLand, progress: Progress{TotalMinutes: 60, InstructorMinutes: 60}})
	return d
}

func laplRating() (*models.ClassRating, *models.License) {
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "LAPL"}
	return rating, license
}

func TestRecencyExpiresOn_LAPL_EarliestRequirementLapse(t *testing.T) {
	dp := laplLog()
	rating, license := laplRating()

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Fatalf("status = %s, want current", result.Status)
	}
	if result.RecencyExpiresOn == nil {
		t.Fatal("recencyExpiresOn is nil, want a date")
	}
	// 12h/12 landings lapse when the flight 120 days ago leaves the window;
	// the training flight 300 days ago leaves it first.
	want := lastDayInWindow(daysAgo(300), 2, 0, 0)
	if *result.RecencyExpiresOn != want {
		t.Errorf("recencyExpiresOn = %s, want %s", *result.RecencyExpiresOn, want)
	}
}

func TestRecencyExpiresOn_LAPL_ExperienceLapsesFirst(t *testing.T) {
	dp := laplLog()
	dp.flights = append(dp.flights, datedFlight{date: daysAgo(5), class: models.ClassTypeSEPLand, progress: Progress{TotalMinutes: 60, InstructorMinutes: 60, Landings: 1}})
	rating, license := laplRating()

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.RecencyExpiresOn == nil {
		t.Fatal("recencyExpiresOn is nil, want a date")
	}
	// 14h and 13 landings: the flights 300 and 120 days ago can leave the window, the one 110 days ago cannot.
	want := lastDayInWindow(daysAgo(110), 2, 0, 0)
	if *result.RecencyExpiresOn != want {
		t.Errorf("recencyExpiresOn = %s, want %s", *result.RecencyExpiresOn, want)
	}
}

func TestRecencyExpiresOn_LAPL_ProficiencyCheckExtends(t *testing.T) {
	dp := laplLog()
	dp.flights = append(dp.flights, datedFlight{date: daysAgo(2), class: models.ClassTypeSEPLand, progress: Progress{TotalMinutes: 60}, profCheck: true})
	rating, license := laplRating()

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.RecencyExpiresOn == nil {
		t.Fatal("recencyExpiresOn is nil, want a date")
	}
	want := lastDayInWindow(daysAgo(2), 2, 0, 0)
	if *result.RecencyExpiresOn != want {
		t.Errorf("recencyExpiresOn = %s, want %s", *result.RecencyExpiresOn, want)
	}
}

func TestRecencyExpiresOn_NotCurrent(t *testing.T) {
	dp := laplLog()
	dp.flights = dp.flights[1:]
	rating, license := laplRating()

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.Status == StatusCurrent {
		t.Fatalf("status = current, want not current")
	}
	if result.RecencyExpiresOn != nil {
		t.Errorf("recencyExpiresOn = %s, want nil", *result.RecencyExpiresOn)
	}
}

func TestRecencyExpiresOn_FAAInstrument(t *testing.T) {
	dp := &datedFlightData{flights: []datedFlight{
		{date: daysAgo(20), class: models.ClassTypeSEPLand, progress: Progress{Approaches: 4, Holds: 1}},
		{date: daysAgo(40), class: models.ClassTypeSEPLand, progress: Progress{Approaches: 3}},
	}}
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}

	result := NewFAAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Fatalf("status = %s, want current", result.Status)
	}
	if result.RecencyExpiresOn == nil {
		t.Fatal("recencyExpiresOn is nil, want a date")
	}
	want := lastDayInWindow(daysAgo(40), 0, 6, 0)
	if *result.RecencyExpiresOn != want {
		t.Errorf("recencyExpiresOn = %s, want %s", *result.RecencyExpiresOn, want)
	}
}

func TestRecencyExpiresOn_RevalidationRuleOmitted(t *testing.T) {
	dp := laplLog()
	rating, license := laplRating()
	license.LicenseType = "PPL"
	exp := daysAgo(-200)
	rating.ExpiryDate = &exp

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.RecencyExpiresOn != nil {
		t.Errorf("recencyExpiresOn = %s, want nil for an expiry-anchored rule", *result.RecencyExpiresOn)
	}
}
