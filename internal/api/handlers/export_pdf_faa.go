package handlers

import (
	"fmt"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/go-pdf/fpdf"
)

// ─────────────────────────────────────────────────────────────────────────────
// FAA — 14 CFR § 61.51 layouts (ASA/Jeppesen-style)
// ─────────────────────────────────────────────────────────────────────────────

// The spread layout follows the classic ASA/Jeppesen paper logbook: aircraft,
// route, landings and remarks on the left page; pilot function, instrument
// and conditions-of-flight times on the right page. The single layout keeps
// every column on one landscape page per batch. Times are decimal hours, as
// customary in FAA logbooks.

const faaRegulation = "FAA · 14 CFR § 61.51"

// Left page of the spread.
var faaLeftGroups = []colGroup{
	{"", 1}, {"AIRCRAFT", 2}, {"ROUTE OF FLIGHT", 2}, {"LANDINGS", 2},
	{"INSTRUMENT", 2}, {"TOTAL TIME", 1}, {"REMARKS AND ENDORSEMENTS", 1},
}
var faaLeftSub = []string{
	"DATE", "TYPE", "IDENT", "FROM", "TO", "DAY", "NIGHT", "APPR", "HOLDS", "", "",
}
var faaLeftBaseW = []float64{
	16, 18, 18, 14, 14, 10, 10, 10, 10, 14, 90,
}
var faaLeftAlign = []string{
	"C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "L",
}

// Right page of the spread: date repeats for cross-reference.
var faaRightGroups = []colGroup{
	{"", 1}, {"PILOT FUNCTION", 5}, {"INSTRUMENT", 2},
	{"CONDITIONS OF FLIGHT", 2}, {"TOTAL TIME", 1},
}
var faaRightSub = []string{
	"DATE", "SOLO", "PIC", "SIC", "DUAL RECD", "INSTR GIVEN",
	"ACTUAL", "SIMULATED", "X-CTRY", "NIGHT", "",
}
var faaRightBaseW = []float64{
	16, 13, 13, 13, 13, 13, 13, 13, 13, 13, 15,
}
var faaRightAlign = []string{
	"C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "C",
}

// Single-page layout: every column on one landscape page.
var faaSingleGroups = []colGroup{
	{"", 1}, {"AIRCRAFT", 2}, {"ROUTE", 2}, {"PILOT FUNCTION", 5},
	{"INSTRUMENT", 4}, {"CONDITIONS", 2}, {"LANDINGS", 2},
	{"TOTAL TIME", 1}, {"REMARKS AND ENDORSEMENTS", 1},
}
var faaSingleSub = []string{
	"DATE", "TYPE", "IDENT", "FROM", "TO",
	"SOLO", "PIC", "SIC", "DUAL", "INSTR",
	"ACTUAL", "SIM", "APPR", "HOLDS",
	"X-CTRY", "NIGHT", "DAY", "NIGHT", "", "",
}
var faaSingleBaseW = []float64{
	16, 16, 17, 13, 13, 11, 11, 11, 11, 11, 11, 11, 8, 8, 11, 11, 8, 8, 12, 42,
}
var faaSingleAlign = []string{
	"C", "C", "C", "C", "C",
	"C", "C", "C", "C", "C",
	"C", "C", "C", "C",
	"C", "C", "C", "C",
	"C", "L",
}

// faaTotals accumulates every numeric FAA column.
type faaTotals struct {
	solo, pic, sic, dual, instr    int
	act, sim, xc, night            int
	ldgD, ldgN, appr, holds, total int
}

func (t *faaTotals) add(f *models.Flight) {
	t.solo += f.SoloTime
	t.pic += f.PICTime
	t.sic += f.SICTime
	t.dual += f.DualTime
	t.instr += f.DualGivenTime
	t.act += f.ActualInstrumentTime
	t.sim += f.SimulatedInstrumentTime
	t.xc += f.CrossCountryTime
	t.night += f.NightTime
	t.ldgD += f.LandingsDay
	t.ldgN += f.LandingsNight
	t.appr += f.ApproachesCount
	t.holds += f.Holds
	t.total += f.TotalTime
}

// addBaseline opens the balance with the pilot's prior experience. act, sim,
// appr and holds stay untouched.
func (t *faaTotals) addBaseline(b *models.FlightBaseline) {
	if !baselineApplies(b) {
		return
	}
	t.solo += b.SoloMinutes
	t.pic += b.PICMinutes
	t.sic += b.SICMinutes
	t.dual += b.DualMinutes
	t.instr += b.DualGivenMinutes
	t.xc += b.CrossCountryMinutes
	t.night += b.NightMinutes
	t.ldgD += b.LandingsDay
	t.ldgN += b.LandingsNight
	t.total += b.TotalMinutes
}

func (t *faaTotals) addAll(o faaTotals) {
	t.solo += o.solo
	t.pic += o.pic
	t.sic += o.sic
	t.dual += o.dual
	t.instr += o.instr
	t.act += o.act
	t.sim += o.sim
	t.xc += o.xc
	t.night += o.night
	t.ldgD += o.ldgD
	t.ldgN += o.ldgN
	t.appr += o.appr
	t.holds += o.holds
	t.total += o.total
}

// faaDec renders minutes as bare decimal hours ("1.5") the way FAA paper
// logbooks do; zero durations stay blank to keep data rows quiet.
func faaDec(v int) string {
	if v == 0 {
		return ""
	}
	return faaDecTotal(v)
}

// faaDecTotal always prints, including "0.0" — used in the totals rows.
func faaDecTotal(v int) string {
	return fmt.Sprintf("%.1f", float64(v)/60.0)
}

func faaRemarks(f *models.Flight, max int) string {
	return truncRunes(flightrules.CombinedRemarks(f, flightrules.FlagIPC, flightrules.FlagFlightReview), max)
}

func generateFAAPDF(flights []*models.Flight, g pageGeometry, userName, layout string, b *models.FlightBaseline) *fpdf.Fpdf {
	d := newDoc(g, faaRegulation, userName, certFAA)
	d.note = baselineFooterNote(b)
	if layout == layoutSingle {
		renderFAASingle(d, flights, b)
	} else {
		renderFAASpread(d, flights, b)
	}
	addGrandSummaryPage(d, flights, b)
	return d.pdf
}

func renderFAASpread(d *pdfDoc, flights []*models.Flight, b *models.FlightBaseline) {
	g, pdf := d.g, d.pdf
	leftW := scaleWidths(faaLeftBaseW, g.usableWidth())
	rightW := scaleWidths(faaRightBaseW, g.usableWidth())

	rpp := g.logRowsPerPage()
	totalSpreads := (len(flights) + rpp - 1) / rpp
	spreadNum := 0

	// Filler page so duplex printing opens each spread as facing pages.
	if len(flights) > 0 {
		d.addBlankPage()
	}

	// Cumulative running total across all spreads, opened with the baseline.
	var cum faaTotals
	cum.addBaseline(b)

	for startIdx := 0; startIdx < len(flights); startIdx += rpp {
		endIdx := startIdx + rpp
		if endIdx > len(flights) {
			endIdx = len(flights)
		}
		spreadNum++
		page := flights[startIdx:endIdx]

		var pt faaTotals
		for _, f := range page {
			pt.add(f)
		}

		// ── LEFT PAGE ───────────────────────────────────────────────────────
		d.startPage(fmt.Sprintf("Spread %d of %d %s Left", spreadNum, totalSpreads, emdash()))
		d.drawHeader(leftW, faaLeftGroups, faaLeftSub)

		pdf.SetFont("Helvetica", "", g.fontBody)
		for i, f := range page {
			cells := []string{
				f.Date.Format("01/02/06"),
				f.AircraftType, f.AircraftReg,
				safeStr(f.DepartureICAO), safeStr(f.ArrivalICAO),
				fmt.Sprintf("%d", f.LandingsDay),
				fmt.Sprintf("%d", f.LandingsNight),
				fmt.Sprintf("%d", f.ApproachesCount),
				fmt.Sprintf("%d", f.Holds),
				faaDec(f.TotalTime),
				faaRemarks(f, 64),
			}
			d.drawDataRow(leftW, cells, faaLeftAlign, i)
		}
		leftCells := func(t faaTotals) []string {
			return []string{
				fmt.Sprintf("%d", t.ldgD), fmt.Sprintf("%d", t.ldgN),
				fmt.Sprintf("%d", t.appr), fmt.Sprintf("%d", t.holds),
				faaDecTotal(t.total), "",
			}
		}
		cumAfter := cum
		cumAfter.addAll(pt)
		d.drawTotalsRow(leftW, 5, "TOTAL THIS PAGE", leftCells(pt), faaLeftAlign, false)
		d.drawTotalsRow(leftW, 5, "TOTAL FROM PREVIOUS PAGES", leftCells(cum), faaLeftAlign, false)
		d.drawTotalsRow(leftW, 5, "TOTAL TIME", leftCells(cumAfter), faaLeftAlign, true)

		// ── RIGHT PAGE ──────────────────────────────────────────────────────
		d.startPage(fmt.Sprintf("Spread %d of %d %s Right", spreadNum, totalSpreads, emdash()))
		d.drawHeader(rightW, faaRightGroups, faaRightSub)

		pdf.SetFont("Helvetica", "", g.fontBody)
		for i, f := range page {
			cells := []string{
				f.Date.Format("01/02/06"),
				faaDec(f.SoloTime),
				faaDec(f.PICTime),
				faaDec(f.SICTime),
				faaDec(f.DualTime),
				faaDec(f.DualGivenTime),
				faaDec(f.ActualInstrumentTime),
				faaDec(f.SimulatedInstrumentTime),
				faaDec(f.CrossCountryTime),
				faaDec(f.NightTime),
				faaDec(f.TotalTime),
			}
			d.drawDataRow(rightW, cells, faaRightAlign, i)
		}
		rightCells := func(t faaTotals) []string {
			return []string{
				faaDecTotal(t.solo),
				faaDecTotal(t.pic),
				faaDecTotal(t.sic),
				faaDecTotal(t.dual),
				faaDecTotal(t.instr),
				faaDecTotal(t.act),
				faaDecTotal(t.sim),
				faaDecTotal(t.xc),
				faaDecTotal(t.night),
				faaDecTotal(t.total),
			}
		}
		d.drawTotalsRow(rightW, 1, "TOTAL THIS PAGE", rightCells(pt), faaRightAlign, false)
		d.drawTotalsRow(rightW, 1, "FROM PREV PAGES", rightCells(cum), faaRightAlign, false)
		cum.addAll(pt)
		d.drawTotalsRow(rightW, 1, "TOTAL TIME", rightCells(cum), faaRightAlign, true)

		d.drawSignatureBlock()
	}

	// Filler page so the totals summary starts on its own sheet.
	if len(flights) > 0 {
		d.addBlankPage()
	}
}

func renderFAASingle(d *pdfDoc, flights []*models.Flight, b *models.FlightBaseline) {
	g, pdf := d.g, d.pdf
	colW := scaleWidths(faaSingleBaseW, g.usableWidth())

	rpp := g.logRowsPerPage()
	totalPages := (len(flights) + rpp - 1) / rpp
	pageNum := 0

	// Cumulative running total, opened with the baseline.
	var cum faaTotals
	cum.addBaseline(b)

	for startIdx := 0; startIdx < len(flights); startIdx += rpp {
		endIdx := startIdx + rpp
		if endIdx > len(flights) {
			endIdx = len(flights)
		}
		pageNum++
		page := flights[startIdx:endIdx]

		d.startPage(fmt.Sprintf("Logbook Page %d of %d", pageNum, totalPages))
		d.drawHeader(colW, faaSingleGroups, faaSingleSub)

		var pt faaTotals
		pdf.SetFont("Helvetica", "", g.fontBody)
		for i, f := range page {
			cells := []string{
				f.Date.Format("01/02/06"),
				f.AircraftType, f.AircraftReg,
				safeStr(f.DepartureICAO), safeStr(f.ArrivalICAO),
				faaDec(f.SoloTime),
				faaDec(f.PICTime),
				faaDec(f.SICTime),
				faaDec(f.DualTime),
				faaDec(f.DualGivenTime),
				faaDec(f.ActualInstrumentTime),
				faaDec(f.SimulatedInstrumentTime),
				fmt.Sprintf("%d", f.ApproachesCount),
				fmt.Sprintf("%d", f.Holds),
				faaDec(f.CrossCountryTime),
				faaDec(f.NightTime),
				fmt.Sprintf("%d", f.LandingsDay),
				fmt.Sprintf("%d", f.LandingsNight),
				faaDec(f.TotalTime),
				faaRemarks(f, 58),
			}
			d.drawDataRow(colW, cells, faaSingleAlign, i)
			pt.add(f)
		}

		singleCells := func(t faaTotals) []string {
			return []string{
				faaDecTotal(t.solo),
				faaDecTotal(t.pic),
				faaDecTotal(t.sic),
				faaDecTotal(t.dual),
				faaDecTotal(t.instr),
				faaDecTotal(t.act),
				faaDecTotal(t.sim),
				fmt.Sprintf("%d", t.appr), fmt.Sprintf("%d", t.holds),
				faaDecTotal(t.xc),
				faaDecTotal(t.night),
				fmt.Sprintf("%d", t.ldgD), fmt.Sprintf("%d", t.ldgN),
				faaDecTotal(t.total), "",
			}
		}
		cumAfter := cum
		cumAfter.addAll(pt)
		d.drawTotalsRow(colW, 5, "TOTAL THIS PAGE", singleCells(pt), faaSingleAlign, false)
		d.drawTotalsRow(colW, 5, "TOTAL FROM PREVIOUS PAGES", singleCells(cum), faaSingleAlign, false)
		cum = cumAfter
		d.drawTotalsRow(colW, 5, "TOTAL TIME", singleCells(cum), faaSingleAlign, true)

		d.drawSignatureBlock()
	}
}
