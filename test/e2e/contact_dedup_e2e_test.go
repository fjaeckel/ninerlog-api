//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
)

type e2eContact struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Email *string `json:"email"`
	Phone *string `json:"phone"`
}

// listContacts returns the caller's whole address book.
func listContacts(t *testing.T, c *E2EClient) []e2eContact {
	t.Helper()
	r := c.GET("/contacts")
	requireStatus(t, r, 200)
	var out []e2eContact
	r.JSON(&out)
	return out
}

// createFlightWithCrew logs a minimal flight with the given crew and returns
// its id together with the crew members as the API echoed them back.
func createFlightWithCrew(t *testing.T, c *E2EClient, reg string, crew []map[string]interface{}) (string, []map[string]interface{}) {
	t.Helper()
	r := c.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": reg, "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		"crewMembers": crew,
	})
	requireStatus(t, r, 201)
	var flight struct {
		ID          string                   `json:"id"`
		CrewMembers []map[string]interface{} `json:"crewMembers"`
	}
	r.JSON(&flight)
	return flight.ID, flight.CrewMembers
}

// flightCrew re-reads a flight's crew from the server.
func flightCrew(t *testing.T, c *E2EClient, flightID string) []map[string]interface{} {
	t.Helper()
	r := c.GET("/flights/" + flightID)
	requireStatus(t, r, 200)
	var flight struct {
		CrewMembers []map[string]interface{} `json:"crewMembers"`
	}
	r.JSON(&flight)
	return flight.CrewMembers
}

// TestFlightCrewCreatesContacts covers crew names on a logged flight creating
// contacts.
func TestFlightCrewCreatesContacts(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cnt-autocreate"), "SecurePass123!", "AutoCreate")

	if got := listContacts(t, c); len(got) != 0 {
		t.Fatalf("new account starts with %d contacts, want 0", len(got))
	}

	_, crew := createFlightWithCrew(t, c, "D-EAUT", []map[string]interface{}{
		{"name": "Hans Müller", "role": "Instructor"},
		{"name": "Erika Koch", "role": "Passenger"},
	})

	contacts := listContacts(t, c)
	if len(contacts) != 2 {
		t.Fatalf("contacts after logging a flight = %d, want 2: %+v", len(contacts), contacts)
	}
	for _, m := range crew {
		if m["contactId"] == nil {
			t.Errorf("crew member %v was returned without a contactId", m["name"])
		}
	}
}

// A person is one contact however many seats they occupy: the role lives on the
// crew entry, not on the contact.
func TestFlightCrewDedupesPersonAcrossRolesAndFlights(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cnt-dedup"), "SecurePass123!", "Dedup")

	// Same person, two roles on one flight, and a different spelling of case.
	_, crew := createFlightWithCrew(t, c, "D-EDUP", []map[string]interface{}{
		{"name": "Hans Müller", "role": "Instructor"},
		{"name": "hans müller", "role": "PIC"},
	})
	if len(crew) != 2 {
		t.Fatalf("crew members = %d, want 2", len(crew))
	}
	first, second := crew[0]["contactId"], crew[1]["contactId"]
	if first == nil || second == nil {
		t.Fatalf("both crew entries must be linked: %+v", crew)
	}
	if first != second {
		t.Errorf("one person in two cockpit roles produced two contacts (%v vs %v)", first, second)
	}

	// A second flight with the same person, padded and upper-cased.
	_, crew2 := createFlightWithCrew(t, c, "D-EDUQ", []map[string]interface{}{
		{"name": "  HANS MÜLLER  ", "role": "SIC"},
	})
	if crew2[0]["contactId"] != first {
		t.Errorf("a later flight created a second contact for the same person: %v vs %v", crew2[0]["contactId"], first)
	}

	if contacts := listContacts(t, c); len(contacts) != 1 {
		t.Errorf("contacts = %d, want 1: %+v", len(contacts), contacts)
	}
}

// Updating a flight's crew links the newcomers too, not just the create path.
func TestFlightUpdateCreatesContacts(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cnt-update"), "SecurePass123!", "UpdCrew")

	flightID, _ := createFlightWithCrew(t, c, "D-EUPD", []map[string]interface{}{
		{"name": "Hans Müller", "role": "Instructor"},
	})

	r := c.PUT("/flights/"+flightID, map[string]interface{}{
		"crewMembers": []map[string]interface{}{
			{"name": "Hans Müller", "role": "Instructor"},
			{"name": "Petra Wolf", "role": "Passenger"},
		},
	})
	requireStatus(t, r, 200)

	contacts := listContacts(t, c)
	if len(contacts) != 2 {
		t.Fatalf("contacts after adding a crew member = %d, want 2: %+v", len(contacts), contacts)
	}
	for _, m := range flightCrew(t, c, flightID) {
		if m["contactId"] == nil {
			t.Errorf("crew member %v is unlinked after the update", m["name"])
		}
	}
}

// POST /contacts is not a way to get a second row for the same person.
func TestCreateDuplicateContactConflicts(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cnt-conflict"), "SecurePass123!", "Conflict")

	requireStatus(t, c.POST("/contacts", map[string]interface{}{"name": "Hans Müller"}), 201)

	// Same person, different case and padding.
	r := c.POST("/contacts", map[string]interface{}{"name": "  hans müller  "})
	assertStatus(t, r, 409)
	if !strings.Contains(strings.ToLower(string(r.Body)), "already exists") {
		t.Errorf("409 body does not name the cause: %s", r.Body)
	}

	if contacts := listContacts(t, c); len(contacts) != 1 {
		t.Errorf("contacts = %d, want 1: %+v", len(contacts), contacts)
	}
}

// The name is unique per user, not globally: two pilots may each know a Hans.
func TestDuplicateContactNameAcrossUsersIsAllowed(t *testing.T) {
	first := NewE2EClient(t)
	registerAndLogin(t, first, uniqueEmail("cnt-user-a"), "SecurePass123!", "UserA")
	requireStatus(t, first.POST("/contacts", map[string]interface{}{"name": "Hans Müller"}), 201)

	second := NewE2EClient(t)
	registerAndLogin(t, second, uniqueEmail("cnt-user-b"), "SecurePass123!", "UserB")
	assertStatus(t, second.POST("/contacts", map[string]interface{}{"name": "Hans Müller"}), 201)
}

// Renaming a contact rewrites the crew entries that reference it and reports
// how many it touched.
func TestContactRenamePropagatesToCrew(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cnt-rename"), "SecurePass123!", "Rename")

	flightID, crew := createFlightWithCrew(t, c, "D-ERNM", []map[string]interface{}{
		{"name": "Hnas Müller", "role": "Instructor"},
	})
	contactID, _ := crew[0]["contactId"].(string)
	if contactID == "" {
		t.Fatal("crew member was not linked to a contact")
	}

	r := c.PUT("/contacts/"+contactID, map[string]interface{}{"name": "Hans Müller"})
	requireStatus(t, r, 200)
	if got := r.Headers.Get("X-Crew-Entries-Renamed"); got != "1" {
		t.Errorf("X-Crew-Entries-Renamed = %q, want \"1\"", got)
	}

	updated := flightCrew(t, c, flightID)
	if len(updated) != 1 {
		t.Fatalf("crew members = %d, want 1", len(updated))
	}
	if updated[0]["name"] != "Hans Müller" {
		t.Errorf("crew name = %v, want the corrected spelling", updated[0]["name"])
	}
}

// TestCrewEntriesRenamedHeaderIsCORSExposed covers X-Crew-Entries-Renamed
// being listed in Access-Control-Expose-Headers.
func TestCrewEntriesRenamedHeaderIsCORSExposed(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cnt-cors"), "SecurePass123!", "CorsCheck")

	r := c.DoWithHeaders("GET", "/contacts", nil, map[string]string{"Origin": "http://localhost:5173"})
	requireStatus(t, r, 200)

	exposed := r.Headers.Get("Access-Control-Expose-Headers")
	if !strings.Contains(exposed, "X-Crew-Entries-Renamed") {
		t.Errorf("Access-Control-Expose-Headers = %q, want it to include X-Crew-Entries-Renamed", exposed)
	}
}

// Renaming onto a name already in use is a merge, and merges are not implicit.
func TestContactRenameOntoExistingNameConflicts(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cnt-rename-conflict"), "SecurePass123!", "RenameConflict")

	r := c.POST("/contacts", map[string]interface{}{"name": "Anna Berg"})
	requireStatus(t, r, 201)
	var anna e2eContact
	r.JSON(&anna)

	r = c.POST("/contacts", map[string]interface{}{"name": "Bert Stein"})
	requireStatus(t, r, 201)
	var bert e2eContact
	r.JSON(&bert)

	assertStatus(t, c.PUT("/contacts/"+bert.ID, map[string]interface{}{"name": "anna berg"}), 409)

	// Both survive under their original names.
	contacts := listContacts(t, c)
	if len(contacts) != 2 {
		t.Fatalf("contacts = %d, want 2: %+v", len(contacts), contacts)
	}
	byID := map[string]string{}
	for _, ct := range contacts {
		byID[ct.ID] = ct.Name
	}
	if byID[anna.ID] != "Anna Berg" || byID[bert.ID] != "Bert Stein" {
		t.Errorf("a refused rename changed the stored names: %+v", contacts)
	}
}

// Deleting a contact is an address-book operation. The logbook keeps the name
// it was flown with; only the link is dropped.
func TestContactDeleteKeepsLoggedCrewName(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cnt-delete"), "SecurePass123!", "DelCrew")

	flightID, crew := createFlightWithCrew(t, c, "D-EDEL", []map[string]interface{}{
		{"name": "Hans Müller", "role": "Instructor"},
	})
	contactID, _ := crew[0]["contactId"].(string)
	if contactID == "" {
		t.Fatal("crew member was not linked to a contact")
	}

	requireStatus(t, c.DELETE("/contacts/"+contactID), 204)

	after := flightCrew(t, c, flightID)
	if len(after) != 1 {
		t.Fatalf("crew members after deleting the contact = %d, want 1", len(after))
	}
	if after[0]["name"] != "Hans Müller" {
		t.Errorf("crew name = %v, want it preserved verbatim", after[0]["name"])
	}
	if after[0]["contactId"] != nil {
		t.Errorf("contactId = %v, want null after the contact was deleted", after[0]["contactId"])
	}
}

// A crew entry may not point at somebody else's address book.
func TestCrewContactIDFromAnotherUserIsRejected(t *testing.T) {
	stranger := NewE2EClient(t)
	registerAndLogin(t, stranger, uniqueEmail("cnt-stranger"), "SecurePass123!", "Stranger")
	r := stranger.POST("/contacts", map[string]interface{}{"name": "Someone Else"})
	requireStatus(t, r, 201)
	var foreign e2eContact
	r.JSON(&foreign)

	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cnt-owner"), "SecurePass123!", "Owner")

	_, crew := createFlightWithCrew(t, c, "D-EFRN", []map[string]interface{}{
		{"name": "Hans Müller", "role": "Instructor", "contactId": foreign.ID},
	})
	if crew[0]["contactId"] == foreign.ID {
		t.Fatal("a flight kept a contact id belonging to another user")
	}
	// It was re-linked to the caller's own contact of that name.
	linked, _ := crew[0]["contactId"].(string)
	if linked == "" {
		t.Fatal("crew member should have been linked to one of the caller's contacts")
	}
	requireStatus(t, c.GET("/contacts/"+linked), 200)
}

func TestExportContactsVCard(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cnt-vcard"), "SecurePass123!", "VCard")

	requireStatus(t, c.POST("/contacts", map[string]interface{}{
		"name": "Hans Müller", "email": "hans@example.com", "phone": "+49 170 1234567",
	}), 201)
	// Logged as crew, giving the export a role to report.
	createFlightWithCrew(t, c, "D-EVCD", []map[string]interface{}{
		{"name": "Hans Müller", "role": "Instructor"},
	})

	r := c.GET("/exports/vcard")
	requireStatus(t, r, 200)

	if ct := r.Headers.Get("Content-Type"); !strings.HasPrefix(ct, "text/vcard") {
		t.Errorf("Content-Type = %q, want text/vcard", ct)
	}
	if cd := r.Headers.Get("Content-Disposition"); !strings.Contains(cd, ".vcf") {
		t.Errorf("Content-Disposition = %q, want a .vcf attachment", cd)
	}

	body := string(r.Body)
	for _, want := range []string{
		"BEGIN:VCARD",
		"VERSION:3.0",
		"FN:Hans Müller",
		"EMAIL;TYPE=INTERNET:hans@example.com",
		"CATEGORIES:Instructor",
		"END:VCARD",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("vCard export is missing %q\n---\n%s", want, body)
		}
	}
}

func TestExportContactsVCardRequiresAuth(t *testing.T) {
	c := NewE2EClient(t)
	assertStatus(t, c.GET("/exports/vcard"), 401)
}

// An empty address book is a valid export, not an error.
func TestExportContactsVCardEmpty(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cnt-vcard-empty"), "SecurePass123!", "VCardEmpty")

	r := c.GET("/exports/vcard")
	requireStatus(t, r, 200)
	if len(r.Body) != 0 {
		t.Errorf("expected an empty body for an empty address book, got %q", r.Body)
	}
}

// A backup carries the address book, so a restore recreates contacts from the
// contacts section and the crew linker finds them by name rather than
// inventing bare ones. Either way the destination ends up with both.
func TestBackupRestoreRestoresContacts(t *testing.T) {
	source := NewE2EClient(t)
	registerAndLogin(t, source, uniqueEmail("cnt-restore-src"), "SecurePass123!", "RestoreSrc")
	createFlightWithCrew(t, source, "D-ERST", []map[string]interface{}{
		{"name": "Hans Müller", "role": "Instructor"},
		{"name": "Erika Koch", "role": "Passenger"},
	})

	r := source.GET("/exports/json")
	requireStatus(t, r, 200)
	var backup map[string]interface{}
	r.JSON(&backup)

	dest := NewE2EClient(t)
	registerAndLogin(t, dest, uniqueEmail("cnt-restore-dst"), "SecurePass123!", "RestoreDst")

	r = dest.POST("/imports/json", backup)
	requireStatus(t, r, 200)
	var summary struct {
		FlightsImported  int `json:"flightsImported"`
		ContactsImported int `json:"contactsImported"`
		ContactsCreated  int `json:"contactsCreated"`
	}
	r.JSON(&summary)
	if summary.FlightsImported != 1 {
		t.Fatalf("flightsImported = %d, want 1", summary.FlightsImported)
	}
	if summary.ContactsImported != 2 {
		t.Errorf("contactsImported = %d, want 2", summary.ContactsImported)
	}
	// The linker matched both names against the contacts just restored, so it
	// had nothing left to create.
	if summary.ContactsCreated != 0 {
		t.Errorf("contactsCreated = %d, want 0 — the restored contacts should have been matched", summary.ContactsCreated)
	}

	if contacts := listContacts(t, dest); len(contacts) != 2 {
		t.Errorf("restored account has %d contacts, want 2: %+v", len(contacts), contacts)
	}
}

// A backup written before the contacts section existed carries crew names and
// nothing else, and must still rebuild the address book from them.
func TestBackupRestoreRebuildsContactsFromLegacyBackup(t *testing.T) {
	source := NewE2EClient(t)
	registerAndLogin(t, source, uniqueEmail("cnt-legacy-src"), "SecurePass123!", "LegacySrc")
	createFlightWithCrew(t, source, "D-ELEG", []map[string]interface{}{
		{"name": "Hans Müller", "role": "Instructor"},
		{"name": "Erika Koch", "role": "Passenger"},
	})

	r := source.GET("/exports/json")
	requireStatus(t, r, 200)
	var backup map[string]interface{}
	r.JSON(&backup)

	// Age the backup: drop the sections a pre-contacts export never had.
	delete(backup, "contacts")
	delete(backup, "customCurrencyRules")
	delete(backup, "notificationPreferences")
	delete(backup, "flightBaseline")

	dest := NewE2EClient(t)
	registerAndLogin(t, dest, uniqueEmail("cnt-legacy-dst"), "SecurePass123!", "LegacyDst")

	r = dest.POST("/imports/json", backup)
	requireStatus(t, r, 200)
	var summary struct {
		FlightsImported  int `json:"flightsImported"`
		ContactsImported int `json:"contactsImported"`
		ContactsCreated  int `json:"contactsCreated"`
	}
	r.JSON(&summary)
	if summary.FlightsImported != 1 {
		t.Fatalf("flightsImported = %d, want 1", summary.FlightsImported)
	}
	if summary.ContactsImported != 0 {
		t.Errorf("contactsImported = %d, want 0 — a legacy backup carries no contacts section", summary.ContactsImported)
	}
	if summary.ContactsCreated != 2 {
		t.Errorf("contactsCreated = %d, want 2 — crew names must rebuild the address book", summary.ContactsCreated)
	}

	if contacts := listContacts(t, dest); len(contacts) != 2 {
		t.Errorf("restored account has %d contacts, want 2: %+v", len(contacts), contacts)
	}
}
