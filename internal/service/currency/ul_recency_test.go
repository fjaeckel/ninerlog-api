package currency

import (
	"context"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// ── Training flight: one flight of at least 1h with an instructor ────────

// threeTwentyMinuteDualFlights is three dual flights of 20 minutes each.
func threeTwentyMinuteDualFlights() Progress {
	return Progress{TotalMinutes: 900, PICMinutes: 600, Landings: 20, Flights: 12, InstructorMinutes: 60, TrainingFlights: 3, LongestTrainingFlightMinutes: 20}
}

// oneSixtyMinuteDualFlight is one dual flight of 60 minutes.
func oneSixtyMinuteDualFlight() Progress {
	return Progress{TotalMinutes: 900, PICMinutes: 600, Landings: 20, Flights: 12, InstructorMinutes: 60, TrainingFlights: 1, LongestTrainingFlightMinutes: 60}
}

func TestTrainingFlight_IsOneFlightOfAnHour(t *testing.T) {
	tests := []struct {
		name      string
		kind      models.ULKind
		authority string
		progress  Progress
		wantMet   bool
		want      Status
	}{
		{"M2 three 20-minute dual flights do not satisfy the training flight", models.ULKindThreeAxis, "DULV", threeTwentyMinuteDualFlights(), false, StatusLapsed},
		{"M2 one 60-minute dual flight satisfies", models.ULKindThreeAxis, "DULV", oneSixtyMinuteDualFlight(), true, StatusCurrent},
		{"UL helicopter three 20-minute dual flights do not satisfy §45(2a)", models.ULKindHelicopter, "LBA", threeTwentyMinuteDualFlights(), false, StatusLapsed},
		{"UL helicopter one 60-minute dual flight satisfies §45(2a)", models.ULKindHelicopter, "LBA", oneSixtyMinuteDualFlight(), true, StatusCurrent},
		{"UL gyroplane three 20-minute dual flights do not satisfy", models.ULKindGyroplane, "DULV", threeTwentyMinuteDualFlights(), false, StatusLapsed},
		{"UL gyroplane one 60-minute dual flight satisfies", models.ULKindGyroplane, "DULV", oneSixtyMinuteDualFlight(), true, StatusCurrent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := newMockFlightDataProvider()
			p := tt.progress
			dp.progressByUL = map[models.ULKind]*Progress{tt.kind: &p}
			rating := ulRating(ulKindPtr(tt.kind))

			result := NewGermanULEvaluator().Evaluate(context.Background(), rating, ulLicense(rating, tt.authority), dp)
			req := reqByKey(result.Requirements, ReqKeyTrainingFlight)
			if req == nil {
				t.Fatalf("no %s requirement", ReqKeyTrainingFlight)
			}
			if req.Met != tt.wantMet || req.Current != float64(p.LongestTrainingFlightMinutes) || req.Unit != "minutes" {
				t.Errorf("training flight = %+v, want met=%v current=%d minutes", req, tt.wantMet, p.LongestTrainingFlightMinutes)
			}
			if result.Status != tt.want {
				t.Errorf("Status = %s, want %s", result.Status, tt.want)
			}
		})
	}
}

func TestLAPL_TrainingFlight_IsOneFlightOfAnHour(t *testing.T) {
	tests := []struct {
		name     string
		progress Progress
		wantMet  bool
		want     Status
	}{
		{"LAPL three 20-minute dual flights do not satisfy FCL.140.A(a)(1)", threeTwentyMinuteDualFlights(), false, StatusLapsed},
		{"LAPL one 60-minute dual flight satisfies FCL.140.A(a)(1)", oneSixtyMinuteDualFlight(), true, StatusCurrent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			license, ratings := crossClassSetup("LAPL(A)", models.ClassTypeSEPLand)
			dp := newMockFlightDataProvider()
			p := tt.progress
			dp.progressByClass[models.ClassTypeSEPLand] = &p

			result := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
			req := findReq(result.Requirements, ReqKeyTrainingFlight)
			if req == nil || req.Met != tt.wantMet || req.Current != float64(p.LongestTrainingFlightMinutes) {
				t.Errorf("training flight = %+v, want met=%v current=%d", req, tt.wantMet, p.LongestTrainingFlightMinutes)
			}
			if result.Status != tt.want {
				t.Errorf("Status = %s, want %s", result.Status, tt.want)
			}
		})
	}
}

func TestRefresherTraining_StaysCumulative(t *testing.T) {
	t.Run("FCL.740.A(b)(1)(ii) SEP refresher from three 20-minute dual flights", func(t *testing.T) {
		license, ratings := crossClassSetup("PPL", models.ClassTypeSEPLand)
		dp := newMockFlightDataProvider()
		p := threeTwentyMinuteDualFlights()
		dp.progressByClass[models.ClassTypeSEPLand] = &p

		result := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
		if req := findReq(result.Requirements, ReqKeyRefresherTraining); req == nil || !req.Met || req.Current != 60 {
			t.Errorf("refresher = %+v, want met with 60 cumulative minutes", req)
		}
	})
	t.Run("FCL.240.G GPL refresher from three 20-minute dual flights", func(t *testing.T) {
		license, ratings := crossClassSetup("GPL", models.ClassTypeGyro)
		dp := newMockFlightDataProvider()
		p := threeTwentyMinuteDualFlights()
		dp.progressByClass[models.ClassTypeGyro] = &p

		result := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
		if req := findReq(result.Requirements, ReqKeyRefresherTraining); req == nil || !req.Met || req.Current != 60 {
			t.Errorf("refresher = %+v, want met with 60 cumulative minutes", req)
		}
		if result.Status != StatusCurrent {
			t.Errorf("Status = %s, want current", result.Status)
		}
	})
}

// ── Status: lapsed for rolling recency, expiring for revalidation ────────

func TestRollingRecencyUnmetIsLapsed(t *testing.T) {
	gliderRating := func() (*models.ClassRating, *models.License) {
		r := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeGlider, LicenseID: uuid.New()}
		return r, &models.License{ID: r.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "SPL"}
	}
	laplRating := func() (*models.ClassRating, *models.License) {
		_, ratings := crossClassSetup("LAPL", models.ClassTypeSEPLand)
		return ratings[0], &models.License{ID: ratings[0].LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "LAPL"}
	}
	gplRating := func() (*models.ClassRating, *models.License) {
		_, ratings := crossClassSetup("GPL", models.ClassTypeGyro)
		return ratings[0], &models.License{ID: ratings[0].LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "GPL"}
	}
	ulKind := func(k models.ULKind, authority string) func() (*models.ClassRating, *models.License) {
		return func() (*models.ClassRating, *models.License) {
			r := ulRating(ulKindPtr(k))
			return r, ulLicense(r, authority)
		}
	}
	tests := []struct {
		name  string
		eval  Evaluator
		setup func() (*models.ClassRating, *models.License)
	}{
		{"lapsed when SPL recency unmet (SFCL.160(a))", NewEASAEvaluator(), gliderRating},
		{"lapsed when SPL TMG recency unmet (SFCL.160(b))", NewEASAEvaluator(), splTMG},
		{"lapsed when LAPL(A) recency unmet (FCL.140.A)", NewEASAEvaluator(), laplRating},
		{"lapsed when GPL recency unmet (FCL.240.G)", NewEASAEvaluator(), gplRating},
		{"lapsed when three-axis §45(2) unmet", NewGermanULEvaluator(), ulKind(models.ULKindThreeAxis, "DULV")},
		{"lapsed when UL helicopter §45(2a) unmet", NewGermanULEvaluator(), ulKind(models.ULKindHelicopter, "LBA")},
		{"lapsed when UL gyroplane unmet", NewGermanULEvaluator(), ulKind(models.ULKindGyroplane, "DULV")},
		{"lapsed when trike DULV unmet", NewGermanULEvaluator(), ulKind(models.ULKindWeightShift, "DULV")},
		{"lapsed when trike DAeC unmet", NewGermanULEvaluator(), ulKind(models.ULKindWeightShift, "DAeC")},
		{"lapsed when powered paraglider unmet", NewGermanULEvaluator(), ulKind(models.ULKindPoweredParaglider, "DULV")},
		{"lapsed when UL sailplane unmet", NewGermanULEvaluator(), ulKind(models.ULKindSailplane, "DAeC")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rating, license := tt.setup()
			result := tt.eval.Evaluate(context.Background(), rating, license, newMockFlightDataProvider())
			if result.Status != StatusLapsed || result.MessageKey != MsgRatingRecencyNotMet {
				t.Errorf("Status = %s, MessageKey = %s, want lapsed / %s", result.Status, result.MessageKey, MsgRatingRecencyNotMet)
			}
		})
	}
}

func TestExpiryAnchoredRevalidationUnmetStaysExpiring(t *testing.T) {
	for _, ct := range []models.ClassType{models.ClassTypeSEPLand, models.ClassTypeMEPLand, models.ClassTypeIR} {
		t.Run(string(ct), func(t *testing.T) {
			license, ratings := crossClassSetup("PPL", ct)
			result := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, newMockFlightDataProvider())
			if result.Status != StatusExpiring {
				t.Errorf("Status = %s, want expiring", result.Status)
			}
		})
	}
	t.Run("past expiry date is expired", func(t *testing.T) {
		license, ratings := crossClassSetup("PPL", models.ClassTypeSEPLand)
		ratings[0].ExpiryDate = futureDate(-1)
		result := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, newMockFlightDataProvider())
		if result.Status != StatusExpired {
			t.Errorf("Status = %s, want expired", result.Status)
		}
	})
}

// ── Kindless ultralight flights ─────────────────────────────────────────

// kindlessSeason is enough ULTRALIGHT flying of no kind to meet §45(2) and the trike rule.
func kindlessSeason() *Progress {
	return &Progress{TotalMinutes: 900, PICMinutes: 900, Landings: 20, Flights: 4, InstructorMinutes: 60, TrainingFlights: 1, LongestTrainingFlightMinutes: 60}
}

// ulHolder is a user with the given licences, each carrying its ratings.
type ulHolder struct {
	licRepo *mockLicenseRepo
	crRepo  *mockCRRepo
	userID  uuid.UUID
}

func newULHolder() *ulHolder {
	return &ulHolder{licRepo: newMockLicenseRepo(), crRepo: newMockCRRepo(), userID: uuid.New()}
}

// licence adds a licence with ratings of the given UL kinds; nil is a rating of no kind.
func (h *ulHolder) licence(authority string, kinds ...*models.ULKind) {
	lic := &models.License{ID: uuid.New(), UserID: h.userID, RegulatoryAuthority: authority, LicenseType: "UL"}
	h.licRepo.licenses[lic.ID] = lic
	for _, k := range kinds {
		h.crRepo.ratings[lic.ID] = append(h.crRepo.ratings[lic.ID], &models.ClassRating{ID: uuid.New(), LicenseID: lic.ID, ClassType: models.ClassTypeUL, ULKind: k})
	}
}

func (h *ulHolder) evaluate(t *testing.T, dp *mockFlightDataProvider) *CurrencyStatusResponse {
	t.Helper()
	result, err := newULService(h.licRepo, h.crRepo, dp).EvaluateAll(context.Background(), h.userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}
	return result
}

func TestKindlessULFlights(t *testing.T) {
	trike := ulKindPtr(models.ULKindWeightShift)
	threeAxis := ulKindPtr(models.ULKindThreeAxis)
	tests := []struct {
		name       string
		setup      func(h *ulHolder)
		wantCounts bool
	}{
		{"S1 kindless flight with trike and three-axis ratings counts for neither", func(h *ulHolder) {
			h.licence("DULV", trike, threeAxis)
		}, false},
		{"kindless flight with trike and three-axis ratings on two licences counts for neither", func(h *ulHolder) {
			h.licence("DULV", trike)
			h.licence("DAeC", threeAxis)
		}, false},
		{"kindless flight with a three-axis rating and a rating of no kind does not count", func(h *ulHolder) {
			h.licence("DULV", threeAxis, nil)
		}, false},
		{"kindless flight counts when every UL rating is three-axis", func(h *ulHolder) {
			h.licence("DULV", threeAxis)
		}, true},
		{"kindless flight counts with the same kind on two licences", func(h *ulHolder) {
			h.licence("DULV", threeAxis)
			h.licence("LBA", threeAxis)
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newULHolder()
			tt.setup(h)
			dp := newMockFlightDataProvider()
			dp.progressByClass[models.ClassTypeUL] = kindlessSeason()
			dp.landingDaysByUL = map[models.ULKind][]LandingDay{"": {{Date: day(1), DayLandings: 5, Takeoffs: 5}}}

			result := h.evaluate(t, dp)
			for _, r := range result.Ratings {
				if r.MessageKey == MsgRatingULKindRequired {
					continue
				}
				wantUnclassified := 4
				if tt.wantCounts {
					wantUnclassified = 0
				}
				if r.UnclassifiedFlights != wantUnclassified {
					t.Errorf("%v: UnclassifiedFlights = %d, want %d", r.CreditedULKinds, r.UnclassifiedFlights, wantUnclassified)
				}
				if counted := r.Progress != nil && r.Progress.Flights > 0; counted != tt.wantCounts {
					t.Errorf("%v: kindless flights counted = %v, want %v (progress %+v)", r.CreditedULKinds, counted, tt.wantCounts, r.Progress)
				}
				if tt.wantCounts != (r.Status == StatusCurrent) {
					t.Errorf("%v: Status = %s, want current=%v", r.CreditedULKinds, r.Status, tt.wantCounts)
				}
			}
			for _, pax := range result.PassengerCurrency {
				if counted := pax.DayLandings > 0; counted != tt.wantCounts {
					t.Errorf("passengers %v: DayLandings = %d, want kindless counted=%v", *pax.ULKind, pax.DayLandings, tt.wantCounts)
				}
			}
		})
	}
}

func TestUnknownWhenULRatingHasNoKind_NoPassengerCurrency(t *testing.T) {
	h := newULHolder()
	h.licence("DULV", nil)
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeUL] = kindlessSeason()

	result := h.evaluate(t, dp)
	if len(result.Ratings) != 1 {
		t.Fatalf("ratings = %d, want 1", len(result.Ratings))
	}
	r := result.Ratings[0]
	if r.Status != StatusUnknown || r.MessageKey != MsgRatingULKindRequired || len(r.Requirements) != 0 {
		t.Errorf("rating = %s / %s with %d requirements, want unknown / %s with none", r.Status, r.MessageKey, len(r.Requirements), MsgRatingULKindRequired)
	}
	if len(result.PassengerCurrency) != 0 {
		t.Errorf("passenger currency = %+v, want none", result.PassengerCurrency)
	}
}

// ── §45a: take-offs and landings ────────────────────────────────────────

func TestGermanUL_PassengerCurrency_TakeoffsAndLandings(t *testing.T) {
	tests := []struct {
		name       string
		days       []LandingDay
		want       Status
		wantCount  int
		wantExpiry *string
	}{
		{"three landings but two take-offs is not current", []LandingDay{{Date: day(1), DayLandings: 3, Takeoffs: 2}}, StatusExpired, 2, nil},
		{"three take-offs but two landings is not current", []LandingDay{{Date: day(1), DayLandings: 2, Takeoffs: 3}}, StatusExpired, 2, nil},
		{"three take-offs and three landings is current", []LandingDay{{Date: day(1), DayLandings: 3, Takeoffs: 3}}, StatusCurrent, 3, ptrStr(dayStr(1 - paxWindowDays))},
		{"expiry follows the older of the third take-off and the third landing", []LandingDay{
			{Date: day(1), DayLandings: 3, Takeoffs: 1},
			{Date: day(20), Takeoffs: 2},
		}, StatusCurrent, 3, ptrStr(dayStr(20 - paxWindowDays))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := newMockFlightDataProvider()
			dp.landingDaysByUL = map[models.ULKind][]LandingDay{models.ULKindThreeAxis: tt.days}
			rating := ulRating(ulKindPtr(models.ULKindThreeAxis))

			result := NewGermanULEvaluator().EvaluateRatingPassengerCurrency(context.Background(), rating, ulLicense(rating, "DULV"), nil, dp)
			if result.DayStatus != tt.want || result.DayLandings != tt.wantCount {
				t.Errorf("DayStatus = %s, DayLandings = %d, want %s, %d", result.DayStatus, result.DayLandings, tt.want, tt.wantCount)
			}
			if (result.DayExpiresOn == nil) != (tt.wantExpiry == nil) || (tt.wantExpiry != nil && *result.DayExpiresOn != *tt.wantExpiry) {
				t.Errorf("DayExpiresOn = %v, want %v", derefStr(result.DayExpiresOn), derefStr(tt.wantExpiry))
			}
			if tt.want == StatusExpired {
				if result.MessageParams == nil || result.MessageParams.Needed == nil || *result.MessageParams.Needed != 3-tt.wantCount {
					t.Errorf("MessageParams = %+v, want needed %d", result.MessageParams, 3-tt.wantCount)
				}
			}
		})
	}
}

func derefStr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func ptrStr(s string) *string { return &s }
