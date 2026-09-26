package pilotprofile

import (
	"reflect"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

var testNow = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

func day(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

func lic(licType, authority, number string) *models.License {
	return &models.License{ID: uuid.New(), LicenseType: licType, RegulatoryAuthority: authority, LicenseNumber: number}
}

func rating(l *models.License, ct models.ClassType) *models.ClassRating {
	return &models.ClassRating{ID: uuid.New(), LicenseID: l.ID, ClassType: ct}
}

func ulRating(l *models.License, k models.ULKind) *models.ClassRating {
	r := rating(l, models.ClassTypeUL)
	r.ULKind = &k
	return r
}

func plane(reg, class string) *models.Aircraft {
	c := class
	return &models.Aircraft{ID: uuid.New(), Registration: reg, AircraftClass: &c, IsActive: true}
}

func ulPlane(reg string, k models.ULKind) *models.Aircraft {
	a := plane(reg, "ULTRALIGHT")
	a.ULKind = &k
	return a
}

func priv(l *models.License, k models.LicencePrivilegeKind, expires *time.Time) *models.LicencePrivilege {
	return &models.LicencePrivilege{ID: uuid.New(), LicenseID: l.ID, Kind: k, ExpiresOn: expires}
}

func ulKind(k models.ULKind) *models.ULKind { return &k }

type deriveCase struct {
	name       string
	licences   []*models.License
	ratings    []*models.ClassRating
	privileges []*models.LicencePrivilege
	fleet      []*models.Aircraft
	flights    []models.DisciplineFlightGroup
	settings   map[models.Discipline]models.DisciplineSetting
	want       map[models.Discipline]models.DisciplineStatus
	ulKinds    []models.ULKind
}

func TestDerive(t *testing.T) {
	ppl := lic("PPL(A)", "EASA", "DE.FCL.1")
	spl := lic("SPL", "LBA", "12345")
	dulv := lic("Luftsportgeräteführer", "DULV", "UL-77")
	atpl := lic("ATPL", "EASA", "A1")
	other := lic("OTHER", "EASA", "O1")
	instructorNotes := "FI(S)"
	fiRating := rating(other, models.ClassTypeOther)
	fiRating.Notes = &instructorNotes
	multiPilot := plane("D-AIPX", "MEP_LAND")
	multiPilot.IsMultiPilot = true
	inactive := plane("D-EOLD", "SEP_LAND")
	inactive.IsActive = false

	recent := day(2026, 8, 2)
	old := day(2023, 6, 1)

	off := func(ds ...models.Discipline) map[models.Discipline]models.DisciplineSetting {
		m := map[models.Discipline]models.DisciplineSetting{}
		for _, d := range ds {
			m[d] = models.DisciplineSetting{Intent: models.IntentOff}
		}
		return m
	}

	cases := []deriveCase{
		{name: "empty account resolves nothing", want: nil},

		// AEROPLANE
		{name: "AEROPLANE strong from SEP rating", licences: []*models.License{ppl}, ratings: []*models.ClassRating{rating(ppl, models.ClassTypeSEPLand)},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineAeroplane: models.StatusActive}},
		{name: "AEROPLANE strong from FAA Private", licences: []*models.License{lic("Private", "FAA", "")},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineAeroplane: models.StatusActive}},
		{name: "AEROPLANE recent from SEP aircraft", fleet: []*models.Aircraft{plane("D-EFGH", "SEP_LAND")},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineAeroplane: models.StatusActive}},
		{name: "AEROPLANE credit from UL THREE_AXIS flights", flights: []models.DisciplineFlightGroup{
			{AircraftClass: "ULTRALIGHT", ULKind: ulKind(models.ULKindThreeAxis), Flights: 4, LastFlight: recent}},
			want:    map[models.Discipline]models.DisciplineStatus{models.DisciplineAeroplane: models.StatusActive, models.DisciplineUltralight: models.StatusActive},
			ulKinds: []models.ULKind{models.ULKindThreeAxis}},
		{name: "inactive aircraft is no evidence", fleet: []*models.Aircraft{inactive}, want: nil},

		// TMG
		{name: "SPL with TMG extension", licences: []*models.License{spl}, ratings: []*models.ClassRating{rating(spl, models.ClassTypeTMG)},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineTMG: models.StatusActive, models.DisciplineSailplane: models.StatusActive}},
		{name: "TMG recent from UL THREE_AXIS_MOTORGLIDER aircraft", fleet: []*models.Aircraft{ulPlane("D-MTMG", models.ULKindThreeAxisMotorglider)},
			want:    map[models.Discipline]models.DisciplineStatus{models.DisciplineTMG: models.StatusActive, models.DisciplineUltralight: models.StatusActive},
			ulKinds: []models.ULKind{models.ULKindThreeAxisMotorglider}},
		{name: "TMG recent from TMG flights", flights: []models.DisciplineFlightGroup{{AircraftClass: "TMG", Flights: 3, LastFlight: recent}},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineTMG: models.StatusActive}},

		// SAILPLANE
		{name: "SAILPLANE strong from GLIDER rating", licences: []*models.License{spl}, ratings: []*models.ClassRating{rating(spl, models.ClassTypeGlider)},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusActive}},
		{name: "SAILPLANE strong from LAPL(S)", licences: []*models.License{lic("LAPL(S)", "EASA", "")},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusActive}},
		{name: "SAILPLANE recent from GLIDER aircraft", fleet: []*models.Aircraft{plane("D-1234", "GLIDER")},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusActive}},
		{name: "SAILPLANE recent from UL SAILPLANE aircraft", fleet: []*models.Aircraft{ulPlane("D-MSGL", models.ULKindSailplane)},
			want:    map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusActive, models.DisciplineUltralight: models.StatusActive},
			ulKinds: []models.ULKind{models.ULKindSailplane}},
		{name: "towed launch is SAILPLANE evidence, never AEROPLANE", flights: []models.DisciplineFlightGroup{
			{AircraftClass: "SEP_LAND", TowedFlights: 2, LastTowed: recent},
			{AircraftClass: "", TowedFlights: 3, LastTowed: recent}},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusActive}},

		// ULTRALIGHT
		{name: "ULTRALIGHT strong from DULV licence with kinds from rating", licences: []*models.License{dulv},
			ratings: []*models.ClassRating{ulRating(dulv, models.ULKindWeightShift), ulRating(dulv, models.ULKindPoweredParaglider)},
			want:    map[models.Discipline]models.DisciplineStatus{models.DisciplineUltralight: models.StatusActive},
			ulKinds: []models.ULKind{models.ULKindWeightShift, models.ULKindPoweredParaglider}},
		{name: "ULTRALIGHT recent from UL aircraft without kind", fleet: []*models.Aircraft{plane("D-MXXX", "ultralight")},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineUltralight: models.StatusActive}},

		// GYROPLANE
		{name: "GYROPLANE strong from GPL", licences: []*models.License{lic("GPL", "EASA", "")},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineGyroplane: models.StatusActive}},
		{name: "GYROPLANE strong from rating", licences: []*models.License{ppl}, ratings: []*models.ClassRating{rating(ppl, models.ClassTypeGyro)},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineGyroplane: models.StatusActive, models.DisciplineAeroplane: models.StatusActive}},
		{name: "GYROPLANE recent from UL gyroplane flights", flights: []models.DisciplineFlightGroup{
			{AircraftClass: "ULTRALIGHT", ULKind: ulKind(models.ULKindGyroplane), Flights: 1, LastFlight: recent}},
			want:    map[models.Discipline]models.DisciplineStatus{models.DisciplineGyroplane: models.StatusActive, models.DisciplineUltralight: models.StatusActive},
			ulKinds: []models.ULKind{models.ULKindGyroplane}},

		// HELICOPTER
		{name: "HELICOPTER strong from (H) licence", licences: []*models.License{lic("PPL(H)", "EASA", "")},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineHelicopter: models.StatusActive}},
		{name: "HELICOPTER recent from UL helicopter", fleet: []*models.Aircraft{ulPlane("D-MHEL", models.ULKindHelicopter)},
			want:    map[models.Discipline]models.DisciplineStatus{models.DisciplineHelicopter: models.StatusActive, models.DisciplineUltralight: models.StatusActive},
			ulKinds: []models.ULKind{models.ULKindHelicopter}},

		// IFR
		{name: "IFR strong from IR rating", licences: []*models.License{ppl}, ratings: []*models.ClassRating{rating(ppl, models.ClassTypeIR)},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineIFR: models.StatusActive, models.DisciplineAeroplane: models.StatusActive}},
		{name: "IFR recent from IFR time", flights: []models.DisciplineFlightGroup{{IFRFlights: 2, LastIFR: recent}},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineIFR: models.StatusActive}},
		{name: "IFR dormant from old approaches", flights: []models.DisciplineFlightGroup{{IFRFlights: 2, LastIFR: old}},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineIFR: models.StatusDormant}},
		{name: "IFR training only by goal", settings: map[models.Discipline]models.DisciplineSetting{models.DisciplineIFR: {Intent: models.IntentGoal}},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineIFR: models.StatusTraining}},

		// MULTI_CREW
		{name: "MULTI_CREW strong from ATPL", licences: []*models.License{atpl},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineMultiCrew: models.StatusActive, models.DisciplineAeroplane: models.StatusActive}},
		{name: "MULTI_CREW recent from multi-pilot aircraft", fleet: []*models.Aircraft{multiPilot},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineMultiCrew: models.StatusActive, models.DisciplineAeroplane: models.StatusActive}},
		{name: "MULTI_CREW recent from SIC time", flights: []models.DisciplineFlightGroup{{MultiCrewFlights: 5, LastMultiCrew: recent}},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineMultiCrew: models.StatusActive}},

		// INSTRUCTOR
		{name: "INSTRUCTOR strong from FI(S) licence", licences: []*models.License{lic("FI(S)", "EASA", "")},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineInstructor: models.StatusActive}},
		{name: "INSTRUCTOR strong from rating text", licences: []*models.License{other}, ratings: []*models.ClassRating{fiRating},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineInstructor: models.StatusActive}},
		{name: "P4 Petra INSTRUCTOR active from FI(S) privilege without dual given", licences: []*models.License{spl},
			privileges: []*models.LicencePrivilege{priv(spl, models.PrivilegeFIS, nil)},
			want:       map[models.Discipline]models.DisciplineStatus{models.DisciplineInstructor: models.StatusActive, models.DisciplineSailplane: models.StatusActive}},
		{name: "INSTRUCTOR active from BI(S) privilege", licences: []*models.License{spl},
			privileges: []*models.LicencePrivilege{priv(spl, models.PrivilegeBIS, nil)},
			want:       map[models.Discipline]models.DisciplineStatus{models.DisciplineInstructor: models.StatusActive, models.DisciplineSailplane: models.StatusActive}},
		{name: "INSTRUCTOR active from FE(S) privilege", licences: []*models.License{spl},
			privileges: []*models.LicencePrivilege{priv(spl, models.PrivilegeFES, day(2027, 1, 1))},
			want:       map[models.Discipline]models.DisciplineStatus{models.DisciplineInstructor: models.StatusActive, models.DisciplineSailplane: models.StatusActive}},
		{name: "expired FI(S) privilege resolves INSTRUCTOR dormant", licences: []*models.License{spl},
			privileges: []*models.LicencePrivilege{priv(spl, models.PrivilegeFIS, day(2025, 3, 31))},
			want:       map[models.Discipline]models.DisciplineStatus{models.DisciplineInstructor: models.StatusDormant, models.DisciplineSailplane: models.StatusActive}},
		{name: "expired FE(S) beside a valid FI(S) resolves INSTRUCTOR active", licences: []*models.License{spl},
			privileges: []*models.LicencePrivilege{priv(spl, models.PrivilegeFES, day(2025, 3, 31)), priv(spl, models.PrivilegeFIS, nil)},
			want:       map[models.Discipline]models.DisciplineStatus{models.DisciplineInstructor: models.StatusActive, models.DisciplineSailplane: models.StatusActive}},
		{name: "towing and cloud-flying privileges give no evidence", licences: []*models.License{other},
			privileges: []*models.LicencePrivilege{priv(other, models.PrivilegeSailplaneTowing, nil), priv(other, models.PrivilegeCloudFlying, nil)},
			want:       nil},
		{name: "INSTRUCTOR recent from dual given", flights: []models.DisciplineFlightGroup{{InstructingFlights: 3, LastInstructing: recent}},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineInstructor: models.StatusActive}},

		// SIMULATOR
		{name: "SIMULATOR recent from FSTD sessions", flights: []models.DisciplineFlightGroup{{SimulatorSessions: 4, LastSimulator: recent}},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineSimulator: models.StatusActive}},
		{name: "simulator and passenger flights are no aircraft evidence", flights: []models.DisciplineFlightGroup{
			{AircraftClass: "SEP_LAND", SimulatorSessions: 2, LastSimulator: recent, PassengerFlights: 3, LastPassenger: recent}},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineSimulator: models.StatusActive}},

		// Status resolution
		{name: "dual-only flights without rating resolve training", fleet: []*models.Aircraft{plane("D-1234", "GLIDER")},
			flights: []models.DisciplineFlightGroup{{AircraftClass: "GLIDER", TowedFlights: 3, TowedDualReceivedFlights: 3, LastTowed: recent}},
			want:    map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusTraining}},
		{name: "J1 dual and supervised-solo flights without rating resolve training", fleet: []*models.Aircraft{plane("D-1234", "GLIDER"), plane("D-2345", "GLIDER")},
			flights: []models.DisciplineFlightGroup{{AircraftClass: "GLIDER", TowedFlights: 12, TowedDualReceivedFlights: 9, LastTowed: recent}},
			want:    map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusTraining}},
		{name: "goal beats fleet and flight evidence", fleet: []*models.Aircraft{plane("D-1234", "GLIDER")},
			flights:  []models.DisciplineFlightGroup{{AircraftClass: "GLIDER", Flights: 5, LastFlight: recent}},
			settings: map[models.Discipline]models.DisciplineSetting{models.DisciplineSailplane: {Intent: models.IntentGoal}},
			want:     map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusTraining}},
		{name: "solo flights without rating and no instruction resolve active",
			flights: []models.DisciplineFlightGroup{{AircraftClass: "SEP_LAND", Flights: 5, LastFlight: recent}},
			want:    map[models.Discipline]models.DisciplineStatus{models.DisciplineAeroplane: models.StatusActive}},
		{name: "dual-only flights with rating resolve active", licences: []*models.License{spl},
			flights: []models.DisciplineFlightGroup{{AircraftClass: "GLIDER", Flights: 2, DualReceivedFlights: 2, LastFlight: recent}},
			want:    map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusActive}},
		{name: "old dual-only flights resolve dormant", flights: []models.DisciplineFlightGroup{{AircraftClass: "GLIDER", Flights: 2, DualReceivedFlights: 2, LastFlight: old}},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusDormant}},
		{name: "off beats strong evidence", licences: []*models.License{spl}, settings: off(models.DisciplineSailplane),
			want: nil},
		{name: "on activates without evidence", settings: map[models.Discipline]models.DisciplineSetting{models.DisciplineGyroplane: {Intent: models.IntentOn}},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineGyroplane: models.StatusActive}},
		{name: "goal with strong evidence resolves active", licences: []*models.License{spl},
			settings: map[models.Discipline]models.DisciplineSetting{models.DisciplineSailplane: {Intent: models.IntentGoal}},
			want:     map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusActive}},
		{name: "dormant after 24 months", flights: []models.DisciplineFlightGroup{{AircraftClass: "GLIDER", Flights: 40, LastFlight: day(2024, 9, 25)}},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusDormant}},
		{name: "recent on the 24-month boundary", flights: []models.DisciplineFlightGroup{{AircraftClass: "GLIDER", Flights: 40, LastFlight: day(2024, 9, 26)}},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusActive}},
		{name: "licence with only old flights resolves dormant", licences: []*models.License{spl},
			flights: []models.DisciplineFlightGroup{{AircraftClass: "", TowedFlights: 30, LastTowed: old}},
			want:    map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusDormant}},
		{name: "licence with no flights resolves active", licences: []*models.License{spl},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusActive}},
		{name: "licence with old flights and a fleet aircraft resolves active", licences: []*models.License{spl},
			fleet:   []*models.Aircraft{plane("D-1234", "GLIDER")},
			flights: []models.DisciplineFlightGroup{{AircraftClass: "GLIDER", Flights: 5, LastFlight: old}},
			want:    map[models.Discipline]models.DisciplineStatus{models.DisciplineSailplane: models.StatusActive}},
		{name: "IR rating with only old IFR time resolves dormant", licences: []*models.License{lic("OTHER", "EASA", "")},
			ratings: []*models.ClassRating{rating(other, models.ClassTypeIR)},
			flights: []models.DisciplineFlightGroup{{IFRFlights: 3, LastIFR: old}},
			want:    map[models.Discipline]models.DisciplineStatus{models.DisciplineIFR: models.StatusDormant}},
		{name: "IR rating and never an IFR flight resolves active", ratings: []*models.ClassRating{rating(other, models.ClassTypeIR)},
			want: map[models.Discipline]models.DisciplineStatus{models.DisciplineIFR: models.StatusActive}},
		{name: "UL-licensed pilot's C42 flights do not make AEROPLANE active",
			licences: []*models.License{dulv, ppl},
			ratings:  []*models.ClassRating{ulRating(dulv, models.ULKindThreeAxis), rating(ppl, models.ClassTypeSEPLand)},
			fleet:    []*models.Aircraft{ulPlane("D-MXYZ", models.ULKindThreeAxis)},
			flights: []models.DisciplineFlightGroup{
				{AircraftClass: "ULTRALIGHT", ULKind: ulKind(models.ULKindThreeAxis), Flights: 20, LastFlight: recent},
				{AircraftClass: "SEP_LAND", Flights: 10, LastFlight: old}},
			want:    map[models.Discipline]models.DisciplineStatus{models.DisciplineUltralight: models.StatusActive, models.DisciplineAeroplane: models.StatusDormant},
			ulKinds: []models.ULKind{models.ULKindThreeAxis}},
		{name: "PPL-only pilot flying a C42 resolves AEROPLANE active",
			licences: []*models.License{ppl}, ratings: []*models.ClassRating{rating(ppl, models.ClassTypeSEPLand)},
			flights: []models.DisciplineFlightGroup{
				{AircraftClass: "ULTRALIGHT", ULKind: ulKind(models.ULKindThreeAxis), Flights: 4, LastFlight: recent},
				{AircraftClass: "SEP_LAND", Flights: 10, LastFlight: old}},
			want:    map[models.Discipline]models.DisciplineStatus{models.DisciplineUltralight: models.StatusActive, models.DisciplineAeroplane: models.StatusActive},
			ulKinds: []models.ULKind{models.ULKindThreeAxis}},
		{name: "unknown licence text gives no evidence", licences: []*models.License{lic("Segelflugschein alt", "LBA", "9")}, want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := models.DefaultPilotProfile(uuid.New())
			if tc.settings != nil {
				settings.Disciplines = tc.settings
			}
			states := Derive(tc.licences, tc.ratings, tc.privileges, tc.fleet, tc.flights, settings, testNow)
			if len(states) != len(models.AllDisciplines()) {
				t.Fatalf("got %d states, want %d", len(states), len(models.AllDisciplines()))
			}
			for i, s := range states {
				if s.Discipline != models.AllDisciplines()[i] {
					t.Errorf("state %d is %s, want %s", i, s.Discipline, models.AllDisciplines()[i])
				}
				want := models.StatusOff
				if w, ok := tc.want[s.Discipline]; ok {
					want = w
				}
				if s.Status != want {
					t.Errorf("%s status = %s, want %s (evidence %+v)", s.Discipline, s.Status, want, s.Evidence)
				}
				if s.Evidence == nil || s.ULKinds == nil {
					t.Errorf("%s evidence and ulKinds must be non-nil", s.Discipline)
				}
				if s.Discipline == models.DisciplineUltralight {
					wantKinds := tc.ulKinds
					if wantKinds == nil {
						wantKinds = []models.ULKind{}
					}
					if !reflect.DeepEqual(s.ULKinds, wantKinds) {
						t.Errorf("ulKinds = %v, want %v", s.ULKinds, wantKinds)
					}
				} else if len(s.ULKinds) != 0 {
					t.Errorf("%s carries ulKinds %v", s.Discipline, s.ULKinds)
				}
			}
		})
	}
}

func TestDerive_UnknownLicenceHasNoEvidence(t *testing.T) {
	states := Derive([]*models.License{lic("Segelflugschein alt", "LBA", "9")}, nil, nil, nil, nil, models.DefaultPilotProfile(uuid.New()), testNow)
	for _, s := range states {
		if len(s.Evidence) != 0 {
			t.Errorf("%s has evidence %+v", s.Discipline, s.Evidence)
		}
	}
}

func stateOf(states []models.DisciplineState, d models.Discipline) models.DisciplineState {
	for _, s := range states {
		if s.Discipline == d {
			return s
		}
	}
	return models.DisciplineState{}
}

func TestDerive_EvidenceRefs(t *testing.T) {
	spl := lic("SPL", "LBA", "12345")
	glider := plane("D-1234", "GLIDER")
	states := Derive(
		[]*models.License{spl},
		[]*models.ClassRating{rating(spl, models.ClassTypeGlider)},
		nil,
		[]*models.Aircraft{glider},
		[]models.DisciplineFlightGroup{{AircraftClass: "GLIDER", Flights: 10, LastFlight: day(2026, 7, 1), TowedFlights: 4, LastTowed: day(2026, 8, 2)}},
		models.DefaultPilotProfile(uuid.New()), testNow)

	ev := stateOf(states, models.DisciplineSailplane).Evidence
	want := []struct {
		source   models.EvidenceSource
		strength models.EvidenceStrength
		ref      string
		refID    *uuid.UUID
	}{
		{models.EvidenceLicence, models.StrengthStrong, "SPL 12345", &spl.ID},
		{models.EvidenceRating, models.StrengthStrong, "GLIDER on SPL 12345", nil},
		{models.EvidenceAircraft, models.StrengthRecent, "D-1234", &glider.ID},
		{models.EvidenceFlights, models.StrengthRecent, "14 flights, last 2026-08-02", nil},
	}
	if len(ev) != len(want) {
		t.Fatalf("evidence = %+v", ev)
	}
	for i, w := range want {
		if ev[i].Source != w.source || ev[i].Strength != w.strength || ev[i].Ref != w.ref {
			t.Errorf("evidence[%d] = %+v, want %+v", i, ev[i], w)
		}
		if w.refID != nil && (ev[i].RefID == nil || *ev[i].RefID != *w.refID) {
			t.Errorf("evidence[%d].refId = %v, want %v", i, ev[i].RefID, *w.refID)
		}
	}
	if ev[3].LastSeen == nil || !ev[3].LastSeen.Equal(*day(2026, 8, 2)) {
		t.Errorf("lastSeen = %v", ev[3].LastSeen)
	}
}

func TestDerive_PrivilegeEvidenceRef(t *testing.T) {
	spl := lic("SPL", "LBA", "12345")
	fi := priv(spl, models.PrivilegeFIS, nil)
	states := Derive([]*models.License{spl}, nil, []*models.LicencePrivilege{fi}, nil, nil,
		models.DefaultPilotProfile(uuid.New()), testNow)
	ev := stateOf(states, models.DisciplineInstructor).Evidence
	if len(ev) != 1 || ev[0].Source != models.EvidenceRating || ev[0].Strength != models.StrengthStrong ||
		ev[0].Ref != "FI(S) on SPL 12345" || ev[0].RefID == nil || *ev[0].RefID != fi.ID {
		t.Errorf("evidence = %+v", ev)
	}
}

func TestDerive_DualOnlyEvidenceSource(t *testing.T) {
	states := Derive(nil, nil, nil, nil,
		[]models.DisciplineFlightGroup{{AircraftClass: "GLIDER", TowedFlights: 3, TowedDualReceivedFlights: 3, LastTowed: day(2026, 9, 1)}},
		models.DefaultPilotProfile(uuid.New()), testNow)
	ev := stateOf(states, models.DisciplineSailplane).Evidence
	if len(ev) != 1 || ev[0].Source != models.EvidenceFlightsDual || ev[0].Ref != "3 dual flights, last 2026-09-01" {
		t.Errorf("evidence = %+v", ev)
	}
}

func TestPendingAcknowledgement(t *testing.T) {
	ack := testNow
	tests := []struct {
		name  string
		state models.DisciplineState
		want  bool
	}{
		{"auto active pending", models.DisciplineState{Status: models.StatusActive, Intent: models.IntentAuto}, true},
		{"auto training pending", models.DisciplineState{Status: models.StatusTraining, Intent: models.IntentAuto}, true},
		{"auto dormant not pending", models.DisciplineState{Status: models.StatusDormant, Intent: models.IntentAuto}, false},
		{"acknowledged not pending", models.DisciplineState{Status: models.StatusActive, Intent: models.IntentAuto, AcknowledgedAt: &ack}, false},
		{"explicit on not pending", models.DisciplineState{Status: models.StatusActive, Intent: models.IntentOn}, false},
		{"explicit goal not pending", models.DisciplineState{Status: models.StatusTraining, Intent: models.IntentGoal}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.state.Discipline = models.DisciplineSailplane
			got := PendingAcknowledgement([]models.DisciplineState{tt.state})
			if (len(got) == 1) != tt.want {
				t.Errorf("pending = %v, want %v", got, tt.want)
			}
		})
	}
}
