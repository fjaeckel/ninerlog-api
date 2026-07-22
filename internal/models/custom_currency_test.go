package models

import "testing"

func validBody() CustomCurrencyRuleBody {
	return CustomCurrencyRuleBody{
		Window: CurrencyWindow{Amount: 90, Unit: "days"},
		Requirements: []CurrencyRequirement{
			{Metric: "landings", Min: 3},
		},
	}
}

func TestCustomCurrencyRuleBody_Validate_Valid(t *testing.T) {
	b := validBody()
	b.Filters = []CurrencyFilter{
		{Field: "aircraft_class", Op: "eq", Value: "SEP_LAND"},
		{Field: "aircraft_type", Op: "in", Values: []string{"C172", "PA28"}},
		{Field: "has_night", Op: "is_true"},
	}
	b.Requirements = append(b.Requirements, CurrencyRequirement{Metric: "night_time", Min: 2, Unit: "hours"})
	if err := b.Validate(); err != nil {
		t.Fatalf("expected valid body, got %v", err)
	}
}

func TestCustomCurrencyRuleBody_Validate_Errors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*CustomCurrencyRuleBody)
	}{
		{"zero window", func(b *CustomCurrencyRuleBody) { b.Window.Amount = 0 }},
		{"bad window unit", func(b *CustomCurrencyRuleBody) { b.Window.Unit = "fortnights" }},
		{"no requirements", func(b *CustomCurrencyRuleBody) { b.Requirements = nil }},
		{"unknown metric", func(b *CustomCurrencyRuleBody) { b.Requirements[0].Metric = "barrel_rolls" }},
		{"zero threshold", func(b *CustomCurrencyRuleBody) { b.Requirements[0].Min = 0 }},
		{"unit on count metric", func(b *CustomCurrencyRuleBody) { b.Requirements[0].Unit = "hours" }},
		{"bad unit on time metric", func(b *CustomCurrencyRuleBody) {
			b.Requirements[0] = CurrencyRequirement{Metric: "pic_time", Min: 5, Unit: "furlongs"}
		}},
		{"unknown filter field", func(b *CustomCurrencyRuleBody) {
			b.Filters = []CurrencyFilter{{Field: "pilot_mood", Op: "eq", Value: "happy"}}
		}},
		{"wrong op for field", func(b *CustomCurrencyRuleBody) {
			b.Filters = []CurrencyFilter{{Field: "aircraft_class", Op: "is_true"}}
		}},
		{"eq without value", func(b *CustomCurrencyRuleBody) {
			b.Filters = []CurrencyFilter{{Field: "aircraft_class", Op: "eq"}}
		}},
		{"in without values", func(b *CustomCurrencyRuleBody) {
			b.Filters = []CurrencyFilter{{Field: "aircraft_type", Op: "in"}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := validBody()
			tc.mutate(&b)
			if err := b.Validate(); err == nil {
				t.Fatalf("expected validation error for %q, got nil", tc.name)
			}
		})
	}
}

func TestCustomCurrencyRuleBody_ValueScanRoundTrip(t *testing.T) {
	body := validBody()
	body.Filters = []CurrencyFilter{{Field: "has_ifr", Op: "is_true"}}

	// Value() must yield a driver-compatible type (string/[]byte), not a struct;
	// a struct here means the type failed to implement driver.Valuer.
	v, err := body.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	switch v.(type) {
	case string, []byte:
	default:
		t.Fatalf("Value() returned %T, want a JSON string/bytes storable as jsonb", v)
	}

	var got CustomCurrencyRuleBody
	if err := got.Scan(v); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got.Window != body.Window || len(got.Requirements) != 1 || len(got.Filters) != 1 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestMetricClassification(t *testing.T) {
	if !IsTimeMetric("pic_time") {
		t.Error("pic_time should be a time metric")
	}
	if IsTimeMetric("landings") {
		t.Error("landings should not be a time metric")
	}
	if !IsValidMetric("holds") || IsValidMetric("nonsense") {
		t.Error("IsValidMetric misclassified a metric")
	}
}
