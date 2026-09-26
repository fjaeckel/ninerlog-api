package handlers

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func sampleCustomReport(rows int) (*models.CustomReport, *models.CustomReportResult) {
	q := "=HYPERLINK(\"x\")"
	start, end := "2025-10-01", "2026-09-24"
	rep := &models.CustomReport{
		ID:   uuid.New(),
		Name: "Nachtflüge / Night — 2026",
		Definition: models.CustomReportDefinition{
			Filter:  models.CustomReportFilter{Q: &q},
			Window:  models.CustomReportWindow{Kind: models.ReportWindowLastMonths},
			GroupBy: models.ReportGroupAircraftType,
			Metric:  "nightTime",
		},
	}
	res := &models.CustomReportResult{
		GroupBy:     models.ReportGroupAircraftType,
		Metric:      "nightTime",
		StartDate:   &start,
		EndDate:     &end,
		OtherGroups: 3,
		GeneratedAt: time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC),
	}
	for i := 0; i < rows; i++ {
		t := models.CustomReportTotals{Flights: i + 1, TotalTime: 125 * (i + 1), NightTime: 61, Landings: 2}
		key := "C172"
		if i%3 == 1 {
			key = "=cmd|' /C calc'!A0"
		}
		res.Rows = append(res.Rows, models.CustomReportRow{Key: key, Label: key, Value: t.NightTime, CustomReportTotals: t})
		res.Totals.Add(t)
	}
	return rep, res
}

func TestRenderCustomReportCSV(t *testing.T) {
	_, res := sampleCustomReport(2)
	out, err := renderCustomReportCSV(res)
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(bytes.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 {
		t.Fatalf("records = %d, want header + 2 rows + total", len(records))
	}
	if records[0][0] != "Aircraft type" || records[0][2] != "Total time" || len(records[0]) != 11 {
		t.Fatalf("header = %v", records[0])
	}
	if records[1][0] != "C172" || records[1][1] != "1" || records[1][2] != "2:05" || records[1][6] != "1:01" {
		t.Fatalf("row = %v", records[1])
	}
	if !strings.HasPrefix(records[2][0], "'=") {
		t.Fatalf("formula not neutralized: %q", records[2][0])
	}
	if records[3][0] != "Total" || records[3][1] != "3" || records[3][2] != "6:15" || records[3][9] != "0:00" {
		t.Fatalf("total = %v", records[3])
	}
}

func TestRenderCustomReportCSVUsesSortableTimeKeys(t *testing.T) {
	res := &models.CustomReportResult{
		GroupBy: models.ReportGroupMonth, Metric: "flights",
		Rows: []models.CustomReportRow{{Key: "2026-01", Label: "Jan 2026"}},
	}
	out, err := renderCustomReportCSV(res)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "\n2026-01,") {
		t.Fatalf("month key missing:\n%s", out)
	}
}

func TestRenderCustomReportPDF(t *testing.T) {
	for _, rows := range []int{0, 3, 150} {
		rep, res := sampleCustomReport(rows)
		out, err := renderCustomReportPDF(rep, res)
		if err != nil {
			t.Fatalf("rows=%d: %v", rows, err)
		}
		if !bytes.HasPrefix(out, []byte("%PDF-")) {
			t.Fatalf("rows=%d: not a PDF", rows)
		}
	}
}

func TestCustomReportFilename(t *testing.T) {
	rep, res := sampleCustomReport(0)
	if got := customReportFilename(rep, res, "csv"); got != "ninerlog_report_nachtfl-ge-night-2026_2026-09-24.csv" {
		t.Fatalf("filename = %q", got)
	}
	rep.Name = "✈✈"
	if got := customReportFilename(rep, res, "pdf"); got != "ninerlog_report_report_2026-09-24.pdf" {
		t.Fatalf("filename = %q", got)
	}
}

func TestCustomReportSummary(t *testing.T) {
	rep, res := sampleCustomReport(0)
	lines := customReportSummary(rep.Definition, res)
	if !strings.Contains(lines[0], "aircraft type") || !strings.Contains(lines[0], "2025-10-01 to 2026-09-24") {
		t.Fatalf("line 0 = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "Search: ") {
		t.Fatalf("line 1 = %q", lines[1])
	}
}

func TestCustomReportExportSoaringColumns(t *testing.T) {
	tests := []struct {
		name   string
		metric string
		totals models.CustomReportTotals
		want   []string
		absent []string
	}{
		{"A2 powered report keeps its columns", "nightTime", models.CustomReportTotals{Flights: 2, TotalTime: 120},
			nil, []string{"Launches", "Outlandings", "Tow flights"}},
		{"L soaring totals add their columns", "flights", models.CustomReportTotals{Flights: 7, Launches: 7, Outlandings: 1},
			[]string{"Launches", "Outlandings"}, []string{"Tow flights"}},
		{"report metric column is always present", "towFlights", models.CustomReportTotals{},
			[]string{"Tow flights"}, []string{"Launches", "Outlandings"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := &models.CustomReportResult{GroupBy: models.ReportGroupLaunchMethod, Metric: tt.metric, Totals: tt.totals}
			res.Rows = []models.CustomReportRow{{Key: "winch", Label: "Winch", CustomReportTotals: tt.totals}}
			out, err := renderCustomReportCSV(res)
			if err != nil {
				t.Fatal(err)
			}
			records, err := csv.NewReader(bytes.NewReader(out)).ReadAll()
			if err != nil {
				t.Fatal(err)
			}
			header := strings.Join(records[0], "|")
			if records[0][0] != "Launch method" {
				t.Errorf("group title = %q", records[0][0])
			}
			for _, w := range tt.want {
				if !strings.Contains(header, w) {
					t.Errorf("header %q lacks %q", header, w)
				}
			}
			for _, a := range tt.absent {
				if strings.Contains(header, a) {
					t.Errorf("header %q has %q", header, a)
				}
			}
			if len(records[1]) != len(records[0]) {
				t.Errorf("row width %d != header width %d", len(records[1]), len(records[0]))
			}
			if _, err := renderCustomReportPDF(&models.CustomReport{Name: "Launches"}, res); err != nil {
				t.Fatalf("pdf: %v", err)
			}
		})
	}
}
