//go:build e2e

package e2e_test

import (
	"net/http"
	"reflect"
	"testing"
)

// ─── Pilot profile (adaptive disciplines) ───────────────────────────────────

type ppEvidence struct {
	Source   string  `json:"source"`
	Strength string  `json:"strength"`
	Ref      string  `json:"ref"`
	RefID    *string `json:"refId"`
	LastSeen *string `json:"lastSeen"`
}

type ppState struct {
	Discipline     string       `json:"discipline"`
	Status         string       `json:"status"`
	Intent         string       `json:"intent"`
	Evidence       []ppEvidence `json:"evidence"`
	ULKinds        []string     `json:"ulKinds"`
	AcknowledgedAt *string      `json:"acknowledgedAt"`
}

type ppProfile struct {
	Mode                   string    `json:"mode"`
	Disciplines            []ppState `json:"disciplines"`
	PendingAcknowledgement []string  `json:"pendingAcknowledgement"`
}

var ppAllDisciplines = []string{
	"AEROPLANE", "TMG", "SAILPLANE", "ULTRALIGHT", "GYROPLANE",
	"HELICOPTER", "IFR", "MULTI_CREW", "INSTRUCTOR", "SIMULATOR",
}

func decodePilotProfile(t *testing.T, resp *Response) ppProfile {
	t.Helper()
	var p ppProfile
	if err := resp.JSON(&p); err != nil {
		t.Fatalf("invalid pilot profile: %v — body=%s", err, string(resp.Body))
	}
	if len(p.Disciplines) != len(ppAllDisciplines) {
		t.Fatalf("disciplines = %d, want %d", len(p.Disciplines), len(ppAllDisciplines))
	}
	for i, d := range p.Disciplines {
		if d.Discipline != ppAllDisciplines[i] {
			t.Errorf("discipline %d = %s, want %s (stable enum order)", i, d.Discipline, ppAllDisciplines[i])
		}
		if d.Evidence == nil || d.ULKinds == nil {
			t.Errorf("%s: evidence and ulKinds must be arrays, never null", d.Discipline)
		}
	}
	if p.PendingAcknowledgement == nil {
		t.Error("pendingAcknowledgement must be an array, never null")
	}
	return p
}

func getPilotProfile(t *testing.T, c *E2EClient) ppProfile {
	t.Helper()
	resp := c.GET("/users/me/pilot-profile")
	requireStatus(t, resp, http.StatusOK)
	return decodePilotProfile(t, resp)
}

func patchPilotProfile(t *testing.T, c *E2EClient, body map[string]interface{}) ppProfile {
	t.Helper()
	resp := c.PATCH("/users/me/pilot-profile", body)
	requireStatus(t, resp, http.StatusOK)
	return decodePilotProfile(t, resp)
}

func (p ppProfile) state(d string) ppState {
	for _, s := range p.Disciplines {
		if s.Discipline == d {
			return s
		}
	}
	return ppState{}
}

// assertStatuses checks every discipline: those named in want, everything else off.
func assertStatuses(t *testing.T, p ppProfile, want map[string]string) {
	t.Helper()
	for _, s := range p.Disciplines {
		w, ok := want[s.Discipline]
		if !ok {
			w = "off"
		}
		if s.Status != w {
			t.Errorf("%s status = %s, want %s (evidence %+v)", s.Discipline, s.Status, w, s.Evidence)
		}
	}
}

func hasEvidence(s ppState, source, strength string) bool {
	for _, e := range s.Evidence {
		if e.Source == source && e.Strength == strength {
			return true
		}
	}
	return false
}

func createLicenceNumbered(t *testing.T, c *E2EClient, authority, licType, number string) string {
	t.Helper()
	resp := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": authority, "licenseType": licType, "licenseNumber": number,
		"issueDate": "2015-04-01", "issuingAuthority": authority,
	})
	requireStatus(t, resp, http.StatusCreated)
	var lic map[string]interface{}
	resp.JSON(&lic)
	return lic["id"].(string)
}

func createMultiPilotAircraft(t *testing.T, c *E2EClient, reg, acType, class string) {
	t.Helper()
	resp := c.POST("/aircraft", map[string]interface{}{
		"registration": reg, "type": acType, "make": "Airbus", "model": acType,
		"aircraftClass": class, "isMultiPilot": true,
	})
	requireStatus(t, resp, http.StatusCreated)
}

// winchCircuits logs n eight-minute winch launches on reg, daysAgo apart from firstDaysAgo.
func winchCircuits(t *testing.T, c *E2EClient, reg string, n, firstDaysAgo int, dual bool) {
	t.Helper()
	for i := 0; i < n; i++ {
		f := map[string]interface{}{
			"date": pastDate(firstDaysAgo + i), "aircraftReg": reg, "aircraftType": "ASK21",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "10:00", "onBlockTime": "10:08",
			"landings": 1, "launchMethod": "winch",
		}
		if dual {
			f["crewMembers"] = []map[string]interface{}{{"name": "Lena FI", "role": "Instructor"}}
		}
		createFlightCur(t, c, f)
	}
}

func TestPilotProfile_Personas(t *testing.T) {
	t.Run("L Lena club glider pilot: SAILPLANE active, everything else off", func(t *testing.T) {
		c := setupCurrencyUser(t, "pp-lena")
		licID := createLicenceNumbered(t, c, "LBA", "SPL", "DE.SFCL.12345")
		createRatingCur(t, c, licID, "GLIDER", nil)
		createAircraftCur(t, c, "D-1234", "ASK21", "GLIDER")
		winchCircuits(t, c, "D-1234", 6, 10, false)

		p := getPilotProfile(t, c)
		assertStatuses(t, p, map[string]string{"SAILPLANE": "active"})
		s := p.state("SAILPLANE")
		if len(s.Evidence) == 0 || s.Evidence[0].Source != "LICENCE" || s.Evidence[0].Ref != "SPL DE.SFCL.12345" || s.Evidence[0].RefID == nil {
			t.Errorf("SAILPLANE licence evidence = %+v", s.Evidence)
		}
		if !hasEvidence(s, "AIRCRAFT", "recent") || !hasEvidence(s, "FLIGHTS", "recent") {
			t.Errorf("SAILPLANE evidence = %+v", s.Evidence)
		}
		if len(p.state("AEROPLANE").Evidence) != 0 {
			t.Errorf("winch launches leaked into AEROPLANE: %+v", p.state("AEROPLANE").Evidence)
		}
		if !reflect.DeepEqual(p.PendingAcknowledgement, []string{"SAILPLANE"}) {
			t.Errorf("pending = %v, want [SAILPLANE]", p.PendingAcknowledgement)
		}
		if p.Mode != "adaptive" {
			t.Errorf("mode = %s", p.Mode)
		}
	})

	t.Run("J1 Jonas student glider pilot: dual glider flights, no licence -> SAILPLANE training", func(t *testing.T) {
		c := setupCurrencyUser(t, "pp-jonas")
		createAircraftCur(t, c, "D-1234", "ASK21", "GLIDER")
		winchCircuits(t, c, "D-1234", 3, 5, true)

		p := getPilotProfile(t, c)
		assertStatuses(t, p, map[string]string{"SAILPLANE": "training"})
		if !hasEvidence(p.state("SAILPLANE"), "FLIGHTS_DUAL", "recent") {
			t.Errorf("SAILPLANE evidence = %+v", p.state("SAILPLANE").Evidence)
		}

		createAircraftCur(t, c, "D-2345", "ASK23", "GLIDER")
		winchCircuits(t, c, "D-2345", 2, 1, false)
		p = getPilotProfile(t, c)
		assertStatuses(t, p, map[string]string{"SAILPLANE": "training"})
		if !hasEvidence(p.state("SAILPLANE"), "FLIGHTS", "recent") {
			t.Errorf("SAILPLANE evidence after supervised solo = %+v", p.state("SAILPLANE").Evidence)
		}
		if !reflect.DeepEqual(p.PendingAcknowledgement, []string{"SAILPLANE"}) {
			t.Errorf("pending = %v, want [SAILPLANE]", p.PendingAcknowledgement)
		}
	})

	t.Run("K2 Karl TMG pilot: TMG active, SAILPLANE dormant (SPL, glider flights only before the window)", func(t *testing.T) {
		c := setupCurrencyUser(t, "pp-karl")
		licID := createLicenceNumbered(t, c, "LBA", "SPL", "DE.SFCL.67")
		createRatingCur(t, c, licID, "TMG", nil)
		createAircraftCur(t, c, "D-KFAL", "SF25", "TMG")
		for i := 0; i < 3; i++ {
			createFlightCur(t, c, map[string]interface{}{
				"date": pastDate(20 + i*7), "aircraftReg": "D-KFAL", "aircraftType": "SF25",
				"departureIcao": "EDNY", "arrivalIcao": "EDTM",
				"offBlockTime": "09:00", "onBlockTime": "10:30",
				"landings": 1, "launchMethod": "self-launch",
			})
		}
		// Glider flying before 2019, on a club glider no longer in his fleet.
		winchCircuits(t, c, "D-0815", 2, 1000, false)

		p := getPilotProfile(t, c)
		assertStatuses(t, p, map[string]string{"TMG": "active", "SAILPLANE": "dormant"})
		if !reflect.DeepEqual(p.PendingAcknowledgement, []string{"TMG"}) {
			t.Errorf("pending = %v, want [TMG] (dormant never pending)", p.PendingAcknowledgement)
		}
		sp := p.state("SAILPLANE")
		if !hasEvidence(sp, "LICENCE", "strong") || !hasEvidence(sp, "FLIGHTS", "dormant") {
			t.Errorf("SAILPLANE evidence = %+v", sp.Evidence)
		}
		if hasEvidence(sp, "FLIGHTS", "recent") {
			t.Errorf("TMG flights counted as SAILPLANE flights: %+v", sp.Evidence)
		}
		if !hasEvidence(p.state("TMG"), "RATING", "strong") || !hasEvidence(p.state("TMG"), "FLIGHTS", "recent") {
			t.Errorf("TMG evidence = %+v", p.state("TMG").Evidence)
		}
	})

	t.Run("M Mehmet three-axis UL pilot with a PPL: ULTRALIGHT[THREE_AXIS] active, AEROPLANE dormant", func(t *testing.T) {
		c := setupCurrencyUser(t, "pp-mehmet")
		ulID := createLicenceNumbered(t, c, "DULV", "Sportpilotenlizenz", "UL-4711")
		createULRatingCur(t, c, ulID, "THREE_AXIS")
		pplID := createLicenceNumbered(t, c, "EASA", "PPL(A)", "DE.FCL.999")
		createRatingCur(t, c, pplID, "SEP_LAND", strPtr(futureDate(200)))
		createULAircraftCur(t, c, "D-MXYZ", "THREE_AXIS")
		for i := 0; i < 4; i++ {
			createFlightCur(t, c, map[string]interface{}{
				"date": pastDate(10 + i*5), "aircraftReg": "D-MXYZ", "aircraftType": "C42",
				"departureIcao": "UL-Platz Musterstadt", "arrivalIcao": "UL-Platz Musterstadt",
				"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
			})
		}
		// A club C172 he last flew three years ago, no longer an active fleet aircraft.
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "D-ECLB", "type": "C172", "make": "Cessna", "model": "172", "aircraftClass": "SEP_LAND",
		})
		requireStatus(t, resp, http.StatusCreated)
		var ac map[string]interface{}
		resp.JSON(&ac)
		for i := 0; i < 3; i++ {
			createFlightCur(t, c, map[string]interface{}{
				"date": pastDate(1100 + i*10), "aircraftReg": "D-ECLB", "aircraftType": "C172",
				"departureIcao": "EDNY", "arrivalIcao": "EDTM",
				"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
			})
		}
		requireStatus(t, c.PATCH("/aircraft/"+ac["id"].(string), map[string]interface{}{"isActive": false}), http.StatusOK)

		p := getPilotProfile(t, c)
		assertStatuses(t, p, map[string]string{"ULTRALIGHT": "active", "AEROPLANE": "dormant"})
		if got := p.state("ULTRALIGHT").ULKinds; !reflect.DeepEqual(got, []string{"THREE_AXIS"}) {
			t.Errorf("ULTRALIGHT ulKinds = %v, want [THREE_AXIS]", got)
		}
		ae := p.state("AEROPLANE")
		if !hasEvidence(ae, "LICENCE", "strong") || !hasEvidence(ae, "FLIGHTS", "dormant") {
			t.Errorf("AEROPLANE evidence = %+v", ae.Evidence)
		}
		if hasEvidence(ae, "FLIGHTS", "recent") || hasEvidence(ae, "AIRCRAFT", "recent") {
			t.Errorf("UL-licensed pilot's C42 fed AEROPLANE: %+v", ae.Evidence)
		}
		if !reflect.DeepEqual(p.PendingAcknowledgement, []string{"ULTRALIGHT"}) {
			t.Errorf("pending = %v, want [ULTRALIGHT]", p.PendingAcknowledgement)
		}
		for _, d := range p.Disciplines {
			if d.Discipline != "ULTRALIGHT" && len(d.ULKinds) != 0 {
				t.Errorf("%s carries ulKinds %v", d.Discipline, d.ULKinds)
			}
		}
	})

	t.Run("A1 Mark airline first officer: AEROPLANE, IFR, MULTI_CREW, SIMULATOR active", func(t *testing.T) {
		c := setupCurrencyUser(t, "pp-mark")
		atplID := createLicenceNumbered(t, c, "EASA", "ATPL", "DE.FCL.A320")
		createRatingCur(t, c, atplID, "IR", strPtr(futureDate(200)))
		createRatingCur(t, c, atplID, "MEP_LAND", strPtr(futureDate(200)))
		createMultiPilotAircraft(t, c, "D-AIPX", "A320", "MEP_LAND")
		for i := 0; i < 3; i++ {
			createFlightCur(t, c, map[string]interface{}{
				"date": pastDate(3 + i*4), "aircraftReg": "D-AIPX", "aircraftType": "A320",
				"departureIcao": "EDDF", "arrivalIcao": "EDDM",
				"offBlockTime": "06:00", "onBlockTime": "07:10", "landings": 1,
				"ifrTime": 70, "multiPilotTime": 70,
			})
		}
		createFlightCur(t, c, map[string]interface{}{
			"date": pastDate(40), "aircraftType": "A320",
			"isSimulator": true, "fstdType": "FFS", "simulatedFlightTime": 240,
		})

		p := getPilotProfile(t, c)
		assertStatuses(t, p, map[string]string{
			"AEROPLANE": "active", "IFR": "active", "MULTI_CREW": "active", "SIMULATOR": "active",
		})
		for _, d := range []string{"SAILPLANE", "ULTRALIGHT", "TMG", "GYROPLANE"} {
			if len(p.state(d).Evidence) != 0 {
				t.Errorf("A1: %s has evidence %+v", d, p.state(d).Evidence)
			}
		}
		if !hasEvidence(p.state("MULTI_CREW"), "LICENCE", "strong") || !hasEvidence(p.state("MULTI_CREW"), "FLIGHTS", "recent") {
			t.Errorf("MULTI_CREW evidence = %+v", p.state("MULTI_CREW").Evidence)
		}
		if !hasEvidence(p.state("SIMULATOR"), "FLIGHTS", "recent") {
			t.Errorf("SIMULATOR evidence = %+v", p.state("SIMULATOR").Evidence)
		}
	})

	t.Run("R2 Ruth empty account: everything off, nothing pending", func(t *testing.T) {
		c := setupCurrencyUser(t, "pp-ruth")
		p := getPilotProfile(t, c)
		assertStatuses(t, p, nil)
		if len(p.PendingAcknowledgement) != 0 {
			t.Errorf("pending = %v", p.PendingAcknowledgement)
		}
		for _, d := range p.Disciplines {
			if d.Intent != "auto" || len(d.Evidence) != 0 || d.AcknowledgedAt != nil {
				t.Errorf("%s = %+v", d.Discipline, d)
			}
		}
		if p.Mode != "adaptive" {
			t.Errorf("mode = %s", p.Mode)
		}
	})
}

func TestPilotProfile_Update(t *testing.T) {
	c := setupCurrencyUser(t, "pp-update")
	licID := createLicenceNumbered(t, c, "LBA", "SPL", "DE.SFCL.1")
	createRatingCur(t, c, licID, "GLIDER", nil)

	t.Run("N1 acknowledge clears pending", func(t *testing.T) {
		before := getPilotProfile(t, c)
		if !reflect.DeepEqual(before.PendingAcknowledgement, []string{"SAILPLANE"}) {
			t.Fatalf("pending = %v", before.PendingAcknowledgement)
		}
		p := patchPilotProfile(t, c, map[string]interface{}{"acknowledge": []string{"SAILPLANE"}})
		if len(p.PendingAcknowledgement) != 0 {
			t.Errorf("pending = %v", p.PendingAcknowledgement)
		}
		first := p.state("SAILPLANE").AcknowledgedAt
		if first == nil {
			t.Fatal("acknowledgedAt not set")
		}
		again := patchPilotProfile(t, c, map[string]interface{}{"acknowledge": []string{"SAILPLANE"}})
		if a := again.state("SAILPLANE").AcknowledgedAt; a == nil || *a != *first {
			t.Errorf("re-acknowledging changed acknowledgedAt: %v -> %v", *first, a)
		}
		if got := getPilotProfile(t, c); len(got.PendingAcknowledgement) != 0 {
			t.Errorf("pending after GET = %v", got.PendingAcknowledgement)
		}
	})

	t.Run("intent off beats the licence", func(t *testing.T) {
		p := patchPilotProfile(t, c, map[string]interface{}{"intents": map[string]string{"SAILPLANE": "off"}})
		s := p.state("SAILPLANE")
		if s.Status != "off" || s.Intent != "off" {
			t.Errorf("SAILPLANE = %+v", s)
		}
		if !hasEvidence(s, "LICENCE", "strong") {
			t.Errorf("evidence must still be reported when off: %+v", s.Evidence)
		}
	})

	t.Run("intents merge and goal resolves training", func(t *testing.T) {
		p := patchPilotProfile(t, c, map[string]interface{}{"intents": map[string]string{"IFR": "goal"}})
		if p.state("SAILPLANE").Intent != "off" {
			t.Errorf("partial merge dropped SAILPLANE intent: %+v", p.state("SAILPLANE"))
		}
		if s := p.state("IFR"); s.Status != "training" || s.Intent != "goal" {
			t.Errorf("IFR = %+v", s)
		}
		if len(p.PendingAcknowledgement) != 0 {
			t.Errorf("explicit intents must not be pending: %v", p.PendingAcknowledgement)
		}
	})

	t.Run("everything mode", func(t *testing.T) {
		p := patchPilotProfile(t, c, map[string]interface{}{"mode": "everything"})
		if p.Mode != "everything" {
			t.Errorf("mode = %s", p.Mode)
		}
		if got := getPilotProfile(t, c); got.Mode != "everything" || got.state("IFR").Intent != "goal" {
			t.Errorf("GET after PATCH = %+v", got)
		}
	})

	t.Run("auto returns to evidence", func(t *testing.T) {
		p := patchPilotProfile(t, c, map[string]interface{}{"intents": map[string]string{"SAILPLANE": "auto"}})
		if s := p.state("SAILPLANE"); s.Status != "active" || s.Intent != "auto" {
			t.Errorf("SAILPLANE = %+v", s)
		}
	})

	t.Run("invalid bodies are rejected with 400", func(t *testing.T) {
		for name, body := range map[string]interface{}{
			"unknown discipline":             map[string]interface{}{"intents": map[string]string{"BALLOON": "on"}},
			"unknown intent":                 map[string]interface{}{"intents": map[string]string{"SAILPLANE": "maybe"}},
			"unknown mode":                   map[string]interface{}{"mode": "sometimes"},
			"unknown discipline acknowledge": map[string]interface{}{"acknowledge": []string{"BALLOON"}},
			"wrong type":                     map[string]interface{}{"intents": []string{"SAILPLANE"}},
		} {
			t.Run(name, func(t *testing.T) {
				assertStatus(t, c.PATCH("/users/me/pilot-profile", body), http.StatusBadRequest)
			})
		}
		if p := getPilotProfile(t, c); p.Mode != "everything" || p.state("IFR").Intent != "goal" {
			t.Errorf("a rejected PATCH changed the profile: %+v", p)
		}
	})

	t.Run("unauthenticated is 401", func(t *testing.T) {
		anon := NewE2EClient(t)
		assertStatus(t, anon.GET("/users/me/pilot-profile"), http.StatusUnauthorized)
		assertStatus(t, anon.PATCH("/users/me/pilot-profile", map[string]interface{}{"mode": "everything"}), http.StatusUnauthorized)
	})
}

func TestPilotProfile_CrossUserIsolation(t *testing.T) {
	a := setupCurrencyUser(t, "pp-iso-a")
	licID := createLicenceNumbered(t, a, "LBA", "SPL", "DE.SFCL.A")
	createRatingCur(t, a, licID, "GLIDER", nil)
	createAircraftCur(t, a, "D-1234", "ASK21", "GLIDER")
	patchPilotProfile(t, a, map[string]interface{}{"mode": "everything", "intents": map[string]string{"IFR": "on"}})

	b := setupCurrencyUser(t, "pp-iso-b")
	p := getPilotProfile(t, b)
	assertStatuses(t, p, nil)
	if p.Mode != "adaptive" || len(p.PendingAcknowledgement) != 0 {
		t.Errorf("user B sees user A's profile: %+v", p)
	}

	patchPilotProfile(t, b, map[string]interface{}{"intents": map[string]string{"IFR": "off"}})
	pa := getPilotProfile(t, a)
	if pa.state("IFR").Intent != "on" || pa.Mode != "everything" || pa.state("SAILPLANE").Status != "active" {
		t.Errorf("user B's PATCH changed user A's profile: %+v", pa)
	}
	for _, e := range pa.state("SAILPLANE").Evidence {
		if e.Source == "AIRCRAFT" && e.Ref != "D-1234" {
			t.Errorf("unexpected aircraft evidence %+v", e)
		}
	}
}
