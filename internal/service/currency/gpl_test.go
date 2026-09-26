package currency

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func gplSetup() (*models.ClassRating, *models.License) {
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeGyro, LicenseID: uuid.New()}
	license := &models.License{
		ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "GPL",
		IssueDate: time.Now().AddDate(-1, 0, 0),
	}
	return rating, license
}

// ── FCL.240.G ───────────────────────────────────────────────────────────

func TestGPL_Current(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeGyro] = &Progress{TotalMinutes: 720, Landings: 12, InstructorMinutes: 60, LongestTrainingFlightMinutes: 60}
	rating, license := gplSetup()

	res := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if res.RuleDescriptionKey != "easa_gpl" || res.Status != StatusCurrent {
		t.Errorf("rule %s status %s, want easa_gpl current", res.RuleDescriptionKey, res.Status)
	}
	for _, key := range []string{ReqKeyTotalTime, ReqKeyLandings, ReqKeyRefresherTraining, ReqKeyProficiencyCheck} {
		if findReq(res.Requirements, key) == nil {
			t.Errorf("requirement %q missing", key)
		}
	}
}

func TestGPL_AnyLicenceTypeUsesGPLRule(t *testing.T) {
	rating, license := gplSetup()
	license.LicenseType = "PPL"
	res := NewEASAEvaluator().Evaluate(context.Background(), rating, license, newMockFlightDataProvider())
	if res.RuleDescriptionKey != "easa_gpl" {
		t.Errorf("rule = %s, want easa_gpl", res.RuleDescriptionKey)
	}
}

func TestGPL_AnnexIGyroplaneCredit(t *testing.T) {
	tests := []struct {
		name string
		mtom int
		want int
	}{
		{"472 kg credited", 472, 720},
		{"450 kg credited", 450, 720},
		{"300 kg not credited", 300, 120},
		{"unknown mass not credited", 0, 120},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := newMockFlightDataProvider()
			dp.progressByClass[models.ClassTypeGyro] = &Progress{TotalMinutes: 120, Landings: 2, InstructorMinutes: 60, LongestTrainingFlightMinutes: 60}
			dp.progressByUL = map[models.ULKind]*Progress{
				models.ULKindGyroplane: {TotalMinutes: 600, Landings: 10, InstructorMinutes: 120, LongestTrainingFlightMinutes: 60},
			}
			dp.ulMTOM = map[models.ULKind]int{models.ULKindGyroplane: tt.mtom}
			rating, license := gplSetup()

			res := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
			if res.Progress.TotalMinutes != tt.want {
				t.Errorf("TotalMinutes = %d, want %d", res.Progress.TotalMinutes, tt.want)
			}
			if res.Progress.InstructorMinutes != 60 {
				t.Errorf("InstructorMinutes = %d, want 60 (UL dual is not the refresher)", res.Progress.InstructorMinutes)
			}
			if dp.lastULSel == nil || dp.lastULSel.MinMTOMKg != 450 || !slices.Equal(dp.lastULSel.Kinds, []models.ULKind{models.ULKindGyroplane}) {
				t.Errorf("UL selector = %+v, want GYROPLANE of at least 450 kg", dp.lastULSel)
			}
		})
	}
}

func TestGPL_ProficiencyCheck(t *testing.T) {
	check := time.Now().AddDate(0, -6, 0)
	dp := newMockFlightDataProvider()
	dp.lastProficiencyCheck = &check
	rating, license := gplSetup()

	res := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if res.Status != StatusCurrent {
		t.Errorf("Status = %s, want current (GPL proficiency check)", res.Status)
	}
}

// ── FCL.205.G(a)(2) and FCL.060(b) ─────────────────────────────────────

func TestGPL_PassengersNeed10hPICAfterIssue(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeGyro] = &Progress{PICMinutes: 480, Landings: 5}
	_, license := gplSetup()

	res := NewEASAEvaluator().EvaluatePassengerCurrency(context.Background(), models.ClassTypeGyro, license, nil, dp)
	if res.DayStatus != StatusExpired || res.MessageKey != MsgPaxGPLExperienceNotMet {
		t.Errorf("DayStatus %s key %s, want expired / %s", res.DayStatus, res.MessageKey, MsgPaxGPLExperienceNotMet)
	}
	if res.MessageParams == nil || res.MessageParams.Needed == nil || *res.MessageParams.Needed != 120 {
		t.Errorf("params = %+v, want 120 minutes needed", res.MessageParams)
	}
	if res.NightPrivilege {
		t.Error("GPL has no night privilege")
	}

	dp.progressByClass[models.ClassTypeGyro].PICMinutes = 600
	res = NewEASAEvaluator().EvaluatePassengerCurrency(context.Background(), models.ClassTypeGyro, license, nil, dp)
	if res.DayStatus != StatusCurrent || res.MessageKey != MsgPaxCurrentDayNoNight {
		t.Errorf("DayStatus %s key %s, want current / no night", res.DayStatus, res.MessageKey)
	}
}

func TestHasNightPrivilege_GermanAndGPL(t *testing.T) {
	tests := []struct {
		licenseType, authority string
		want                   bool
	}{
		{"PPL", "LBA", true},
		{"UL", "LBA", false},
		{"Luftsportgeräteführer", "DULV", false},
		{"UL", "DAeC", false},
		{"GPL", "EASA", false},
		{"PPL", "EASA", true},
	}
	for _, tt := range tests {
		if got := HasNightPrivilege(tt.licenseType, tt.authority); got != tt.want {
			t.Errorf("HasNightPrivilege(%q, %q) = %v, want %v", tt.licenseType, tt.authority, got, tt.want)
		}
	}
}

// ── German UL gyroplane counts Part-FCL gyroplanes ─────────────────────

func TestGermanULGyroplane_CountsGyroplaneClassTime(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeGyro] = &Progress{TotalMinutes: 600, PICMinutes: 600, Landings: 10, InstructorMinutes: 120, LongestTrainingFlightMinutes: 60}
	dp.progressByUL = map[models.ULKind]*Progress{
		models.ULKindGyroplane: {TotalMinutes: 120, PICMinutes: 60, Landings: 2, InstructorMinutes: 60, LongestTrainingFlightMinutes: 60},
	}
	rating := ulRating(ulKindPtr(models.ULKindGyroplane))

	res := NewGermanULEvaluator().Evaluate(context.Background(), rating, ulLicense(rating, "DULV"), dp)
	if res.Progress.TotalMinutes != 720 || res.Progress.Landings != 12 {
		t.Errorf("Progress = %+v, want gyroplane class and UL gyroplane pooled", res.Progress)
	}
	if res.Progress.InstructorMinutes != 60 {
		t.Errorf("InstructorMinutes = %d, want 60 (training flight on a UL gyroplane)", res.Progress.InstructorMinutes)
	}
	if res.Status != StatusCurrent {
		t.Errorf("Status = %s, want current", res.Status)
	}
}
