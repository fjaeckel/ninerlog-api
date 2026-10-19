package currency

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// asOfFlightData is a datedFlightData that can read as of a past day.
type asOfFlightData struct {
	*datedFlightData
}

func (a asOfFlightData) AsOf(day time.Time) FlightDataProvider {
	bounded := &datedFlightData{}
	for _, f := range a.flights {
		if !f.date.After(day) {
			bounded.flights = append(bounded.flights, f)
		}
	}
	return bounded
}

// landingDays returns one landing day per entry, newest first: days ago and landings.
func landingDays(today time.Time, entries ...[2]int) []LandingDay {
	var out []LandingDay
	for _, e := range entries {
		out = append(out, LandingDay{Date: today.AddDate(0, 0, -e[0]), DayLandings: e[1]})
	}
	return out
}

func TestPaxLapseDate(t *testing.T) {
	today := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	day := func(ago int) *time.Time { d := today.AddDate(0, 0, -ago); return &d }

	tests := []struct {
		name    string
		history []LandingDay
		want    *time.Time
	}{
		{
			// binding landing 100 days ago counts through day 10 ago
			name:    "lapsed after the binding landing left the window",
			history: landingDays(today, [2]int{92, 1}, [2]int{95, 1}, [2]int{100, 1}),
			want:    day(9),
		},
		{
			name:    "a later landing that does not restore currency keeps the lapse date",
			history: landingDays(today, [2]int{2, 1}, [2]int{92, 1}, [2]int{95, 1}, [2]int{100, 1}),
			want:    day(9),
		},
		{
			name:    "regained and lapsed again reports the newer lapse",
			history: landingDays(today, [2]int{93, 3}, [2]int{300, 3}),
			want:    day(2),
		},
		{
			name:    "never current",
			history: landingDays(today, [2]int{20, 1}, [2]int{400, 1}),
			want:    nil,
		},
		{
			name:    "no landings",
			history: nil,
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paxLapseDate(tt.history, 3, allLandings, nil, today.Add(9*time.Hour))
			if !sameDay(got, tt.want) {
				t.Errorf("paxLapseDate = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPaxLapseDate_NightWaiverExtendsCurrency(t *testing.T) {
	today := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	night := []LandingDay{{Date: today.AddDate(0, 0, -100), NightLandings: 1}}
	irExpiry := today.AddDate(0, 0, -5)

	got := paxLapseDate(night, 1, nightLandings, &irExpiry, today)
	want := today.AddDate(0, 0, -4)
	if !sameDay(got, &want) {
		t.Errorf("with IR valid through 5 days ago: lapse = %v, want %v", got, want)
	}

	got = paxLapseDate(night, 1, nightLandings, nil, today)
	want = today.AddDate(0, 0, -9)
	if !sameDay(got, &want) {
		t.Errorf("without IR: lapse = %v, want %v", got, want)
	}
}

func TestFillPaxLapseDates_OnlyUnderWithLapseDates(t *testing.T) {
	fetched := 0
	fetch := func(time.Time) ([]LandingDay, error) { fetched++; return nil, nil }
	result := PassengerCurrency{DayStatus: StatusExpired, DayRequired: 3}

	fillPaxLapseDates(context.Background(), &result, fetch, nil, time.Now())
	if fetched != 0 {
		t.Errorf("history fetched %d times without WithLapseDates", fetched)
	}
	fillPaxLapseDates(WithLapseDates(context.Background()), &result, fetch, nil, time.Now())
	if fetched != 1 {
		t.Errorf("history fetched %d times under WithLapseDates, want 1", fetched)
	}
}

func TestEASANightWaiver_IRValidThroughExpiryDate(t *testing.T) {
	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL(A)"}
	today := models.CalendarDate(time.Now())

	tests := []struct {
		name      string
		irExpiry  time.Time
		wantNight Status
	}{
		{"IR expiring today waives night recency", today, StatusCurrent},
		{"IR expiring tomorrow waives night recency", today.AddDate(0, 0, 1), StatusCurrent},
		{"IR expired yesterday no longer waives", today.AddDate(0, 0, -1), StatusExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expiry := tt.irExpiry
			peers := []*models.ClassRating{{ID: uuid.New(), ClassType: models.ClassTypeIR, ExpiryDate: &expiry}}
			pax := NewEASAEvaluator().EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, peers, &datedFlightData{})
			if pax.NightStatus != tt.wantNight {
				t.Errorf("night status = %s, want %s", pax.NightStatus, tt.wantNight)
			}
		})
	}
}

// shiftedLAPLLog is laplLog moved back by n days.
func shiftedLAPLLog(n int) *datedFlightData {
	d := laplLog()
	for i := range d.flights {
		d.flights[i].date = d.flights[i].date.AddDate(0, 0, -n)
	}
	return d
}

func TestRatingLapsedOn_LAPL(t *testing.T) {
	ctx := WithLapseDates(context.Background())
	rating, license := laplRating()

	// the training flight, 800 days ago, is the first requirement to leave the window
	training := daysAgo(800)
	want := training.AddDate(2, 0, 0)

	t.Run("lapsed rolling rule reports its first non-current day", func(t *testing.T) {
		result := NewEASAEvaluator().Evaluate(ctx, rating, license, asOfFlightData{shiftedLAPLLog(500)})
		if result.Status == StatusCurrent {
			t.Fatalf("status = %s, want lapsed", result.Status)
		}
		if !sameDay(result.LapsedOn, &want) {
			t.Errorf("lapsedOn = %v, want %v", result.LapsedOn, want)
		}
	})

	t.Run("a later flight that does not restore currency keeps the lapse date", func(t *testing.T) {
		dp := shiftedLAPLLog(500)
		dp.flights = append(dp.flights, datedFlight{date: daysAgo(3), class: models.ClassTypeSEPLand, progress: Progress{TotalMinutes: 60, PICMinutes: 60, Landings: 1}})
		result := NewEASAEvaluator().Evaluate(ctx, rating, license, asOfFlightData{dp})
		if !sameDay(result.LapsedOn, &want) {
			t.Errorf("lapsedOn = %v, want %v", result.LapsedOn, want)
		}
	})

	t.Run("not computed without WithLapseDates", func(t *testing.T) {
		result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, asOfFlightData{shiftedLAPLLog(500)})
		if result.LapsedOn != nil {
			t.Errorf("lapsedOn = %v, want nil", result.LapsedOn)
		}
	})

	t.Run("nil when the provider cannot read as of a day", func(t *testing.T) {
		result := NewEASAEvaluator().Evaluate(ctx, rating, license, shiftedLAPLLog(500))
		if result.LapsedOn != nil {
			t.Errorf("lapsedOn = %v, want nil", result.LapsedOn)
		}
	})

	t.Run("current rule has no lapse date", func(t *testing.T) {
		result := NewEASAEvaluator().Evaluate(ctx, rating, license, asOfFlightData{laplLog()})
		if result.Status != StatusCurrent || result.LapsedOn != nil {
			t.Errorf("status = %s, lapsedOn = %v; want current, nil", result.Status, result.LapsedOn)
		}
	})
}

func TestCustomLapsedOn(t *testing.T) {
	body := &models.CustomCurrencyRuleBody{
		Window:       models.CurrencyWindow{Amount: 30, Unit: "days"},
		Requirements: []models.CurrencyRequirement{{Metric: "landings", Min: 3}},
	}
	flown := fixedNow().AddDate(0, 0, -40)
	want := models.CalendarDate(flown.AddDate(0, 0, 30))

	tests := []struct {
		name  string
		rows  []map[string]int64
		dates []time.Time
		want  *time.Time
	}{
		{"lapse is the day the flight left the window", []map[string]int64{{"landings": 3}}, []time.Time{flown}, &want},
		{
			"a later flight that does not restore the rule keeps the lapse date",
			[]map[string]int64{{"landings": 3}, {"landings": 1}},
			[]time.Time{flown, fixedNow().AddDate(0, 0, -5)},
			&want,
		},
		{"never met", []map[string]int64{{"landings": 2}}, []time.Time{flown}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &fakeCustomData{aggregates: map[string]int64{"landings": 0}, rowsByMetric: tt.rows, rowDates: tt.dates}
			res, err := newTestEvaluator(data).Evaluate(WithLapseDates(context.Background()), uuid.New(), body)
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if res.Status != StatusExpired {
				t.Fatalf("status = %s, want expired", res.Status)
			}
			if !sameDay(res.LapsedOn, tt.want) {
				t.Errorf("lapsedOn = %v, want %v", res.LapsedOn, tt.want)
			}
		})
	}

	t.Run("not computed without WithLapseDates", func(t *testing.T) {
		data := &fakeCustomData{aggregates: map[string]int64{"landings": 0}}
		res, err := newTestEvaluator(data).Evaluate(context.Background(), uuid.New(), body)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if res.LapsedOn != nil || data.rowsRequested {
			t.Errorf("lapsedOn = %v, rows requested = %v; want nil, false", res.LapsedOn, data.rowsRequested)
		}
	})
}

// sameDay reports whether two optional dates name the same calendar day.
func sameDay(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Format("2006-01-02") == b.Format("2006-01-02")
}
