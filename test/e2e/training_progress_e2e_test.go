//go:build e2e

package e2e_test

import (
	"net/http"
	"testing"
)

// ─── Training progress (WP-29) ──────────────────────────────────────────────

type tpItem struct {
	Key           string `json:"key"`
	Required      int    `json:"required"`
	Current       int    `json:"current"`
	Unit          string `json:"unit"`
	Met           bool   `json:"met"`
	Informational bool   `json:"informational"`
	MessageKey    string `json:"messageKey"`
}

type tpProgramme struct {
	ID            string   `json:"id"`
	Discipline    string   `json:"discipline"`
	TitleKey      string   `json:"titleKey"`
	LegalBasis    string   `json:"legalBasis"`
	Items         []tpItem `json:"items"`
	AllMet        bool     `json:"allMet"`
	SignedFlights int      `json:"signedFlights"`
}

type tpProgress struct {
	Programmes []tpProgramme `json:"programmes"`
}

func getTrainingProgress(t *testing.T, c *E2EClient, query string) tpProgress {
	t.Helper()
	resp := c.GET("/training/progress" + query)
	requireStatus(t, resp, http.StatusOK)
	var p tpProgress
	if err := resp.JSON(&p); err != nil {
		t.Fatalf("decode training progress: %v — body=%s", err, string(resp.Body))
	}
	if p.Programmes == nil {
		t.Fatalf("programmes must be an array, never null: %s", string(resp.Body))
	}
	return p
}

func (p tpProgress) ids() []string {
	out := []string{}
	for _, pr := range p.Programmes {
		out = append(out, pr.ID)
	}
	return out
}

func (p tpProgramme) item(t *testing.T, key string) tpItem {
	t.Helper()
	for _, it := range p.Items {
		if it.Key == key {
			return it
		}
	}
	t.Fatalf("%s: item %s missing in %+v", p.ID, key, p.Items)
	return tpItem{}
}

func assertItem(t *testing.T, p tpProgramme, key string, required, current int, met bool) {
	t.Helper()
	it := p.item(t, key)
	if it.Required != required || it.Current != current || it.Met != met {
		t.Errorf("%s = %+v, want required %d current %d met %v", key, it, required, current, met)
	}
}

func assertIDs(t *testing.T, p tpProgress, want ...string) {
	t.Helper()
	got := p.ids()
	if len(got) != len(want) {
		t.Fatalf("programmes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("programmes = %v, want %v", got, want)
		}
	}
}

// createFlightID posts a flight and returns its id.
func createFlightID(t *testing.T, c *E2EClient, body map[string]interface{}) string {
	t.Helper()
	resp := c.POST("/flights", body)
	requireStatus(t, resp, http.StatusCreated)
	var f struct {
		ID string `json:"id"`
	}
	if err := resp.JSON(&f); err != nil {
		t.Fatalf("decode flight: %v", err)
	}
	return f.ID
}

func jonasDual(date, reg string) map[string]interface{} {
	return map[string]interface{}{
		"date": date, "aircraftReg": reg, "aircraftType": "ASK21",
		"departureIcao": "EDNY", "arrivalIcao": "EDNY",
		"departureTime": "10:00", "arrivalTime": "10:30",
		"landings": 1, "launchMethod": "winch",
		"crewMembers": []map[string]interface{}{{"name": "FI Weber", "role": "Instructor"}},
	}
}

func jonasSupervisedSolo(date, reg string) map[string]interface{} {
	return map[string]interface{}{
		"date": date, "aircraftReg": reg, "aircraftType": "ASK23",
		"departureIcao": "EDNY", "arrivalIcao": "EDNY",
		"departureTime": "11:00", "arrivalTime": "11:20",
		"landings": 1, "launchMethod": "winch", "spicTime": 20,
	}
}

func TestTrainingProgress_Personas(t *testing.T) {
	t.Run("J1 Jonas SPL progress from dual and supervised-solo flights", func(t *testing.T) {
		c := setupCurrencyUser(t, "tp-jonas")
		createAircraftCur(t, c, "D-1234", "ASK21", "GLIDER")
		createAircraftCur(t, c, "D-2345", "ASK23", "GLIDER")
		first := createFlightID(t, c, jonasDual(pastDate(12), "D-1234"))
		for i := 0; i < 2; i++ {
			createFlightID(t, c, jonasDual(pastDate(10+i), "D-1234"))
		}
		for i := 0; i < 2; i++ {
			createFlightID(t, c, jonasSupervisedSolo(pastDate(3+i), "D-2345"))
		}
		resp := c.POST("/flights/"+first+"/signatures/live", map[string]interface{}{
			"signerName": "FI Weber", "credentialNumber": "DE.FI(S).77", "signatureImage": testSignaturePNG(),
		})
		requireStatus(t, resp, http.StatusCreated)

		p := getTrainingProgress(t, c, "")
		assertIDs(t, p, "SPL")
		spl := p.Programmes[0]
		if spl.Discipline != "SAILPLANE" || spl.LegalBasis != "SFCL.130" || spl.TitleKey != "training.programme.spl" {
			t.Errorf("SPL header = %+v", spl)
		}
		assertItem(t, spl, "training.spl.instruction_time", 900, 130, false)
		assertItem(t, spl, "training.spl.dual_time", 600, 90, false)
		assertItem(t, spl, "training.spl.supervised_solo_time", 120, 40, false)
		assertItem(t, spl, "training.spl.launches", 45, 5, false)
		assertItem(t, spl, "training.spl.cross_country", 1, 0, false)
		if it := spl.item(t, "training.spl.dual_time"); it.Unit != "minutes" || it.MessageKey != "training.not_met" {
			t.Errorf("dual item = %+v", it)
		}
		if it := spl.item(t, "training.spl.launches"); it.Unit != "launches" {
			t.Errorf("launches unit = %s", it.Unit)
		}
		if spl.AllMet {
			t.Error("allMet = true")
		}
		if spl.SignedFlights != 1 {
			t.Errorf("signedFlights = %d, want 1", spl.SignedFlights)
		}
		if len(spl.Items) != 5 {
			t.Errorf("Jonas holds no licence, so no credit item: %+v", spl.Items)
		}

		// Dual cross-country EDNY-EDDS (~117 km) meets the 100 km alternative.
		xc := jonasDual(pastDate(2), "D-1234")
		xc["arrivalIcao"] = "EDDS"
		xc["arrivalTime"] = "12:00"
		createFlightID(t, c, xc)
		spl = getTrainingProgress(t, c, "").Programmes[0]
		cc := spl.item(t, "training.spl.cross_country")
		if !cc.Met || cc.Current != 1 || cc.MessageKey != "training.met" {
			t.Errorf("cross-country = %+v", cc)
		}

		t.Run("J3 the SPL turns SAILPLANE active and the programme leaves", func(t *testing.T) {
			licID := createLicenceNumbered(t, c, "LBA", "SPL", "DE.SFCL.4711")
			createRatingCur(t, c, licID, "GLIDER", nil)
			assertIDs(t, getTrainingProgress(t, c, ""))
			again := getTrainingProgress(t, c, "?programme=SPL")
			assertIDs(t, again, "SPL")
			assertItem(t, again.Programmes[0], "training.spl.dual_time", 600, 210, false)
		})
	})

	t.Run("N1 Anna SPL progress with SFCL.130(b) credit note", func(t *testing.T) {
		c := setupCurrencyUser(t, "tp-anna")
		licID := createLicenceNumbered(t, c, "EASA", "PPL(A)", "DE.FCL.ANNA")
		createRatingCur(t, c, licID, "SEP_LAND", nil)
		createAircraftCur(t, c, "D-EANA", "C172", "SEP_LAND")
		for i := 0; i < 10; i++ {
			createFlightID(t, c, map[string]interface{}{
				"date": pastDate(60 + i), "aircraftReg": "D-EANA", "aircraftType": "C172",
				"departureIcao": "EDNY", "arrivalIcao": "EDNY",
				"offBlockTime": "09:00", "onBlockTime": "11:30", "landings": 1,
			})
		}
		createAircraftCur(t, c, "D-1234", "ASK21", "GLIDER")
		createFlightID(t, c, jonasDual(pastDate(5), "D-1234"))

		p := getPilotProfile(t, c)
		if p.state("AEROPLANE").Status != "active" || p.state("SAILPLANE").Status != "training" {
			t.Fatalf("Anna profile: AEROPLANE %s, SAILPLANE %s", p.state("AEROPLANE").Status, p.state("SAILPLANE").Status)
		}

		progress := getTrainingProgress(t, c, "")
		assertIDs(t, progress, "SPL")
		spl := progress.Programmes[0]
		assertItem(t, spl, "training.spl.dual_time", 600, 30, false)
		credit := spl.item(t, "training.spl.credit_sfcl130b")
		if !credit.Informational || credit.Required != 420 || credit.Current < 140 || credit.Current > 160 ||
			credit.MessageKey != "training.credit_available" || credit.Unit != "minutes" {
			t.Errorf("credit = %+v, want ~150 of 420 minutes, informational", credit)
		}
	})

	t.Run("explicit programme query", func(t *testing.T) {
		c := setupCurrencyUser(t, "tp-explicit")
		createAircraftCur(t, c, "D-KTMG", "SF25C", "TMG")
		createFlightID(t, c, map[string]interface{}{
			"date": pastDate(4), "aircraftReg": "D-KTMG", "aircraftType": "SF25C",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "10:00", "onBlockTime": "11:00", "landings": 3,
			"crewMembers": []map[string]interface{}{{"name": "FI Weber", "role": "Instructor"}},
		})

		p := getTrainingProgress(t, c, "?programme=UL_WEIGHT_SHIFT&programme=SPL_TMG_EXTENSION&programme=SPL_TMG_EXTENSION")
		assertIDs(t, p, "SPL_TMG_EXTENSION", "UL_WEIGHT_SHIFT")
		tmg := p.Programmes[0]
		if tmg.Discipline != "TMG" || tmg.LegalBasis != "SFCL.150(b)" {
			t.Errorf("TMG header = %+v", tmg)
		}
		assertItem(t, tmg, "training.tmg.instruction_time", 360, 60, false)
		assertItem(t, tmg, "training.tmg.dual_time", 240, 60, false)
		assertItem(t, tmg, "training.tmg.solo_cross_country", 1, 0, false)
		ws := p.Programmes[1]
		if ws.Discipline != "ULTRALIGHT" || ws.LegalBasis != "LuftPersV §42" || len(ws.Items) != 3 {
			t.Errorf("weight-shift = %+v", ws)
		}
		assertItem(t, ws, "training.ul.total_time", 1500, 0, false)
	})

	t.Run("unknown programme is 400", func(t *testing.T) {
		c := setupCurrencyUser(t, "tp-unknown")
		for _, q := range []string{"?programme=PPL", "?programme=spl", "?programme=SPL&programme=LAPL"} {
			resp := c.GET("/training/progress" + q)
			requireStatus(t, resp, http.StatusBadRequest)
		}
	})

	t.Run("unauthenticated is 401", func(t *testing.T) {
		c := NewE2EClient(t)
		requireStatus(t, c.GET("/training/progress"), http.StatusUnauthorized)
	})

	t.Run("A1 Mark airline pilot has no programmes", func(t *testing.T) {
		c := setupCurrencyUser(t, "tp-mark")
		createLicenceNumbered(t, c, "EASA", "ATPL", "DE.FCL.MARK")
		createMultiPilotAircraft(t, c, "D-AIPX", "A320", "MEP_LAND")
		createFlightID(t, c, map[string]interface{}{
			"date": pastDate(3), "aircraftReg": "D-AIPX", "aircraftType": "A320",
			"departureIcao": "EDDF", "arrivalIcao": "EDDM",
			"offBlockTime": "06:00", "onBlockTime": "07:10", "landings": 1,
		})
		assertIDs(t, getTrainingProgress(t, c, ""))
	})

	t.Run("R1 empty account has no programmes", func(t *testing.T) {
		c := setupCurrencyUser(t, "tp-ruth")
		assertIDs(t, getTrainingProgress(t, c, ""))
	})
}
