package customreport

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

type fakeRepo struct {
	reports  map[uuid.UUID]*models.CustomReport
	groups   []repository.CustomReportGroup
	lastOpts *repository.FlightQueryOptions
	lastBy   string
}

func newFakeRepo() *fakeRepo { return &fakeRepo{reports: map[uuid.UUID]*models.CustomReport{}} }

func (f *fakeRepo) Create(_ context.Context, r *models.CustomReport) error {
	r.ID = uuid.New()
	r.Position = len(f.reports)
	cp := *r
	f.reports[r.ID] = &cp
	return nil
}

func (f *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*models.CustomReport, error) {
	r, ok := f.reports[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	cp := *r
	return &cp, nil
}

func (f *fakeRepo) ListByUserID(_ context.Context, userID uuid.UUID) ([]*models.CustomReport, error) {
	out := []*models.CustomReport{}
	for pos := 0; pos < len(f.reports)+1; pos++ {
		for _, r := range f.reports {
			if r.UserID == userID && r.Position == pos {
				cp := *r
				out = append(out, &cp)
			}
		}
	}
	return out, nil
}

func (f *fakeRepo) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	l, _ := f.ListByUserID(ctx, userID)
	return len(l), nil
}

func (f *fakeRepo) Update(_ context.Context, r *models.CustomReport) error {
	if _, ok := f.reports[r.ID]; !ok {
		return repository.ErrNotFound
	}
	cp := *r
	f.reports[r.ID] = &cp
	return nil
}

func (f *fakeRepo) Delete(_ context.Context, id uuid.UUID) error {
	delete(f.reports, id)
	return nil
}

func (f *fakeRepo) SetPositions(_ context.Context, _ uuid.UUID, ids []uuid.UUID) error {
	for i, id := range ids {
		f.reports[id].Position = i
	}
	return nil
}

func (f *fakeRepo) Aggregate(_ context.Context, _ uuid.UUID, opts *repository.FlightQueryOptions, groupBy string) ([]repository.CustomReportGroup, error) {
	f.lastOpts, f.lastBy = opts, groupBy
	return f.groups, nil
}

type fakeScope struct {
	err     error
	applied *uuid.UUID
}

func (s *fakeScope) Apply(_ context.Context, _ uuid.UUID, licenseID uuid.UUID, opts *repository.FlightQueryOptions) error {
	if s.err != nil {
		return s.err
	}
	s.applied = &licenseID
	opts.FilterByRegistrations = true
	opts.AircraftRegistrations = []string{"D-EABC"}
	return nil
}

var fixedNow = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

func newTestService() (*Service, *fakeRepo, *fakeScope) {
	repo, scope := newFakeRepo(), &fakeScope{}
	s := NewService(repo, scope)
	s.now = func() time.Time { return fixedNow }
	return s, repo, scope
}

func strp(s string) *string { return &s }
func intp(i int) *int       { return &i }

func def(groupBy, metric string, w models.CustomReportWindow) models.CustomReportDefinition {
	return models.CustomReportDefinition{GroupBy: groupBy, Metric: metric, Window: w}
}

var all = models.CustomReportWindow{Kind: models.ReportWindowAll}

func group(key string, flights, total int) repository.CustomReportGroup {
	return repository.CustomReportGroup{Key: key, Totals: models.CustomReportTotals{Flights: flights, TotalTime: total}}
}

func keys(rows []models.CustomReportRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Key)
	}
	return out
}

func TestCreateValidatesAndStores(t *testing.T) {
	s, repo, _ := newTestService()
	user := uuid.New()

	_, err := s.Create(context.Background(), user, Input{Name: "  ", Definition: def("month", "flights", all)})
	if !IsValidationError(err) {
		t.Fatalf("blank name: err = %v", err)
	}
	_, err = s.Create(context.Background(), user, Input{Name: "x", Definition: def("month", "fuel", all)})
	if !IsValidationError(err) {
		t.Fatalf("bad metric: err = %v", err)
	}
	d := def("month", "flights", all)
	d.Filter.Q = strp("reg:(")
	_, err = s.Create(context.Background(), user, Input{Name: "x", Definition: d})
	if !IsValidationError(err) || !strings.Contains(err.Error(), "invalid search query") {
		t.Fatalf("bad q: err = %v", err)
	}

	rep, err := s.Create(context.Background(), user, Input{Name: " Night ", Definition: def("month", "nightTime", all)})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Name != "Night" || len(repo.reports) != 1 {
		t.Fatalf("stored %+v", rep)
	}
}

func TestCreateEnforcesQuota(t *testing.T) {
	s, _, _ := newTestService()
	user := uuid.New()
	for i := 0; i < MaxReportsPerUser; i++ {
		if _, err := s.Create(context.Background(), user, Input{Name: "r", Definition: def("year", "flights", all)}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.Create(context.Background(), user, Input{Name: "r", Definition: def("year", "flights", all)})
	if !IsValidationError(err) {
		t.Fatalf("over quota: err = %v", err)
	}
}

func TestLicenceScope(t *testing.T) {
	s, repo, scope := newTestService()
	user, lic := uuid.New(), uuid.New()
	d := def("registration", "flights", all)
	d.Filter.LogbookLicenseID = &lic

	if _, err := s.Evaluate(context.Background(), user, d); err != nil {
		t.Fatal(err)
	}
	if scope.applied == nil || *scope.applied != lic || !repo.lastOpts.FilterByRegistrations {
		t.Fatalf("scope not applied: %+v", repo.lastOpts)
	}

	scope.err = service.ErrUnauthorizedAccess
	if _, err := s.Evaluate(context.Background(), user, d); !IsValidationError(err) {
		t.Fatalf("foreign licence: err = %v", err)
	}
	scope.err = errors.New("db down")
	if _, err := s.Evaluate(context.Background(), user, d); err == nil || IsValidationError(err) {
		t.Fatalf("db error surfaced as %v", err)
	}
}

func TestOwnershipIsNotFound(t *testing.T) {
	s, _, _ := newTestService()
	owner, other := uuid.New(), uuid.New()
	rep, err := s.Create(context.Background(), owner, Input{Name: "mine", Definition: def("year", "flights", all)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), other, rep.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Get by other: %v", err)
	}
	if _, err := s.Update(context.Background(), other, rep.ID, Input{Name: "x", Definition: def("year", "flights", all)}); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Update by other: %v", err)
	}
	if err := s.Delete(context.Background(), other, rep.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Delete by other: %v", err)
	}
	if _, _, err := s.Result(context.Background(), other, rep.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Result by other: %v", err)
	}
}

func TestReorder(t *testing.T) {
	s, _, _ := newTestService()
	user := uuid.New()
	var ids []uuid.UUID
	for _, n := range []string{"a", "b", "c"} {
		r, err := s.Create(context.Background(), user, Input{Name: n, Definition: def("year", "flights", all)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID)
	}
	for _, bad := range [][]uuid.UUID{
		{ids[0], ids[1]},
		{ids[0], ids[1], ids[1]},
		{ids[0], ids[1], uuid.New()},
	} {
		if _, err := s.Reorder(context.Background(), user, bad); !IsValidationError(err) {
			t.Errorf("Reorder(%v) err = %v", bad, err)
		}
	}
	got, err := s.Reorder(context.Background(), user, []uuid.UUID{ids[2], ids[0], ids[1]})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range got {
		names = append(names, r.Name)
	}
	if !reflect.DeepEqual(names, []string{"c", "a", "b"}) {
		t.Fatalf("order = %v", names)
	}
}

func TestEvaluateFilterTranslation(t *testing.T) {
	s, repo, _ := newTestService()
	d := def("route", "flights", all)
	d.Filter = models.CustomReportFilter{
		Q: strp("night>0"), AircraftReg: strp("deabc"), DepartureICAO: strp("EDDF"),
		ArrivalICAO: strp("EDDM"), Role: strp("dual"),
	}
	if _, err := s.Evaluate(context.Background(), uuid.New(), d); err != nil {
		t.Fatal(err)
	}
	o := repo.lastOpts
	if o.Query == nil || o.AircraftReg == nil || *o.DepartureICAO != "EDDF" || *o.ArrivalICAO != "EDDM" {
		t.Fatalf("opts = %+v", o)
	}
	if o.IsDual == nil || !*o.IsDual || o.IsPIC != nil {
		t.Fatalf("role not mapped: %+v", o)
	}
	if o.StartDate != nil || o.EndDate != nil {
		t.Fatalf("all-time window bounded: %+v", o)
	}
	if repo.lastBy != "route" {
		t.Fatalf("groupBy = %s", repo.lastBy)
	}
}

func TestResolveWindow(t *testing.T) {
	d := func(s string) string { return s }
	tests := []struct {
		w          models.CustomReportWindow
		start, end string
	}{
		{all, "", ""},
		{models.CustomReportWindow{Kind: models.ReportWindowLastMonths, Months: intp(1)}, d("2026-09-01"), "2026-09-24"},
		{models.CustomReportWindow{Kind: models.ReportWindowLastMonths, Months: intp(12)}, d("2025-10-01"), "2026-09-24"},
		{models.CustomReportWindow{Kind: models.ReportWindowYearToDate}, "2026-01-01", "2026-09-24"},
		{models.CustomReportWindow{Kind: models.ReportWindowRange, StartDate: strp("2024-03-05")}, "2024-03-05", ""},
	}
	fmtp := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Format("2006-01-02")
	}
	for _, tt := range tests {
		s, e, err := resolveWindow(tt.w, fixedNow)
		if err != nil {
			t.Fatal(err)
		}
		if fmtp(s) != tt.start || fmtp(e) != tt.end {
			t.Errorf("%+v: got %s..%s, want %s..%s", tt.w, fmtp(s), fmtp(e), tt.start, tt.end)
		}
	}
}

func TestMonthGroupingIsGapFilledAcrossWindow(t *testing.T) {
	s, repo, _ := newTestService()
	repo.groups = []repository.CustomReportGroup{group("2026-07", 2, 120), group("2026-09", 1, 45)}
	res, err := s.Evaluate(context.Background(), uuid.New(),
		def("month", "totalTime", models.CustomReportWindow{Kind: models.ReportWindowLastMonths, Months: intp(4)}))
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(res.Rows); !reflect.DeepEqual(got, []string{"2026-06", "2026-07", "2026-08", "2026-09"}) {
		t.Fatalf("keys = %v", got)
	}
	if res.Rows[1].Value != 120 || res.Rows[0].Value != 0 || res.Rows[1].Label != "Jul 2026" {
		t.Fatalf("rows = %+v", res.Rows)
	}
	if res.Totals.Flights != 3 || res.Totals.TotalTime != 165 {
		t.Fatalf("totals = %+v", res.Totals)
	}
	if *res.StartDate != "2026-06-01" || *res.EndDate != "2026-09-24" {
		t.Fatalf("window = %s..%s", *res.StartDate, *res.EndDate)
	}
	if repo.lastOpts.StartDate == nil || repo.lastOpts.EndDate == nil {
		t.Fatal("window not passed to query")
	}
}

func TestYearGroupingAllTimeSpansFlights(t *testing.T) {
	s, repo, _ := newTestService()
	repo.groups = []repository.CustomReportGroup{group("2021", 1, 60), group("2024", 2, 90)}
	res, err := s.Evaluate(context.Background(), uuid.New(), def("year", "flights", all))
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(res.Rows); !reflect.DeepEqual(got, []string{"2021", "2022", "2023", "2024"}) {
		t.Fatalf("keys = %v", got)
	}
}

func TestFutureRangeEndIsCapped(t *testing.T) {
	s, repo, _ := newTestService()
	repo.groups = []repository.CustomReportGroup{group("2026-08", 1, 60)}
	w := models.CustomReportWindow{Kind: models.ReportWindowRange, StartDate: strp("2026-07-01"), EndDate: strp("2030-12-31")}
	res, err := s.Evaluate(context.Background(), uuid.New(), def("month", "flights", w))
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(res.Rows); !reflect.DeepEqual(got, []string{"2026-07", "2026-08", "2026-09"}) {
		t.Fatalf("keys = %v", got)
	}
}

func TestEmptyAllTimeMonthReport(t *testing.T) {
	s, _, _ := newTestService()
	res, err := s.Evaluate(context.Background(), uuid.New(), def("month", "flights", all))
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows == nil || len(res.Rows) != 0 {
		t.Fatalf("rows = %#v", res.Rows)
	}
}

func TestDayOfWeekHasSevenRows(t *testing.T) {
	s, repo, _ := newTestService()
	repo.groups = []repository.CustomReportGroup{group("3", 4, 200)}
	res, err := s.Evaluate(context.Background(), uuid.New(), def("dayOfWeek", "flights", all))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 7 || res.Rows[2].Value != 4 || res.Rows[2].Label != "Wednesday" || res.Rows[6].Label != "Sunday" {
		t.Fatalf("rows = %+v", res.Rows)
	}
}

func TestRankedGroupingSortsAndLimits(t *testing.T) {
	s, repo, _ := newTestService()
	repo.groups = []repository.CustomReportGroup{
		group("", 1, 10), group("C172", 3, 300), group("DA40", 5, 200), group("PA28", 3, 300),
	}
	d := def("aircraftType", "totalTime", all)
	d.Limit = intp(2)
	res, err := s.Evaluate(context.Background(), uuid.New(), d)
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(res.Rows); !reflect.DeepEqual(got, []string{"C172", "PA28"}) {
		t.Fatalf("keys = %v", got)
	}
	if res.OtherGroups != 2 || res.Totals.Flights != 12 {
		t.Fatalf("otherGroups = %d, totals = %+v", res.OtherGroups, res.Totals)
	}

	d.Limit = nil
	res, err = s.Evaluate(context.Background(), uuid.New(), d)
	if err != nil {
		t.Fatal(err)
	}
	if last := res.Rows[len(res.Rows)-1]; last.Key != "" || last.Label != "(none)" {
		t.Fatalf("last row = %+v", last)
	}
}

func TestLabel(t *testing.T) {
	for _, tt := range []struct{ by, key, want string }{
		{"month", "2026-01", "Jan 2026"},
		{"dayOfWeek", "7", "Sunday"},
		{"dayOfWeek", "9", "9"},
		{"route", "EDDF-EDDM", "EDDF-EDDM"},
		{"registration", "", "(none)"},
	} {
		if got := Label(tt.by, tt.key); got != tt.want {
			t.Errorf("Label(%s, %q) = %q, want %q", tt.by, tt.key, got, tt.want)
		}
	}
}
