package service

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func ulAircraft(userID uuid.UUID, reg string, kind models.ULKind, mtom *int) *models.Aircraft {
	class := string(models.ClassTypeUL)
	return &models.Aircraft{
		UserID: userID, Registration: reg, Type: "UL", Make: "M", Model: "M",
		AircraftClass: &class, ULKind: &kind, MTOMKg: mtom,
	}
}

func sepAircraft(userID uuid.UUID, reg string) *models.Aircraft {
	class := string(models.ClassTypeSEPLand)
	return &models.Aircraft{UserID: userID, Registration: reg, Type: "C172", Make: "Cessna", Model: "172", AircraftClass: &class}
}

func ulFlight(userID uuid.UUID, reg string, night, nightLandings int) *models.Flight {
	return &models.Flight{
		UserID: userID, Date: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		AircraftReg: reg, AircraftType: "C42", TotalTime: 60, IsPIC: true, PICTime: 60,
		NightTime: night, LandingsNight: nightLandings, LandingsDay: 1 - min(nightLandings, 1),
		AllLandings: 1,
	}
}

func warningCodes(ws []models.Warning) []models.WarningCode {
	out := make([]models.WarningCode, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.Code)
	}
	return out
}

func sameCodes(a, b []models.WarningCode) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCheckFlights_ULNight(t *testing.T) {
	userID := uuid.New()
	repo := &sessionAircraftRepo{aircraft: []*models.Aircraft{
		ulAircraft(userID, "D-MXYZ", models.ULKindThreeAxis, nil),
		ulAircraft(userID, "D-MTRK", models.ULKindWeightShift, nil),
		sepAircraft(userID, "D-EFGH"),
	}}
	tests := []struct {
		name   string
		flight *models.Flight
		want   []models.WarningCode
	}{
		{"M4 night time on Mehmet's C42 warns", ulFlight(userID, "D-MXYZ", 25, 0), []models.WarningCode{models.WarningULNightFlight}},
		{"M4 night landing only on a UL warns", ulFlight(userID, "D-MXYZ", 0, 1), []models.WarningCode{models.WarningULNightFlight}},
		{"S1 weight-shift trike at night warns", ulFlight(userID, "d-mtrk", 10, 1), []models.WarningCode{models.WarningULNightFlight}},
		{"day flight on a UL has no warning", ulFlight(userID, "D-MXYZ", 0, 0), []models.WarningCode{}},
		{"A2 night on a SEP has no warning", ulFlight(userID, "D-EFGH", 60, 1), []models.WarningCode{}},
		{"aircraft outside the fleet has no warning", ulFlight(userID, "D-MNEW", 60, 1), []models.WarningCode{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewFlightService(newMockFlightRepo(), nil)
			svc.SetAircraftRepository(repo)
			got := svc.CheckFlights(context.Background(), userID, []*models.Flight{tt.flight})
			if len(got) != 1 {
				t.Fatalf("got %d warning lists, want 1", len(got))
			}
			if codes := warningCodes(got[0]); !sameCodes(codes, tt.want) {
				t.Errorf("codes = %v, want %v", codes, tt.want)
			}
		})
	}
}

func TestCheckFlights_ULNightParams(t *testing.T) {
	userID := uuid.New()
	svc := NewFlightService(newMockFlightRepo(), nil)
	svc.SetAircraftRepository(&sessionAircraftRepo{aircraft: []*models.Aircraft{ulAircraft(userID, "D-MXYZ", models.ULKindThreeAxis, nil)}})
	ws := svc.CheckFlights(context.Background(), userID, []*models.Flight{ulFlight(userID, "D-MXYZ", 25, 1)})[0]
	if len(ws) != 1 {
		t.Fatalf("warnings = %v", ws)
	}
	w := ws[0]
	if w.Severity != models.WarningSeverityWarning || w.Params["nightTime"] != 25 || w.Params["landingsNight"] != 1 ||
		w.Params["registration"] != "D-MXYZ" || w.Params["ulKind"] != "THREE_AXIS" {
		t.Errorf("warning = %+v", w)
	}
}

func TestCheckFlights_BatchLegs(t *testing.T) {
	userID := uuid.New()
	svc := NewFlightService(newMockFlightRepo(), nil)
	svc.SetAircraftRepository(&sessionAircraftRepo{aircraft: []*models.Aircraft{ulAircraft(userID, "D-MXYZ", models.ULKindThreeAxis, nil)}})
	legs := []*models.Flight{
		ulFlight(userID, "D-MXYZ", 0, 0),
		ulFlight(userID, "D-MXYZ", 15, 1),
		ulFlight(userID, "D-MXYZ", 0, 0),
		ulFlight(userID, "D-MXYZ", 30, 1),
	}
	if err := svc.CreateFlightBatch(context.Background(), legs); err != nil {
		t.Fatalf("CreateFlightBatch: %v", err)
	}
	got := svc.CheckFlights(context.Background(), userID, legs)
	want := []int{0, 1, 0, 1}
	for i := range want {
		if len(got[i]) != want[i] {
			t.Errorf("leg %d: %d warnings, want %d", i, len(got[i]), want[i])
		}
	}
}

func TestCheckFlights_NoAircraftRepository(t *testing.T) {
	userID := uuid.New()
	svc := NewFlightService(newMockFlightRepo(), nil)
	got := svc.CheckFlights(context.Background(), userID, []*models.Flight{ulFlight(userID, "D-MXYZ", 60, 1)})
	if len(got[0]) != 0 {
		t.Errorf("warnings = %v, want none", got[0])
	}
}

func TestAircraftWarnings_MTOM(t *testing.T) {
	userID := uuid.New()
	kg := func(v int) *int { return &v }
	tests := []struct {
		name string
		ac   *models.Aircraft
		want []models.WarningCode
	}{
		{"UL above 600 kg warns", ulAircraft(userID, "D-MXYZ", models.ULKindThreeAxis, kg(650)), []models.WarningCode{models.WarningULMTOMExceeds600}},
		{"UL at 600 kg is silent", ulAircraft(userID, "D-MXYZ", models.ULKindThreeAxis, kg(600)), []models.WarningCode{}},
		{"UL at 472 kg is silent", ulAircraft(userID, "D-MTRK", models.ULKindWeightShift, kg(472)), []models.WarningCode{}},
		{"S2 paraglider at 120 kg is the 120 kg class", ulAircraft(userID, "PPG-VIPER", models.ULKindPoweredParaglider, kg(120)), []models.WarningCode{models.WarningUL120kgClass}},
		{"UL at 121 kg is silent", ulAircraft(userID, "D-MTRK", models.ULKindWeightShift, kg(121)), []models.WarningCode{}},
		{"UL without MTOM is silent", ulAircraft(userID, "D-MXYZ", models.ULKindThreeAxis, nil), []models.WarningCode{}},
		{"A2 SEP above 600 kg is silent", func() *models.Aircraft { a := sepAircraft(userID, "D-EFGH"); a.MTOMKg = kg(1111); return a }(), []models.WarningCode{}},
		{"A2 SEP at 100 kg is silent", func() *models.Aircraft { a := sepAircraft(userID, "D-EFGH"); a.MTOMKg = kg(100); return a }(), []models.WarningCode{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AircraftWarnings(tt.ac)
			if codes := warningCodes(got); !sameCodes(codes, tt.want) {
				t.Errorf("codes = %v, want %v", codes, tt.want)
			}
			for _, w := range got {
				if w.Params["mtomKg"] != *tt.ac.MTOMKg {
					t.Errorf("mtomKg param = %v", w.Params["mtomKg"])
				}
			}
		})
	}
	if w := AircraftWarnings(ulAircraft(userID, "PPG", models.ULKindPoweredParaglider, kg(100))); w[0].Severity != models.WarningSeverityInfo {
		t.Errorf("ul_120kg_class severity = %s, want info", w[0].Severity)
	}
}

func TestFlightRegistration_PoweredParagliderName(t *testing.T) {
	userID := uuid.New()
	fleet := &sessionAircraftRepo{aircraft: []*models.Aircraft{
		ulAircraft(userID, "APCO", models.ULKindPoweredParaglider, nil),
		ulAircraft(userID, "PPG-VIPER", models.ULKindPoweredParaglider, nil),
		ulAircraft(userID, "DMTRK", models.ULKindWeightShift, nil),
	}}
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"S2 paraglider name matching a nationality shape is kept", "Apco", "APCO"},
		{"S2 paraglider name is kept", " PPG-Viper ", "PPG-VIPER"},
		{"name not in the fleet is canonicalised", "Ozon", "OZON"},
		{"short name not in the fleet is canonicalised", "PPG1", "PP-G1"},
		{"a trike's registration is canonicalised", "dmtrk", "D-MTRK"},
		{"A2 German registration is canonicalised", "deabc", "D-EABC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockFlightRepo()
			svc := NewFlightService(repo, nil)
			svc.SetAircraftRepository(fleet)
			f := ulFlight(userID, tt.raw, 0, 0)
			if err := svc.CreateFlight(context.Background(), f); err != nil {
				t.Fatalf("CreateFlight: %v", err)
			}
			if f.AircraftReg != tt.want {
				t.Errorf("AircraftReg = %q, want %q", f.AircraftReg, tt.want)
			}
		})
	}
}
