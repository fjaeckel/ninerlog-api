package handlers

import (
	"fmt"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/gin-gonic/gin"
	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"
)

// ─────────────────────────────────────────────────────────────────────────────
// EASA — AMC1 FCL.050 layouts
// ─────────────────────────────────────────────────────────────────────────────

// The spread layout splits the AMC1 FCL.050 columns across two facing pages.
// Each flight produces one row on the left page and one row on the right page;
// printed double-sided, the bound spread reproduces the paper logbook. The
// single layout condenses all columns onto one landscape page per batch.

const easaRegulation = "EASA Part-FCL · AMC1 FCL.050"

// Left page: cols 1–12
var easaLeftGroups = []colGroup{
	{"", 1}, {"DEPARTURE", 2}, {"ARRIVAL", 2}, {"AIRCRAFT", 2},
	{"SINGLE-PILOT", 2}, {"MULTI-PILOT", 1}, {"TOTAL TIME", 1}, {"NAME PIC", 1},
}
var easaLeftSub = []string{
	"DATE", "PLACE", "TIME", "PLACE", "TIME", "TYPE", "REG",
	"SE", "ME", "TIME", "", "",
}
var easaLeftBaseW = []float64{
	18, 18, 14, 18, 14, 22, 22, 14, 14, 14, 16, 86,
}
var easaLeftAlign = []string{
	"C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "L",
}

// Right page: date (for cross-reference) + cols 13–24
var easaRightGroups = []colGroup{
	{"", 1}, {"LANDINGS", 2}, {"OPERATIONAL CONDITION TIME", 2},
	{"PILOT FUNCTION TIME", 4}, {"FSTD SESSION", 3}, {"REMARKS AND ENDORSEMENTS", 1},
}
var easaRightSub = []string{
	"DATE", "DAY", "NIGHT", "NIGHT", "IFR",
	"PIC", "CO-PILOT", "DUAL", "INSTRUCTOR",
	"DATE", "TYPE", "TIME", "",
}
var easaRightBaseW = []float64{
	18, 12, 12, 16, 16, 16, 16, 16, 16, 14, 20, 14, 82,
}
var easaRightAlign = []string{
	"C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "L",
}

// Single-page layout: all AMC1 FCL.050 columns condensed onto one landscape
// page. The FSTD date column is dropped (the session date is column 1).
var easaSingleGroups = []colGroup{
	{"", 1}, {"DEPARTURE", 2}, {"ARRIVAL", 2}, {"AIRCRAFT", 2},
	{"SINGLE-PILOT", 2}, {"MULTI-PILOT", 1}, {"TOTAL TIME", 1}, {"NAME PIC", 1},
	{"LANDINGS", 2}, {"OP. CONDITION", 2}, {"PILOT FUNCTION TIME", 4},
	{"FSTD", 2}, {"REMARKS", 1},
}
var easaSingleSub = []string{
	"DATE", "PLACE", "TIME", "PLACE", "TIME", "TYPE", "REG",
	"SE", "ME", "TIME", "", "",
	"DAY", "NGT", "NIGHT", "IFR",
	"PIC", "CO-P", "DUAL", "INST",
	"TYPE", "TIME", "",
}
var easaSingleBaseW = []float64{
	14, 12, 10, 12, 10, 14, 14, 9, 9, 9, 11, 26,
	7, 7, 10, 10, 10, 10, 10, 10, 13, 10, 30,
}
var easaSingleAlign = []string{
	"C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "L",
	"C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "L",
}

// easaRow holds the per-flight derived values shared by the layouts so
// left/right page totals can never drift apart.
type easaRow struct {
	f                            *models.Flight
	spSE, spME, mp               int
	ifr                          int
	fstdDate, fstdType, fstdTime string
	picName, remarks             string
}

func buildEASARows(page []*models.Flight, regToClass map[string]string, userName string, remarksMax int) []easaRow {
	rows := make([]easaRow, len(page))
	for i, f := range page {
		rd := easaRow{f: f}
		acClass := regToClass[strings.ToUpper(f.AircraftReg)]
		rd.spSE, rd.spME, rd.mp = flightrules.RowTimes(f, acClass)
		rd.ifr = flightrules.EffectiveIFRTime(f)
		rd.fstdDate, rd.fstdType, _ = flightrules.FSTDFields(f, "02.01", fmtDec)
		if rd.fstdDate != "" {
			rd.fstdTime = fmtDec(f.SimulatedFlightTime)
		}
		rd.picName = flightrules.DisplayPICName(f, userName)
		rd.remarks = truncRunes(flightrules.CombinedRemarks(f), remarksMax)
		rows[i] = rd
	}
	return rows
}

// easaFSTDTotal returns the FSTD minutes a flight contributes to totals.
func easaFSTDTotal(f *models.Flight) int {
	if f.FSTDType != nil && *f.FSTDType != "" {
		return f.SimulatedFlightTime
	}
	return 0
}

// easaTotals accumulates every numeric EASA column, shared by the left and
// right pages of a spread and by the single-page layout.
type easaTotals struct {
	se, me, mp, total                       int
	ldgD, ldgN                              int
	night, ifr, pic, sic, dual, instr, fstd int
}

func (t *easaTotals) add(rd easaRow) {
	f := rd.f
	t.se += rd.spSE
	t.me += rd.spME
	t.mp += rd.mp
	t.total += f.TotalTime
	t.ldgD += f.LandingsDay
	t.ldgN += f.LandingsNight
	t.night += f.NightTime
	t.ifr += rd.ifr
	t.pic += f.PICTime
	t.sic += f.SICTime
	t.dual += f.DualTime
	t.instr += f.DualGivenTime
	t.fstd += easaFSTDTotal(f)
}

func (t *easaTotals) addAll(o easaTotals) {
	t.se += o.se
	t.me += o.me
	t.mp += o.mp
	t.total += o.total
	t.ldgD += o.ldgD
	t.ldgN += o.ldgN
	t.night += o.night
	t.ifr += o.ifr
	t.pic += o.pic
	t.sic += o.sic
	t.dual += o.dual
	t.instr += o.instr
	t.fstd += o.fstd
}

// addBaseline opens the balance with the pilot's prior experience. se, me
// and fstd stay untouched.
func (t *easaTotals) addBaseline(b *models.FlightBaseline) {
	if !baselineApplies(b) {
		return
	}
	t.mp += b.MultiPilotMinutes
	t.total += b.TotalMinutes
	t.ldgD += b.LandingsDay
	t.ldgN += b.LandingsNight
	t.night += b.NightMinutes
	t.ifr += b.IFRMinutes
	t.pic += b.PICMinutes
	t.sic += b.SICMinutes
	t.dual += b.DualMinutes
	t.instr += b.DualGivenMinutes
}

// Value cells for the three totals rows, one builder per layout.
func easaLeftTotCells(t easaTotals) []string {
	return []string{fmtDec(t.se), fmtDec(t.me), fmtDec(t.mp), fmtDec(t.total), ""}
}

func easaRightTotCells(t easaTotals) []string {
	return []string{
		fmt.Sprintf("%d", t.ldgD), fmt.Sprintf("%d", t.ldgN),
		fmtDec(t.night), fmtDec(t.ifr),
		fmtDec(t.pic), fmtDec(t.sic),
		fmtDec(t.dual), fmtDec(t.instr),
		"", "", fmtDec(t.fstd), "",
	}
}

func easaSingleTotCells(t easaTotals) []string {
	return []string{
		fmtDec(t.se), fmtDec(t.me), fmtDec(t.mp), fmtDec(t.total), "",
		fmt.Sprintf("%d", t.ldgD), fmt.Sprintf("%d", t.ldgN),
		fmtDec(t.night), fmtDec(t.ifr),
		fmtDec(t.pic), fmtDec(t.sic), fmtDec(t.dual), fmtDec(t.instr),
		"", fmtDec(t.fstd), "",
	}
}

func generateEASAPDF(flights []*models.Flight, g pageGeometry, h *APIHandler, c *gin.Context, userID uuid.UUID, layout string, b *models.FlightBaseline, sigs map[uuid.UUID]*models.FlightSignature) *fpdf.Fpdf {
	aircraftList, _ := h.aircraftService.ListAircraft(c.Request.Context(), userID)
	regToClass := make(map[string]string)
	for _, ac := range aircraftList {
		if ac.AircraftClass != nil {
			regToClass[strings.ToUpper(ac.Registration)] = *ac.AircraftClass
		}
	}
	userName := h.getUserNameFromContext(c)
	return renderEASA(flights, g, regToClass, userName, layout, b, sigs)
}

// renderEASA performs the actual EASA PDF rendering. Extracted so tests can
// invoke it without a full APIHandler. `sigs` carries the instructor sign-off
// of each signed flight, keyed by flight ID.
func renderEASA(flights []*models.Flight, g pageGeometry, regToClass map[string]string, userName, layout string, b *models.FlightBaseline, sigs map[uuid.UUID]*models.FlightSignature) *fpdf.Fpdf {
	d := newDoc(g, easaRegulation, userName, certEASA)
	d.note = baselineFooterNote(b)
	d.sigs = sigs
	if layout == layoutSingle {
		renderEASASingle(d, flights, regToClass, userName, b)
	} else {
		renderEASASpread(d, flights, regToClass, userName, b)
	}
	addGrandSummaryPage(d, flights, b)
	return d.pdf
}

func renderEASASpread(d *pdfDoc, flights []*models.Flight, regToClass map[string]string, userName string, b *models.FlightBaseline) {
	g, pdf := d.g, d.pdf
	leftW := scaleWidths(easaLeftBaseW, g.usableWidth())
	rightW := scaleWidths(easaRightBaseW, g.usableWidth())

	rpp := g.logRowsPerPage()
	totalSpreads := (len(flights) + rpp - 1) / rpp
	spreadNum := 0

	// Filler page so duplex printing opens each spread as facing pages.
	if len(flights) > 0 {
		d.addBlankPage()
	}

	// Cumulative running total across all spreads, opened with the baseline.
	var cum easaTotals
	cum.addBaseline(b)

	for startIdx := 0; startIdx < len(flights); startIdx += rpp {
		endIdx := startIdx + rpp
		if endIdx > len(flights) {
			endIdx = len(flights)
		}
		spreadNum++
		rows := buildEASARows(flights[startIdx:endIdx], regToClass, userName, 38)

		var pt easaTotals
		for _, rd := range rows {
			pt.add(rd)
		}
		cumAfter := cum
		cumAfter.addAll(pt)

		// ── LEFT PAGE ───────────────────────────────────────────────────────
		d.startPage(fmt.Sprintf("Spread %d of %d %s Left", spreadNum, totalSpreads, emdash()))
		d.drawHeader(leftW, easaLeftGroups, easaLeftSub)

		pdf.SetFont("Helvetica", "", g.fontBody)
		for i, rd := range rows {
			f := rd.f
			cells := []string{
				f.Date.Format("02.01.06"),
				safeStr(f.DepartureICAO), fmtTime(f.OffBlockTime),
				safeStr(f.ArrivalICAO), fmtTime(f.OnBlockTime),
				f.AircraftType, f.AircraftReg,
				fmtDec(rd.spSE), fmtDec(rd.spME), fmtDec(rd.mp),
				fmtDec(f.TotalTime),
				rd.picName,
			}
			d.drawDataRow(leftW, cells, easaLeftAlign, i, f.ID)
		}
		d.drawTotalsRow(leftW, 7, "TOTAL THIS PAGE", easaLeftTotCells(pt), easaLeftAlign, false)
		d.drawTotalsRow(leftW, 7, "TOTAL FROM PREVIOUS PAGES", easaLeftTotCells(cum), easaLeftAlign, false)
		d.drawTotalsRow(leftW, 7, "TOTAL TIME", easaLeftTotCells(cumAfter), easaLeftAlign, true)

		// ── RIGHT PAGE ──────────────────────────────────────────────────────
		d.startPage(fmt.Sprintf("Spread %d of %d %s Right", spreadNum, totalSpreads, emdash()))
		d.drawHeader(rightW, easaRightGroups, easaRightSub)

		pdf.SetFont("Helvetica", "", g.fontBody)
		for i, rd := range rows {
			f := rd.f
			cells := []string{
				f.Date.Format("02.01.06"),
				fmt.Sprintf("%d", f.LandingsDay),
				fmt.Sprintf("%d", f.LandingsNight),
				fmtDec(f.NightTime), fmtDec(rd.ifr),
				fmtDec(f.PICTime), fmtDec(f.SICTime),
				fmtDec(f.DualTime), fmtDec(f.DualGivenTime),
				rd.fstdDate, rd.fstdType, rd.fstdTime,
				rd.remarks,
			}
			d.drawDataRow(rightW, cells, easaRightAlign, i, f.ID)
		}
		d.drawTotalsRow(rightW, 1, "TOTAL THIS PAGE", easaRightTotCells(pt), easaRightAlign, false)
		d.drawTotalsRow(rightW, 1, "FROM PREV PAGES", easaRightTotCells(cum), easaRightAlign, false)
		d.drawTotalsRow(rightW, 1, "TOTAL TIME", easaRightTotCells(cumAfter), easaRightAlign, true)

		cum = cumAfter
		d.drawSignatureBlock()
	}

	// Filler page so the totals summary starts on its own sheet.
	if len(flights) > 0 {
		d.addBlankPage()
	}
}

func renderEASASingle(d *pdfDoc, flights []*models.Flight, regToClass map[string]string, userName string, b *models.FlightBaseline) {
	g, pdf := d.g, d.pdf
	colW := scaleWidths(easaSingleBaseW, g.usableWidth())

	rpp := g.logRowsPerPage()
	totalPages := (len(flights) + rpp - 1) / rpp
	pageNum := 0

	// Cumulative running total, opened with the baseline.
	var cum easaTotals
	cum.addBaseline(b)

	for startIdx := 0; startIdx < len(flights); startIdx += rpp {
		endIdx := startIdx + rpp
		if endIdx > len(flights) {
			endIdx = len(flights)
		}
		pageNum++
		rows := buildEASARows(flights[startIdx:endIdx], regToClass, userName, 26)

		d.startPage(fmt.Sprintf("Logbook Page %d of %d", pageNum, totalPages))
		d.drawHeader(colW, easaSingleGroups, easaSingleSub)

		var pt easaTotals
		pdf.SetFont("Helvetica", "", g.fontBody)
		for i, rd := range rows {
			f := rd.f
			cells := []string{
				f.Date.Format("02.01.06"),
				safeStr(f.DepartureICAO), fmtTime(f.OffBlockTime),
				safeStr(f.ArrivalICAO), fmtTime(f.OnBlockTime),
				f.AircraftType, f.AircraftReg,
				fmtDec(rd.spSE), fmtDec(rd.spME), fmtDec(rd.mp),
				fmtDec(f.TotalTime),
				truncRunes(rd.picName, 16),
				fmt.Sprintf("%d", f.LandingsDay),
				fmt.Sprintf("%d", f.LandingsNight),
				fmtDec(f.NightTime), fmtDec(rd.ifr),
				fmtDec(f.PICTime), fmtDec(f.SICTime),
				fmtDec(f.DualTime), fmtDec(f.DualGivenTime),
				rd.fstdType, rd.fstdTime,
				rd.remarks,
			}
			d.drawDataRow(colW, cells, easaSingleAlign, i, f.ID)
			pt.add(rd)
		}

		cumAfter := cum
		cumAfter.addAll(pt)
		d.drawTotalsRow(colW, 7, "TOTAL THIS PAGE", easaSingleTotCells(pt), easaSingleAlign, false)
		d.drawTotalsRow(colW, 7, "TOTAL FROM PREVIOUS PAGES", easaSingleTotCells(cum), easaSingleAlign, false)
		cum = cumAfter
		d.drawTotalsRow(colW, 7, "TOTAL TIME", easaSingleTotCells(cum), easaSingleAlign, true)

		d.drawSignatureBlock()
	}
}
