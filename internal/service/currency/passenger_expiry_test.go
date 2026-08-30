package currency

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// day returns midnight UTC n days before today.
func day(n int) time.Time {
	return truncateDay(time.Now().AddDate(0, 0, -n))
}

// dayStr formats midnight UTC n days before today as YYYY-MM-DD.
func dayStr(n int) string {
	return day(n).Format("2006-01-02")
}

func TestPaxWindowStart(t *testing.T) {
	now := time.Date(2026, 8, 30, 17, 45, 12, 0, time.UTC)
	got := paxWindowStart(now)
	want := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("paxWindowStart = %s, want %s", got, want)
	}
}

// TestPaxWindowStart_TimeOfDayIndependent pins the window to the date, so a
// currency evaluation does not change result over the course of a day.
func TestPaxWindowStart_TimeOfDayIndependent(t *testing.T) {
	morning := paxWindowStart(time.Date(2026, 8, 30, 0, 1, 0, 0, time.UTC))
	evening := paxWindowStart(time.Date(2026, 8, 30, 23, 59, 0, 0, time.UTC))
	if !morning.Equal(evening) {
		t.Errorf("window start differs within a day: %s vs %s", morning, evening)
	}
}

func TestPaxExpiryDate(t *testing.T) {
	tests := []struct {
		name     string
		days     []LandingDay
		required int
		count    paxLandingCount
		want     *time.Time
	}{
		{
			name:     "exactly the required landings — oldest one sets the expiry",
			days:     []LandingDay{{Date: day(10), DayLandings: 1}, {Date: day(30), DayLandings: 1}, {Date: day(50), DayLandings: 1}},
			required: 3,
			count:    allLandings,
			want:     ptrTime(day(50).AddDate(0, 0, 90)),
		},
		{
			name:     "surplus landings — the third most recent sets the expiry",
			days:     []LandingDay{{Date: day(5), DayLandings: 1}, {Date: day(10), DayLandings: 1}, {Date: day(20), DayLandings: 1}, {Date: day(80), DayLandings: 4}},
			required: 3,
			count:    allLandings,
			want:     ptrTime(day(20).AddDate(0, 0, 90)),
		},
		{
			name:     "several landings on one date",
			days:     []LandingDay{{Date: day(7), DayLandings: 3}, {Date: day(60), DayLandings: 5}},
			required: 3,
			count:    allLandings,
			want:     ptrTime(day(7).AddDate(0, 0, 90)),
		},
		{
			name:     "night landings count toward the day requirement",
			days:     []LandingDay{{Date: day(4), NightLandings: 2}, {Date: day(40), DayLandings: 1}},
			required: 3,
			count:    allLandings,
			want:     ptrTime(day(40).AddDate(0, 0, 90)),
		},
		{
			name:     "day landings do not count toward the night requirement",
			days:     []LandingDay{{Date: day(4), DayLandings: 9}, {Date: day(40), NightLandings: 1}},
			required: 1,
			count:    nightLandings,
			want:     ptrTime(day(40).AddDate(0, 0, 90)),
		},
		{
			name:     "requirement not met",
			days:     []LandingDay{{Date: day(10), DayLandings: 2}},
			required: 3,
			count:    allLandings,
			want:     nil,
		},
		{
			name:     "no landings at all",
			days:     nil,
			required: 3,
			count:    allLandings,
			want:     nil,
		},
		{
			name:     "waived requirement never expires",
			days:     []LandingDay{{Date: day(10), DayLandings: 5}},
			required: 0,
			count:    nightLandings,
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paxExpiryDate(tt.days, tt.required, tt.count)
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("paxExpiryDate = %s, want nil", got.Format("2006-01-02"))
			case tt.want != nil && got == nil:
				t.Errorf("paxExpiryDate = nil, want %s", tt.want.Format("2006-01-02"))
			case tt.want != nil && !got.Equal(*tt.want):
				t.Errorf("paxExpiryDate = %s, want %s", got.Format("2006-01-02"), tt.want.Format("2006-01-02"))
			}
		})
	}
}

// TestPaxExpiryDate_LastDayInclusive pins the boundary: a landing flown exactly
// paxWindowDays ago is still inside the window that paxWindowStart opens.
func TestPaxExpiryDate_LastDayInclusive(t *testing.T) {
	oldest := day(paxWindowDays)
	if oldest.Before(paxWindowStart(time.Now())) {
		t.Fatalf("landing on %s falls outside the window starting %s", oldest, paxWindowStart(time.Now()))
	}
	got := paxExpiryDate([]LandingDay{{Date: oldest, DayLandings: 3}}, 3, allLandings)
	if got == nil {
		t.Fatal("paxExpiryDate = nil, want today")
	}
	if want := truncateDay(time.Now()); !got.Equal(want) {
		t.Errorf("paxExpiryDate = %s, want %s (currency lasts through today)", got.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestPaxExpiryNote(t *testing.T) {
	a, b := "2026-11-15", "2026-10-02"
	tests := []struct {
		name       string
		day, night *string
		want       string
	}{
		{name: "neither", want: ""},
		{name: "day only", day: &a, want: " — day expires 2026-11-15"},
		{name: "night only", night: &b, want: " — night expires 2026-10-02"},
		{name: "both", day: &a, night: &b, want: " — day expires 2026-11-15, night expires 2026-10-02"},
		{name: "same date collapses", day: &a, night: &a, want: " — expires 2026-11-15"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := paxExpiryNote(tt.day, tt.night); got != tt.want {
				t.Errorf("paxExpiryNote = %q, want %q", got, tt.want)
			}
		})
	}
}

// ── Evaluator-level expiry ──────────────────────────────────────────────

func TestEASA_PassengerCurrency_ExpiryDates(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.landingDays = map[models.ClassType][]LandingDay{
		models.ClassTypeSEPLand: {
			{Date: day(5), DayLandings: 1},
			{Date: day(20), NightLandings: 1},
			{Date: day(45), DayLandings: 1},
			{Date: day(70), DayLandings: 2},
		},
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, nil, dp)

	if result.DayStatus != StatusCurrent || result.NightStatus != StatusCurrent {
		t.Fatalf("statuses = %s/%s, want current/current", result.DayStatus, result.NightStatus)
	}
	// Third most recent landing is the one on day −45.
	if want := dayStr(45 - paxWindowDays); result.DayExpiresOn == nil || *result.DayExpiresOn != want {
		t.Errorf("DayExpiresOn = %v, want %s", result.DayExpiresOn, want)
	}
	// FCL.060(b)(2)(i) needs one night landing — the one on day −20.
	if want := dayStr(20 - paxWindowDays); result.NightExpiresOn == nil || *result.NightExpiresOn != want {
		t.Errorf("NightExpiresOn = %v, want %s", result.NightExpiresOn, want)
	}
	if !strings.Contains(result.Message, *result.DayExpiresOn) {
		t.Errorf("Message = %q, want it to name the day expiry %s", result.Message, *result.DayExpiresOn)
	}
}

// TestEASA_PassengerCurrency_IRWaiverHasNoNightExpiry — a waived requirement
// cannot lapse, so no night expiry is reported.
func TestEASA_PassengerCurrency_IRWaiverHasNoNightExpiry(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.landingDays = map[models.ClassType][]LandingDay{
		models.ClassTypeSEPLand: {{Date: day(3), DayLandings: 3}},
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	peerRatings := []*models.ClassRating{
		{ID: uuid.New(), LicenseID: license.ID, ClassType: models.ClassTypeIR, ExpiryDate: futureDate(6)},
	}

	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, peerRatings, dp)

	if result.NightExpiresOn != nil {
		t.Errorf("NightExpiresOn = %s, want nil (night requirement waived)", *result.NightExpiresOn)
	}
	if result.DayExpiresOn == nil {
		t.Fatal("DayExpiresOn = nil, want a date")
	}
}

// TestEASA_PassengerCurrency_NoNightPrivilegeHasNoNightExpiry — LAPL has no
// night privilege, so night recency is not evaluated.
func TestEASA_PassengerCurrency_NoNightPrivilegeHasNoNightExpiry(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.landingDays = map[models.ClassType][]LandingDay{
		models.ClassTypeSEPLand: {{Date: day(3), DayLandings: 3, NightLandings: 2}},
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "LAPL"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, nil, dp)

	if result.NightExpiresOn != nil {
		t.Errorf("NightExpiresOn = %s, want nil (no night privilege)", *result.NightExpiresOn)
	}
}

// TestPassengerCurrency_NotCurrentHasNoExpiry — an unmet requirement has
// nothing to expire.
func TestPassengerCurrency_NotCurrentHasNoExpiry(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.landingDays = map[models.ClassType][]LandingDay{
		models.ClassTypeSEPLand: {{Date: day(3), DayLandings: 2}},
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, nil, dp)

	if result.DayStatus != StatusExpired {
		t.Fatalf("DayStatus = %s, want expired", result.DayStatus)
	}
	if result.DayExpiresOn != nil {
		t.Errorf("DayExpiresOn = %s, want nil", *result.DayExpiresOn)
	}
	if strings.Contains(result.Message, "expires") {
		t.Errorf("Message = %q, want no expiry note", result.Message)
	}
}

// TestPassengerCurrency_OutOfWindowLandingsIgnored — landings older than the
// window neither count nor set an expiry.
func TestPassengerCurrency_OutOfWindowLandingsIgnored(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.landingDays = map[models.ClassType][]LandingDay{
		models.ClassTypeSEPLand: {
			{Date: day(2), DayLandings: 2},
			{Date: day(120), DayLandings: 5},
		},
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, nil, dp)

	if result.DayLandings != 2 {
		t.Errorf("DayLandings = %d, want 2", result.DayLandings)
	}
	if result.DayExpiresOn != nil {
		t.Errorf("DayExpiresOn = %s, want nil", *result.DayExpiresOn)
	}
}

func TestFAA_PassengerCurrency_ExpiryDates(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.landingDays = map[models.ClassType][]LandingDay{
		models.ClassTypeSEPLand: {
			{Date: day(5), NightLandings: 1},
			{Date: day(15), NightLandings: 1},
			{Date: day(35), NightLandings: 1},
		},
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PRIVATE"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, nil, dp)

	if result.DayStatus != StatusCurrent || result.NightStatus != StatusCurrent {
		t.Fatalf("statuses = %s/%s, want current/current", result.DayStatus, result.NightStatus)
	}
	// §61.57(a) and (b) both need 3 — same landing sets both expiries.
	want := dayStr(35 - paxWindowDays)
	if result.DayExpiresOn == nil || *result.DayExpiresOn != want {
		t.Errorf("DayExpiresOn = %v, want %s", result.DayExpiresOn, want)
	}
	if result.NightExpiresOn == nil || *result.NightExpiresOn != want {
		t.Errorf("NightExpiresOn = %v, want %s", result.NightExpiresOn, want)
	}
	if !strings.Contains(result.Message, " — expires "+want) {
		t.Errorf("Message = %q, want a collapsed expiry note for %s", result.Message, want)
	}
}

// TestFAA_PassengerCurrency_NightExpiresBeforeDay — day currency can outlive
// night currency; both dates are reported.
func TestFAA_PassengerCurrency_NightExpiresBeforeDay(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.landingDays = map[models.ClassType][]LandingDay{
		models.ClassTypeSEPLand: {
			{Date: day(10), DayLandings: 3},
			{Date: day(60), NightLandings: 3},
		},
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PRIVATE"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, nil, dp)

	if want := dayStr(10 - paxWindowDays); result.DayExpiresOn == nil || *result.DayExpiresOn != want {
		t.Errorf("DayExpiresOn = %v, want %s", result.DayExpiresOn, want)
	}
	if want := dayStr(60 - paxWindowDays); result.NightExpiresOn == nil || *result.NightExpiresOn != want {
		t.Errorf("NightExpiresOn = %v, want %s", result.NightExpiresOn, want)
	}
}

// TestFAA_PassengerCurrency_NoNightPrivilege — Sport pilots get no night expiry.
func TestFAA_PassengerCurrency_NoNightPrivilegeHasNoNightExpiry(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.landingDays = map[models.ClassType][]LandingDay{
		models.ClassTypeSEPLand: {{Date: day(8), DayLandings: 2, NightLandings: 3}},
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "SPORT"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, nil, dp)

	if result.NightExpiresOn != nil {
		t.Errorf("NightExpiresOn = %s, want nil (no night privilege)", *result.NightExpiresOn)
	}
	if want := dayStr(8 - paxWindowDays); result.DayExpiresOn == nil || *result.DayExpiresOn != want {
		t.Errorf("DayExpiresOn = %v, want %s", result.DayExpiresOn, want)
	}
}

func TestGermanUL_PassengerCurrency_ExpiryDate(t *testing.T) {
	eval := NewGermanULEvaluator()
	dp := newMockFlightDataProvider()
	dp.landingDays = map[models.ClassType][]LandingDay{
		models.ClassTypeSEPLand: {
			{Date: day(12), DayLandings: 2},
			{Date: day(55), DayLandings: 1},
		},
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "DULV", LicenseType: "UL"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, nil, dp)

	want := dayStr(55 - paxWindowDays)
	if result.DayExpiresOn == nil || *result.DayExpiresOn != want {
		t.Errorf("DayExpiresOn = %v, want %s", result.DayExpiresOn, want)
	}
	if !strings.Contains(result.Message, "gültig bis "+want) {
		t.Errorf("Message = %q, want it to name %s", result.Message, want)
	}
	if result.NightExpiresOn != nil {
		t.Errorf("NightExpiresOn = %s, want nil (UL has no night flying)", *result.NightExpiresOn)
	}
}

// TestPassengerCurrency_DataErrorReportsUnknown — a failed read reports unknown
// rather than a fabricated expiry.
func TestPassengerCurrency_DataErrorReportsUnknown(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.landingDaysErr = errors.New("db down")

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	result := NewEASAEvaluator().EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, nil, dp)

	if result.DayStatus != StatusUnknown || result.NightStatus != StatusUnknown {
		t.Errorf("statuses = %s/%s, want unknown/unknown", result.DayStatus, result.NightStatus)
	}
	if result.DayExpiresOn != nil || result.NightExpiresOn != nil {
		t.Error("expiry dates set despite a failed read")
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
