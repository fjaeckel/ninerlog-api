package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// ---- fakes ----

type fakeLogbookRepo struct {
	modes     map[uuid.UUID]*models.LogbookMode
	events    []models.LogbookModeEvent
	certs     []models.LogbookCertification
	revisions []models.FlightRevision
	current   map[uuid.UUID][2]json.RawMessage
}

func newFakeLogbookRepo() *fakeLogbookRepo {
	return &fakeLogbookRepo{
		modes:   map[uuid.UUID]*models.LogbookMode{},
		current: map[uuid.UUID][2]json.RawMessage{},
	}
}

func (f *fakeLogbookRepo) mode(userID uuid.UUID) *models.LogbookMode {
	if f.modes[userID] == nil {
		f.modes[userID] = &models.LogbookMode{}
	}
	return f.modes[userID]
}

func (f *fakeLogbookRepo) GetMode(_ context.Context, userID uuid.UUID) (*models.LogbookMode, error) {
	m := *f.mode(userID)
	return &m, nil
}

func (f *fakeLogbookRepo) SetStrict(_ context.Context, userID uuid.UUID, strict bool, trigger string, licenseID *uuid.UUID) (bool, error) {
	m := f.mode(userID)
	if m.Strict == strict {
		return false, nil
	}
	m.Strict = strict
	if strict {
		now := time.Now()
		m.Since = &now
	}
	f.events = append(f.events, models.LogbookModeEvent{
		UserID: userID, Strict: strict, Trigger: trigger, LicenseID: licenseID, CreatedAt: time.Now(),
	})
	return true, nil
}

func (f *fakeLogbookRepo) RestoreStrictSince(_ context.Context, userID uuid.UUID, since time.Time) error {
	f.mode(userID).Since = &since
	return nil
}

func (f *fakeLogbookRepo) ListModeEvents(_ context.Context, userID uuid.UUID) ([]models.LogbookModeEvent, error) {
	out := []models.LogbookModeEvent{}
	for _, e := range f.events {
		if e.UserID == userID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeLogbookRepo) ImportModeEvent(_ context.Context, e *models.LogbookModeEvent) (bool, error) {
	for _, existing := range f.events {
		if existing.UserID == e.UserID && existing.Strict == e.Strict && existing.Trigger == e.Trigger && existing.CreatedAt.Equal(e.CreatedAt) {
			return false, nil
		}
	}
	f.events = append(f.events, *e)
	return true, nil
}

func (f *fakeLogbookRepo) CreateCertification(_ context.Context, c *models.LogbookCertification) error {
	c.ID = uuid.New()
	c.CertifiedAt = time.Now()
	f.certs = append(f.certs, *c)
	return nil
}

func (f *fakeLogbookRepo) ImportCertification(_ context.Context, c *models.LogbookCertification) (bool, error) {
	for _, existing := range f.certs {
		if existing.UserID == c.UserID && existing.CertifiedThrough.Equal(c.CertifiedThrough) && existing.CertifiedAt.Equal(c.CertifiedAt) {
			return false, nil
		}
	}
	f.certs = append(f.certs, *c)
	return true, nil
}

func (f *fakeLogbookRepo) ListCertifications(_ context.Context, userID uuid.UUID) ([]models.LogbookCertification, error) {
	out := []models.LogbookCertification{}
	for _, c := range f.certs {
		if c.UserID == userID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeLogbookRepo) LatestCertifiedThrough(_ context.Context, userID uuid.UUID) (*time.Time, error) {
	var latest *time.Time
	for _, c := range f.certs {
		if c.UserID == userID && (latest == nil || c.CertifiedThrough.After(*latest)) {
			t := c.CertifiedThrough
			latest = &t
		}
	}
	return latest, nil
}

func (f *fakeLogbookRepo) CurrentFlightState(_ context.Context, _, flightID uuid.UUID) (json.RawMessage, json.RawMessage, error) {
	state, ok := f.current[flightID]
	if !ok {
		return nil, nil, repository.ErrNotFound
	}
	return state[0], state[1], nil
}

func (f *fakeLogbookRepo) ListRevisions(_ context.Context, userID, flightID uuid.UUID) ([]models.FlightRevision, error) {
	out := []models.FlightRevision{}
	for _, r := range f.revisions {
		if r.UserID == userID && r.FlightID == flightID {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RevisionNo < out[j].RevisionNo })
	return out, nil
}

func (f *fakeLogbookRepo) ListUserRevisions(_ context.Context, userID uuid.UUID) ([]models.FlightRevision, error) {
	out := []models.FlightRevision{}
	for _, r := range f.revisions {
		if r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeLogbookRepo) ListVoided(_ context.Context, userID uuid.UUID) ([]models.FlightRevision, error) {
	out := []models.FlightRevision{}
	for _, r := range f.revisions {
		if r.UserID == userID && r.Kind == models.RevisionKindDelete {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ChangedAt.After(out[j].ChangedAt) })
	return out, nil
}

func (f *fakeLogbookRepo) ImportRevisions(_ context.Context, revs []models.FlightRevision) (int, error) {
	stored := 0
next:
	for _, rev := range revs {
		for _, existing := range f.revisions {
			if existing.FlightID == rev.FlightID && existing.RevisionNo == rev.RevisionNo {
				continue next
			}
		}
		f.revisions = append(f.revisions, rev)
		stored++
	}
	return stored, nil
}

// recordingFlightRepo records the change context of each write.
type recordingFlightRepo struct {
	*mockFlightRepo
	writes []repository.ChangeContext
	crews  [][]models.FlightCrewMember
}

func (r *recordingFlightRepo) record(ctx context.Context) {
	cc, _ := repository.ChangeContextFrom(ctx)
	r.writes = append(r.writes, cc)
}

func (r *recordingFlightRepo) Update(ctx context.Context, f *models.Flight) error {
	r.record(ctx)
	return r.mockFlightRepo.Update(ctx, f)
}

func (r *recordingFlightRepo) UpdateWithCrew(ctx context.Context, f *models.Flight, crew []models.FlightCrewMember) error {
	r.record(ctx)
	r.crews = append(r.crews, crew)
	return r.mockFlightRepo.Update(ctx, f)
}

func (r *recordingFlightRepo) Delete(ctx context.Context, id uuid.UUID) error {
	r.record(ctx)
	return r.mockFlightRepo.Delete(ctx, id)
}

type fakeCrewRepo struct {
	crew map[uuid.UUID][]models.FlightCrewMember
}

func (f *fakeCrewRepo) SetCrewMembers(_ context.Context, id uuid.UUID, m []models.FlightCrewMember) error {
	f.crew[id] = m
	return nil
}
func (f *fakeCrewRepo) GetByFlightID(_ context.Context, id uuid.UUID) ([]models.FlightCrewMember, error) {
	return f.crew[id], nil
}
func (f *fakeCrewRepo) GetByFlightIDs(_ context.Context, ids []uuid.UUID) (map[uuid.UUID][]models.FlightCrewMember, error) {
	out := map[uuid.UUID][]models.FlightCrewMember{}
	for _, id := range ids {
		out[id] = f.crew[id]
	}
	return out, nil
}
func (f *fakeCrewRepo) DeleteByFlightID(_ context.Context, id uuid.UUID) error {
	delete(f.crew, id)
	return nil
}

// ---- environment ----

var testNow = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

type logbookEnv struct {
	userID   uuid.UUID
	logbook  *LogbookService
	lbRepo   *fakeLogbookRepo
	flights  *recordingFlightRepo
	licenses *mockLicenseRepo
	sigs     *mockFlightSignatureRepo
	crew     *fakeCrewRepo
}

func newLogbookEnv(t *testing.T, strict bool, licenceCountries ...string) *logbookEnv {
	t.Helper()
	e := &logbookEnv{
		userID:   uuid.New(),
		lbRepo:   newFakeLogbookRepo(),
		flights:  &recordingFlightRepo{mockFlightRepo: newMockFlightRepo()},
		licenses: newMockLicenseRepo(),
		sigs:     newMockFlightSignatureRepo(),
		crew:     &fakeCrewRepo{crew: map[uuid.UUID][]models.FlightCrewMember{}},
	}
	e.logbook = NewLogbookService(e.lbRepo, e.licenses, e.flights, e.sigs)
	e.logbook.now = func() time.Time { return testNow }
	e.logbook.departureLon = func(string) (float64, bool) { return 0, false }
	e.lbRepo.mode(e.userID).Strict = strict
	for _, country := range licenceCountries {
		c := country
		lic := &models.License{UserID: e.userID, RegulatoryAuthority: "EASA", LicenseType: "PPL", IssuingCountry: &c}
		if err := e.licenses.Create(context.Background(), lic); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

// ---- tests ----

func TestLogbookService_SetStrictStoresEventsOnlyOnChange(t *testing.T) {
	e := newLogbookEnv(t, false)
	ctx := context.Background()

	for _, step := range []struct {
		strict     bool
		wantEvents int
	}{{true, 1}, {true, 1}, {false, 2}, {false, 2}} {
		view, err := e.logbook.SetStrict(ctx, e.userID, step.strict)
		if err != nil {
			t.Fatal(err)
		}
		if view.Strict != step.strict || len(view.Events) != step.wantEvents {
			t.Fatalf("after strict=%v: strict=%v events=%d", step.strict, view.Strict, len(view.Events))
		}
	}
	if e.lbRepo.events[0].Trigger != models.LogbookTriggerManual {
		t.Errorf("trigger = %q", e.lbRepo.events[0].Trigger)
	}
}

func TestLogbookService_SyncFromLicences(t *testing.T) {
	tests := []struct {
		name        string
		strict      bool
		countries   []string
		wantStrict  bool
		wantEvents  int
		wantLicence bool
	}{
		{"required licence switches strict on", false, []string{"DE"}, true, 1, true},
		{"suggested licence does not", false, []string{"US"}, false, 0, false},
		{"licence without country does not", false, []string{""}, false, 0, false},
		{"already strict stores nothing", true, []string{"CH"}, true, 0, false},
		{"no licence at all never switches off", true, nil, true, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newLogbookEnv(t, tt.strict)
			var saved uuid.UUID
			for _, c := range tt.countries {
				lic := &models.License{UserID: e.userID, RegulatoryAuthority: "EASA", LicenseType: "PPL"}
				if c != "" {
					code := c
					lic.IssuingCountry = &code
				}
				if err := e.licenses.Create(context.Background(), lic); err != nil {
					t.Fatal(err)
				}
				saved = lic.ID
			}

			if err := e.logbook.SyncFromLicences(context.Background(), e.userID, saved); err != nil {
				t.Fatal(err)
			}
			mode, _ := e.lbRepo.GetMode(context.Background(), e.userID)
			if mode.Strict != tt.wantStrict || len(e.lbRepo.events) != tt.wantEvents {
				t.Fatalf("strict=%v events=%d", mode.Strict, len(e.lbRepo.events))
			}
			if tt.wantLicence {
				ev := e.lbRepo.events[0]
				if ev.Trigger != models.LogbookTriggerLicence || ev.LicenseID == nil || *ev.LicenseID != saved {
					t.Errorf("event = %+v", ev)
				}
			}
		})
	}
}

func TestLogbookService_GetModeReportsLicencesAndWindow(t *testing.T) {
	e := newLogbookEnv(t, true, "CH", "US")
	view, err := e.logbook.GetMode(context.Background(), e.userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Required) != 1 || len(view.Suggested) != 1 {
		t.Fatalf("required=%d suggested=%d", len(view.Required), len(view.Suggested))
	}
	if view.DraftWindow != "hours_48" || !view.OverridesDownwardOnly {
		t.Errorf("window=%q downward=%v", view.DraftWindow, view.OverridesDownwardOnly)
	}

	manual := newLogbookEnv(t, true)
	view, _ = manual.logbook.GetMode(context.Background(), manual.userID)
	if view.DraftWindow != "flight_day" {
		t.Errorf("manual strict window = %q, want flight_day", view.DraftWindow)
	}

	off := newLogbookEnv(t, false, "CH")
	view, _ = off.logbook.GetMode(context.Background(), off.userID)
	if view.DraftWindow != "" || view.OverridesDownwardOnly || view.Since != nil {
		t.Errorf("non-strict view = %+v", view)
	}
}

func TestLogbookService_CreateCertification(t *testing.T) {
	today := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		strict  bool
		latest  *time.Time
		through time.Time
		wantErr error
	}{
		{"strict mode off", false, nil, today, ErrStrictModeRequired},
		{"future date", true, nil, today.AddDate(0, 0, 1), ErrCertificationInFuture},
		{"before the latest", true, &today, today.AddDate(0, 0, -1), ErrCertificationBeforeLatest},
		{"today", true, nil, today, nil},
		{"same as the latest", true, &today, today, nil},
		{"after the latest", true, ptrTime(today.AddDate(0, 0, -10)), today.AddDate(0, 0, -2), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newLogbookEnv(t, tt.strict)
			if tt.latest != nil {
				e.lbRepo.certs = append(e.lbRepo.certs, models.LogbookCertification{UserID: e.userID, CertifiedThrough: *tt.latest})
			}
			cert, err := e.logbook.CreateCertification(context.Background(), e.userID, tt.through)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && (cert == nil || !cert.CertifiedThrough.Equal(dateOnly(tt.through))) {
				t.Errorf("certification = %+v", cert)
			}
		})
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestLogbookService_ListRevisionsDiffsAgainstNextState(t *testing.T) {
	e := newLogbookEnv(t, true)
	flightID := uuid.New()
	snap := func(total int, remarks any) json.RawMessage {
		b, _ := json.Marshal(map[string]any{
			"id": flightID, "user_id": e.userID, "total_time": total, "remarks": remarks,
			"updated_at": "2026-01-01T00:00:00Z", "all_landings": 1, "is_ipc": false,
		})
		return b
	}
	crew := func(names ...string) json.RawMessage {
		list := []map[string]any{}
		for _, n := range names {
			list = append(list, map[string]any{"name": n, "role": "Instructor", "contactId": nil})
		}
		b, _ := json.Marshal(list)
		return b
	}
	e.lbRepo.revisions = []models.FlightRevision{
		{ID: uuid.New(), UserID: e.userID, FlightID: flightID, RevisionNo: 1, Kind: "update", Snapshot: snap(60, nil), CrewSnapshot: crew("Pat"), Source: "user_edit"},
		{ID: uuid.New(), UserID: e.userID, FlightID: flightID, RevisionNo: 2, Kind: "update", Snapshot: snap(75, nil), CrewSnapshot: crew("Pat"), Source: "recalc"},
	}
	e.lbRepo.current[flightID] = [2]json.RawMessage{snap(75, "late note"), crew("Sam")}

	views, err := e.logbook.ListRevisions(context.Background(), e.userID, flightID)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 2 || views[0].RevisionNo != 2 || views[1].RevisionNo != 1 {
		t.Fatalf("order = %+v", views)
	}

	newest := views[0].Changes
	if len(newest) != 2 || newest[0].Field != "crewMembers" && newest[1].Field != "crewMembers" {
		t.Fatalf("newest changes = %+v", newest)
	}
	fields := map[string]FieldChange{}
	for _, c := range newest {
		fields[c.Field] = c
	}
	if c, ok := fields["remarks"]; !ok || c.OldValue != nil || c.NewValue != "late note" {
		t.Errorf("remarks change = %+v", c)
	}
	if c, ok := fields["crewMembers"]; !ok {
		t.Errorf("crew change missing")
	} else if old := c.OldValue.([]CrewEntry); len(old) != 1 || old[0].Name != "Pat" {
		t.Errorf("crew old = %+v", c.OldValue)
	}

	oldest := views[1].Changes
	if len(oldest) != 1 || oldest[0].Field != "totalTime" || oldest[0].OldValue != json.Number("60") || oldest[0].NewValue != json.Number("75") {
		t.Errorf("oldest changes = %+v", oldest)
	}
}

func TestLogbookService_ListRevisionsOwnershipAndDeleted(t *testing.T) {
	e := newLogbookEnv(t, true)
	flightID := uuid.New()
	snap, _ := json.Marshal(map[string]any{"id": flightID, "total_time": 60})

	if _, err := e.logbook.ListRevisions(context.Background(), e.userID, flightID); !errors.Is(err, ErrFlightNotFound) {
		t.Fatalf("unknown flight: error = %v", err)
	}

	e.lbRepo.revisions = []models.FlightRevision{
		{ID: uuid.New(), UserID: e.userID, FlightID: flightID, RevisionNo: 1, Kind: "delete", Snapshot: snap, CrewSnapshot: json.RawMessage("[]"), Source: "user_edit"},
	}
	views, err := e.logbook.ListRevisions(context.Background(), e.userID, flightID)
	if err != nil || len(views) != 1 || len(views[0].Changes) != 0 {
		t.Fatalf("deleted flight: views=%+v err=%v", views, err)
	}

	if _, err := e.logbook.ListRevisions(context.Background(), uuid.New(), flightID); !errors.Is(err, ErrFlightNotFound) {
		t.Fatalf("other user: error = %v", err)
	}
}

func TestLogbookService_ListVoided(t *testing.T) {
	e := newLogbookEnv(t, true)
	snap := func(reg string) json.RawMessage {
		b, _ := json.Marshal(map[string]any{
			"date": "2026-03-01", "aircraft_reg": reg, "aircraft_type": "C172",
			"departure_icao": "EDDF", "arrival_icao": nil, "total_time": 80, "remarks": "r",
		})
		return b
	}
	reason := "duplicate"
	e.lbRepo.revisions = []models.FlightRevision{
		{ID: uuid.New(), UserID: e.userID, FlightID: uuid.New(), RevisionNo: 1, Kind: "delete", Snapshot: snap("D-OLD"), Source: "user_edit", Reason: &reason, ChangedAt: testNow.Add(-time.Hour)},
		{ID: uuid.New(), UserID: e.userID, FlightID: uuid.New(), RevisionNo: 1, Kind: "delete", Snapshot: snap("D-NEW"), Source: "user_edit", ChangedAt: testNow},
		{ID: uuid.New(), UserID: e.userID, FlightID: uuid.New(), RevisionNo: 1, Kind: "update", Snapshot: snap("D-UPD"), Source: "user_edit"},
		{ID: uuid.New(), UserID: uuid.New(), FlightID: uuid.New(), RevisionNo: 1, Kind: "delete", Snapshot: snap("D-OTHER"), Source: "user_edit"},
	}
	voided, err := e.logbook.ListVoided(context.Background(), e.userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(voided) != 2 || voided[0].AircraftReg != "D-NEW" || voided[1].AircraftReg != "D-OLD" {
		t.Fatalf("voided = %+v", voided)
	}
	v := voided[1]
	if v.Reason == nil || *v.Reason != "duplicate" || v.TotalTime != 80 || v.DepartureICAO == nil || v.ArrivalICAO != nil ||
		v.Date.Format("2006-01-02") != "2026-03-01" {
		t.Errorf("void record = %+v", v)
	}
}

func TestCamelField(t *testing.T) {
	tests := map[string]string{
		"total_time":        "totalTime",
		"departure_icao":    "departureIcao",
		"all_landings":      "allLandings",
		"is_ipc":            "isIpc",
		"picus_time":        "picusTime",
		"date":              "date",
		"fstd_type":         "fstdType",
		"sic_time_override": "sicTimeOverride",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			if got := CamelField(in); got != want {
				t.Errorf("CamelField(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

func TestLogbookService_ImportLogbook(t *testing.T) {
	ctx := context.Background()
	oldFlight, deletedFlight, restoredFlight := uuid.New(), uuid.New(), uuid.New()
	snap := func(id uuid.UUID) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"id": id, "user_id": uuid.New(), "total_time": 60})
		return b
	}
	crew := json.RawMessage(`[{"name":"Pat","role":"Instructor","contactId":"` + uuid.NewString() + `"}]`)
	licenseOld, licenseNew := uuid.New(), uuid.New()
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	build := func() LogbookImport {
		return LogbookImport{
			Strict: true,
			Since:  &since,
			Events: []models.LogbookModeEvent{
				{Strict: true, Trigger: "licence", LicenseID: &licenseOld, CreatedAt: since},
			},
			Revisions: []models.FlightRevision{
				{FlightID: oldFlight, RevisionNo: 1, Kind: "update", Snapshot: snap(oldFlight), CrewSnapshot: crew, Source: "user_edit", ChangedAt: since},
				{FlightID: oldFlight, RevisionNo: 2, Kind: "update", Snapshot: snap(oldFlight), CrewSnapshot: crew, Source: "weird", ChangedAt: since},
				{FlightID: deletedFlight, RevisionNo: 1, Kind: "update", Snapshot: snap(deletedFlight), CrewSnapshot: crew, Source: "import", ChangedAt: since},
				{FlightID: deletedFlight, RevisionNo: 2, Kind: "delete", Snapshot: snap(deletedFlight), CrewSnapshot: crew, Source: "user_edit", ChangedAt: since},
			},
			Certifications: []models.LogbookCertification{{CertifiedThrough: since, CertifiedAt: since}},
			FlightIDs:      map[uuid.UUID]uuid.UUID{oldFlight: restoredFlight},
			LicenseIDs:     map[uuid.UUID]uuid.UUID{licenseOld: licenseNew},
		}
	}

	t.Run("remaps ids, switches strict on with an import event", func(t *testing.T) {
		e := newLogbookEnv(t, false)
		res, err := ImportLogbook(ctx, e.lbRepo, e.userID, build())
		if err != nil {
			t.Fatal(err)
		}
		if res.Revisions != 4 || res.Certifications != 1 {
			t.Fatalf("result = %+v", res)
		}

		mode, _ := e.lbRepo.GetMode(ctx, e.userID)
		if !mode.Strict || mode.Since == nil || !mode.Since.Equal(since) {
			t.Errorf("mode = %+v", mode)
		}
		if len(e.lbRepo.events) != 2 || e.lbRepo.events[0].Trigger != "licence" || e.lbRepo.events[1].Trigger != "import" {
			t.Fatalf("events = %+v", e.lbRepo.events)
		}
		if lid := e.lbRepo.events[0].LicenseID; lid == nil || *lid != licenseNew {
			t.Errorf("restored event licence = %v", lid)
		}

		byFlight := map[uuid.UUID][]models.FlightRevision{}
		for _, r := range e.lbRepo.revisions {
			byFlight[r.FlightID] = append(byFlight[r.FlightID], r)
		}
		if len(byFlight[restoredFlight]) != 2 {
			t.Fatalf("restored flight revisions = %d", len(byFlight[restoredFlight]))
		}
		if len(byFlight) != 2 {
			t.Fatalf("distinct flight ids = %d, want 2 (restored + one fresh id for the deleted flight)", len(byFlight))
		}
		for id, revs := range byFlight {
			for _, r := range revs {
				var obj map[string]any
				if err := json.Unmarshal(r.Snapshot, &obj); err != nil {
					t.Fatal(err)
				}
				if obj["id"] != id.String() || obj["user_id"] != e.userID.String() {
					t.Errorf("snapshot ids = %v / %v", obj["id"], obj["user_id"])
				}
				if r.UserID != e.userID {
					t.Errorf("revision user = %s", r.UserID)
				}
				if string(r.CrewSnapshot) == string(crew) {
					t.Error("crew snapshot kept stale contact ids")
				}
			}
		}
		if byFlight[restoredFlight][1].Source != "unknown" {
			t.Errorf("unrecognised source stored as %q", byFlight[restoredFlight][1].Source)
		}
		if byFlight[restoredFlight][0].RevisionNo != 1 || byFlight[restoredFlight][1].RevisionNo != 2 {
			t.Error("revision numbers not kept")
		}
	})

	t.Run("an already strict account only gains the history", func(t *testing.T) {
		e := newLogbookEnv(t, true)
		if _, err := ImportLogbook(ctx, e.lbRepo, e.userID, build()); err != nil {
			t.Fatal(err)
		}
		for _, ev := range e.lbRepo.events {
			if ev.Trigger == "import" {
				t.Errorf("unexpected import event %+v", ev)
			}
		}
	})

	t.Run("a non-strict backup leaves the account non-strict", func(t *testing.T) {
		e := newLogbookEnv(t, false)
		in := build()
		in.Strict = false
		if _, err := ImportLogbook(ctx, e.lbRepo, e.userID, in); err != nil {
			t.Fatal(err)
		}
		if mode, _ := e.lbRepo.GetMode(ctx, e.userID); mode.Strict {
			t.Error("account switched to strict")
		}
	})

	t.Run("importing twice stores nothing new", func(t *testing.T) {
		e := newLogbookEnv(t, false)
		if _, err := ImportLogbook(ctx, e.lbRepo, e.userID, build()); err != nil {
			t.Fatal(err)
		}
		in := build()
		in.FlightIDs = map[uuid.UUID]uuid.UUID{oldFlight: restoredFlight}
		res, err := ImportLogbook(ctx, e.lbRepo, e.userID, in)
		if err != nil {
			t.Fatal(err)
		}
		if res.Certifications != 0 {
			t.Errorf("certifications = %d", res.Certifications)
		}
	})

	t.Run("rejects values the schema cannot store", func(t *testing.T) {
		tests := map[string]func(*LogbookImport){
			"revision kind":   func(in *LogbookImport) { in.Revisions[0].Kind = "rewrite" },
			"revision number": func(in *LogbookImport) { in.Revisions[0].RevisionNo = 0 },
			"event trigger":   func(in *LogbookImport) { in.Events[0].Trigger = "admin" },
			"snapshot":        func(in *LogbookImport) { in.Revisions[0].Snapshot = json.RawMessage(`[1]`) },
		}
		for name, mutate := range tests {
			t.Run(name, func(t *testing.T) {
				e := newLogbookEnv(t, false)
				in := build()
				mutate(&in)
				if _, err := ImportLogbook(ctx, e.lbRepo, e.userID, in); !errors.Is(err, ErrInvalidLogbookImport) {
					t.Fatalf("error = %v, want ErrInvalidLogbookImport", err)
				}
				if len(e.lbRepo.revisions) != 0 || len(e.lbRepo.events) != 0 {
					t.Error("a rejected import stored rows")
				}
			})
		}
	})
}

func TestFlightService_ChangeContextOnWrites(t *testing.T) {
	ctx := context.Background()
	newService := func(t *testing.T) (*FlightService, *recordingFlightRepo, *models.Flight) {
		t.Helper()
		repo := &recordingFlightRepo{mockFlightRepo: newMockFlightRepo()}
		f := &models.Flight{
			UserID: uuid.New(), Date: testNow, AircraftReg: "D-EABC", AircraftType: "C172",
			TotalTime: 60, PICTime: 60, IsPIC: true,
		}
		if err := repo.Create(ctx, f); err != nil {
			t.Fatal(err)
		}
		return NewFlightService(repo, nil), repo, f
	}
	edit := func(f *models.Flight) *models.Flight {
		cp := *f
		cp.Remarks = new(string)
		return &cp
	}

	t.Run("a user update is recorded as user_edit", func(t *testing.T) {
		svc, repo, f := newService(t)
		if err := svc.UpdateFlight(ctx, edit(f), f.UserID); err != nil {
			t.Fatal(err)
		}
		if len(repo.writes) != 1 || repo.writes[0].Source != repository.ChangeSourceUserEdit {
			t.Fatalf("writes = %+v", repo.writes)
		}
	})

	t.Run("the source of a system update is passed through", func(t *testing.T) {
		svc, repo, f := newService(t)
		opts := FlightWriteOptions{Source: repository.ChangeSourceRecalc}
		if err := svc.UpdateFlightWith(ctx, edit(f), f.UserID, opts); err != nil {
			t.Fatal(err)
		}
		if repo.writes[0].Source != repository.ChangeSourceRecalc || repo.writes[0].IsCorrection {
			t.Fatalf("context = %+v", repo.writes[0])
		}
	})

	t.Run("a crew replacement goes through the single-transaction write", func(t *testing.T) {
		svc, repo, f := newService(t)
		crew := []models.FlightCrewMember{{Name: "Pat", Role: models.CrewRole("Instructor")}}
		if err := svc.UpdateFlightWith(ctx, edit(f), f.UserID, FlightWriteOptions{Crew: &crew}); err != nil {
			t.Fatal(err)
		}
		if len(repo.crews) != 1 || len(repo.crews[0]) != 1 {
			t.Fatalf("crew writes = %+v", repo.crews)
		}
	})

	t.Run("a delete carries its trimmed reason", func(t *testing.T) {
		svc, repo, f := newService(t)
		if err := svc.DeleteFlightWithReason(ctx, f.ID, f.UserID, "  entered twice "); err != nil {
			t.Fatal(err)
		}
		cc := repo.writes[0]
		if cc.Source != repository.ChangeSourceUserEdit || cc.Reason == nil || *cc.Reason != "entered twice" {
			t.Fatalf("context = %+v", cc)
		}
	})

	t.Run("a delete without a reason carries none", func(t *testing.T) {
		svc, repo, f := newService(t)
		if err := svc.DeleteFlight(ctx, f.ID, f.UserID); err != nil {
			t.Fatal(err)
		}
		if repo.writes[0].Reason != nil {
			t.Fatalf("reason = %v", *repo.writes[0].Reason)
		}
	})

	t.Run("only the owner may update or delete", func(t *testing.T) {
		svc, repo, f := newService(t)
		other := uuid.New()
		if err := svc.UpdateFlight(ctx, edit(f), other); !errors.Is(err, ErrUnauthorizedFlight) {
			t.Errorf("update error = %v", err)
		}
		if err := svc.DeleteFlight(ctx, f.ID, other); !errors.Is(err, ErrUnauthorizedFlight) {
			t.Errorf("delete error = %v", err)
		}
		if len(repo.writes) != 0 {
			t.Errorf("writes = %d", len(repo.writes))
		}
	})
}
