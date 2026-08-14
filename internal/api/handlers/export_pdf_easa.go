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

func generateEASAPDF(flights []*models.Flight, g pageGeometry, h *APIHandler, c *gin.Context, userID uuid.UUID, layout string) *fpdf.Fpdf {
	aircraftList, _ := h.aircraftService.ListAircraft(c.Request.Context(), userID)
	regToClass := make(map[string]string)
	for _, ac := range aircraftList {
		if ac.AircraftClass != nil {
			regToClass[strings.ToUpper(ac.Registration)] = *ac.AircraftClass
		}
	}
	userName := h.getUserNameFromContext(c)
	return renderEASA(flights, g, regToClass, userName, layout)
}

// renderEASA performs the actual EASA PDF rendering. Extracted so tests can
// invoke it without a full APIHandler.
func renderEASA(flights []*models.Flight, g pageGeometry, regToClass map[string]string, userName, layout string) *fpdf.Fpdf {
	d := newDoc(g, easaRegulation, userName, certEASA)
	if layout == layoutSingle {
		renderEASASingle(d, flights, regToClass, userName)
	} else {
		renderEASASpread(d, flights, regToClass, userName)
	}
	addGrandSummaryPage(d, flights)
	return d.pdf
}

func renderEASASpread(d *pdfDoc, flights []*models.Flight, regToClass map[string]string, userName string) {
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

	// Cumulative running totals across all spreads. Left- and right-page
	// columns differ, so they are tracked separately.
	var (
		cumLSE, cumLME, cumLMP, cumLTotal                                   int
		cumRLdgD, cumRLdgN                                                  int
		cumRNight, cumRIFR, cumRPIC, cumRSIC, cumRDual, cumRInstr, cumRFSTD int
	)

	for startIdx := 0; startIdx < len(flights); startIdx += rpp {
		endIdx := startIdx + rpp
		if endIdx > len(flights) {
			endIdx = len(flights)
		}
		spreadNum++
		rows := buildEASARows(flights[startIdx:endIdx], regToClass, userName, 38)

		// ── LEFT PAGE ───────────────────────────────────────────────────────
		d.startPage(fmt.Sprintf("Spread %d of %d %s Left", spreadNum, totalSpreads, emdash()))
		d.drawHeader(leftW, easaLeftGroups, easaLeftSub)

		var lTotal, lSE, lME, lMP int
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
			d.drawDataRow(leftW, cells, easaLeftAlign, i)
			lTotal += f.TotalTime
			lSE += rd.spSE
			lME += rd.spME
			lMP += rd.mp
		}
		d.drawTotalsRow(leftW, 7, "TOTAL THIS PAGE", []string{
			fmtDec(lSE), fmtDec(lME), fmtDec(lMP), fmtDec(lTotal), "",
		}, easaLeftAlign, false)
		d.drawTotalsRow(leftW, 7, "TOTAL FROM PREVIOUS PAGES", []string{
			fmtDec(cumLSE), fmtDec(cumLME), fmtDec(cumLMP), fmtDec(cumLTotal), "",
		}, easaLeftAlign, false)
		cumLSE += lSE
		cumLME += lME
		cumLMP += lMP
		cumLTotal += lTotal
		d.drawTotalsRow(leftW, 7, "TOTAL TIME", []string{
			fmtDec(cumLSE), fmtDec(cumLME), fmtDec(cumLMP), fmtDec(cumLTotal), "",
		}, easaLeftAlign, true)

		// ── RIGHT PAGE ──────────────────────────────────────────────────────
		d.startPage(fmt.Sprintf("Spread %d of %d %s Right", spreadNum, totalSpreads, emdash()))
		d.drawHeader(rightW, easaRightGroups, easaRightSub)

		var rNight, rIFR, rPIC, rSIC, rDual, rInstr, rFSTD int
		var rLdgD, rLdgN int
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
			d.drawDataRow(rightW, cells, easaRightAlign, i)
			rLdgD += f.LandingsDay
			rLdgN += f.LandingsNight
			rNight += f.NightTime
			rIFR += rd.ifr
			rPIC += f.PICTime
			rSIC += f.SICTime
			rDual += f.DualTime
			rInstr += f.DualGivenTime
			rFSTD += easaFSTDTotal(f)
		}
		d.drawTotalsRow(rightW, 1, "TOTAL THIS PAGE", []string{
			fmt.Sprintf("%d", rLdgD), fmt.Sprintf("%d", rLdgN),
			fmtDec(rNight), fmtDec(rIFR),
			fmtDec(rPIC), fmtDec(rSIC),
			fmtDec(rDual), fmtDec(rInstr),
			"", "", fmtDec(rFSTD), "",
		}, easaRightAlign, false)
		d.drawTotalsRow(rightW, 1, "FROM PREV PAGES", []string{
			fmt.Sprintf("%d", cumRLdgD), fmt.Sprintf("%d", cumRLdgN),
			fmtDec(cumRNight), fmtDec(cumRIFR),
			fmtDec(cumRPIC), fmtDec(cumRSIC),
			fmtDec(cumRDual), fmtDec(cumRInstr),
			"", "", fmtDec(cumRFSTD), "",
		}, easaRightAlign, false)
		cumRLdgD += rLdgD
		cumRLdgN += rLdgN
		cumRNight += rNight
		cumRIFR += rIFR
		cumRPIC += rPIC
		cumRSIC += rSIC
		cumRDual += rDual
		cumRInstr += rInstr
		cumRFSTD += rFSTD
		d.drawTotalsRow(rightW, 1, "TOTAL TIME", []string{
			fmt.Sprintf("%d", cumRLdgD), fmt.Sprintf("%d", cumRLdgN),
			fmtDec(cumRNight), fmtDec(cumRIFR),
			fmtDec(cumRPIC), fmtDec(cumRSIC),
			fmtDec(cumRDual), fmtDec(cumRInstr),
			"", "", fmtDec(cumRFSTD), "",
		}, easaRightAlign, true)

		d.drawSignatureBlock()
	}

	// Filler page so the totals summary starts on its own sheet.
	if len(flights) > 0 {
		d.addBlankPage()
	}
}

func renderEASASingle(d *pdfDoc, flights []*models.Flight, regToClass map[string]string, userName string) {
	g, pdf := d.g, d.pdf
	colW := scaleWidths(easaSingleBaseW, g.usableWidth())

	rpp := g.logRowsPerPage()
	totalPages := (len(flights) + rpp - 1) / rpp
	pageNum := 0

	var (
		cumSE, cumME, cumMP, cumTotal                                int
		cumLdgD, cumLdgN                                             int
		cumNight, cumIFR, cumPIC, cumSIC, cumDual, cumInstr, cumFSTD int
	)

	for startIdx := 0; startIdx < len(flights); startIdx += rpp {
		endIdx := startIdx + rpp
		if endIdx > len(flights) {
			endIdx = len(flights)
		}
		pageNum++
		rows := buildEASARows(flights[startIdx:endIdx], regToClass, userName, 26)

		d.startPage(fmt.Sprintf("Logbook Page %d of %d", pageNum, totalPages))
		d.drawHeader(colW, easaSingleGroups, easaSingleSub)

		var pSE, pME, pMP, pTotal int
		var pLdgD, pLdgN int
		var pNight, pIFR, pPIC, pSIC, pDual, pInstr, pFSTD int
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
			d.drawDataRow(colW, cells, easaSingleAlign, i)
			pSE += rd.spSE
			pME += rd.spME
			pMP += rd.mp
			pTotal += f.TotalTime
			pLdgD += f.LandingsDay
			pLdgN += f.LandingsNight
			pNight += f.NightTime
			pIFR += rd.ifr
			pPIC += f.PICTime
			pSIC += f.SICTime
			pDual += f.DualTime
			pInstr += f.DualGivenTime
			pFSTD += easaFSTDTotal(f)
		}

		totCells := func(se, me, mp, tot, ldgD, ldgN, night, ifr, pic, sic, dual, instr, fstd int) []string {
			return []string{
				fmtDec(se), fmtDec(me), fmtDec(mp), fmtDec(tot), "",
				fmt.Sprintf("%d", ldgD), fmt.Sprintf("%d", ldgN),
				fmtDec(night), fmtDec(ifr),
				fmtDec(pic), fmtDec(sic), fmtDec(dual), fmtDec(instr),
				"", fmtDec(fstd), "",
			}
		}
		d.drawTotalsRow(colW, 7, "TOTAL THIS PAGE",
			totCells(pSE, pME, pMP, pTotal, pLdgD, pLdgN, pNight, pIFR, pPIC, pSIC, pDual, pInstr, pFSTD),
			easaSingleAlign, false)
		d.drawTotalsRow(colW, 7, "TOTAL FROM PREVIOUS PAGES",
			totCells(cumSE, cumME, cumMP, cumTotal, cumLdgD, cumLdgN, cumNight, cumIFR, cumPIC, cumSIC, cumDual, cumInstr, cumFSTD),
			easaSingleAlign, false)
		cumSE += pSE
		cumME += pME
		cumMP += pMP
		cumTotal += pTotal
		cumLdgD += pLdgD
		cumLdgN += pLdgN
		cumNight += pNight
		cumIFR += pIFR
		cumPIC += pPIC
		cumSIC += pSIC
		cumDual += pDual
		cumInstr += pInstr
		cumFSTD += pFSTD
		d.drawTotalsRow(colW, 7, "TOTAL TIME",
			totCells(cumSE, cumME, cumMP, cumTotal, cumLdgD, cumLdgN, cumNight, cumIFR, cumPIC, cumSIC, cumDual, cumInstr, cumFSTD),
			easaSingleAlign, true)

		d.drawSignatureBlock()
	}
}
