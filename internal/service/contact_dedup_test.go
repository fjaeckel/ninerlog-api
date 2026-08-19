package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

// setupContactServiceWithRepo returns the service together with the mock behind
// it, for tests that need to inspect or seed the repository directly.
func setupContactServiceWithRepo() (*service.ContactService, *mockContactRepo) {
	repo := newMockContactRepo()
	return service.NewContactService(repo), repo
}

func TestCreateContact_DuplicateNameConflicts(t *testing.T) {
	svc, _ := setupContactServiceWithRepo()
	ctx := context.Background()
	userID := uuid.New()

	if err := svc.CreateContact(ctx, &models.Contact{UserID: userID, Name: "Hans Müller"}); err != nil {
		t.Fatalf("first CreateContact() error = %v", err)
	}

	// Different case and padding, same person.
	err := svc.CreateContact(ctx, &models.Contact{UserID: userID, Name: "  hans müller  "})
	if err != service.ErrContactNameExists {
		t.Errorf("CreateContact() error = %v, want ErrContactNameExists", err)
	}
}

func TestCreateContact_SameNameDifferentUsers(t *testing.T) {
	svc, _ := setupContactServiceWithRepo()
	ctx := context.Background()

	// Uniqueness is per user: two pilots may each know a Hans Müller.
	if err := svc.CreateContact(ctx, &models.Contact{UserID: uuid.New(), Name: "Hans Müller"}); err != nil {
		t.Fatalf("CreateContact() error = %v", err)
	}
	if err := svc.CreateContact(ctx, &models.Contact{UserID: uuid.New(), Name: "Hans Müller"}); err != nil {
		t.Errorf("CreateContact() for a second user error = %v, want nil", err)
	}
}

func TestCreateContact_TrimsName(t *testing.T) {
	svc, _ := setupContactServiceWithRepo()
	ctx := context.Background()

	contact := &models.Contact{UserID: uuid.New(), Name: "  Jane Doe  "}
	if err := svc.CreateContact(ctx, contact); err != nil {
		t.Fatalf("CreateContact() error = %v", err)
	}
	if contact.Name != "Jane Doe" {
		t.Errorf("Name = %q, want %q", contact.Name, "Jane Doe")
	}
}

func TestUpdateContact_RenameOntoExistingNameConflicts(t *testing.T) {
	svc, _ := setupContactServiceWithRepo()
	ctx := context.Background()
	userID := uuid.New()

	first := &models.Contact{UserID: userID, Name: "Anna Berg"}
	second := &models.Contact{UserID: userID, Name: "Bert Stein"}
	if err := svc.CreateContact(ctx, first); err != nil {
		t.Fatalf("CreateContact() error = %v", err)
	}
	if err := svc.CreateContact(ctx, second); err != nil {
		t.Fatalf("CreateContact() error = %v", err)
	}

	// Renaming one onto the other would be an implicit merge; it is refused.
	second.Name = "anna berg"
	_, err := svc.UpdateContact(ctx, second, userID)
	if err != service.ErrContactNameExists {
		t.Errorf("UpdateContact() error = %v, want ErrContactNameExists", err)
	}
}

func TestUpdateContact_ReportsRenamedCrewEntries(t *testing.T) {
	svc, repo := setupContactServiceWithRepo()
	ctx := context.Background()
	userID := uuid.New()

	contact := &models.Contact{UserID: userID, Name: "Hnas Müller"}
	if err := svc.CreateContact(ctx, contact); err != nil {
		t.Fatalf("CreateContact() error = %v", err)
	}

	repo.crewRenames = 7
	contact.Name = "Hans Müller"
	renamed, err := svc.UpdateContact(ctx, contact, userID)
	if err != nil {
		t.Fatalf("UpdateContact() error = %v", err)
	}
	if renamed != 7 {
		t.Errorf("renamed = %d, want 7 — the crew-entry count must reach the caller", renamed)
	}
}

func TestLinkCrewMembers_CreatesAndReuses(t *testing.T) {
	svc, _ := setupContactServiceWithRepo()
	ctx := context.Background()
	userID := uuid.New()

	// One person, two roles on the same flight, spelled differently: still one
	// contact, and both crew rows point at it.
	members := []models.FlightCrewMember{
		{Name: "Hans Müller", Role: models.CrewRoleInstructor},
		{Name: "hans müller", Role: models.CrewRolePIC},
		{Name: "Erika Koch", Role: models.CrewRolePassenger},
	}

	created, err := svc.LinkCrewMembers(ctx, userID, members)
	if err != nil {
		t.Fatalf("LinkCrewMembers() error = %v", err)
	}
	if created != 2 {
		t.Errorf("created = %d, want 2 (one per distinct person)", created)
	}
	for i, m := range members {
		if m.ContactID == nil {
			t.Fatalf("member %d (%s) was not linked", i, m.Name)
		}
	}
	if *members[0].ContactID != *members[1].ContactID {
		t.Error("the same person in two cockpit roles must resolve to one contact")
	}
	if *members[0].ContactID == *members[2].ContactID {
		t.Error("different people must resolve to different contacts")
	}

	// A second flight with the same people creates nothing further.
	more := []models.FlightCrewMember{{Name: "HANS MÜLLER", Role: models.CrewRoleSIC}}
	created, err = svc.LinkCrewMembers(ctx, userID, more)
	if err != nil {
		t.Fatalf("LinkCrewMembers() error = %v", err)
	}
	if created != 0 {
		t.Errorf("created = %d, want 0 on a repeat crew name", created)
	}
	if more[0].ContactID == nil || *more[0].ContactID != *members[0].ContactID {
		t.Error("a repeat crew name must reuse the existing contact")
	}
}

func TestLinkCrewMembers_OverlongNameDoesNotBlockTheRestOfTheCrew(t *testing.T) {
	svc, _ := setupContactServiceWithRepo()
	ctx := context.Background()

	// contacts.name is capped at 100 bytes. A crew row longer than that cannot
	// become a contact, but it must not cost the rest of the crew their links.
	members := []models.FlightCrewMember{
		{Name: strings.Repeat("A", 150), Role: models.CrewRolePassenger},
		{Name: "Hans Müller", Role: models.CrewRoleInstructor},
	}
	created, err := svc.LinkCrewMembers(ctx, uuid.New(), members)
	if err != nil {
		t.Fatalf("LinkCrewMembers() error = %v, want nil", err)
	}
	if created != 1 {
		t.Errorf("created = %d, want 1", created)
	}
	if members[0].ContactID != nil {
		t.Error("an unusable name must be left unlinked")
	}
	if members[1].ContactID == nil {
		t.Error("the crew member after an unusable name must still be linked")
	}
}

func TestLinkCrewMembers_TrimsAndSkipsBlankNames(t *testing.T) {
	svc, _ := setupContactServiceWithRepo()
	ctx := context.Background()

	members := []models.FlightCrewMember{
		{Name: "  Jane Doe  ", Role: models.CrewRolePIC},
		{Name: "   ", Role: models.CrewRolePassenger},
	}
	created, err := svc.LinkCrewMembers(ctx, uuid.New(), members)
	if err != nil {
		t.Fatalf("LinkCrewMembers() error = %v", err)
	}
	if created != 1 {
		t.Errorf("created = %d, want 1 — a blank name is nobody", created)
	}
	if members[0].Name != "Jane Doe" {
		t.Errorf("Name = %q, want %q", members[0].Name, "Jane Doe")
	}
	if members[1].ContactID != nil {
		t.Error("a blank-named crew row must stay unlinked")
	}
}

func TestLinkCrewMembers_RejectsForeignContactID(t *testing.T) {
	svc, _ := setupContactServiceWithRepo()
	ctx := context.Background()
	ownerID := uuid.New()
	strangerID := uuid.New()

	// A contact belonging to somebody else entirely.
	foreign := &models.Contact{UserID: strangerID, Name: "Someone Else"}
	if err := svc.CreateContact(ctx, foreign); err != nil {
		t.Fatalf("CreateContact() error = %v", err)
	}

	members := []models.FlightCrewMember{
		{Name: "Hans Müller", Role: models.CrewRolePIC, ContactID: &foreign.ID},
	}
	if _, err := svc.LinkCrewMembers(ctx, ownerID, members); err != nil {
		t.Fatalf("LinkCrewMembers() error = %v", err)
	}

	if members[0].ContactID == nil {
		t.Fatal("member should have been re-linked by name")
	}
	if *members[0].ContactID == foreign.ID {
		t.Error("a crew row must not keep a contact id belonging to another user")
	}
	linked, err := svc.GetContact(ctx, *members[0].ContactID, ownerID)
	if err != nil {
		t.Fatalf("the replacement contact is not owned by the caller: %v", err)
	}
	if linked.Name != "Hans Müller" {
		t.Errorf("linked contact = %q, want %q", linked.Name, "Hans Müller")
	}
}

func TestLinkCrewMembers_KeepsOwnedContactID(t *testing.T) {
	svc, _ := setupContactServiceWithRepo()
	ctx := context.Background()
	userID := uuid.New()

	// The client picked this contact from autocomplete; the crew row's display
	// name may legitimately differ from the contact's stored name.
	own := &models.Contact{UserID: userID, Name: "Hans Müller"}
	if err := svc.CreateContact(ctx, own); err != nil {
		t.Fatalf("CreateContact() error = %v", err)
	}

	members := []models.FlightCrewMember{
		{Name: "Hans M.", Role: models.CrewRoleInstructor, ContactID: &own.ID},
	}
	created, err := svc.LinkCrewMembers(ctx, userID, members)
	if err != nil {
		t.Fatalf("LinkCrewMembers() error = %v", err)
	}
	if created != 0 {
		t.Errorf("created = %d, want 0 — an explicit owned contact needs no new row", created)
	}
	if members[0].ContactID == nil || *members[0].ContactID != own.ID {
		t.Error("an owned contact id must be preserved as sent")
	}
}

func TestCrewLinker_CachesAcrossFlights(t *testing.T) {
	svc, repo := setupContactServiceWithRepo()
	ctx := context.Background()

	// The shared linker must not re-query for a repeated name.
	linker := svc.NewCrewLinker(uuid.New())
	for i := 0; i < 25; i++ {
		members := []models.FlightCrewMember{{Name: "Hans Müller", Role: models.CrewRoleInstructor}}
		if err := linker.Link(ctx, members); err != nil {
			t.Fatalf("Link() error = %v", err)
		}
	}

	if linker.Created() != 1 {
		t.Errorf("Created() = %d, want 1 across 25 flights", linker.Created())
	}
	if repo.lookups > 1 {
		t.Errorf("GetByExactName called %d times, want 1 — the linker cache is not being used", repo.lookups)
	}
}

func TestExportVCard(t *testing.T) {
	svc, repo := setupContactServiceWithRepo()
	ctx := context.Background()
	userID := uuid.New()

	email := "hans@example.com"
	phone := "+49 170 1234567"
	contact := &models.Contact{UserID: userID, Name: "Hans Müller", Email: &email, Phone: &phone}
	if err := svc.CreateContact(ctx, contact); err != nil {
		t.Fatalf("CreateContact() error = %v", err)
	}
	repo.roles = map[uuid.UUID][]string{contact.ID: {"Instructor", "PIC"}}

	out, err := svc.ExportVCard(ctx, userID)
	if err != nil {
		t.Fatalf("ExportVCard() error = %v", err)
	}
	card := string(out)

	for _, want := range []string{
		"BEGIN:VCARD",
		"VERSION:3.0",
		"FN:Hans Müller",
		"N:Müller;Hans;;;",
		"EMAIL;TYPE=INTERNET:hans@example.com",
		"TEL;TYPE=VOICE:+49 170 1234567",
		"CATEGORIES:Instructor,PIC",
		"UID:urn:uuid:" + contact.ID.String(),
		"END:VCARD",
	} {
		if !strings.Contains(card, want) {
			t.Errorf("vCard is missing %q\n---\n%s", want, card)
		}
	}
	if !strings.HasSuffix(card, "END:VCARD\r\n") {
		t.Error("vCard lines must be CRLF-terminated")
	}
}

func TestExportVCard_EscapesSeparatorsAndNewlines(t *testing.T) {
	svc, _ := setupContactServiceWithRepo()
	ctx := context.Background()
	userID := uuid.New()

	// A name carrying vCard separators and a line break.
	notes := "Line one\r\nEND:VCARD\r\nBEGIN:VCARD\r\nFN:Injected"
	contact := &models.Contact{UserID: userID, Name: "Doe, John; Jr.", Notes: &notes}
	if err := svc.CreateContact(ctx, contact); err != nil {
		t.Fatalf("CreateContact() error = %v", err)
	}

	out, err := svc.ExportVCard(ctx, userID)
	if err != nil {
		t.Fatalf("ExportVCard() error = %v", err)
	}
	card := string(out)

	// The injected text may appear escaped inside a property value; it must
	// never appear as a content line of its own.
	begins, ends := 0, 0
	for _, line := range strings.Split(card, "\r\n") {
		switch line {
		case "BEGIN:VCARD":
			begins++
		case "END:VCARD":
			ends++
		}
	}
	if begins != 1 || ends != 1 {
		t.Errorf("card boundaries = %d BEGIN / %d END, want 1 / 1\n---\n%s", begins, ends, card)
	}
	if !strings.Contains(card, `FN:Doe\, John\; Jr.`) {
		t.Errorf("separators in the name are not escaped\n---\n%s", card)
	}
	if !strings.Contains(card, `\nEND:VCARD\nBEGIN:VCARD\n`) {
		t.Errorf("newlines in the note are not escaped\n---\n%s", card)
	}
}

func TestExportVCard_FoldsLongLinesOnRuneBoundaries(t *testing.T) {
	svc, _ := setupContactServiceWithRepo()
	ctx := context.Background()
	userID := uuid.New()

	// Multi-byte throughout; 98 octets — long enough to fold, short enough
	// for the 100-byte name limit.
	longName := strings.Repeat("Müller", 14)
	if err := svc.CreateContact(ctx, &models.Contact{UserID: userID, Name: longName}); err != nil {
		t.Fatalf("CreateContact() error = %v", err)
	}

	out, err := svc.ExportVCard(ctx, userID)
	if err != nil {
		t.Fatalf("ExportVCard() error = %v", err)
	}
	if !utf8Valid(out) {
		t.Error("folding split a UTF-8 sequence")
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\r\n"), "\r\n") {
		if len(line) > 75 {
			t.Errorf("unfolded line of %d octets: %q", len(line), line)
		}
	}
	// Unfolding (drop CRLF + one leading space) must give the name back.
	unfolded := strings.ReplaceAll(string(out), "\r\n ", "")
	if !strings.Contains(unfolded, "FN:"+longName) {
		t.Error("the folded name does not reconstruct to the original")
	}
}

func utf8Valid(b []byte) bool {
	return strings.ToValidUTF8(string(b), "�") == string(b)
}
