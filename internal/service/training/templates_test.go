package training

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
)

func glider(dual, pic, spic, launches int) repository.TrainingFlight {
	return repository.TrainingFlight{AircraftClass: "GLIDER", DualMinutes: dual, PICMinutes: pic, SPICMinutes: spic, Launches: launches, Landings: launches}
}

func withXC(f repository.TrainingFlight, nm float64) repository.TrainingFlight {
	f.CrossCountryMinutes = 60
	f.DistanceNM = nm
	return f
}

func signed(f repository.TrainingFlight) repository.TrainingFlight {
	f.Signed = true
	return f
}

func ul(kind models.ULKind, dual, pic, spic int) repository.TrainingFlight {
	k := kind
	return repository.TrainingFlight{AircraftClass: "ULTRALIGHT", ULKind: &k, DualMinutes: dual, PICMinutes: pic, SPICMinutes: spic, Launches: 1, Landings: 1}
}

func tmg(dual, pic, spic int) repository.TrainingFlight {
	return repository.TrainingFlight{AircraftClass: "TMG", DualMinutes: dual, PICMinutes: pic, SPICMinutes: spic, Launches: 1, Landings: 1}
}

func itemOf(p models.TrainingProgramme, key string) (models.TrainingItem, bool) {
	for _, it := range p.Items {
		if it.Key == key {
			return it, true
		}
	}
	return models.TrainingItem{}, false
}

type wantItem struct {
	key     string
	current int
	met     bool
	msg     string
}

func TestEvaluate(t *testing.T) {
	repeat := func(n int, f repository.TrainingFlight) []repository.TrainingFlight {
		out := make([]repository.TrainingFlight, n)
		for i := range out {
			out[i] = f
		}
		return out
	}
	concat := func(parts ...[]repository.TrainingFlight) []repository.TrainingFlight {
		var out []repository.TrainingFlight
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}

	tests := []struct {
		name       string
		id         models.TrainingProgrammeID
		flights    []repository.TrainingFlight
		credit     *Credit
		items      []wantItem
		allMet     bool
		signed     int
		itemCount  int
		discipline models.Discipline
		legal      string
	}{
		{
			name: "SPL empty logbook", id: models.TrainingSPL, discipline: models.DisciplineSailplane, legal: "SFCL.130", itemCount: 5,
			items: []wantItem{
				{"training.spl.instruction_time", 0, false, MsgNotMet},
				{"training.spl.dual_time", 0, false, MsgNotMet},
				{"training.spl.supervised_solo_time", 0, false, MsgNotMet},
				{"training.spl.launches", 0, false, MsgNotMet},
				{"training.spl.cross_country", 0, false, MsgNotMet},
			},
		},
		{
			name: "J1 SPL dual and supervised solo split", id: models.TrainingSPL, itemCount: 5, signed: 2,
			flights: []repository.TrainingFlight{signed(glider(30, 0, 0, 1)), signed(glider(25, 0, 0, 1)), glider(0, 0, 20, 1)},
			items: []wantItem{
				{"training.spl.instruction_time", 75, false, MsgNotMet},
				{"training.spl.dual_time", 55, false, MsgNotMet},
				{"training.spl.supervised_solo_time", 20, false, MsgNotMet},
				{"training.spl.launches", 3, false, MsgNotMet},
			},
		},
		{
			name: "SPL PIC time is not instruction and its launches do not count", id: models.TrainingSPL, itemCount: 5,
			flights: []repository.TrainingFlight{glider(0, 120, 0, 3), glider(10, 0, 0, 1)},
			items: []wantItem{
				{"training.spl.instruction_time", 10, false, MsgNotMet},
				{"training.spl.launches", 1, false, MsgNotMet},
			},
		},
		{
			name: "SPL winch series counts every launch", id: models.TrainingSPL, itemCount: 5,
			flights: []repository.TrainingFlight{glider(40, 0, 0, 8), glider(0, 0, 18, 6)},
			items:   []wantItem{{"training.spl.launches", 14, false, MsgNotMet}},
		},
		{
			name: "SPL only GLIDER flights count", id: models.TrainingSPL, itemCount: 5,
			flights: []repository.TrainingFlight{tmg(600, 0, 0), ul(models.ULKindSailplane, 60, 0, 0)},
			items:   []wantItem{{"training.spl.dual_time", 0, false, MsgNotMet}},
		},
		{
			name: "SPL all met at the thresholds", id: models.TrainingSPL, itemCount: 5, allMet: true,
			flights: concat(repeat(30, glider(20, 0, 0, 1)), repeat(15, glider(0, 0, 8, 1)), []repository.TrainingFlight{withXC(glider(0, 0, 180, 0), 27)}),
			items: []wantItem{
				{"training.spl.instruction_time", 900, true, MsgMet},
				{"training.spl.dual_time", 600, true, MsgMet},
				{"training.spl.supervised_solo_time", 300, true, MsgMet},
				{"training.spl.launches", 45, true, MsgMet},
				{"training.spl.cross_country", 1, true, MsgMet},
			},
		},
		{
			name: "SPL solo cross-country short of 50 km", id: models.TrainingSPL, itemCount: 5,
			flights: []repository.TrainingFlight{withXC(glider(0, 0, 60, 1), 26)},
			items:   []wantItem{{"training.spl.cross_country", 0, false, MsgNotMet}},
		},
		{
			name: "SPL dual cross-country needs 100 km", id: models.TrainingSPL, itemCount: 5,
			flights: []repository.TrainingFlight{withXC(glider(90, 0, 0, 1), 40)},
			items:   []wantItem{{"training.spl.cross_country", 0, false, MsgNotMet}},
		},
		{
			name: "SPL dual cross-country of 100 km", id: models.TrainingSPL, itemCount: 5,
			flights: []repository.TrainingFlight{withXC(glider(90, 0, 0, 1), 54)},
			items:   []wantItem{{"training.spl.cross_country", 1, true, MsgMet}},
		},
		{
			name: "SPL cross-country with unknown distance", id: models.TrainingSPL, itemCount: 5,
			flights: []repository.TrainingFlight{withXC(glider(0, 0, 60, 1), 0)},
			items:   []wantItem{{"training.spl.cross_country", 1, true, MsgCrossCountryUnknown}},
		},
		{
			name: "SPL verified cross-country wins over unknown distance", id: models.TrainingSPL, itemCount: 5,
			flights: []repository.TrainingFlight{withXC(glider(0, 0, 60, 1), 0), withXC(glider(0, 60, 0, 1), 30)},
			items:   []wantItem{{"training.spl.cross_country", 1, true, MsgMet}},
		},
		{
			name: "SPL local flight is no cross-country", id: models.TrainingSPL, itemCount: 5,
			flights: []repository.TrainingFlight{glider(0, 0, 60, 1)},
			items:   []wantItem{{"training.spl.cross_country", 0, false, MsgNotMet}},
		},
		{
			name: "N1 SPL credit is 10 % of PIC time", id: models.TrainingSPL, itemCount: 6, credit: &Credit{PICMinutes: 1500},
			items: []wantItem{{"training.spl.credit_sfcl130b", 150, true, MsgCreditAvailable}},
		},
		{
			name: "SPL credit capped at 7 h", id: models.TrainingSPL, itemCount: 6, credit: &Credit{PICMinutes: 60000},
			items: []wantItem{{"training.spl.credit_sfcl130b", 420, true, MsgCreditAvailable}},
		},
		{
			name: "SPL credit without PIC time", id: models.TrainingSPL, itemCount: 6, credit: &Credit{},
			items: []wantItem{{"training.spl.credit_sfcl130b", 0, false, MsgCreditNone}},
		},
		{
			name: "TMG extension counts TMG flights", id: models.TrainingSPLTMGExtension, discipline: models.DisciplineTMG, legal: "SFCL.150(b)", itemCount: 3, signed: 1,
			flights: []repository.TrainingFlight{signed(tmg(240, 0, 0)), tmg(0, 0, 60), glider(600, 0, 0, 1)},
			items: []wantItem{
				{"training.tmg.instruction_time", 300, false, MsgNotMet},
				{"training.tmg.dual_time", 240, true, MsgMet},
				{"training.tmg.solo_cross_country", 0, false, MsgNotMet},
			},
		},
		{
			name: "TMG extension all met", id: models.TrainingSPLTMGExtension, itemCount: 3, allMet: true,
			flights: []repository.TrainingFlight{tmg(240, 0, 0), tmg(0, 0, 60), withXC(tmg(0, 0, 60), 81)},
			items: []wantItem{
				{"training.tmg.instruction_time", 360, true, MsgMet},
				{"training.tmg.solo_cross_country", 1, true, MsgMet},
			},
		},
		{
			name: "TMG dual cross-country does not count", id: models.TrainingSPLTMGExtension, itemCount: 3,
			flights: []repository.TrainingFlight{withXC(tmg(120, 0, 0), 100)},
			items:   []wantItem{{"training.tmg.solo_cross_country", 0, false, MsgNotMet}},
		},
		{
			name: "TMG solo cross-country short of 150 km", id: models.TrainingSPLTMGExtension, itemCount: 3,
			flights: []repository.TrainingFlight{withXC(tmg(0, 120, 0), 79)},
			items:   []wantItem{{"training.tmg.solo_cross_country", 0, false, MsgNotMet}},
		},
		{
			name: "UL three-axis counts THREE_AXIS and motorglider", id: models.TrainingULThreeAxis, discipline: models.DisciplineUltralight, legal: "LuftPersV §42", itemCount: 2,
			flights: []repository.TrainingFlight{ul(models.ULKindThreeAxis, 600, 0, 0), ul(models.ULKindThreeAxisMotorglider, 0, 120, 0), ul(models.ULKindWeightShift, 600, 0, 0), ul(models.ULKindThreeAxis, 0, 0, 60)},
			items: []wantItem{
				{"training.ul.total_time", 780, false, MsgNotMet},
				{"training.ul.solo_time", 180, false, MsgNotMet},
			},
		},
		{
			name: "UL three-axis all met", id: models.TrainingULThreeAxis, itemCount: 2, allMet: true,
			flights: []repository.TrainingFlight{ul(models.ULKindThreeAxis, 1500, 0, 0), ul(models.ULKindThreeAxis, 0, 300, 0)},
			items:   []wantItem{{"training.ul.total_time", 1800, true, MsgMet}, {"training.ul.solo_time", 300, true, MsgMet}},
		},
		{
			name: "UL weight-shift needs 10 h dual", id: models.TrainingULWeightShift, discipline: models.DisciplineUltralight, itemCount: 3,
			flights: []repository.TrainingFlight{ul(models.ULKindWeightShift, 540, 0, 0), ul(models.ULKindWeightShift, 0, 960, 0), ul(models.ULKindThreeAxis, 600, 0, 0)},
			items: []wantItem{
				{"training.ul.total_time", 1500, true, MsgMet},
				{"training.ul.dual_time", 540, false, MsgNotMet},
				{"training.ul.solo_time", 960, true, MsgMet},
			},
		},
		{
			name: "UL kindless flights count for no programme", id: models.TrainingULWeightShift, itemCount: 3,
			flights: []repository.TrainingFlight{{AircraftClass: "ULTRALIGHT", DualMinutes: 600}},
			items:   []wantItem{{"training.ul.dual_time", 0, false, MsgNotMet}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Evaluate(tt.id, tt.flights, tt.credit)
			if p.ID != tt.id {
				t.Errorf("id = %s", p.ID)
			}
			if len(p.Items) != tt.itemCount {
				t.Errorf("items = %d, want %d: %+v", len(p.Items), tt.itemCount, p.Items)
			}
			if tt.discipline != "" && p.Discipline != tt.discipline {
				t.Errorf("discipline = %s, want %s", p.Discipline, tt.discipline)
			}
			if tt.legal != "" && p.LegalBasis != tt.legal {
				t.Errorf("legalBasis = %s, want %s", p.LegalBasis, tt.legal)
			}
			if p.TitleKey == "" {
				t.Error("titleKey is empty")
			}
			if p.AllMet != tt.allMet {
				t.Errorf("allMet = %v, want %v", p.AllMet, tt.allMet)
			}
			if p.SignedFlights != tt.signed {
				t.Errorf("signedFlights = %d, want %d", p.SignedFlights, tt.signed)
			}
			for _, w := range tt.items {
				it, ok := itemOf(p, w.key)
				if !ok {
					t.Errorf("missing item %s", w.key)
					continue
				}
				if it.Current != w.current || it.Met != w.met || it.MessageKey != w.msg {
					t.Errorf("%s = %+v, want current %d met %v msg %s", w.key, it, w.current, w.met, w.msg)
				}
			}
		})
	}
}

func TestEvaluate_CreditNeverAffectsAllMet(t *testing.T) {
	var flights []repository.TrainingFlight
	for i := 0; i < 45; i++ {
		flights = append(flights, glider(20, 0, 3, 1))
	}
	flights = append(flights, withXC(glider(120, 0, 0, 0), 60))
	p := Evaluate(models.TrainingSPL, flights, &Credit{})
	credit, ok := itemOf(p, "training.spl.credit_sfcl130b")
	if !ok || !credit.Informational || credit.Met {
		t.Fatalf("credit item = %+v", credit)
	}
	if !p.AllMet {
		t.Errorf("allMet = false with every requirement met: %+v", p.Items)
	}
	for _, it := range p.Items {
		if it.Key != credit.Key && it.Informational {
			t.Errorf("%s is informational", it.Key)
		}
	}
}
