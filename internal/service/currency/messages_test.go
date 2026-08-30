package currency

import (
	"context"
	"strings"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// knownMessageKeys is the catalogue every emitted key must belong to. It is the
// Go half of the cross-repo contract in docs/CURRENCY_MESSAGES.md; a key added
// here without a matching entry in the client catalogues renders as its
// deprecated English fallback.
var knownMessageKeys = map[string]bool{
	MsgRatingNoExpiryDate: true, MsgRatingNoExpiryDateManual: true,
	MsgRatingEvaluationFailed: true, MsgRatingExpired: true,
	MsgRatingExpiring: true, MsgRatingValidUntil: true, MsgRatingWindowNotOpen: true,
	MsgRatingRevalidationNotMet: true, MsgRatingRevalidationNotMetProfCheck: true,
	MsgRatingRevalidationExpiringMet: true, MsgRatingRevalidationCurrent: true,
	MsgRatingRecencyNotMet: true, MsgRatingRecencyCurrent: true,
	MsgRatingIRHoursAndCheckNotMet: true, MsgRatingIRHoursNotMet: true,
	MsgRatingIRCheckNotMet: true, MsgRatingIRCurrent: true,
	MsgRatingIRLapsedSafetyPilot: true, MsgRatingIRExpiredIPC: true,
	MsgRatingIRNotApplicable: true, MsgRatingPaxNotCurrent: true,
	MsgRatingPaxDayCurrentNightNo: true, MsgRatingPaxCurrentDayNight: true,
	MsgRatingGliderNotCurrent: true, MsgRatingGliderCurrent: true,
	MsgPaxEvaluationFailed: true, MsgPaxNotCurrent: true,
	MsgPaxCurrentDayNoNight: true, MsgPaxCurrentDayNightIRWaived: true,
	MsgPaxCurrentDayNight: true, MsgPaxDayCurrentNightNot: true,
	MsgPaxCurrentPrivilegeSeparat:   true,
	MsgFlightReviewEvaluationFailed: true, MsgFlightReviewNoneOnRecord: true,
	MsgFlightReviewExpired: true, MsgFlightReviewExpiring: true,
	MsgFlightReviewCurrent: true,
	MsgRequirementProgress: true, MsgRequirementProfCheckCompleted: true,
	MsgRequirementProfCheckMissing: true,
	MsgLaunchMethodProgress:        true,
}

var knownNameKeys = map[string]bool{
	ReqKeyTotalTime: true, ReqKeyPICTime: true, ReqKeyIFRTime: true,
	ReqKeyLandings: true, ReqKeyDayLandings: true, ReqKeyNightLandings: true,
	ReqKeyRefresherTraining: true, ReqKeyTrainingFlight: true,
	ReqKeyProficiencyCheck: true, ReqKeyApproaches: true, ReqKeyHolds: true,
	ReqKeyRouteSectors: true, ReqKeyLaunches: true, ReqKeyLaunchesAndLanding: true,
}

// evaluatorCase is one (authority, licenseType) pair to sweep.
type evaluatorCase struct {
	authority   string
	licenseType string
	eval        Evaluator
}

func allEvaluatorCases() []evaluatorCase {
	easa := NewEASAEvaluator()
	faa := NewFAAEvaluator()
	ul := NewGermanULEvaluator()
	other := NewOtherEvaluator()
	return []evaluatorCase{
		{"EASA", "PPL", easa}, {"EASA", "CPL", easa}, {"EASA", "ATPL", easa},
		{"EASA", "LAPL", easa}, {"EASA", "LAPL(S)", easa}, {"EASA", "SPL", easa},
		{"FAA", "PRIVATE", faa}, {"FAA", "COMMERCIAL", faa}, {"FAA", "ATP", faa},
		{"FAA", "SPORT", faa}, {"FAA", "RECREATIONAL", faa}, {"FAA", "GLIDER", faa},
		{"DULV", "UL", ul}, {"LBA", "UL", ul},
		{"CAA-UK", "PPL", other},
	}
}

var allClassTypes = []models.ClassType{
	models.ClassTypeSEPLand, models.ClassTypeSEPSea, models.ClassTypeMEPLand,
	models.ClassTypeMEPSea, models.ClassTypeSETLand, models.ClassTypeSETSea,
	models.ClassTypeTMG, models.ClassTypeIR,
}

// checkKeys asserts one result carries a catalogued key everywhere it carries text.
func checkKeys(t *testing.T, label string, r ClassRatingCurrency) {
	t.Helper()
	if r.MessageKey == "" {
		t.Errorf("%s: MessageKey empty (message %q)", label, r.Message)
	} else if !knownMessageKeys[r.MessageKey] {
		t.Errorf("%s: MessageKey %q not in catalogue", label, r.MessageKey)
	}
	for i, req := range r.Requirements {
		if req.NameKey == "" {
			t.Errorf("%s: requirement[%d] %q has no NameKey", label, i, req.Name)
		} else if !knownNameKeys[req.NameKey] {
			t.Errorf("%s: requirement[%d] NameKey %q not in catalogue", label, i, req.NameKey)
		}
		if req.MessageKey == "" {
			t.Errorf("%s: requirement[%d] %q has no MessageKey", label, i, req.Name)
		} else if !knownMessageKeys[req.MessageKey] {
			t.Errorf("%s: requirement[%d] MessageKey %q not in catalogue", label, i, req.MessageKey)
		}
	}
	for i, lm := range r.LaunchMethodCurrency {
		if lm.MessageKey != MsgLaunchMethodProgress {
			t.Errorf("%s: launchMethod[%d] MessageKey = %q", label, i, lm.MessageKey)
		}
	}
}

// TestEveryRatingResultCarriesAKey sweeps every evaluator across every class
// type, both with and without an expiry date, and fails if any reachable
// branch emits prose without a catalogued key.
func TestEveryRatingResultCarriesAKey(t *testing.T) {
	ctx := context.Background()
	for _, ec := range allEvaluatorCases() {
		for _, ct := range allClassTypes {
			for _, expiry := range []string{"expired", "soon", "far", "none"} {
				license := &models.License{
					ID: uuid.New(), UserID: uuid.New(),
					RegulatoryAuthority: ec.authority, LicenseType: ec.licenseType,
				}
				rating := &models.ClassRating{
					ID: uuid.New(), LicenseID: license.ID, ClassType: ct,
				}
				switch expiry {
				case "expired":
					rating.ExpiryDate = futureDate(-3)
				case "soon":
					rating.ExpiryDate = futureDate(1)
				case "far":
					rating.ExpiryDate = futureDate(24)
				}

				dp := newMockFlightDataProvider()
				label := strings.Join([]string{ec.authority, ec.licenseType, string(ct), expiry}, "/")
				checkKeys(t, label+" [no data]", ec.eval.Evaluate(ctx, rating, license, dp))

				// Again with enough experience to reach the "requirements met" branches.
				dp2 := newMockFlightDataProvider()
				dp2.progressByClass[ct] = &Progress{
					TotalMinutes: 6000, PICMinutes: 6000, IFRMinutes: 6000,
					InstructorMinutes: 600, Landings: 40, NightLandings: 20,
					DayLandings: 20, Flights: 40, Approaches: 20, Holds: 10,
				}
				dp2.progressAll = dp2.progressByClass[ct]
				dp2.launchCounts = map[string]int{"winch": 9, "aerotow": 9, "self-launch": 9}
				now := futureDate(0)
				dp2.lastProficiencyCheck = now
				dp2.lastFlightReview = now
				checkKeys(t, label+" [full data]", ec.eval.Evaluate(ctx, rating, license, dp2))
			}
		}
	}
}

// TestEveryPassengerResultCarriesAKey does the same for Tier 2.
func TestEveryPassengerResultCarriesAKey(t *testing.T) {
	ctx := context.Background()
	for _, ec := range allEvaluatorCases() {
		paxEval, ok := ec.eval.(PassengerCurrencyEvaluator)
		if !ok {
			continue
		}
		for _, ct := range allClassTypes {
			for _, landings := range []int{0, 2, 5} {
				for _, withIR := range []bool{false, true} {
					license := &models.License{
						ID: uuid.New(), UserID: uuid.New(),
						RegulatoryAuthority: ec.authority, LicenseType: ec.licenseType,
					}
					var peers []*models.ClassRating
					if withIR {
						peers = []*models.ClassRating{{
							ID: uuid.New(), LicenseID: license.ID,
							ClassType: models.ClassTypeIR, ExpiryDate: futureDate(12),
						}}
					}
					dp := newMockFlightDataProvider()
					if landings > 0 {
						dp.landingDays = map[models.ClassType][]LandingDay{
							ct: {{Date: day(5), DayLandings: landings, NightLandings: landings - 1}},
						}
					}
					res := paxEval.EvaluatePassengerCurrency(ctx, ct, license, peers, dp)
					label := strings.Join([]string{ec.authority, ec.licenseType, string(ct)}, "/")
					if res.MessageKey == "" {
						t.Errorf("%s (landings=%d, ir=%v): MessageKey empty (message %q)", label, landings, withIR, res.Message)
					} else if !knownMessageKeys[res.MessageKey] {
						t.Errorf("%s: MessageKey %q not in catalogue", label, res.MessageKey)
					}
				}
			}
		}
	}
}

// TestFlightReviewCarriesAKey covers the per-pilot FAA §61.56 branches.
func TestFlightReviewCarriesAKey(t *testing.T) {
	ctx := context.Background()
	eval := NewFAAEvaluator()
	for _, name := range []string{"none", "recent", "old", "error"} {
		dp := newMockFlightDataProvider()
		switch name {
		case "recent":
			dp.lastFlightReview = futureDate(-1)
		case "old":
			dp.lastFlightReview = futureDate(-40)
		}
		res := eval.EvaluateFlightReview(ctx, uuid.New(), dp)
		if res.MessageKey == "" {
			t.Errorf("flight review %s: MessageKey empty (message %q)", name, res.Message)
		} else if !knownMessageKeys[res.MessageKey] {
			t.Errorf("flight review %s: MessageKey %q not in catalogue", name, res.MessageKey)
		}
	}
}

// TestMessageParamsMatchKeys pins the params each key promises to carry, so a
// client can rely on them being present.
func TestMessageParamsMatchKeys(t *testing.T) {
	needsDays := map[string]bool{
		MsgRatingExpiring: true, MsgRatingRevalidationExpiringMet: true,
		MsgFlightReviewExpiring: true,
	}
	needsNeeded := map[string]bool{
		MsgRatingPaxNotCurrent: true, MsgRatingPaxDayCurrentNightNo: true,
		MsgRatingGliderNotCurrent: true, MsgPaxNotCurrent: true,
		MsgPaxDayCurrentNightNot: true,
	}
	needsDate := map[string]bool{
		MsgRatingWindowNotOpen: true, MsgFlightReviewExpired: true,
		MsgFlightReviewCurrent: true, MsgFlightReviewExpiring: true,
		MsgRequirementProfCheckCompleted: true,
	}

	ctx := context.Background()
	seen := map[string]*MessageParams{}

	record := func(key string, p *MessageParams) {
		if _, ok := seen[key]; !ok {
			seen[key] = p
		}
	}
	for _, ec := range allEvaluatorCases() {
		for _, ct := range allClassTypes {
			for _, exp := range []int{-3, 1, 24} {
				license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: ec.authority, LicenseType: ec.licenseType}
				rating := &models.ClassRating{ID: uuid.New(), LicenseID: license.ID, ClassType: ct, ExpiryDate: futureDate(exp)}
				dp := newMockFlightDataProvider()
				dp.progressByClass[ct] = &Progress{TotalMinutes: 6000, PICMinutes: 6000, IFRMinutes: 6000, InstructorMinutes: 600, Landings: 40, NightLandings: 20, Flights: 40, Approaches: 20, Holds: 10}
				dp.progressAll = dp.progressByClass[ct]
				dp.lastProficiencyCheck = futureDate(0)
				r := ec.eval.Evaluate(ctx, rating, license, dp)
				record(r.MessageKey, r.MessageParams)
				for _, req := range r.Requirements {
					record(req.MessageKey, req.MessageParams)
				}
				if pe, ok := ec.eval.(PassengerCurrencyEvaluator); ok {
					dp.landingDays = map[models.ClassType][]LandingDay{ct: {{Date: day(5), DayLandings: 1}}}
					p := pe.EvaluatePassengerCurrency(ctx, ct, license, nil, dp)
					record(p.MessageKey, p.MessageParams)
				}
			}
		}
	}

	for key, params := range seen {
		if needsDays[key] && (params == nil || params.Days == nil) {
			t.Errorf("key %q must carry params.days", key)
		}
		if needsNeeded[key] && (params == nil || params.Needed == nil) {
			t.Errorf("key %q must carry params.needed", key)
		}
		if needsDate[key] && (params == nil || params.Date == nil) {
			t.Errorf("key %q must carry params.date", key)
		}
	}
}
