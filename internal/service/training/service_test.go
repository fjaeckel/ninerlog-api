package training

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type stubProfile struct {
	states []models.DisciplineState
	err    error
}

func (s stubProfile) Get(context.Context, uuid.UUID) (*models.DerivedPilotProfile, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &models.DerivedPilotProfile{Disciplines: s.states}, nil
}

type stubLicences struct {
	list []*models.License
}

func (s stubLicences) GetByUserID(context.Context, uuid.UUID, *time.Time) ([]*models.License, error) {
	return s.list, nil
}

type stubRepo struct {
	flights  []repository.TrainingFlight
	pic      int
	listed   int
	picCalls int
	err      error
}

func (s *stubRepo) ListTrainingFlights(context.Context, uuid.UUID) ([]repository.TrainingFlight, error) {
	s.listed++
	return s.flights, s.err
}

func (s *stubRepo) OtherCategoryPICMinutes(context.Context, uuid.UUID) (int, error) {
	s.picCalls++
	return s.pic, nil
}

func state(d models.Discipline, st models.DisciplineStatus, kinds ...models.ULKind) models.DisciplineState {
	if kinds == nil {
		kinds = []models.ULKind{}
	}
	return models.DisciplineState{Discipline: d, Status: st, ULKinds: kinds}
}

func ids(p *models.TrainingProgress) []models.TrainingProgrammeID {
	out := []models.TrainingProgrammeID{}
	for _, pr := range p.Programmes {
		out = append(out, pr.ID)
	}
	return out
}

func TestService_Progress(t *testing.T) {
	ppl := &models.License{LicenseType: "PPL(A)", RegulatoryAuthority: "EASA"}
	spl := &models.License{LicenseType: "SPL", RegulatoryAuthority: "LBA"}
	tests := []struct {
		name       string
		states     []models.DisciplineState
		licences   []*models.License
		requested  []models.TrainingProgrammeID
		want       []models.TrainingProgrammeID
		wantErr    error
		wantListed int
		wantCredit bool
	}{
		{name: "R1 empty account has no programmes", want: []models.TrainingProgrammeID{}},
		{name: "A1 active disciplines give no programmes",
			states: []models.DisciplineState{state(models.DisciplineAeroplane, models.StatusActive), state(models.DisciplineMultiCrew, models.StatusActive)},
			want:   []models.TrainingProgrammeID{}},
		{name: "J1 SAILPLANE training gives SPL", states: []models.DisciplineState{state(models.DisciplineSailplane, models.StatusTraining)},
			want: []models.TrainingProgrammeID{models.TrainingSPL}, wantListed: 1},
		{name: "J3 SAILPLANE active gives nothing", states: []models.DisciplineState{state(models.DisciplineSailplane, models.StatusActive)},
			licences: []*models.License{spl}, want: []models.TrainingProgrammeID{}},
		{name: "N1 PPL holder in SPL training gets the credit", licences: []*models.License{ppl},
			states: []models.DisciplineState{state(models.DisciplineAeroplane, models.StatusActive), state(models.DisciplineSailplane, models.StatusTraining)},
			want:   []models.TrainingProgrammeID{models.TrainingSPL}, wantListed: 1, wantCredit: true},
		{name: "SPL holder asking for SPL gets no credit", licences: []*models.License{spl}, requested: []models.TrainingProgrammeID{models.TrainingSPL},
			want: []models.TrainingProgrammeID{models.TrainingSPL}, wantListed: 1},
		{name: "TMG training gives the extension", states: []models.DisciplineState{state(models.DisciplineTMG, models.StatusTraining)},
			want: []models.TrainingProgrammeID{models.TrainingSPLTMGExtension}, wantListed: 1},
		{name: "UL training by kind", states: []models.DisciplineState{state(models.DisciplineUltralight, models.StatusTraining, models.ULKindWeightShift, models.ULKindThreeAxisMotorglider)},
			want: []models.TrainingProgrammeID{models.TrainingULThreeAxis, models.TrainingULWeightShift}, wantListed: 1},
		{name: "UL training without a template kind gives nothing", states: []models.DisciplineState{state(models.DisciplineUltralight, models.StatusTraining, models.ULKindPoweredParaglider)},
			want: []models.TrainingProgrammeID{}},
		{name: "dormant SAILPLANE gives nothing", states: []models.DisciplineState{state(models.DisciplineSailplane, models.StatusDormant)},
			want: []models.TrainingProgrammeID{}},
		{name: "explicit request merges and dedupes in fixed order",
			states:    []models.DisciplineState{state(models.DisciplineSailplane, models.StatusTraining)},
			requested: []models.TrainingProgrammeID{models.TrainingULWeightShift, models.TrainingSPL, models.TrainingSPL},
			want:      []models.TrainingProgrammeID{models.TrainingSPL, models.TrainingULWeightShift}, wantListed: 1},
		{name: "unknown programme", requested: []models.TrainingProgrammeID{"PPL"}, wantErr: models.ErrUnknownTrainingProgramme},
		{name: "lower-case programme is unknown", requested: []models.TrainingProgrammeID{"spl"}, wantErr: models.ErrUnknownTrainingProgramme},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubRepo{pic: 3000}
			svc := NewService(stubProfile{states: tt.states}, stubLicences{list: tt.licences}, repo)
			got, err := svc.Progress(context.Background(), uuid.New(), tt.requested)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(ids(got), tt.want) {
				t.Errorf("programmes = %v, want %v", ids(got), tt.want)
			}
			if repo.listed != tt.wantListed {
				t.Errorf("flight reads = %d, want %d", repo.listed, tt.wantListed)
			}
			hasCredit := false
			for _, p := range got.Programmes {
				if _, ok := itemOf(p, "training.spl.credit_sfcl130b"); ok {
					hasCredit = true
				}
			}
			if hasCredit != tt.wantCredit {
				t.Errorf("credit item present = %v, want %v", hasCredit, tt.wantCredit)
			}
		})
	}
}

func TestService_ProgressErrors(t *testing.T) {
	boom := errors.New("boom")
	svc := NewService(stubProfile{err: boom}, stubLicences{}, &stubRepo{})
	if _, err := svc.Progress(context.Background(), uuid.New(), nil); !errors.Is(err, boom) {
		t.Errorf("profile error = %v", err)
	}
	svc = NewService(stubProfile{}, stubLicences{}, &stubRepo{err: boom})
	if _, err := svc.Progress(context.Background(), uuid.New(), []models.TrainingProgrammeID{models.TrainingSPL}); !errors.Is(err, boom) {
		t.Errorf("repository error = %v", err)
	}
}

func TestIsOtherCategoryLicence(t *testing.T) {
	tests := []struct {
		kind models.LicenceKind
		want bool
	}{
		{models.LicenceKindPPLA, true},
		{models.LicenceKindGPL, true},
		{models.LicenceKindHelicopter, true},
		{models.LicenceKindSPL, false},
		{models.LicenceKindLAPLS, false},
		{models.LicenceKindUL, false},
		{models.LicenceKindUnknown, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			if got := IsOtherCategoryLicence(tt.kind); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
