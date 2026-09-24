package handlers

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/go-pdf/fpdf"
)

var customReportMetricTitles = map[string]string{
	"flights":          "Flights",
	"totalTime":        "Total time",
	"picTime":          "PIC",
	"dualTime":         "Dual",
	"dualGivenTime":    "Dual given",
	"nightTime":        "Night",
	"ifrTime":          "IFR",
	"crossCountryTime": "Cross-country",
	"fstdTime":         "FSTD",
	"landings":         "Landings",
}

var customReportGroupTitles = map[string]string{
	models.ReportGroupMonth:        "Month",
	models.ReportGroupYear:         "Year",
	models.ReportGroupDayOfWeek:    "Day of week",
	models.ReportGroupAircraftType: "Aircraft type",
	models.ReportGroupRegistration: "Registration",
	models.ReportGroupDeparture:    "Departure",
	models.ReportGroupArrival:      "Arrival",
	models.ReportGroupRoute:        "Route",
}

// reportHM formats minutes as H:MM, including zero.
func reportHM(minutes int) string {
	return fmt.Sprintf("%d:%02d", minutes/60, minutes%60)
}

// reportCell formats one metric of t.
func reportCell(t models.CustomReportTotals, metric string) string {
	v := t.Metric(metric)
	if models.IsDurationMetric(metric) {
		return reportHM(v)
	}
	return strconv.Itoa(v)
}

// reportRowName is the group column of an exported row: sortable keys for
// month and year, the label otherwise.
func reportRowName(groupBy string, r models.CustomReportRow) string {
	if r.Key != "" && (groupBy == models.ReportGroupMonth || groupBy == models.ReportGroupYear) {
		return r.Key
	}
	return r.Label
}

func renderCustomReportCSV(res *models.CustomReportResult) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	header := []string{customReportGroupTitles[res.GroupBy]}
	for _, m := range models.ReportMetrics {
		header = append(header, customReportMetricTitles[m])
	}
	csvWrite(w, header)

	for _, r := range res.Rows {
		rec := []string{reportRowName(res.GroupBy, r)}
		for _, m := range models.ReportMetrics {
			rec = append(rec, reportCell(r.CustomReportTotals, m))
		}
		csvWrite(w, rec)
	}
	total := []string{"Total"}
	for _, m := range models.ReportMetrics {
		total = append(total, reportCell(res.Totals, m))
	}
	csvWrite(w, total)

	w.Flush()
	return buf.Bytes(), w.Error()
}

// customReportFilename returns an ASCII download name for a report export.
func customReportFilename(rep *models.CustomReport, res *models.CustomReportResult, ext string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(rep.Name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	slug := strings.TrimSuffix(b.String(), "-")
	if len(slug) > 60 {
		slug = strings.TrimSuffix(slug[:60], "-")
	}
	if slug == "" {
		slug = "report"
	}
	return fmt.Sprintf("ninerlog_report_%s_%s.%s", slug, res.GeneratedAt.Format("2006-01-02"), ext)
}

// customReportSummary returns the window and filter lines of a report.
func customReportSummary(def models.CustomReportDefinition, res *models.CustomReportResult) []string {
	window := "All time"
	switch {
	case res.StartDate != nil && res.EndDate != nil:
		window = *res.StartDate + " to " + *res.EndDate
	case res.StartDate != nil:
		window = "From " + *res.StartDate
	case res.EndDate != nil:
		window = "Until " + *res.EndDate
	}

	var filters []string
	f := def.Filter
	if f.Q != nil {
		filters = append(filters, "Search: "+*f.Q)
	}
	if f.AircraftReg != nil {
		filters = append(filters, "Registration: "+*f.AircraftReg)
	}
	if f.DepartureICAO != nil {
		filters = append(filters, "From: "+*f.DepartureICAO)
	}
	if f.ArrivalICAO != nil {
		filters = append(filters, "To: "+*f.ArrivalICAO)
	}
	if f.Role != nil {
		filters = append(filters, "Role: "+strings.ToUpper(*f.Role))
	}
	if f.LogbookLicenseID != nil {
		filters = append(filters, "Single licence logbook")
	}
	filterLine := "All flights"
	if len(filters) > 0 {
		filterLine = strings.Join(filters, "  |  ")
	}

	return []string{
		fmt.Sprintf("By %s  |  %s  |  %s", strings.ToLower(customReportGroupTitles[res.GroupBy]),
			customReportMetricTitles[res.Metric], window),
		filterLine,
	}
}

const (
	crPageW   = 210.0
	crPageH   = 297.0
	crMargin  = 12.0
	crBottom  = 16.0
	crContent = crPageW - 2*crMargin
)

var (
	crInk    = [3]int{15, 23, 42}
	crMuted  = [3]int{100, 116, 139}
	crBar    = [3]int{37, 99, 235}
	crTrack  = [3]int{241, 245, 249}
	crStripe = [3]int{248, 250, 252}
	crRule   = [3]int{203, 213, 225}
)

func renderCustomReportPDF(rep *models.CustomReport, res *models.CustomReportResult) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(crMargin, crMargin, crMargin)
	pdf.SetAutoPageBreak(false, crBottom)
	pdf.AliasNbPages("{nb}")
	pdf.SetTitle(rep.Name, true)
	pdf.SetCreator("NinerLog", true)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	pdf.SetFooterFunc(func() {
		pdf.SetY(crPageH - 10)
		pdf.SetFont("Helvetica", "", 7)
		setText(pdf, crMuted)
		pdf.CellFormat(crContent/2, 4, tr("NinerLog  |  generated "+res.GeneratedAt.Format("2006-01-02 15:04")+" UTC"), "", 0, "L", false, 0, "")
		pdf.CellFormat(crContent/2, 4, fmt.Sprintf("Page %d/{nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
	})
	pdf.AddPage()

	ensure := func(h float64) bool {
		if pdf.GetY()+h > crPageH-crBottom {
			pdf.AddPage()
			return true
		}
		return false
	}

	// Title and summary.
	setText(pdf, crInk)
	pdf.SetFont("Helvetica", "B", 16)
	pdf.MultiCell(crContent, 7, tr(rep.Name), "", "L", false)
	pdf.SetFont("Helvetica", "", 9)
	setText(pdf, crMuted)
	for _, line := range customReportSummary(rep.Definition, res) {
		pdf.MultiCell(crContent, 4.5, tr(line), "", "L", false)
	}
	pdf.Ln(2)

	// Totals strip.
	setText(pdf, crInk)
	pdf.SetFont("Helvetica", "B", 9)
	var totals []string
	for _, m := range []string{"flights", "totalTime", "picTime", "nightTime", "ifrTime", "landings"} {
		totals = append(totals, customReportMetricTitles[m]+" "+reportCell(res.Totals, m))
	}
	pdf.MultiCell(crContent, 5, tr(strings.Join(totals, "   ")), "", "L", false)
	pdf.Ln(3)

	// Bar chart of the report metric.
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(crContent, 6, tr(customReportMetricTitles[res.Metric]+" by "+strings.ToLower(customReportGroupTitles[res.GroupBy])), "", 1, "L", false, 0, "")
	pdf.Ln(1)
	if len(res.Rows) == 0 {
		pdf.SetFont("Helvetica", "I", 9)
		setText(pdf, crMuted)
		pdf.CellFormat(crContent, 6, "No flights match this report.", "", 1, "L", false, 0, "")
	}
	maxVal := 0
	for _, r := range res.Rows {
		if r.Value > maxVal {
			maxVal = r.Value
		}
	}
	const labelW, valueW, barRowH = 38.0, 20.0, 5.0
	barW := crContent - labelW - valueW
	pdf.SetFont("Helvetica", "", 8)
	for _, r := range res.Rows {
		ensure(barRowH)
		x, y := pdf.GetX(), pdf.GetY()
		setText(pdf, crInk)
		pdf.CellFormat(labelW, barRowH, truncateToWidth(pdf, tr(r.Label), labelW-2), "", 0, "L", false, 0, "")
		setFill(pdf, crTrack)
		pdf.Rect(x+labelW, y+1, barW, barRowH-2, "F")
		if maxVal > 0 && r.Value > 0 {
			setFill(pdf, crBar)
			pdf.Rect(x+labelW, y+1, barW*float64(r.Value)/float64(maxVal), barRowH-2, "F")
		}
		pdf.SetX(x + labelW + barW)
		pdf.CellFormat(valueW, barRowH, reportCell(r.CustomReportTotals, res.Metric), "", 1, "R", false, 0, "")
	}
	if res.OtherGroups > 0 {
		pdf.SetFont("Helvetica", "I", 8)
		setText(pdf, crMuted)
		pdf.CellFormat(crContent, 5, fmt.Sprintf("%d more groups not shown; they are included in the totals.", res.OtherGroups), "", 1, "L", false, 0, "")
	}
	pdf.Ln(4)

	// Table of every metric.
	const nameW, rowH = 30.0, 5.5
	colW := (crContent - nameW) / float64(len(models.ReportMetrics))
	header := func() {
		pdf.SetFont("Helvetica", "B", 7)
		setText(pdf, crInk)
		setDraw(pdf, crRule)
		pdf.CellFormat(nameW, rowH, tr(customReportGroupTitles[res.GroupBy]), "B", 0, "L", false, 0, "")
		for _, m := range models.ReportMetrics {
			title := customReportMetricTitles[m]
			if m == "crossCountryTime" {
				title = "XC"
			}
			pdf.CellFormat(colW, rowH, title, "B", 0, "R", false, 0, "")
		}
		pdf.Ln(-1)
		pdf.SetFont("Helvetica", "", 7.5)
	}
	ensure(rowH * 3)
	header()
	for i, r := range res.Rows {
		if ensure(rowH) {
			header()
		}
		setFill(pdf, crStripe)
		fill := i%2 == 1
		pdf.CellFormat(nameW, rowH, truncateToWidth(pdf, tr(reportRowName(res.GroupBy, r)), nameW-2), "", 0, "L", fill, 0, "")
		for _, m := range models.ReportMetrics {
			pdf.CellFormat(colW, rowH, reportCell(r.CustomReportTotals, m), "", 0, "R", fill, 0, "")
		}
		pdf.Ln(-1)
	}
	if ensure(rowH) {
		header()
	}
	pdf.SetFont("Helvetica", "B", 7.5)
	pdf.CellFormat(nameW, rowH, "Total", "T", 0, "L", false, 0, "")
	for _, m := range models.ReportMetrics {
		pdf.CellFormat(colW, rowH, reportCell(res.Totals, m), "T", 0, "R", false, 0, "")
	}
	pdf.Ln(-1)

	if err := pdf.Error(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// truncateToWidth shortens a CP1252-translated string to fit width mm in the
// current font, appending "..." when cut.
func truncateToWidth(pdf *fpdf.Fpdf, s string, width float64) string {
	if pdf.GetStringWidth(s) <= width {
		return s
	}
	b := s
	for len(b) > 0 && pdf.GetStringWidth(b+"...") > width {
		b = b[:len(b)-1]
	}
	return b + "..."
}
