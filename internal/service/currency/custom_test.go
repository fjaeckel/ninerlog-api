package currency

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// fixedNow gives the evaluator a deterministic clock so windowSince is stable.
func fixedNow() time.Time { return time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC) }

func newTestEvaluator(t *testing.T) (*CustomEvaluator, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	e := NewCustomEvaluator(db)
	e.now = fixedNow
	return e, mock, func() { db.Close() }
}

func TestEvaluate_CountRequirementMet(t *testing.T) {
	e, mock, done := newTestEvaluator(t)
	defer done()

	userID := uuid.New()
	body := &models.CustomCurrencyRuleBody{
		Window:       models.CurrencyWindow{Amount: 90, Unit: "days"},
		Requirements: []models.CurrencyRequirement{{Metric: "landings", Min: 3}},
	}

	mock.ExpectQuery("FROM flights f").
		WithArgs(userID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"m0"}).AddRow(int64(5)))
	// A met rule triggers the per-flight lapse query. Recent flight => far-off
	// lapse => stays current, not expiring.
	mock.ExpectQuery("ORDER BY f.date ASC").
		WillReturnRows(sqlmock.NewRows([]string{"date", "m0"}).AddRow(fixedNow(), int64(5)))

	res, err := e.Evaluate(context.Background(), userID, body)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Status != StatusCurrent {
		t.Errorf("status = %v, want current", res.Status)
	}
	if len(res.Requirements) != 1 || !res.Requirements[0].Met {
		t.Fatalf("expected one met requirement, got %+v", res.Requirements)
	}
	if res.Requirements[0].Current != 5 || res.Requirements[0].Required != 3 {
		t.Errorf("current/required = %v/%v", res.Requirements[0].Current, res.Requirements[0].Required)
	}
	if res.WindowLabel != "last 90 days" {
		t.Errorf("windowLabel = %q", res.WindowLabel)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestEvaluate_NotMetIsExpired(t *testing.T) {
	e, mock, done := newTestEvaluator(t)
	defer done()

	userID := uuid.New()
	body := &models.CustomCurrencyRuleBody{
		Window:       models.CurrencyWindow{Amount: 6, Unit: "months"},
		Requirements: []models.CurrencyRequirement{{Metric: "approaches", Min: 6}},
	}

	mock.ExpectQuery("FROM flights f").
		WithArgs(userID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"m0"}).AddRow(int64(2)))

	res, err := e.Evaluate(context.Background(), userID, body)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Status != StatusExpired {
		t.Errorf("status = %v, want expired", res.Status)
	}
	if res.Requirements[0].Met {
		t.Error("requirement should not be met")
	}
}

func TestEvaluate_TimeMetricConvertsToHours(t *testing.T) {
	e, mock, done := newTestEvaluator(t)
	defer done()

	userID := uuid.New()
	body := &models.CustomCurrencyRuleBody{
		Window:       models.CurrencyWindow{Amount: 12, Unit: "months"},
		Requirements: []models.CurrencyRequirement{{Metric: "total_time", Min: 10}}, // 10 hours
	}

	// 720 minutes == 12 hours >= 10
	mock.ExpectQuery("FROM flights f").
		WithArgs(userID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"m0"}).AddRow(int64(720)))
	mock.ExpectQuery("ORDER BY f.date ASC").
		WillReturnRows(sqlmock.NewRows([]string{"date", "m0"}).AddRow(fixedNow(), int64(720)))

	res, err := e.Evaluate(context.Background(), userID, body)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !res.Requirements[0].Met || res.Requirements[0].Current != 12 {
		t.Errorf("expected 12h met, got %+v", res.Requirements[0])
	}
	if res.Requirements[0].Unit != "hours" {
		t.Errorf("unit = %q, want hours", res.Requirements[0].Unit)
	}
}

func TestEvaluate_FilterValuesAreBound(t *testing.T) {
	e, mock, done := newTestEvaluator(t)
	defer done()

	userID := uuid.New()
	body := &models.CustomCurrencyRuleBody{
		Window: models.CurrencyWindow{Amount: 90, Unit: "days"},
		Filters: []models.CurrencyFilter{
			{Field: "aircraft_class", Op: "eq", Value: "SEP_LAND"},
			{Field: "aircraft_type", Op: "in", Values: []string{"C172", "PA28"}},
			{Field: "has_night", Op: "is_true"},
		},
		Requirements: []models.CurrencyRequirement{{Metric: "night_landings", Min: 1}},
	}

	// Args: userID, since, then filter values in order. is_true binds nothing.
	mock.ExpectQuery("FROM flights f").
		WithArgs(userID, sqlmock.AnyArg(), "SEP_LAND", "C172", "PA28").
		WillReturnRows(sqlmock.NewRows([]string{"m0"}).AddRow(int64(1)))
	// The per-flight query binds the same parameters in the same order.
	mock.ExpectQuery("ORDER BY f.date ASC").
		WithArgs(userID, sqlmock.AnyArg(), "SEP_LAND", "C172", "PA28").
		WillReturnRows(sqlmock.NewRows([]string{"date", "m0"}).AddRow(fixedNow(), int64(1)))

	if _, err := e.Evaluate(context.Background(), userID, body); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestEvaluate_DedupesRepeatedMetric(t *testing.T) {
	e, mock, done := newTestEvaluator(t)
	defer done()

	userID := uuid.New()
	// Two requirements on the same metric should aggregate it once (single column).
	body := &models.CustomCurrencyRuleBody{
		Window: models.CurrencyWindow{Amount: 90, Unit: "days"},
		Requirements: []models.CurrencyRequirement{
			{Metric: "landings", Min: 3},
			{Metric: "landings", Min: 10},
		},
	}

	mock.ExpectQuery("FROM flights f").
		WithArgs(userID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"m0"}).AddRow(int64(5)))

	res, err := e.Evaluate(context.Background(), userID, body)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(res.Requirements) != 2 {
		t.Fatalf("want 2 requirements, got %d", len(res.Requirements))
	}
	if !res.Requirements[0].Met || res.Requirements[1].Met {
		t.Errorf("expected first met (>=3) and second not (>=10): %+v", res.Requirements)
	}
	if res.Status != StatusExpired {
		t.Errorf("status should be expired when any requirement unmet")
	}
}

func TestBuildFilterClause(t *testing.T) {
	t.Run("eq", func(t *testing.T) {
		args := []interface{}{"u", "since"}
		clause, err := buildFilterClause(models.CurrencyFilter{Field: "aircraft_class", Op: "eq", Value: "SEP_LAND"}, &args)
		if err != nil {
			t.Fatal(err)
		}
		if clause != "a.aircraft_class = $3" {
			t.Errorf("clause = %q", clause)
		}
		if len(args) != 3 || args[2] != "SEP_LAND" {
			t.Errorf("args = %v", args)
		}
	})
	t.Run("in", func(t *testing.T) {
		args := []interface{}{"u", "since"}
		clause, err := buildFilterClause(models.CurrencyFilter{Field: "aircraft_type", Op: "in", Values: []string{"A", "B"}}, &args)
		if err != nil {
			t.Fatal(err)
		}
		if clause != "f.aircraft_type IN ($3, $4)" {
			t.Errorf("clause = %q", clause)
		}
	})
	t.Run("is_true uses fixed predicate, binds nothing", func(t *testing.T) {
		args := []interface{}{"u", "since"}
		clause, err := buildFilterClause(models.CurrencyFilter{Field: "has_ifr", Op: "is_true"}, &args)
		if err != nil {
			t.Fatal(err)
		}
		if clause != "f.ifr_time > 0" {
			t.Errorf("clause = %q", clause)
		}
		if len(args) != 2 {
			t.Errorf("is_true should not bind args, got %v", args)
		}
	})
	t.Run("unknown field rejected", func(t *testing.T) {
		args := []interface{}{}
		if _, err := buildFilterClause(models.CurrencyFilter{Field: "evil'; DROP", Op: "eq", Value: "x"}, &args); err == nil {
			t.Error("expected error for unknown field")
		}
	})
}

func TestWindowSinceAndLabel(t *testing.T) {
	now := fixedNow()
	if got := windowSince(now, models.CurrencyWindow{Amount: 90, Unit: "days"}); !got.Equal(now.AddDate(0, 0, -90)) {
		t.Errorf("days window = %v", got)
	}
	if got := windowSince(now, models.CurrencyWindow{Amount: 2, Unit: "weeks"}); !got.Equal(now.AddDate(0, 0, -14)) {
		t.Errorf("weeks window = %v", got)
	}
	if got := windowSince(now, models.CurrencyWindow{Amount: 6, Unit: "months"}); !got.Equal(now.AddDate(0, -6, 0)) {
		t.Errorf("months window = %v", got)
	}
	if lbl := windowLabel(models.CurrencyWindow{Amount: 1, Unit: "years"}); lbl != "last 1 year" {
		t.Errorf("singular label = %q", lbl)
	}
}

func TestEvaluate_ExpiringSoon(t *testing.T) {
	e, mock, done := newTestEvaluator(t)
	defer done()

	userID := uuid.New()
	body := &models.CustomCurrencyRuleBody{
		Window:       models.CurrencyWindow{Amount: 30, Unit: "days"},
		Requirements: []models.CurrencyRequirement{{Metric: "landings", Min: 3}},
	}

	// Met exactly (3). The only contributing flight was 25 days ago, so it ages
	// out of the 30-day window in 5 days -> within the ~15-day threshold.
	flightDate := fixedNow().AddDate(0, 0, -25)
	mock.ExpectQuery("FROM flights f").
		WithArgs(userID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"m0"}).AddRow(int64(3)))
	mock.ExpectQuery("ORDER BY f.date ASC").
		WillReturnRows(sqlmock.NewRows([]string{"date", "m0"}).AddRow(flightDate, int64(3)))

	res, err := e.Evaluate(context.Background(), userID, body)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Status != StatusExpiring {
		t.Errorf("status = %v, want expiring", res.Status)
	}
	wantExpiry := flightDate.AddDate(0, 0, 30).Format("2006-01-02")
	if res.ExpiresOn == nil || *res.ExpiresOn != wantExpiry {
		t.Errorf("expiresOn = %v, want %s", res.ExpiresOn, wantExpiry)
	}
}

func TestEvaluate_EarliestRequirementDrivesExpiry(t *testing.T) {
	e, mock, done := newTestEvaluator(t)
	defer done()

	userID := uuid.New()
	body := &models.CustomCurrencyRuleBody{
		Window: models.CurrencyWindow{Amount: 90, Unit: "days"},
		Requirements: []models.CurrencyRequirement{
			{Metric: "landings", Min: 1},
			{Metric: "approaches", Min: 1},
		},
	}

	// landings: sole flight 80 days ago -> lapses in 10 days.
	// approaches: sole flight 10 days ago -> lapses in 80 days.
	// Earliest (10 days) should drive the rule expiry.
	landingDate := fixedNow().AddDate(0, 0, -80)
	approachDate := fixedNow().AddDate(0, 0, -10)
	mock.ExpectQuery("FROM flights f").
		WithArgs(userID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"m0", "m1"}).AddRow(int64(1), int64(1)))
	mock.ExpectQuery("ORDER BY f.date ASC").
		WillReturnRows(sqlmock.NewRows([]string{"date", "m0", "m1"}).
			AddRow(landingDate, int64(1), int64(0)).
			AddRow(approachDate, int64(0), int64(1)))

	res, err := e.Evaluate(context.Background(), userID, body)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	wantExpiry := landingDate.AddDate(0, 0, 90).Format("2006-01-02")
	if res.ExpiresOn == nil || *res.ExpiresOn != wantExpiry {
		t.Errorf("expiresOn = %v, want %s (earliest requirement)", res.ExpiresOn, wantExpiry)
	}
}

func TestWindowEndAndThreshold(t *testing.T) {
	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := windowEnd(d, models.CurrencyWindow{Amount: 90, Unit: "days"}); !got.Equal(d.AddDate(0, 0, 90)) {
		t.Errorf("windowEnd days = %v", got)
	}
	if rawThreshold(models.CurrencyRequirement{Metric: "pic_time", Min: 2}) != 120 {
		t.Errorf("hours threshold should convert to minutes")
	}
	if rawThreshold(models.CurrencyRequirement{Metric: "pic_time", Min: 90, Unit: "minutes"}) != 90 {
		t.Errorf("minutes threshold should stay as-is")
	}
	if rawThreshold(models.CurrencyRequirement{Metric: "landings", Min: 3}) != 3 {
		t.Errorf("count threshold should stay as-is")
	}
	if expiringThresholdDays(models.CurrencyWindow{Amount: 7, Unit: "days"}) != 3 {
		t.Errorf("short window threshold should be half the window")
	}
	if expiringThresholdDays(models.CurrencyWindow{Amount: 1, Unit: "years"}) != 30 {
		t.Errorf("long window threshold should cap at 30")
	}
}

func TestFormatAmount(t *testing.T) {
	if formatAmount(5) != "5" {
		t.Errorf("whole number should have no decimal")
	}
	if formatAmount(12.5) != "12.5" {
		t.Errorf("fractional formatting wrong")
	}
}
