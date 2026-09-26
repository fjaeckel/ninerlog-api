package currency

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func ptr[T any](v T) *T { return &v }

func label(msg []any) string { return fmt.Sprint(msg...) }

func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface:
		return rv.IsNil()
	}
	return false
}

func checkEqual(t *testing.T, want, got any, msg ...any) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("%s: want %+v, got %+v", label(msg), want, got)
	}
}

func mustEqual(t *testing.T, want, got any, msg ...any) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("%s: want %+v, got %+v", label(msg), want, got)
	}
}

func checkTrue(t *testing.T, cond bool, msg ...any) {
	t.Helper()
	if !cond {
		t.Errorf("%s: want true", label(msg))
	}
}

func mustTrue(t *testing.T, cond bool, msg ...any) {
	t.Helper()
	if !cond {
		t.Fatalf("%s: want true", label(msg))
	}
}

func checkFalse(t *testing.T, cond bool, msg ...any) {
	t.Helper()
	if cond {
		t.Errorf("%s: want false", label(msg))
	}
}

func mustFalse(t *testing.T, cond bool, msg ...any) {
	t.Helper()
	if cond {
		t.Fatalf("%s: want false", label(msg))
	}
}

func checkNil(t *testing.T, v any, msg ...any) {
	t.Helper()
	if !isNil(v) {
		t.Errorf("%s: want nil, got %+v", label(msg), v)
	}
}

func checkNotNil(t *testing.T, v any, msg ...any) {
	t.Helper()
	if isNil(v) {
		t.Errorf("%s: want non-nil", label(msg))
	}
}

func mustNotNil(t *testing.T, v any, msg ...any) {
	t.Helper()
	if isNil(v) {
		t.Fatalf("%s: want non-nil", label(msg))
	}
}

func checkEmpty(t *testing.T, s string, msg ...any) {
	t.Helper()
	if s != "" {
		t.Errorf("%s: want empty, got %q", label(msg), s)
	}
}

func mustNotEmpty(t *testing.T, v any, msg ...any) {
	t.Helper()
	if reflect.ValueOf(v).Len() == 0 {
		t.Fatalf("%s: want non-empty", label(msg))
	}
}

func mustNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mustLen(t *testing.T, v any, n int) {
	t.Helper()
	if l := reflect.ValueOf(v).Len(); l != n {
		t.Fatalf("len = %d, want %d: %+v", l, n, v)
	}
}

// logFlight is one flight in a flightLog.
type logFlight struct {
	date      time.Time
	class     models.ClassType
	ulKind    *models.ULKind
	launch    string
	total     int
	pic       int
	dual      int
	landings  int
	night     int
	takeoffs  int
	profCheck bool
}

// flightLog is a DailyFlightDataProvider computing every read from a flight list.
type flightLog struct {
	flights []logFlight
}

var _ DailyFlightDataProvider = (*flightLog)(nil)

func (l *flightLog) progressOf(f logFlight) Progress {
	p := Progress{
		Flights: 1, TotalMinutes: f.total, PICMinutes: f.pic, InstructorMinutes: f.dual,
		Landings: f.landings + f.night, DayLandings: f.landings, NightLandings: f.night,
		Launches: max(f.takeoffs, 1),
	}
	if f.dual > 0 {
		p.TrainingFlights = 1
		p.LongestTrainingFlightMinutes = f.total
	}
	return p
}

func towedOK(f logFlight, includeTowed bool) bool {
	return includeTowed || !models.IsTowedLaunch(f.launch)
}

func (l *flightLog) selectFlights(keep func(logFlight) bool, since time.Time) []logFlight {
	var out []logFlight
	for _, f := range l.flights {
		if !f.date.Before(since) && keep(f) {
			out = append(out, f)
		}
	}
	return out
}

func (l *flightLog) byClass(classTypes []models.ClassType, includeTowed bool) func(logFlight) bool {
	return func(f logFlight) bool { return slices.Contains(classTypes, f.class) && towedOK(f, includeTowed) }
}

func (l *flightLog) byUL(sel ULSelector, includeTowed bool) func(logFlight) bool {
	return func(f logFlight) bool {
		if f.class != models.ClassTypeUL || !towedOK(f, includeTowed) {
			return false
		}
		if f.ulKind == nil {
			return sel.IncludeUnspecified
		}
		return slices.Contains(sel.Kinds, *f.ulKind)
	}
}

func (l *flightLog) sum(fs []logFlight) *Progress {
	total := &Progress{}
	for _, f := range fs {
		p := l.progressOf(f)
		addProgress(total, &p, true)
	}
	return total
}

func (l *flightLog) daily(fs []logFlight) []DailyProgress {
	byDate := map[time.Time]*Progress{}
	for _, f := range fs {
		if byDate[f.date] == nil {
			byDate[f.date] = &Progress{}
		}
		p := l.progressOf(f)
		addProgress(byDate[f.date], &p, true)
	}
	var out []DailyProgress
	for d, p := range byDate {
		out = append(out, DailyProgress{Date: d, Progress: *p})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}

func (l *flightLog) GetProgressByAircraftClass(_ context.Context, _ uuid.UUID, classTypes []models.ClassType, includeTowed bool, since time.Time) (*Progress, error) {
	return l.sum(l.selectFlights(l.byClass(classTypes, includeTowed), since)), nil
}

func (l *flightLog) GetProgressAll(_ context.Context, _ uuid.UUID, since time.Time) (*Progress, error) {
	return l.sum(l.selectFlights(func(logFlight) bool { return true }, since)), nil
}

func (l *flightLog) GetProgressByULKind(_ context.Context, _ uuid.UUID, sel ULSelector, includeTowed bool, since time.Time) (*Progress, error) {
	return l.sum(l.selectFlights(l.byUL(sel, includeTowed), since)), nil
}

func (l *flightLog) GetDailyProgressByAircraftClass(_ context.Context, _ uuid.UUID, classTypes []models.ClassType, includeTowed bool, since time.Time) ([]DailyProgress, error) {
	return l.daily(l.selectFlights(l.byClass(classTypes, includeTowed), since)), nil
}

func (l *flightLog) GetDailyProgressAll(_ context.Context, _ uuid.UUID, since time.Time) ([]DailyProgress, error) {
	return l.daily(l.selectFlights(func(logFlight) bool { return true }, since)), nil
}

func (l *flightLog) GetDailyProgressByULKind(_ context.Context, _ uuid.UUID, sel ULSelector, includeTowed bool, since time.Time) ([]DailyProgress, error) {
	return l.daily(l.selectFlights(l.byUL(sel, includeTowed), since)), nil
}

func (l *flightLog) GetLastFlightReview(context.Context, uuid.UUID) (*time.Time, error) {
	return nil, nil
}

func latest(fs []logFlight) *time.Time {
	var d *time.Time
	for _, f := range fs {
		if d == nil || f.date.After(*d) {
			d = ptr(f.date)
		}
	}
	return d
}

func (l *flightLog) GetLastProficiencyCheck(_ context.Context, _ uuid.UUID, classTypes []models.ClassType, since time.Time) (*time.Time, error) {
	ir := len(classTypes) == 1 && classTypes[0] == models.ClassTypeIR
	return latest(l.selectFlights(func(f logFlight) bool {
		if !f.profCheck {
			return false
		}
		return ir || (slices.Contains(classTypes, f.class) && (f.class == models.ClassTypeGlider || !models.IsTowedLaunch(f.launch)))
	}, since)), nil
}

func (l *flightLog) GetLastProficiencyCheckByULKind(_ context.Context, _ uuid.UUID, sel ULSelector, since time.Time) (*time.Time, error) {
	match := l.byUL(sel, true)
	return latest(l.selectFlights(func(f logFlight) bool { return f.profCheck && match(f) }, since)), nil
}

func (l *flightLog) GetLaunchCounts(_ context.Context, _ uuid.UUID, classType models.ClassType, since time.Time) (map[string]int, error) {
	counts := map[string]int{}
	for _, f := range l.selectFlights(func(f logFlight) bool { return f.class == classType && f.launch != "" }, since) {
		counts[f.launch] += max(f.takeoffs, 1)
	}
	return counts, nil
}

func (l *flightLog) GetDailyLaunchCounts(_ context.Context, _ uuid.UUID, classType models.ClassType, since time.Time) ([]DailyLaunches, error) {
	var out []DailyLaunches
	for _, f := range l.selectFlights(func(f logFlight) bool { return f.class == classType && f.launch != "" }, since) {
		out = append(out, DailyLaunches{Date: f.date, Method: f.launch, Launches: max(f.takeoffs, 1)})
	}
	return out, nil
}

func (l *flightLog) landingDays(fs []logFlight, withTakeoffs bool) []LandingDay {
	byDate := map[time.Time]*LandingDay{}
	for _, f := range fs {
		if byDate[f.date] == nil {
			byDate[f.date] = &LandingDay{Date: f.date}
		}
		d := byDate[f.date]
		d.DayLandings += f.landings
		d.NightLandings += f.night
		if withTakeoffs {
			d.Takeoffs += max(f.takeoffs, 1)
		}
	}
	var out []LandingDay
	for _, d := range byDate {
		if d.DayLandings+d.NightLandings+d.Takeoffs > 0 {
			out = append(out, *d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out
}

func (l *flightLog) GetLandingDaysByAircraftClass(_ context.Context, _ uuid.UUID, classType models.ClassType, includeTowed, picOnly bool, since time.Time) ([]LandingDay, error) {
	return l.landingDays(l.selectFlights(func(f logFlight) bool {
		return f.class == classType && towedOK(f, includeTowed) && (!picOnly || f.pic > 0)
	}, since), false), nil
}

func (l *flightLog) GetLandingDaysByULKind(_ context.Context, _ uuid.UUID, sel ULSelector, includeTowed bool, since time.Time) ([]LandingDay, error) {
	return l.landingDays(l.selectFlights(l.byUL(sel, includeTowed), since), true), nil
}

// today returns midnight UTC today.
func today() time.Time { return midnightUTC(time.Now().UTC()) }

// winchFlights returns n glider winch circuits of 30 minutes PIC, one every
// `every` days, the newest `newest` days ago.
func winchFlights(n, newest, every int) []logFlight {
	var fs []logFlight
	for i := 0; i < n; i++ {
		fs = append(fs, logFlight{
			date: today().AddDate(0, 0, -(newest + i*every)), class: models.ClassTypeGlider,
			launch: "winch", total: 30, pic: 30, landings: 1, takeoffs: 1,
		})
	}
	return fs
}

// dualGliderFlights returns n dual aerotow glider flights of 60 minutes, `ago` days ago.
func dualGliderFlights(n, ago int) []logFlight {
	var fs []logFlight
	for i := 0; i < n; i++ {
		fs = append(fs, logFlight{
			date: today().AddDate(0, 0, -ago), class: models.ClassTypeGlider,
			launch: "aerotow", total: 60, dual: 60, landings: 1, takeoffs: 1,
		})
	}
	return fs
}

// evalAll runs the service for one licence and its ratings over log.
func evalAll(t *testing.T, log *flightLog, license *models.License, ratings ...*models.ClassRating) *CurrencyStatusResponse {
	t.Helper()
	svc := newTestService(log, license, ratings...)
	res, err := svc.EvaluateAll(context.Background(), license.UserID)
	mustNoError(t, err)
	return res
}

func newTestService(log *flightLog, license *models.License, ratings ...*models.ClassRating) *Service {
	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())
	reg.Register(NewFAAEvaluator())
	reg.RegisterMulti(NewGermanULEvaluator(), NewGermanULEvaluator().Authorities()...)
	lr := &mockLicenseRepo{licenses: map[uuid.UUID]*models.License{license.ID: license}}
	cr := &mockCRRepo{ratings: map[uuid.UUID][]*models.ClassRating{license.ID: ratings}}
	return NewService(reg, lr, cr, log)
}

func requirement(t *testing.T, r ClassRatingCurrency, nameKey string) Requirement {
	t.Helper()
	for _, req := range r.Requirements {
		if req.NameKey == nameKey {
			return req
		}
	}
	t.Fatalf("no requirement %s in %+v", nameKey, r.Requirements)
	return Requirement{}
}

func launchMethod(t *testing.T, r ClassRatingCurrency, method string) LaunchMethodCurrency {
	t.Helper()
	for _, lm := range r.LaunchMethodCurrency {
		if lm.Method == method {
			return lm
		}
	}
	t.Fatalf("no launch method %s in %+v", method, r.LaunchMethodCurrency)
	return LaunchMethodCurrency{}
}

func splLicence() (*models.License, *models.ClassRating) {
	userID := uuid.New()
	license := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "SPL"}
	rating := &models.ClassRating{ID: uuid.New(), LicenseID: license.ID, ClassType: models.ClassTypeGlider}
	return license, rating
}

func TestLastDayCounted(t *testing.T) {
	d := func(s string) time.Time { v, _ := time.Parse("2006-01-02", s); return v }
	tests := []struct {
		name   string
		flown  string
		window windowSpec
		want   string
	}{
		{"24 months", "2025-05-10", windowSpec{years: 2}, "2027-05-09"},
		{"24 months from leap day", "2024-02-29", windowSpec{years: 2}, "2026-02-28"},
		{"24 months from first of month", "2025-03-01", windowSpec{years: 2}, "2027-02-28"},
		{"12 months", "2025-12-31", windowSpec{years: 1}, "2026-12-30"},
		{"6 months from month end", "2025-08-31", windowSpec{months: 6}, "2026-02-28"},
		{"90 days", "2026-01-01", windowSpec{days: 90}, "2026-03-31"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lastDayCounted(d(tt.flown), tt.window)
			checkEqual(t, tt.want, got.Format("2006-01-02"))
			back := func(x time.Time) time.Time { return x.AddDate(-tt.window.years, -tt.window.months, -tt.window.days) }
			checkTrue(t, back(got).Before(d(tt.flown)), "flight counts on the last day")
			checkFalse(t, back(got.AddDate(0, 0, 1)).Before(d(tt.flown)), "flight no longer counts the day after")
		})
	}
}

func TestValidUntil_SPLLaunches(t *testing.T) {
	license, rating := splLicence()
	// 15 winch launches, every 20 days, the newest 5 days ago; the 15th newest is X.
	flights := winchFlights(15, 5, 20)
	x := flights[14].date
	flights = append(flights, dualGliderFlights(2, 3)...)
	log := &flightLog{flights: flights}

	res := evalAll(t, log, license, rating)
	mustLen(t, res.Ratings, 1)
	r := res.Ratings[0]
	mustEqual(t, StatusCurrent, r.Status)

	launches := requirement(t, r, ReqKeyLaunches)
	mustTrue(t, launches.Met)
	// 17 launches: the two dual flights are newer, so the 15th newest launch is flights[12].
	want := flights[12].date.AddDate(2, 0, -1).Format("2006-01-02")
	checkEqual(t, want, *launches.ValidUntil)
	checkEmpty(t, launches.RemedyKey)

	t.Run("fifteen launches exactly: X + 24 months - 1 day", func(t *testing.T) {
		only := &flightLog{flights: winchFlights(15, 5, 20)}
		r := evalAll(t, only, license, rating).Ratings[0]
		lr := requirement(t, r, ReqKeyLaunches)
		mustTrue(t, lr.Met)
		checkEqual(t, x.AddDate(2, 0, -1).Format("2006-01-02"), *lr.ValidUntil)
	})

	t.Run("unmet requirement has no validUntil", func(t *testing.T) {
		check := requirement(t, r, ReqKeyProficiencyCheck)
		checkFalse(t, check.Met)
		checkNil(t, check.ValidUntil)
	})

	t.Run("rating validUntil is the earliest experience row", func(t *testing.T) {
		mustNotNil(t, r.ValidUntil)
		earliest := ""
		for _, req := range r.Requirements {
			if req.ValidUntil != nil && (earliest == "" || *req.ValidUntil < earliest) {
				earliest = *req.ValidUntil
			}
		}
		checkEqual(t, earliest, *r.ValidUntil)
	})

	t.Run("launch method validUntil", func(t *testing.T) {
		winch := launchMethod(t, r, "winch")
		mustTrue(t, winch.Met)
		// fifth newest winch launch
		checkEqual(t, flights[4].date.AddDate(2, 0, -1).Format("2006-01-02"), *winch.ValidUntil)
	})

	t.Run("boundary: met on validUntil, unmet the day after", func(t *testing.T) {
		svc := newTestService(log, license, rating)
		until, _ := time.Parse("2006-01-02", *launches.ValidUntil)
		on, err := svc.EvaluateAsOf(context.Background(), license.UserID, until)
		mustNoError(t, err)
		checkTrue(t, requirement(t, on.Ratings[0], ReqKeyLaunches).Met)
		after, err := svc.EvaluateAsOf(context.Background(), license.UserID, until.AddDate(0, 0, 1))
		mustNoError(t, err)
		lr := requirement(t, after.Ratings[0], ReqKeyLaunches)
		checkFalse(t, lr.Met)
		checkEqual(t, RemedyFlyMore, lr.RemedyKey)
		checkEqual(t, 1, *lr.RemedyParams.Missing)
		checkEqual(t, "launches", *lr.RemedyParams.Unit)
	})
}

func TestValidUntil_ProficiencyCheckAlternative(t *testing.T) {
	license, rating := splLicence()
	check := today().AddDate(0, -3, 0)
	log := &flightLog{flights: append(winchFlights(15, 5, 20), dualGliderFlights(2, 3)...)}
	log.flights = append(log.flights, logFlight{date: check, class: models.ClassTypeGlider, launch: "aerotow", total: 45, pic: 45, landings: 1, profCheck: true})

	r := evalAll(t, log, license, rating).Ratings[0]
	mustEqual(t, StatusCurrent, r.Status)
	req := requirement(t, r, ReqKeyProficiencyCheck)
	mustTrue(t, req.Met)
	checkEqual(t, check.AddDate(2, 0, -1).Format("2006-01-02"), *req.ValidUntil)
	// The rating stays current while either alternative is met.
	checkEqual(t, *req.ValidUntil, *r.ValidUntil)
}

func TestValidUntil_ExpiryAnchoredRuleOmitted(t *testing.T) {
	userID := uuid.New()
	license := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	expiry := today().AddDate(0, 8, 0)
	rating := &models.ClassRating{ID: uuid.New(), LicenseID: license.ID, ClassType: models.ClassTypeSEPLand, ExpiryDate: &expiry}
	var fs []logFlight
	for i := 0; i < 12; i++ {
		fs = append(fs, logFlight{date: today().AddDate(0, 0, -i-1), class: models.ClassTypeSEPLand, total: 70, pic: 70, dual: 10, landings: 1, takeoffs: 1})
	}
	r := evalAll(t, &flightLog{flights: fs}, license, rating).Ratings[0]
	mustNotEmpty(t, r.Requirements)
	checkNil(t, r.ValidUntil)
	for _, req := range r.Requirements {
		checkNil(t, req.ValidUntil, req.NameKey)
	}
}

func TestRemedyKeys(t *testing.T) {
	uid := uuid.New()
	lic := func(auth, lt string) *models.License {
		return &models.License{ID: uuid.New(), UserID: uid, RegulatoryAuthority: auth, LicenseType: lt}
	}
	rat := func(l *models.License, ct models.ClassType, expiry *time.Time) *models.ClassRating {
		return &models.ClassRating{ID: uuid.New(), LicenseID: l.ID, ClassType: ct, ExpiryDate: expiry}
	}
	expiry := today().AddDate(0, 6, 0)
	type want struct {
		nameKey string
		key     string
		missing *int
		unit    string
	}
	tests := []struct {
		name    string
		license *models.License
		class   models.ClassType
		expiry  *time.Time
		flights []logFlight
		want    []want
	}{
		{
			name: "SFCL.160(a) SPL", license: lic("EASA", "SPL"), class: models.ClassTypeGlider,
			flights: winchFlights(12, 5, 1),
			want: []want{
				{ReqKeyFlightTime, RemedyFlyMore, ptr(300 - 360), "minutes"},
				{ReqKeyLaunches, RemedyFlyMore, ptr(3), "launches"},
				{ReqKeyTrainingFlights, RemedyFlyMore, ptr(2), "flights"},
				{ReqKeyProficiencyCheck, RemedyProficiencyCheck, nil, ""},
			},
		},
		{
			name: "FCL.140.A LAPL(A)", license: lic("EASA", "LAPL"), class: models.ClassTypeSEPLand,
			flights: []logFlight{{date: today().AddDate(0, 0, -2), class: models.ClassTypeSEPLand, total: 600, pic: 600, landings: 12, takeoffs: 12}},
			want: []want{
				{ReqKeyTotalTime, RemedyFlyMore, ptr(120), "minutes"},
				{ReqKeyLandings, "", nil, ""},
				{ReqKeyTrainingFlight, RemedyTrainingFlight, nil, ""},
				{ReqKeyProficiencyCheck, RemedyProficiencyCheck, nil, ""},
			},
		},
		{
			name: "SFCL.160(b) SPL TMG", license: lic("EASA", "SPL"), class: models.ClassTypeTMG,
			want: []want{
				{ReqKeyTMGLandings, RemedyFlyMore, ptr(12), "landings"},
				{ReqKeyTMGTrainingFlight, RemedyTrainingFlight, nil, ""},
				{ReqKeyProficiencyCheck, RemedyProficiencyCheck, nil, ""},
			},
		},
		{
			name: "FCL.740.A(b)(1) SEP", license: lic("EASA", "PPL"), class: models.ClassTypeSEPLand, expiry: &expiry,
			want: []want{
				{ReqKeyRefresherTraining, RemedyFlyMore, ptr(60), "minutes"},
				{ReqKeyLandings, RemedyFlyMore, ptr(12), "landings"},
			},
		},
		{
			name: "FCL.740.A(b)(2) MEP", license: lic("EASA", "PPL"), class: models.ClassTypeMEPLand, expiry: &expiry,
			want: []want{
				{ReqKeyRouteSectors, RemedyFlyMore, ptr(10), "flights"},
				{ReqKeyProficiencyCheck, RemedyProficiencyCheck, nil, ""},
			},
		},
		{
			name: "FAA 61.57", license: lic("FAA", "PRIVATE"), class: models.ClassTypeSEPLand,
			want: []want{{ReqKeyDayLandings, RemedyFlyMore, ptr(3), "landings"}},
		},
		{
			name: "FAA 61.56 glider flight review", license: lic("FAA", "GLIDER"), class: models.ClassTypeGlider,
			want: []want{
				{ReqKeyFlightReview, "", nil, ""},
				{ReqKeyTrainingFlights, RemedyFlyMore, ptr(3), "flights"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalAll(t, &flightLog{flights: tt.flights}, tt.license, rat(tt.license, tt.class, tt.expiry)).Ratings[0]
			for _, w := range tt.want {
				req := requirement(t, r, w.nameKey)
				if w.missing != nil && *w.missing <= 0 {
					checkTrue(t, req.Met, w.nameKey)
					checkEmpty(t, req.RemedyKey, w.nameKey)
					continue
				}
				checkEqual(t, w.key, req.RemedyKey, w.nameKey)
				if w.missing == nil {
					checkNil(t, req.RemedyParams, w.nameKey)
					continue
				}
				mustNotNil(t, req.RemedyParams, w.nameKey)
				checkEqual(t, *w.missing, *req.RemedyParams.Missing, w.nameKey)
				checkEqual(t, w.unit, *req.RemedyParams.Unit, w.nameKey)
			}
		})
	}
}

func TestRemedyKeys_LaunchMethodDual(t *testing.T) {
	license, rating := splLicence()
	fs := append(winchFlights(15, 5, 1), dualGliderFlights(3, 2)...)
	r := evalAll(t, &flightLog{flights: fs}, license, rating).Ratings[0]
	aerotow := launchMethod(t, r, "aerotow")
	mustFalse(t, aerotow.Met)
	checkNil(t, aerotow.ValidUntil)
	checkEqual(t, RemedyLaunchMethodDual, aerotow.RemedyKey)
	checkEqual(t, "aerotow", *aerotow.RemedyParams.Method)
	checkEqual(t, 2, *aerotow.RemedyParams.Missing)
	winch := launchMethod(t, r, "winch")
	checkTrue(t, winch.Met)
	checkEmpty(t, winch.RemedyKey)
	checkNotNil(t, winch.ValidUntil)
}

func TestEvaluateAsOf(t *testing.T) {
	t.Run("SPL current today, lapsed three months ahead", func(t *testing.T) {
		license, rating := splLicence()
		// 15 launches; the three oldest leave the window within two months.
		fs := winchFlights(12, 5, 5)
		for i := 0; i < 3; i++ {
			fs = append(fs, logFlight{date: today().AddDate(-2, 0, 30+i*10), class: models.ClassTypeGlider, launch: "winch", total: 30, pic: 30, landings: 1, takeoffs: 1})
		}
		fs = append(fs, dualGliderFlights(2, 4)...)
		svc := newTestService(&flightLog{flights: fs}, license, rating)

		now, err := svc.EvaluateAll(context.Background(), license.UserID)
		mustNoError(t, err)
		checkEqual(t, StatusCurrent, now.Ratings[0].Status)

		ahead, err := svc.EvaluateAsOf(context.Background(), license.UserID, today().AddDate(0, 3, 0))
		mustNoError(t, err)
		r := ahead.Ratings[0]
		checkEqual(t, StatusLapsed, r.Status)
		checkEqual(t, MsgRatingRecencyNotMet, r.MessageKey)
		lr := requirement(t, r, ReqKeyLaunches)
		checkFalse(t, lr.Met)
		checkEqual(t, 1, *lr.RemedyParams.Missing, "the three oldest of 17 launches aged out, leaving 14")

		// validUntil agrees with the as-of evaluation.
		until, _ := time.Parse("2006-01-02", *now.Ratings[0].ValidUntil)
		onDay, _ := svc.EvaluateAsOf(context.Background(), license.UserID, until)
		checkEqual(t, StatusCurrent, onDay.Ratings[0].Status)
		dayAfter, _ := svc.EvaluateAsOf(context.Background(), license.UserID, until.AddDate(0, 0, 1))
		checkEqual(t, StatusLapsed, dayAfter.Ratings[0].Status)
	})

	t.Run("expiry-anchored rating expires by date", func(t *testing.T) {
		license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}
		expiry := today().AddDate(0, 0, 30)
		rating := &models.ClassRating{ID: uuid.New(), LicenseID: license.ID, ClassType: models.ClassTypeSEPLand, ExpiryDate: &expiry}
		svc := newTestService(&flightLog{}, license, rating)
		before, err := svc.EvaluateAsOf(context.Background(), license.UserID, expiry.AddDate(0, 0, -1))
		mustNoError(t, err)
		checkEqual(t, StatusExpiring, before.Ratings[0].Status)
		after, err := svc.EvaluateAsOf(context.Background(), license.UserID, expiry.AddDate(0, 0, 1))
		mustNoError(t, err)
		checkEqual(t, StatusExpired, after.Ratings[0].Status)
	})

	t.Run("passenger currency lapses after dayExpiresOn", func(t *testing.T) {
		license, rating := splLicence()
		fs := winchFlights(3, 10, 1)
		svc := newTestService(&flightLog{flights: fs}, license, rating)
		now, err := svc.EvaluateAll(context.Background(), license.UserID)
		mustNoError(t, err)
		mustLen(t, now.PassengerCurrency, 1)
		pax := now.PassengerCurrency[0]
		mustEqual(t, StatusCurrent, pax.DayStatus)
		expires, _ := time.Parse("2006-01-02", *pax.DayExpiresOn)

		on, _ := svc.EvaluateAsOf(context.Background(), license.UserID, expires)
		checkEqual(t, StatusCurrent, on.PassengerCurrency[0].DayStatus)
		after, _ := svc.EvaluateAsOf(context.Background(), license.UserID, expires.AddDate(0, 0, 1))
		checkEqual(t, StatusExpired, after.PassengerCurrency[0].DayStatus)
		checkEqual(t, MsgPaxNotCurrent, after.PassengerCurrency[0].MessageKey)
	})
}

func TestDailyCache_MatchesAggregates(t *testing.T) {
	license, _ := splLicence()
	log := &flightLog{flights: append(winchFlights(20, 1, 30), dualGliderFlights(2, 50)...)}
	cache := newDailyCache(log)
	ctx := context.Background()
	wide := today().AddDate(-2, 0, 0)
	for _, since := range []time.Time{wide, wide.AddDate(0, 6, 0), today().AddDate(0, -1, 0)} {
		got, err := cache.GetProgressByAircraftClass(ctx, license.UserID, []models.ClassType{models.ClassTypeGlider}, true, since)
		mustNoError(t, err)
		want, _ := log.GetProgressByAircraftClass(ctx, license.UserID, []models.ClassType{models.ClassTypeGlider}, true, since)
		checkEqual(t, want, got, since.String())
		gotLaunch, _ := cache.GetLaunchCounts(ctx, license.UserID, models.ClassTypeGlider, since)
		wantLaunch, _ := log.GetLaunchCounts(ctx, license.UserID, models.ClassTypeGlider, since)
		checkEqual(t, wantLaunch, gotLaunch, since.String())
	}
}

func TestValidUntil_GermanULThreeAxis(t *testing.T) {
	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "DULV", LicenseType: "UL"}
	rating := &models.ClassRating{ID: uuid.New(), LicenseID: license.ID, ClassType: models.ClassTypeUL, ULKind: ptr(models.ULKindThreeAxis)}
	threeAxis := ptr(models.ULKindThreeAxis)
	var fs []logFlight
	for i := 0; i < 12; i++ {
		fs = append(fs, logFlight{date: today().AddDate(0, -i, -1), class: models.ClassTypeUL, ulKind: threeAxis, total: 70, pic: 70, landings: 1, takeoffs: 1})
	}
	training := today().AddDate(0, -2, -3)
	fs = append(fs, logFlight{date: training, class: models.ClassTypeUL, ulKind: threeAxis, total: 60, dual: 60, landings: 1, takeoffs: 1})

	r := evalAll(t, &flightLog{flights: fs}, license, rating).Ratings[0]
	mustEqual(t, StatusCurrent, r.Status)
	tf := requirement(t, r, ReqKeyTrainingFlight)
	mustTrue(t, tf.Met)
	checkEqual(t, training.AddDate(2, 0, -1).Format("2006-01-02"), *tf.ValidUntil, "training flight")
	landings := requirement(t, r, ReqKeyLandings)
	// 13 landings with the training flight: the twelfth newest is the 10-months-ago flight.
	checkEqual(t, today().AddDate(0, -10, -1).AddDate(2, 0, -1).Format("2006-01-02"), *landings.ValidUntil, "landings")
	mustNotNil(t, r.ValidUntil)
	checkEqual(t, *landings.ValidUntil, *r.ValidUntil, "rating")
}
