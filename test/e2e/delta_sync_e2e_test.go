//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// updatedSince is the delta-sync query parameter added for issue #158. The
// contract a sync engine relies on is narrow and easy to break silently:
// strictly-after semantics (so replaying the watermark cannot loop), full
// timestamp precision (the `q=updatedAt>YYYY-MM-DD` workaround only had day
// granularity), composition with the endpoint's other filters, and a paginated
// envelope whose total counts the delta rather than the whole logbook.

func sinceParam(ts string) string {
	return "updatedSince=" + url.QueryEscape(ts)
}

// listRecords GETs a bare-array list endpoint and returns the decoded records.
func listRecords(t *testing.T, c *E2EClient, path string) []map[string]interface{} {
	t.Helper()
	r := c.GET(path)
	requireStatus(t, r, http.StatusOK)
	var out []map[string]interface{}
	if err := r.JSON(&out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return out
}

// listPaged GETs a paginated list endpoint and returns its data and total.
func listPaged(t *testing.T, c *E2EClient, path string) ([]map[string]interface{}, int) {
	t.Helper()
	r := c.GET(path)
	requireStatus(t, r, http.StatusOK)
	var page struct {
		Data       []map[string]interface{} `json:"data"`
		Pagination struct {
			Total int `json:"total"`
		} `json:"pagination"`
	}
	if err := r.JSON(&page); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return page.Data, page.Pagination.Total
}

// updatedAtOf returns the record's updatedAt as the API reports it — the value
// a sync client would store as its watermark.
func updatedAtOf(t *testing.T, record map[string]interface{}) string {
	t.Helper()
	ts, ok := record["updatedAt"].(string)
	if !ok || ts == "" {
		t.Fatalf("record has no updatedAt: %v", record)
	}
	return ts
}

func idOf(t *testing.T, record map[string]interface{}) string {
	t.Helper()
	id, ok := record["id"].(string)
	if !ok || id == "" {
		t.Fatalf("record has no id: %v", record)
	}
	return id
}

func TestDeltaSyncContacts(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("delta-contact"), "SecurePass123!", "Delta Contact")

	requireStatus(t, c.POST("/contacts", map[string]interface{}{"name": "Anna Instructor"}), http.StatusCreated)
	first := listRecords(t, c, "/contacts")
	if len(first) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(first))
	}
	watermark := updatedAtOf(t, first[0])
	annaID := idOf(t, first[0])

	// Replaying the watermark must return nothing: the client already holds
	// everything up to and including that instant.
	if got := listRecords(t, c, "/contacts?"+sinceParam(watermark)); len(got) != 0 {
		t.Errorf("replaying the watermark returned %d contacts, want 0", len(got))
	}

	requireStatus(t, c.POST("/contacts", map[string]interface{}{"name": "Ben Examiner"}), http.StatusCreated)
	delta := listRecords(t, c, "/contacts?"+sinceParam(watermark))
	if len(delta) != 1 || delta[0]["name"] != "Ben Examiner" {
		t.Fatalf("delta returned %d contacts %v, want only Ben Examiner", len(delta), delta)
	}
	benWatermark := updatedAtOf(t, delta[0])

	// Editing an existing record must bring it back into a later window —
	// a sync client that missed updates would silently diverge.
	requireStatus(t, c.PUT("/contacts/"+annaID, map[string]interface{}{"name": "Anna Examiner"}), http.StatusOK)
	afterEdit := listRecords(t, c, "/contacts?"+sinceParam(benWatermark))
	if len(afterEdit) != 1 || idOf(t, afterEdit[0]) != annaID {
		t.Fatalf("after editing Anna the delta was %v, want only Anna", afterEdit)
	}

	// Without the parameter the endpoint is unchanged.
	if all := listRecords(t, c, "/contacts"); len(all) != 2 {
		t.Errorf("unfiltered list returned %d contacts, want 2", len(all))
	}
}

func TestDeltaSyncLicensesAndCredentials(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("delta-lic"), "SecurePass123!", "Delta Lic")

	t.Run("licenses", func(t *testing.T) {
		requireStatus(t, c.POST("/licenses", map[string]interface{}{
			"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "DE-PPL-158",
			"issueDate": "2023-01-15", "issuingAuthority": "LBA",
		}), http.StatusCreated)
		first := listRecords(t, c, "/licenses")
		if len(first) != 1 {
			t.Fatalf("expected 1 license, got %d", len(first))
		}
		watermark := updatedAtOf(t, first[0])

		if got := listRecords(t, c, "/licenses?"+sinceParam(watermark)); len(got) != 0 {
			t.Errorf("replaying the watermark returned %d licenses, want 0", len(got))
		}

		requireStatus(t, c.POST("/licenses", map[string]interface{}{
			"regulatoryAuthority": "FAA", "licenseType": "PPL", "licenseNumber": "US-PPL-158",
			"issueDate": "2024-02-20", "issuingAuthority": "FAA",
		}), http.StatusCreated)
		delta := listRecords(t, c, "/licenses?"+sinceParam(watermark))
		if len(delta) != 1 || delta[0]["licenseNumber"] != "US-PPL-158" {
			t.Fatalf("delta returned %d licenses %v, want only US-PPL-158", len(delta), delta)
		}
	})

	t.Run("credentials", func(t *testing.T) {
		requireStatus(t, c.POST("/credentials", map[string]interface{}{
			"credentialType": "EASA_CLASS2_MEDICAL", "credentialNumber": "MED-158",
			"issueDate": "2024-01-15", "expiryDate": futureDate(365), "issuingAuthority": "AME Smith",
		}), http.StatusCreated)
		first := listRecords(t, c, "/credentials")
		if len(first) != 1 {
			t.Fatalf("expected 1 credential, got %d", len(first))
		}
		watermark := updatedAtOf(t, first[0])

		if got := listRecords(t, c, "/credentials?"+sinceParam(watermark)); len(got) != 0 {
			t.Errorf("replaying the watermark returned %d credentials, want 0", len(got))
		}

		requireStatus(t, c.POST("/credentials", map[string]interface{}{
			"credentialType": "LANG_ICAO_LEVEL4", "issueDate": "2023-06-01",
			"expiryDate": futureDate(730), "issuingAuthority": "LBA",
		}), http.StatusCreated)
		delta := listRecords(t, c, "/credentials?"+sinceParam(watermark))
		if len(delta) != 1 || delta[0]["credentialType"] != "LANG_ICAO_LEVEL4" {
			t.Fatalf("delta returned %d credentials %v, want only the language proficiency", len(delta), delta)
		}
	})
}

func TestDeltaSyncAircraft(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("delta-ac"), "SecurePass123!", "Delta AC")

	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": "D-EAAA", "type": "C172", "make": "Cessna", "model": "172S",
		"aircraftClass": "SEP_LAND",
	}), http.StatusCreated)
	first, total := listPaged(t, c, "/aircraft")
	if total != 1 {
		t.Fatalf("expected 1 aircraft, got total %d", total)
	}
	watermark := updatedAtOf(t, first[0])

	if _, deltaTotal := listPaged(t, c, "/aircraft?"+sinceParam(watermark)); deltaTotal != 0 {
		t.Errorf("replaying the watermark reported total %d, want 0", deltaTotal)
	}

	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": "D-EBBB", "type": "PA28", "make": "Piper", "model": "Archer",
		"aircraftClass": "SEP_LAND",
	}), http.StatusCreated)

	// The pagination envelope must describe the delta, not the fleet: a total
	// of 2 here would send the client paging for a record it can never reach.
	delta, deltaTotal := listPaged(t, c, "/aircraft?"+sinceParam(watermark))
	if deltaTotal != 1 {
		t.Errorf("delta pagination.total = %d, want 1", deltaTotal)
	}
	if len(delta) != 1 || delta[0]["registration"] != "D-EBBB" {
		t.Fatalf("delta returned %v, want only D-EBBB", delta)
	}

	if _, allTotal := listPaged(t, c, "/aircraft"); allTotal != 2 {
		t.Errorf("unfiltered pagination.total = %d, want 2", allTotal)
	}
}

func TestDeltaSyncFlights(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("delta-flight"), "SecurePass123!", "Delta Flight")

	newFlight := func(reg, remarks string) {
		t.Helper()
		requireStatus(t, c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": reg, "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
			"remarks": remarks,
		}), http.StatusCreated)
	}

	newFlight("D-EAAA", "first")
	first, total := listPaged(t, c, "/flights")
	if total != 1 {
		t.Fatalf("expected 1 flight, got total %d", total)
	}
	watermark := updatedAtOf(t, first[0])
	firstID := idOf(t, first[0])

	if _, deltaTotal := listPaged(t, c, "/flights?"+sinceParam(watermark)); deltaTotal != 0 {
		t.Errorf("replaying the watermark reported total %d, want 0", deltaTotal)
	}

	// Both flights are logged on the same calendar day, so day-granular
	// `q=updatedAt>YYYY-MM-DD` cannot separate them. This is the gap
	// updatedSince closes.
	newFlight("D-EBBB", "second")
	delta, deltaTotal := listPaged(t, c, "/flights?"+sinceParam(watermark))
	if deltaTotal != 1 {
		t.Errorf("delta pagination.total = %d, want 1", deltaTotal)
	}
	if len(delta) != 1 || delta[0]["aircraftReg"] != "D-EBBB" {
		t.Fatalf("delta returned %v, want only D-EBBB", delta)
	}
	secondWatermark := updatedAtOf(t, delta[0])

	t.Run("combines with the other filters", func(t *testing.T) {
		// updatedSince is ANDed, not substituted: the only flight in the
		// window is D-EBBB, so asking for D-EAAA within it yields nothing.
		_, narrowed := listPaged(t, c, "/flights?"+sinceParam(watermark)+"&aircraftReg=D-EAAA")
		if narrowed != 0 {
			t.Errorf("updatedSince AND aircraftReg reported total %d, want 0", narrowed)
		}
		_, matching := listPaged(t, c, "/flights?"+sinceParam(watermark)+"&aircraftReg=D-EBBB")
		if matching != 1 {
			t.Errorf("updatedSince AND aircraftReg=D-EBBB reported total %d, want 1", matching)
		}
	})

	t.Run("an edit re-enters a later window", func(t *testing.T) {
		requireStatus(t, c.PUT("/flights/"+firstID, map[string]interface{}{
			"date": today(), "aircraftReg": "D-EAAA", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
			"remarks": "amended after sync",
		}), http.StatusOK)

		edited, editedTotal := listPaged(t, c, "/flights?"+sinceParam(secondWatermark))
		if editedTotal != 1 {
			t.Fatalf("after the edit pagination.total = %d, want 1", editedTotal)
		}
		if idOf(t, edited[0]) != firstID {
			t.Errorf("delta returned flight %s, want the edited %s", idOf(t, edited[0]), firstID)
		}
	})

	t.Run("pages the delta", func(t *testing.T) {
		// Everything logged so far is newer than the epoch, so this pages the
		// full result set through the delta filter.
		epoch := "1970-01-01T00:00:00Z"
		page1, total := listPaged(t, c, "/flights?"+sinceParam(epoch)+"&page=1&pageSize=1")
		if total != 2 {
			t.Fatalf("delta pagination.total = %d, want 2", total)
		}
		if len(page1) != 1 {
			t.Fatalf("page 1 held %d flights, want 1", len(page1))
		}
		page2, _ := listPaged(t, c, "/flights?"+sinceParam(epoch)+"&page=2&pageSize=1")
		if len(page2) != 1 {
			t.Fatalf("page 2 held %d flights, want 1", len(page2))
		}
		if idOf(t, page1[0]) == idOf(t, page2[0]) {
			t.Error("page 2 repeated page 1 — the delta filter broke pagination")
		}
	})
}

var deltaSyncEndpoints = []string{"/flights", "/aircraft", "/contacts", "/credentials", "/licenses"}

// A malformed watermark must be rejected outright. Silently ignoring it would
// hand the client a full listing it would mistake for "nothing changed since",
// which is the worst possible failure for a sync loop.
func TestDeltaSyncRejectsMalformedUpdatedSince(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("delta-bad"), "SecurePass123!", "Delta Bad")

	bad := []string{"yesterday", "2026-08-05 10:08:45", "1754388525", "2026-13-45T00:00:00Z", "T10:08:45Z"}

	for _, ep := range deltaSyncEndpoints {
		for _, value := range bad {
			t.Run(fmt.Sprintf("%s %s", ep, value), func(t *testing.T) {
				assertStatus(t, c.GET(ep+"?"+sinceParam(value)), http.StatusBadRequest)
			})
		}
	}
}

// The accepted spellings, pinned so the bound cannot silently narrow (which
// would break sync clients) or widen into the malformed cases above.
func TestDeltaSyncAcceptedFormats(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("delta-fmt"), "SecurePass123!", "Delta Fmt")

	hourAgo := time.Now().Add(-time.Hour)
	accepted := map[string]string{
		"utc":            hourAgo.UTC().Format(time.RFC3339),
		"sub-second":     hourAgo.UTC().Format("2006-01-02T15:04:05.000000Z"),
		"non-utc offset": hourAgo.Format("2006-01-02T15:04:05-07:00"),
		"date only":      hourAgo.UTC().Format("2006-01-02"),
		"empty (no-op)":  "",
	}

	for name, value := range accepted {
		for _, ep := range deltaSyncEndpoints {
			t.Run(fmt.Sprintf("%s %s", ep, name), func(t *testing.T) {
				assertStatus(t, c.GET(ep+"?"+sinceParam(value)), http.StatusOK)
			})
		}
	}

	// A date-only watermark means midnight UTC on that date, so today's date
	// excludes nothing logged before today and includes what was logged since.
	requireStatus(t, c.POST("/contacts", map[string]interface{}{"name": "Logged Today"}), http.StatusCreated)
	todayOnly := listRecords(t, c, "/contacts?"+sinceParam(time.Now().UTC().Format("2006-01-02")))
	if len(todayOnly) != 1 {
		t.Errorf("date-only watermark returned %d contacts, want 1", len(todayOnly))
	}
	tomorrowOnly := listRecords(t, c, "/contacts?"+sinceParam(time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")))
	if len(tomorrowOnly) != 0 {
		t.Errorf("a future date-only watermark returned %d contacts, want 0", len(tomorrowOnly))
	}

	// An empty value must behave exactly like omitting the parameter, not like
	// a year-zero watermark that quietly reintroduces a full pull under a name
	// that says otherwise.
	if got := listRecords(t, c, "/contacts?"+sinceParam("")); len(got) != 1 {
		t.Errorf("empty updatedSince returned %d contacts, want the full list of 1", len(got))
	}
}

// Delta pulls stay inside the caller's own data — the filter must never widen
// the user scope.
func TestDeltaSyncIsUserScoped(t *testing.T) {
	owner := NewE2EClient(t)
	registerAndLogin(t, owner, uniqueEmail("delta-owner"), "SecurePass123!", "Delta Owner")
	requireStatus(t, owner.POST("/contacts", map[string]interface{}{"name": "Owner Contact"}), http.StatusCreated)

	intruder := NewE2EClient(t)
	registerAndLogin(t, intruder, uniqueEmail("delta-intruder"), "SecurePass123!", "Delta Intruder")

	epoch := "1970-01-01T00:00:00Z"
	if got := listRecords(t, intruder, "/contacts?"+sinceParam(epoch)); len(got) != 0 {
		t.Errorf("another user's delta pull returned %d contacts, want 0", len(got))
	}
}
