package models

import (
	"strings"
	"testing"
)

func strp(s string) *string { return &s }
func intp(i int) *int       { return &i }

func validReportDef() CustomReportDefinition {
	return CustomReportDefinition{
		Window:  CustomReportWindow{Kind: ReportWindowAll},
		GroupBy: ReportGroupMonth,
		Metric:  "totalTime",
	}
}

func TestCustomReportDefinitionValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(d *CustomReportDefinition)
		wantErr string
	}{
		{"valid", func(d *CustomReportDefinition) {}, ""},
		{"bad groupBy", func(d *CustomReportDefinition) { d.GroupBy = "pilot" }, "invalid groupBy"},
		{"bad metric", func(d *CustomReportDefinition) { d.Metric = "fuel" }, "invalid metric"},
		{"limit zero", func(d *CustomReportDefinition) { d.Limit = intp(0) }, "limit must be"},
		{"limit too big", func(d *CustomReportDefinition) { d.Limit = intp(101) }, "limit must be"},
		{"bad window", func(d *CustomReportDefinition) { d.Window.Kind = "forever" }, "invalid window kind"},
		{"lastMonths missing months", func(d *CustomReportDefinition) { d.Window.Kind = ReportWindowLastMonths }, "window months"},
		{"lastMonths too many", func(d *CustomReportDefinition) {
			d.Window = CustomReportWindow{Kind: ReportWindowLastMonths, Months: intp(121)}
		}, "window months"},
		{"lastMonths ok", func(d *CustomReportDefinition) {
			d.Window = CustomReportWindow{Kind: ReportWindowLastMonths, Months: intp(12)}
		}, ""},
		{"range bad date", func(d *CustomReportDefinition) {
			d.Window = CustomReportWindow{Kind: ReportWindowRange, StartDate: strp("2024-13-01")}
		}, "startDate must be a date"},
		{"range inverted", func(d *CustomReportDefinition) {
			d.Window = CustomReportWindow{Kind: ReportWindowRange, StartDate: strp("2024-02-01"), EndDate: strp("2024-01-01")}
		}, "endDate must not be before"},
		{"range open end", func(d *CustomReportDefinition) {
			d.Window = CustomReportWindow{Kind: ReportWindowRange, StartDate: strp("2024-02-01")}
		}, ""},
		{"query too long", func(d *CustomReportDefinition) { d.Filter.Q = strp(strings.Repeat("a", 1001)) }, "too long"},
		{"reg too long", func(d *CustomReportDefinition) { d.Filter.AircraftReg = strp(strings.Repeat("D", 21)) }, "aircraftReg is too long"},
		{"icao control char", func(d *CustomReportDefinition) { d.Filter.DepartureICAO = strp("ED\nF") }, "invalid characters"},
		{"bad role", func(d *CustomReportDefinition) { d.Filter.Role = strp("sic") }, "invalid role"},
		{"pic role", func(d *CustomReportDefinition) { d.Filter.Role = strp("pic") }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validReportDef()
			tt.mutate(&d)
			err := d.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestCustomReportDefinitionNormalize(t *testing.T) {
	d := CustomReportDefinition{
		Filter:  CustomReportFilter{Q: strp("  "), AircraftReg: strp(" D-EABC ")},
		Window:  CustomReportWindow{Kind: ReportWindowAll, Months: intp(3), StartDate: strp("2024-01-01")},
		GroupBy: ReportGroupYear,
		Metric:  "flights",
		Limit:   intp(5),
	}
	d.Normalize()
	if d.Filter.Q != nil {
		t.Errorf("blank q kept: %q", *d.Filter.Q)
	}
	if d.Filter.AircraftReg == nil || *d.Filter.AircraftReg != "D-EABC" {
		t.Errorf("aircraftReg = %v", d.Filter.AircraftReg)
	}
	if d.Window.Months != nil || d.Window.StartDate != nil {
		t.Errorf("window fields not cleared for kind all: %+v", d.Window)
	}
	if d.Limit != nil {
		t.Errorf("limit kept for time grouping")
	}
}

func TestCustomReportDefinitionJSONBRoundTrip(t *testing.T) {
	d := validReportDef()
	d.Filter.Q = strp("night>0")
	v, err := d.Value()
	if err != nil {
		t.Fatal(err)
	}
	var back CustomReportDefinition
	if err := back.Scan([]byte(v.(string))); err != nil {
		t.Fatal(err)
	}
	if back.Filter.Q == nil || *back.Filter.Q != "night>0" || back.GroupBy != d.GroupBy {
		t.Fatalf("round trip mismatch: %+v", back)
	}
}

func TestCustomReportTotalsMetric(t *testing.T) {
	tot := CustomReportTotals{Flights: 1, TotalTime: 2, PicTime: 3, DualTime: 4, DualGivenTime: 5,
		NightTime: 6, IfrTime: 7, CrossCountryTime: 8, FstdTime: 9, Landings: 10}
	for i, m := range ReportMetrics {
		if got := tot.Metric(m); got != i+1 {
			t.Errorf("Metric(%s) = %d, want %d", m, got, i+1)
		}
	}
	sum := tot
	sum.Add(tot)
	if sum.Landings != 20 || sum.FstdTime != 18 {
		t.Errorf("Add = %+v", sum)
	}
}
