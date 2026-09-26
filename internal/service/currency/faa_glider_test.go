package currency

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func TestFAAGliderRating(t *testing.T) {
	tests := []struct {
		name          string
		licenseType   string
		class         models.ClassType
		lastReview    *time.Time
		progress      map[models.ClassType]*Progress
		progressErr   error
		wantStatus    Status
		wantKey       string
		wantDate      bool
		wantReviewMet bool
		wantTraining  int
	}{
		{
			name: "FAA glider: pax rule does not expire the rating", licenseType: "PRIVATE", class: models.ClassTypeGlider,
			lastReview: pastDate(100),
			wantStatus: StatusCurrent, wantKey: MsgFlightReviewCurrent, wantDate: true, wantReviewMet: true,
		},
		{
			name: "FAA glider: pax currency does not make the rating current", licenseType: "PRIVATE", class: models.ClassTypeGlider,
			progress:   map[models.ClassType]*Progress{models.ClassTypeGlider: {Landings: 12, PICMinutes: 300, Flights: 12}},
			wantStatus: StatusExpired, wantKey: MsgFlightReviewNoneOnRecord,
		},
		{
			name: "61.56 glider alternative: three instructional flights", licenseType: "PRIVATE", class: models.ClassTypeGlider,
			progress:   map[models.ClassType]*Progress{models.ClassTypeGlider: {TrainingFlights: 3, InstructorMinutes: 30, Flights: 3}},
			wantStatus: StatusCurrent, wantKey: MsgRatingFlightReviewGliderAlt, wantTraining: 3,
		},
		{
			name: "61.56 glider alternative: two instructional flights are not enough", licenseType: "PRIVATE", class: models.ClassTypeGlider,
			progress:   map[models.ClassType]*Progress{models.ClassTypeGlider: {TrainingFlights: 2, Flights: 2}},
			wantStatus: StatusExpired, wantKey: MsgFlightReviewNoneOnRecord, wantTraining: 2,
		},
		{
			name: "61.56 glider alternative: instructional flights on other classes do not count", licenseType: "PRIVATE", class: models.ClassTypeGlider,
			progress:   map[models.ClassType]*Progress{models.ClassTypeSEPLand: {TrainingFlights: 5, Flights: 5}},
			wantStatus: StatusExpired, wantKey: MsgFlightReviewNoneOnRecord,
		},
		{
			name: "61.56 glider alternative replaces an expired review", licenseType: "PRIVATE", class: models.ClassTypeGlider,
			lastReview: pastDate(800),
			progress:   map[models.ClassType]*Progress{models.ClassTypeGlider: {TrainingFlights: 4, Flights: 4}},
			wantStatus: StatusCurrent, wantKey: MsgRatingFlightReviewGliderAlt, wantTraining: 4,
		},
		{
			name: "61.56 expired review without alternative", licenseType: "PRIVATE", class: models.ClassTypeGlider,
			lastReview: pastDate(800),
			wantStatus: StatusExpired, wantKey: MsgFlightReviewExpired, wantDate: true,
		},
		{
			name: "61.56 expiring review without alternative", licenseType: "PRIVATE", class: models.ClassTypeGlider,
			lastReview: pastDate(700),
			wantStatus: StatusExpiring, wantKey: MsgFlightReviewExpiring, wantDate: true, wantReviewMet: true,
		},
		{
			name: "FAA glider licence with legacy SEP rating uses the flight review", licenseType: "Glider", class: models.ClassTypeSEPLand,
			lastReview: pastDate(30),
			wantStatus: StatusCurrent, wantKey: MsgFlightReviewCurrent, wantDate: true, wantReviewMet: true,
		},
		{
			name: "flight data error", licenseType: "PRIVATE", class: models.ClassTypeGlider,
			progressErr: errors.New("db down"),
			wantStatus:  StatusUnknown, wantKey: MsgRatingEvaluationFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := newMockFlightDataProvider()
			for ct, p := range tt.progress {
				dp.progressByClass[ct] = p
			}
			dp.lastFlightReview = tt.lastReview
			dp.progressErr = tt.progressErr
			rating := &models.ClassRating{ID: uuid.New(), ClassType: tt.class, LicenseID: uuid.New()}
			license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: tt.licenseType}

			r := NewFAAEvaluator().Evaluate(context.Background(), rating, license, dp)

			if r.Status != tt.wantStatus {
				t.Errorf("Status = %s, want %s", r.Status, tt.wantStatus)
			}
			if r.MessageKey != tt.wantKey {
				t.Errorf("MessageKey = %q, want %q", r.MessageKey, tt.wantKey)
			}
			if got := r.MessageParams != nil && r.MessageParams.Date != nil; got != tt.wantDate {
				t.Errorf("date param present = %v, want %v", got, tt.wantDate)
			}
			if r.RuleDescriptionKey != "faa_flight_review" {
				t.Errorf("RuleDescriptionKey = %q, want faa_flight_review", r.RuleDescriptionKey)
			}
			if tt.progressErr != nil {
				return
			}
			if len(r.Requirements) != 2 {
				t.Fatalf("requirements = %+v, want flight review + training flights", r.Requirements)
			}
			review, training := r.Requirements[0], r.Requirements[1]
			if review.NameKey != ReqKeyFlightReview || review.Met != tt.wantReviewMet {
				t.Errorf("review requirement = %+v, want %s met=%v", review, ReqKeyFlightReview, tt.wantReviewMet)
			}
			if training.NameKey != ReqKeyTrainingFlights || training.Required != 3 || int(training.Current) != tt.wantTraining {
				t.Errorf("training requirement = %+v, want %s %d/3", training, ReqKeyTrainingFlights, tt.wantTraining)
			}
			for _, req := range r.Requirements {
				if req.NameKey == "requirement.launches_and_landings" || req.NameKey == ReqKeyDayLandings {
					t.Errorf("rating carries passenger requirement %s", req.NameKey)
				}
			}
			if len(dp.lastClasses) != 1 || dp.lastClasses[0] != models.ClassTypeGlider || !dp.includeTowed[models.ClassTypeGlider] {
				t.Errorf("counted classes %v towed=%v, want GLIDER with towed launches", dp.lastClasses, dp.includeTowed[models.ClassTypeGlider])
			}
		})
	}
}

func TestFAAFlightReviewWindowStart(t *testing.T) {
	now := time.Date(2026, 9, 26, 15, 0, 0, 0, time.UTC)
	start := faaFlightReviewWindowStart(now)
	if want := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Fatalf("window start = %s, want %s", start, want)
	}
	if s := faaFlightReviewStatus(now, start).Status; s == StatusExpired {
		t.Errorf("review on window start is %s, want valid", s)
	}
	if s := faaFlightReviewStatus(now, start.AddDate(0, 0, -1)).Status; s != StatusExpired {
		t.Errorf("review the day before window start is %s, want expired", s)
	}
}

func TestFAAGliderPassengerCurrency(t *testing.T) {
	tests := []struct {
		name        string
		licenseType string
		class       models.ClassType
		wantNight   bool
		wantPICOnly bool
		wantTowed   bool
		wantRuleKey string
		wantMsg     string
	}{
		{"FAA Private with glider rating: no night requirement", "PRIVATE", models.ClassTypeGlider, false, true, true, "faa_glider", MsgPaxCurrentDayNoNight},
		{"FAA Commercial with glider rating: no night requirement", "Commercial", models.ClassTypeGlider, false, true, true, "faa_glider", MsgPaxCurrentDayNoNight},
		{"FAA glider licence with legacy SEP rating: PIC only", "Glider", models.ClassTypeSEPLand, false, true, true, "faa_glider", MsgPaxCurrentDayNoNight},
		{"FAA Private SEP unchanged", "PRIVATE", models.ClassTypeSEPLand, true, false, false, "faa_pax_day_night", MsgPaxCurrentDayNight},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := newMockFlightDataProvider()
			dp.landingDays = map[models.ClassType][]LandingDay{
				tt.class: {{Date: day(3), DayLandings: 3, NightLandings: 3}},
			}
			license := &models.License{UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: tt.licenseType}

			pax := NewFAAEvaluator().EvaluatePassengerCurrency(context.Background(), tt.class, license, nil, dp)

			if pax.NightPrivilege != tt.wantNight {
				t.Errorf("NightPrivilege = %v, want %v", pax.NightPrivilege, tt.wantNight)
			}
			if !tt.wantNight && (pax.NightRequired != 0 || pax.NightStatus != StatusUnknown || pax.NightExpiresOn != nil) {
				t.Errorf("night = required %d status %s expires %v, want not evaluated", pax.NightRequired, pax.NightStatus, pax.NightExpiresOn)
			}
			if pax.DayStatus != StatusCurrent || pax.DayLandings != 6 {
				t.Errorf("day = %s with %d landings, want current with 6", pax.DayStatus, pax.DayLandings)
			}
			if dp.picOnly[tt.class] != tt.wantPICOnly {
				t.Errorf("picOnly = %v, want %v", dp.picOnly[tt.class], tt.wantPICOnly)
			}
			if dp.includeTowed[tt.class] != tt.wantTowed {
				t.Errorf("includeTowed = %v, want %v", dp.includeTowed[tt.class], tt.wantTowed)
			}
			if pax.RuleDescriptionKey != tt.wantRuleKey {
				t.Errorf("RuleDescriptionKey = %q, want %q", pax.RuleDescriptionKey, tt.wantRuleKey)
			}
			if pax.MessageKey != tt.wantMsg {
				t.Errorf("MessageKey = %q, want %q", pax.MessageKey, tt.wantMsg)
			}
		})
	}
}
