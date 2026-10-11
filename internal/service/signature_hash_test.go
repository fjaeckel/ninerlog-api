package service

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func hashFixture() (*models.Flight, []models.FlightCrewMember) {
	dep, arr := "EDDF", "EDDM"
	off, on := "08:00:00", "09:30:00"
	remarks := "smooth"
	f := &models.Flight{
		Date: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), AircraftReg: "D-EABC", AircraftType: "C172",
		DepartureICAO: &dep, ArrivalICAO: &arr, OffBlockTime: &off, OnBlockTime: &on,
		TotalTime: 90, PICTime: 90, AllLandings: 1, LandingsDay: 1, TakeoffsDay: 1, Remarks: &remarks,
	}
	crew := []models.FlightCrewMember{
		{Name: "Pat", Role: models.CrewRole("Instructor")},
		{Name: "Alex", Role: models.CrewRole("Student")},
	}
	return f, crew
}

func TestSignedFieldsHash_Stable(t *testing.T) {
	f, crew := hashFixture()
	first := SignedFieldsHash(f, crew)
	if len(first) != 64 {
		t.Fatalf("hash length = %d, want 64 hex characters", len(first))
	}
	if again := SignedFieldsHash(f, crew); again != first {
		t.Error("hash differs between calls")
	}

	reversed := []models.FlightCrewMember{crew[1], crew[0]}
	if got := SignedFieldsHash(f, reversed); got != first {
		t.Error("hash depends on crew order")
	}

	noise := *f
	noise.ID = uuid.New()
	noise.UpdatedAt = time.Now()
	noise.Distance = 99
	noise.SignatureID = &noise.ID
	if got := SignedFieldsHash(&noise, crew); got != first {
		t.Error("hash depends on fields that are not signed")
	}
}

func TestSignedFieldsHash_ChangesWithEverySignedField(t *testing.T) {
	str := func(s string) *string { return &s }
	mutations := map[string]func(*models.Flight){
		"date":                    func(f *models.Flight) { f.Date = f.Date.AddDate(0, 0, 1) },
		"aircraftReg":             func(f *models.Flight) { f.AircraftReg = "D-EXYZ" },
		"aircraftType":            func(f *models.Flight) { f.AircraftType = "PA28" },
		"departureIcao":           func(f *models.Flight) { f.DepartureICAO = str("EDDK") },
		"arrivalIcao":             func(f *models.Flight) { f.ArrivalICAO = nil },
		"offBlockTime":            func(f *models.Flight) { f.OffBlockTime = str("08:05:00") },
		"onBlockTime":             func(f *models.Flight) { f.OnBlockTime = str("09:35:00") },
		"departureTime":           func(f *models.Flight) { f.DepartureTime = str("08:10:00") },
		"arrivalTime":             func(f *models.Flight) { f.ArrivalTime = str("09:20:00") },
		"totalTime":               func(f *models.Flight) { f.TotalTime++ },
		"picTime":                 func(f *models.Flight) { f.PICTime++ },
		"dualTime":                func(f *models.Flight) { f.DualTime++ },
		"nightTime":               func(f *models.Flight) { f.NightTime++ },
		"ifrTime":                 func(f *models.Flight) { f.IFRTime++ },
		"soloTime":                func(f *models.Flight) { f.SoloTime++ },
		"crossCountryTime":        func(f *models.Flight) { f.CrossCountryTime++ },
		"sicTime":                 func(f *models.Flight) { f.SICTime++ },
		"dualGivenTime":           func(f *models.Flight) { f.DualGivenTime++ },
		"picusTime":               func(f *models.Flight) { f.PICUSTime++ },
		"spicTime":                func(f *models.Flight) { f.SPICTime++ },
		"examinerTime":            func(f *models.Flight) { f.ExaminerTime++ },
		"reliefTime":              func(f *models.Flight) { f.ReliefTime++ },
		"multiPilotTime":          func(f *models.Flight) { f.MultiPilotTime++ },
		"simulatedFlightTime":     func(f *models.Flight) { f.SimulatedFlightTime++ },
		"groundTrainingTime":      func(f *models.Flight) { f.GroundTrainingTime++ },
		"actualInstrumentTime":    func(f *models.Flight) { f.ActualInstrumentTime++ },
		"simulatedInstrumentTime": func(f *models.Flight) { f.SimulatedInstrumentTime++ },
		"landingsDay":             func(f *models.Flight) { f.LandingsDay++ },
		"landingsNight":           func(f *models.Flight) { f.LandingsNight++ },
		"allLandings":             func(f *models.Flight) { f.AllLandings++ },
		"takeoffsDay":             func(f *models.Flight) { f.TakeoffsDay++ },
		"takeoffsNight":           func(f *models.Flight) { f.TakeoffsNight++ },
		"remarks":                 func(f *models.Flight) { f.Remarks = str("bumpy") },
		"instructorName":          func(f *models.Flight) { f.InstructorName = str("Pat") },
		"instructorComments":      func(f *models.Flight) { f.InstructorComments = str("good") },
		"picName":                 func(f *models.Flight) { f.PICName = str("Pat") },
	}

	base, crew := hashFixture()
	want := SignedFieldsHash(base, crew)
	seen := map[string]string{want: "base"}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			f, c := hashFixture()
			mutate(f)
			got := SignedFieldsHash(f, c)
			if got == want {
				t.Fatalf("hash unchanged after changing %s", name)
			}
			if other, dup := seen[got]; dup {
				t.Fatalf("hash after %s equals hash after %s", name, other)
			}
			seen[got] = name
		})
	}

	crewChanges := map[string][]models.FlightCrewMember{
		"crew name":    {{Name: "Pete", Role: models.CrewRole("Instructor")}, crew[1]},
		"crew role":    {{Name: "Pat", Role: models.CrewRole("Safety Pilot")}, crew[1]},
		"crew removed": {crew[0]},
		"crew added":   append(append([]models.FlightCrewMember{}, crew...), models.FlightCrewMember{Name: "Z", Role: models.CrewRole("Student")}),
	}
	for name, changed := range crewChanges {
		t.Run(name, func(t *testing.T) {
			if SignedFieldsHash(base, changed) == want {
				t.Fatalf("hash unchanged after %s", name)
			}
		})
	}
}

func TestSignatureService_StoresAndVerifiesFieldsHash(t *testing.T) {
	ctx := context.Background()
	svc, sigRepo, flightRepo, _ := newSignatureTestService()
	crewRepo := &fakeCrewRepo{crew: map[uuid.UUID][]models.FlightCrewMember{}}
	svc.SetCrewRepository(crewRepo)

	userID := uuid.New()
	flight := seedFlight(t, flightRepo, userID)
	crewRepo.crew[flight.ID] = []models.FlightCrewMember{{Name: "Pat", Role: models.CrewRole("Instructor")}}

	sig, err := svc.SignLive(ctx, flight.ID, userID, "Pat", nil, []byte(testSigImage), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if sig.SignedFieldsHash == nil || *sig.SignedFieldsHash != SignedFieldsHash(flight, crewRepo.crew[flight.ID]) {
		t.Fatalf("stored hash = %v", sig.SignedFieldsHash)
	}

	stored, _ := sigRepo.GetByID(ctx, sig.ID)
	valid, err := svc.VerifyFieldsHashes(ctx, flight.ID, userID, []*models.FlightSignature{stored})
	if err != nil || !valid[sig.ID] {
		t.Fatalf("valid = %v, err = %v", valid, err)
	}

	crewRepo.crew[flight.ID] = nil
	valid, _ = svc.VerifyFieldsHashes(ctx, flight.ID, userID, []*models.FlightSignature{stored})
	if valid[sig.ID] {
		t.Error("hash still valid after the crew changed")
	}

	noHash := &models.FlightSignature{ID: uuid.New()}
	valid, _ = svc.VerifyFieldsHashes(ctx, flight.ID, userID, []*models.FlightSignature{noHash})
	if _, present := valid[noHash.ID]; present {
		t.Error("a signature without a hash was reported")
	}
}

func TestSignatureService_DeferredSignatureStoresFieldsHash(t *testing.T) {
	ctx := context.Background()
	svc, sigRepo, flightRepo, userRepo := newSignatureTestService()
	userID := uuid.New()
	userRepo.users[userID] = &models.User{ID: userID, Email: "o@example.com", Name: "Owner"}
	flight := seedFlight(t, flightRepo, userID)

	_, token, err := svc.CreateRequest(ctx, flight.ID, userID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sig, _, _, err := svc.CompleteFromToken(ctx, token, "Pat", nil, []byte(testSigImage), "", "")
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := sigRepo.GetByID(ctx, sig.ID)
	if stored.SignedFieldsHash == nil || *stored.SignedFieldsHash != SignedFieldsHash(flight, nil) {
		t.Fatalf("stored hash = %v", stored.SignedFieldsHash)
	}
}
