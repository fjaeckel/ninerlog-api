package currency

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// fixedNow pins the evaluator's clock.
func fixedNow() time.Time { return time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC) }

// fakeCustomData is a canned CustomFlightDataProvider. Aggregates returns one
// value per requested metric in request order; Rows returns the per-flight
// contributions for lapse computation. It records the last requests so tests
// can assert what the evaluator asked for.
type fakeCustomData struct {
	aggregates    map[string]int64
	rows          []FlightMetricRow // Values index-aligned with requested metrics
	rowsByMetric  []map[string]int64
	rowDates      []time.Time
	lastMetrics   []string
	lastFilters   []models.CurrencyFilter
	rowsRequested bool
}

func (f *fakeCustomData) AggregateMetrics(_ context.Context, _ uuid.UUID, _ time.Time, metrics []string, filters []models.CurrencyFilter) ([]int64, error) {
	f.lastMetrics = metrics
	f.lastFilters = filters
	out := make([]int64, len(metrics))
	for i, m := range metrics {
		out[i] = f.aggregates[m]
	}
	return out, nil
}

func (f *fakeCustomData) MetricRowsByDate(_ context.Context, _ uuid.UUID, _ time.Time, metrics []string, _ []models.CurrencyFilter) ([]FlightMetricRow, error) {
	f.rowsRequested = true
	if f.rowsByMetric != nil {
		rows := make([]FlightMetricRow, len(f.rowsByMetric))
		for i, byMetric := range f.rowsByMetric {
			vals := make([]int64, len(metrics))
			for j, m := range metrics {
				vals[j] = byMetric[m]
			}
			rows[i] = FlightMetricRow{Date: f.rowDates[i], Values: vals}
		}
		return rows, nil
	}
	return f.rows, nil
}

func newTestEvaluator(data *fakeCustomData) *CustomEvaluator {
	e := NewCustomEvaluator(data)
	e.now = fixedNow
	return e
}

func TestEvaluate_CountRequirementMet(t *testing.T) {
	data := &fakeCustomData{
		aggregates: map[string]int64{"landings": 5},
		// Recent flight => far-off lapse => stays current, not expiring.
		rowsByMetric: []map[string]int64{{"landings": 5}},
		rowDates:     []time.Time{fixedNow()},
	}
	e := newTestEvaluator(data)

	userID := uuid.New()
	body := &models.CustomCurrencyRuleBody{
		Window:       models.CurrencyWindow{Amount: 90, Unit: "days"},
		Requirements: []models.CurrencyRequirement{{Metric: "landings", Min: 3}},
	}

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
	if !data.rowsRequested {
		t.Error("a met rule should trigger the per-flight lapse computation")
	}
}

func TestEvaluate_NotMetIsExpired(t *testing.T) {
	data := &fakeCustomData{aggregates: map[string]int64{"approaches": 2}}
	e := newTestEvaluator(data)

	userID := uuid.New()
	body := &models.CustomCurrencyRuleBody{
		Window:       models.CurrencyWindow{Amount: 6, Unit: "months"},
		Requirements: []models.CurrencyRequirement{{Metric: "approaches", Min: 6}},
	}

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
	if data.rowsRequested {
		t.Error("an unmet rule must not run the lapse computation")
	}
}

func TestEvaluate_TimeMetricConvertsToHours(t *testing.T) {
	// 720 minutes == 12 hours >= 10
	data := &fakeCustomData{
		aggregates:   map[string]int64{"total_time": 720},
		rowsByMetric: []map[string]int64{{"total_time": 720}},
		rowDates:     []time.Time{fixedNow()},
	}
	e := newTestEvaluator(data)

	userID := uuid.New()
	body := &models.CustomCurrencyRuleBody{
		Window:       models.CurrencyWindow{Amount: 12, Unit: "months"},
		Requirements: []models.CurrencyRequirement{{Metric: "total_time", Min: 10}}, // 10 hours
	}

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

func TestEvaluate_PassesFiltersThrough(t *testing.T) {
	data := &fakeCustomData{
		aggregates:   map[string]int64{"night_landings": 1},
		rowsByMetric: []map[string]int64{{"night_landings": 1}},
		rowDates:     []time.Time{fixedNow()},
	}
	e := newTestEvaluator(data)

	userID := uuid.New()
	filters := []models.CurrencyFilter{
		{Field: "aircraft_class", Op: "eq", Value: "SEP_LAND"},
		{Field: "aircraft_type", Op: "in", Values: []string{"C172", "PA28"}},
		{Field: "has_night", Op: "is_true"},
	}
	body := &models.CustomCurrencyRuleBody{
		Window:       models.CurrencyWindow{Amount: 90, Unit: "days"},
		Filters:      filters,
		Requirements: []models.CurrencyRequirement{{Metric: "night_landings", Min: 1}},
	}

	if _, err := e.Evaluate(context.Background(), userID, body); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(data.lastFilters) != len(filters) {
		t.Fatalf("provider saw %d filters, want %d", len(data.lastFilters), len(filters))
	}
	for i := range filters {
		if data.lastFilters[i].Field != filters[i].Field || data.lastFilters[i].Op != filters[i].Op {
			t.Errorf("filter %d = %+v, want %+v", i, data.lastFilters[i], filters[i])
		}
	}
}

func TestEvaluate_DedupesRepeatedMetric(t *testing.T) {
	data := &fakeCustomData{aggregates: map[string]int64{"landings": 5}}
	e := newTestEvaluator(data)

	userID := uuid.New()
	// Two requirements on the same metric should aggregate it once.
	body := &models.CustomCurrencyRuleBody{
		Window: models.CurrencyWindow{Amount: 90, Unit: "days"},
		Requirements: []models.CurrencyRequirement{
			{Metric: "landings", Min: 3},
			{Metric: "landings", Min: 10},
		},
	}

	res, err := e.Evaluate(context.Background(), userID, body)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(data.lastMetrics) != 1 || data.lastMetrics[0] != "landings" {
		t.Errorf("provider should be asked for the metric once, got %v", data.lastMetrics)
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
	// Met exactly (3). The only contributing flight was 25 days ago, so it ages
	// out of the 30-day window in 5 days -> within the ~15-day threshold.
	flightDate := fixedNow().AddDate(0, 0, -25)
	data := &fakeCustomData{
		aggregates:   map[string]int64{"landings": 3},
		rowsByMetric: []map[string]int64{{"landings": 3}},
		rowDates:     []time.Time{flightDate},
	}
	e := newTestEvaluator(data)

	userID := uuid.New()
	body := &models.CustomCurrencyRuleBody{
		Window:       models.CurrencyWindow{Amount: 30, Unit: "days"},
		Requirements: []models.CurrencyRequirement{{Metric: "landings", Min: 3}},
	}

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
	// landings: sole flight 80 days ago -> lapses in 10 days.
	// approaches: sole flight 10 days ago -> lapses in 80 days.
	// Earliest (10 days) should drive the rule expiry.
	landingDate := fixedNow().AddDate(0, 0, -80)
	approachDate := fixedNow().AddDate(0, 0, -10)
	data := &fakeCustomData{
		aggregates: map[string]int64{"landings": 1, "approaches": 1},
		rowsByMetric: []map[string]int64{
			{"landings": 1, "approaches": 0},
			{"landings": 0, "approaches": 1},
		},
		rowDates: []time.Time{landingDate, approachDate},
	}
	e := newTestEvaluator(data)

	userID := uuid.New()
	body := &models.CustomCurrencyRuleBody{
		Window: models.CurrencyWindow{Amount: 90, Unit: "days"},
		Requirements: []models.CurrencyRequirement{
			{Metric: "landings", Min: 1},
			{Metric: "approaches", Min: 1},
		},
	}

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
