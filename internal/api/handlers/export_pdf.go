package handlers

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/fjaeckel/ninerlog-api/pkg/duration"
	"github.com/gin-gonic/gin"
	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"
)

// ─────────────────────────────────────────────────────────────────────────────
// Page geometry
// ─────────────────────────────────────────────────────────────────────────────

// pageGeometry describes the printable area of a landscape page. All values in mm.
type pageGeometry struct {
	sizeName  string  // "A4", "A5", "Letter"
	width     float64 // landscape width (long edge)
	height    float64 // landscape height (short edge)
	marginLR  float64 // left/right margin
	marginTB  float64 // top/bottom margin
	titleH    float64 // title strip height
	rowH      float64 // data row height
	headerH   float64 // header row height
	fontTitle float64
	fontHdr   float64
	fontBody  float64
	fontFoot  float64
}

func (g pageGeometry) usableWidth() float64  { return g.width - 2*g.marginLR }
func (g pageGeometry) usableHeight() float64 { return g.height - 2*g.marginTB - g.titleH }

// rowsPerPage returns how many data rows fit on one page (excluding 2 header rows
// and a single page-totals row).
func (g pageGeometry) rowsPerPage() int {
	avail := g.usableHeight() - 2*g.headerH - g.rowH // minus 2 header rows + 1 totals row
	n := int(avail / g.rowH)
	if n < 5 {
		n = 5
	}
	return n
}

// easaRowsPerPage is like rowsPerPage but reserves space for three totals rows
// (this page / previous pages / total time) instead of one.
func (g pageGeometry) easaRowsPerPage() int {
	avail := g.usableHeight() - 2*g.headerH - 3*g.rowH
	n := int(avail / g.rowH)
	if n < 5 {
		n = 5
	}
	return n
}

func geometryFor(sizeName string) pageGeometry {
	// Base A4-landscape geometry.
	base := pageGeometry{
		sizeName: "A4",
		width:    297, height: 210,
		marginLR: 10, marginTB: 8, titleH: 8,
		rowH: 5, headerH: 4.5,
		fontTitle: 11, fontHdr: 5, fontBody: 5, fontFoot: 7,
	}
	switch strings.ToLower(sizeName) {
	case "a5":
		// A5 landscape: 210 × 148 mm. Scale fonts/rows down proportionally.
		s := 210.0 / 297.0
		return pageGeometry{
			sizeName: "A5",
			width:    210, height: 148,
			marginLR: 7, marginTB: 6, titleH: 6,
			rowH: base.rowH * s, headerH: base.headerH * s,
			fontTitle: 9, fontHdr: 4, fontBody: 4, fontFoot: 6,
		}
	case "letter":
		// US Letter landscape: 279.4 × 215.9 mm.
		return pageGeometry{
			sizeName: "Letter",
			width:    279.4, height: 215.9,
			marginLR: 10, marginTB: 8, titleH: 8,
			rowH: 5, headerH: 4.5,
			fontTitle: 11, fontHdr: 5, fontBody: 5, fontFoot: 7,
		}
	case "a4":
		fallthrough
	default:
		return base
	}
}

// fpdfPageSize returns the fpdf SizeType for a given geometry. We always render
// in landscape, so width is the long edge.
func (g pageGeometry) fpdfSize() fpdf.SizeType {
	// Heights here are the *portrait* dimensions; fpdf flips them when "L" is used.
	// We construct the doc with custom size to support all three sizes uniformly.
	return fpdf.SizeType{Wd: g.height, Ht: g.width}
}

// scaleWidths scales a slice of base column widths to fit `target` mm exactly.
func scaleWidths(base []float64, target float64) []float64 {
	var sum float64
	for _, w := range base {
		sum += w
	}
	if sum <= 0 {
		return base
	}
	scale := target / sum
	out := make([]float64, len(base))
	for i, w := range base {
		out[i] = w * scale
	}
	return out
}

// newPDF constructs a new fpdf document for the given geometry.
func newPDF(g pageGeometry) *fpdf.Fpdf {
	pdf := fpdf.NewCustom(&fpdf.InitType{
		OrientationStr: "L",
		UnitStr:        "mm",
		Size:           g.fpdfSize(),
	})
	pdf.SetMargins(g.marginLR, g.marginTB, g.marginLR)
	pdf.SetAutoPageBreak(true, g.marginTB)
	return pdf
}

// ─────────────────────────────────────────────────────────────────────────────
// Handler
// ─────────────────────────────────────────────────────────────────────────────

// ExportFlightsPDF implements GET /exports/pdf
func (h *APIHandler) ExportFlightsPDF(c *gin.Context, params generated.ExportFlightsPDFParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	flights, err := h.flightService.ListFlights(c.Request.Context(), userID, nil)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve flights")
		return
	}
	// Populate crew members so DisplayPICName can resolve the instructor
	// (PIC of record on Dual flights) from the flight_crew_members table.
	h.attachCrewMembers(c.Request.Context(), flights)

	if params.LogbookLicenseId != nil {
		licenseID := uuid.UUID(*params.LogbookLicenseId)
		classRatings, err := h.classRatingService.ListClassRatings(c.Request.Context(), licenseID, userID)
		if err == nil && len(classRatings) > 0 {
			allowedClasses := make(map[string]bool)
			for _, cr := range classRatings {
				allowedClasses[string(cr.ClassType)] = true
			}
			aircraftList, _ := h.aircraftService.ListAircraft(c.Request.Context(), userID)
			regToClass := make(map[string]string)
			for _, ac := range aircraftList {
				if ac.AircraftClass != nil {
					regToClass[strings.ToUpper(ac.Registration)] = *ac.AircraftClass
				}
			}
			var filtered []*models.Flight
			for _, f := range flights {
				acClass := regToClass[strings.ToUpper(f.AircraftReg)]
				if acClass != "" && allowedClasses[acClass] {
					filtered = append(filtered, f)
				}
			}
			flights = filtered
		}
	}

	sort.Slice(flights, func(i, j int) bool {
		return flights[i].Date.Before(flights[j].Date)
	})

	format := "easa"
	if params.Format != nil {
		format = string(*params.Format)
	}
	pageSize := "a4"
	if params.PageSize != nil {
		pageSize = string(*params.PageSize)
	}
	geom := geometryFor(pageSize)

	var pdf *fpdf.Fpdf
	switch format {
	case "faa":
		pdf = generateFAAPDF(flights, geom)
	case "summary":
		pdf = generateSummaryPDF(flights, geom)
	default:
		pdf = generateEASAPDF(flights, geom, h, c, userID)
	}
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=ninerlog_%s_%s_%s.pdf",
		format, strings.ToLower(geom.sizeName), time.Now().Format("2006-01-02")))
	if err := pdf.Output(c.Writer); err != nil {
		log.Printf("pdf export output error: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// EASA — book-style two-page spread
// ─────────────────────────────────────────────────────────────────────────────

// EASA columns are split across two facing pages. Each `flight` row produces
// one row on the left page and one row on the right page; when the document is
// printed double-sided, the bound spread reproduces the AMC1 FCL.050 layout.

// Left page: cols 1–12
var easaLeftHeaders1 = []string{
	"", "DEPARTURE", "", "ARRIVAL", "", "AIRCRAFT", "",
	"SINGLE PILOT", "", "MULTI", "TOTAL", "PIC",
}
var easaLeftHeaders2 = []string{
	"DATE", "PLACE", "TIME", "PLACE", "TIME", "TYPE", "REG",
	"SE", "ME", "PILOT", "TIME", "NAME",
}
var easaLeftBaseW = []float64{
	18, 18, 14, 18, 14, 22, 22, 14, 14, 14, 16, 86,
}
var easaLeftAlign = []string{
	"C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "L",
}

// Right page: date (for cross-reference) + cols 13–24
var easaRightHeaders1 = []string{
	"", "LANDINGS", "", "OPERATIONAL", "CONDITION", "PILOT FUNCTION", "", "", "",
	"FSTD", "", "", "",
}
var easaRightHeaders2 = []string{
	"DATE", "DAY", "NIGHT", "NIGHT", "IFR",
	"PIC", "CO-PLT", "DUAL", "INSTR",
	"DATE", "TYPE", "TIME", "REMARKS",
}
var easaRightBaseW = []float64{
	18, 12, 12, 16, 16, 16, 16, 16, 16, 14, 22, 14, 82,
}
var easaRightAlign = []string{
	"C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "L",
}

func generateEASAPDF(flights []*models.Flight, g pageGeometry, h *APIHandler, c *gin.Context, userID uuid.UUID) *fpdf.Fpdf {
	aircraftList, _ := h.aircraftService.ListAircraft(c.Request.Context(), userID)
	regToClass := make(map[string]string)
	for _, ac := range aircraftList {
		if ac.AircraftClass != nil {
			regToClass[strings.ToUpper(ac.Registration)] = *ac.AircraftClass
		}
	}
	userName := h.getUserNameFromContext(c)
	return renderEASA(flights, g, regToClass, userName)
}

// renderEASA performs the actual EASA PDF rendering. Extracted so tests can
// invoke it without a full APIHandler.
func renderEASA(flights []*models.Flight, g pageGeometry, regToClass map[string]string, userName string) *fpdf.Fpdf {
	pdf := newPDF(g)
	tr := pdf.UnicodeTranslatorFromDescriptor("") // CP1252 mapper for em-dash etc.

	leftW := scaleWidths(easaLeftBaseW, g.usableWidth())
	rightW := scaleWidths(easaRightBaseW, g.usableWidth())

	rpp := g.easaRowsPerPage()
	spreadNum := 0

	// Cumulative running totals across all spreads. Left- and right-page
	// columns are different, so they are tracked separately.
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
		page := flights[startIdx:endIdx]

		// Build per-row derived values (shared between left/right pages so totals match)
		type rowData struct {
			f                            *models.Flight
			spSE, spME, mp               int
			fstdDate, fstdType, fstdTime string
			picName, remarks             string
		}
		rows := make([]rowData, len(page))
		for i, f := range page {
			rd := rowData{f: f}
			acClass := regToClass[strings.ToUpper(f.AircraftReg)]
			rd.spSE, rd.spME, rd.mp = flightrules.RowTimes(f, acClass)
			rd.fstdDate, rd.fstdType, _ = flightrules.FSTDFields(f, "02.01", fmtDec)
			if rd.fstdDate != "" {
				rd.fstdTime = fmtDec(f.SimulatedFlightTime)
			}
			rd.picName = flightrules.DisplayPICName(f, userName)
			rem := flightrules.CombinedRemarks(f)
			if len([]rune(rem)) > 38 {
				rem = string([]rune(rem)[:35]) + "..."
			}
			rd.remarks = rem
			rows[i] = rd
		}

		// ── LEFT PAGE ───────────────────────────────────────────────────────
		pdf.AddPage()
		drawTitle(pdf, g, tr, fmt.Sprintf("Pilot Logbook (EASA) %s Spread %d - Left", emdash(), spreadNum))
		drawHeaderRow(pdf, g, leftW, easaLeftHeaders1, easaLeftHeaders2)

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
			drawDataRow(pdf, g, leftW, cells, easaLeftAlign, i, tr)
			lTotal += f.TotalTime
			lSE += rd.spSE
			lME += rd.spME
			lMP += rd.mp
		}
		drawTotalsRow(pdf, g, leftW, []string{
			"", "", "", "", "", "", "TOTAL THIS PAGE",
			fmtDec(lSE), fmtDec(lME), fmtDec(lMP), fmtDec(lTotal), "",
		}, easaLeftAlign, tr)
		drawTotalsRow(pdf, g, leftW, []string{
			"", "", "", "", "", "", "FROM PREV PAGES",
			fmtDec(cumLSE), fmtDec(cumLME), fmtDec(cumLMP), fmtDec(cumLTotal), "",
		}, easaLeftAlign, tr)
		cumLSE += lSE
		cumLME += lME
		cumLMP += lMP
		cumLTotal += lTotal
		drawTotalsRow(pdf, g, leftW, []string{
			"", "", "", "", "", "", "TOTAL TIME",
			fmtDec(cumLSE), fmtDec(cumLME), fmtDec(cumLMP), fmtDec(cumLTotal), "",
		}, easaLeftAlign, tr)

		// ── RIGHT PAGE ──────────────────────────────────────────────────────
		pdf.AddPage()
		drawTitle(pdf, g, tr, fmt.Sprintf("Pilot Logbook (EASA) %s Spread %d - Right", emdash(), spreadNum))
		drawHeaderRow(pdf, g, rightW, easaRightHeaders1, easaRightHeaders2)

		var rNight, rIFR, rPIC, rSIC, rDual, rInstr, rFSTD int
		var rLdgD, rLdgN int
		pdf.SetFont("Helvetica", "", g.fontBody)
		for i, rd := range rows {
			f := rd.f
			ifrTime := flightrules.EffectiveIFRTime(f)
			cells := []string{
				f.Date.Format("02.01.06"),
				fmt.Sprintf("%d", f.LandingsDay),
				fmt.Sprintf("%d", f.LandingsNight),
				fmtDec(f.NightTime), fmtDec(ifrTime),
				fmtDec(f.PICTime), fmtDec(f.SICTime),
				fmtDec(f.DualTime), fmtDec(f.DualGivenTime),
				rd.fstdDate, rd.fstdType, rd.fstdTime,
				rd.remarks,
			}
			drawDataRow(pdf, g, rightW, cells, easaRightAlign, i, tr)
			rLdgD += f.LandingsDay
			rLdgN += f.LandingsNight
			rNight += f.NightTime
			rIFR += ifrTime
			rPIC += f.PICTime
			rSIC += f.SICTime
			rDual += f.DualTime
			rInstr += f.DualGivenTime
			if f.FSTDType != nil && *f.FSTDType != "" {
				rFSTD += f.SimulatedFlightTime
			}
		}
		drawTotalsRow(pdf, g, rightW, []string{
			"TOTAL THIS PAGE",
			fmt.Sprintf("%d", rLdgD), fmt.Sprintf("%d", rLdgN),
			fmtDec(rNight), fmtDec(rIFR),
			fmtDec(rPIC), fmtDec(rSIC),
			fmtDec(rDual), fmtDec(rInstr),
			"", "", fmtDec(rFSTD), "",
		}, easaRightAlign, tr)
		drawTotalsRow(pdf, g, rightW, []string{
			"FROM PREV PAGES",
			fmt.Sprintf("%d", cumRLdgD), fmt.Sprintf("%d", cumRLdgN),
			fmtDec(cumRNight), fmtDec(cumRIFR),
			fmtDec(cumRPIC), fmtDec(cumRSIC),
			fmtDec(cumRDual), fmtDec(cumRInstr),
			"", "", fmtDec(cumRFSTD), "",
		}, easaRightAlign, tr)
		cumRLdgD += rLdgD
		cumRLdgN += rLdgN
		cumRNight += rNight
		cumRIFR += rIFR
		cumRPIC += rPIC
		cumRSIC += rSIC
		cumRDual += rDual
		cumRInstr += rInstr
		cumRFSTD += rFSTD
		drawTotalsRow(pdf, g, rightW, []string{
			"TOTAL TIME",
			fmt.Sprintf("%d", cumRLdgD), fmt.Sprintf("%d", cumRLdgN),
			fmtDec(cumRNight), fmtDec(cumRIFR),
			fmtDec(cumRPIC), fmtDec(cumRSIC),
			fmtDec(cumRDual), fmtDec(cumRInstr),
			"", "", fmtDec(cumRFSTD), "",
		}, easaRightAlign, tr)
	}

	addGrandSummaryPage(pdf, flights, g, tr)
	return pdf
}

// ─────────────────────────────────────────────────────────────────────────────
// FAA — single-landscape layout, scaled to page
// ─────────────────────────────────────────────────────────────────────────────

var faaBaseW = []float64{
	16, 16, 18, 14, 14, 12, 12, 12, 12, 12, 12, 12, 12, 12, 9, 9, 9, 8, 12, 44,
}
var faaHeaders = []string{
	"DATE", "A/C TYPE", "A/C IDENT", "FROM", "TO",
	"SOLO", "PIC", "SIC", "DUAL", "INSTR",
	"ACT INST", "SIM INST", "XC", "NIGHT",
	"D LDG", "N LDG", "APPR", "HOLD",
	"TOTAL", "REMARKS/ENDORSEMENTS",
}
var faaAlign = []string{
	"C", "C", "C", "C", "C",
	"C", "C", "C", "C", "C",
	"C", "C", "C", "C",
	"C", "C", "C", "C",
	"C", "L",
}

func generateFAAPDF(flights []*models.Flight, g pageGeometry) *fpdf.Fpdf {
	pdf := newPDF(g)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	colW := scaleWidths(faaBaseW, g.usableWidth())
	rpp := g.rowsPerPage()
	pageNum := 0

	for startIdx := 0; startIdx < len(flights); startIdx += rpp {
		endIdx := startIdx + rpp
		if endIdx > len(flights) {
			endIdx = len(flights)
		}
		pageNum++

		pdf.AddPage()
		drawTitle(pdf, g, tr, fmt.Sprintf("Pilot Logbook (FAA) %s Page %d", emdash(), pageNum))

		// FAA only has one header row — pass empty group row to keep layout simple.
		drawHeaderRow(pdf, g, colW, nil, faaHeaders)

		var pTotal, pPIC, pSIC, pSolo, pDual, pInstr int
		var pAct, pSim, pXC, pNight int
		var pLdgD, pLdgN, pAppr, pHolds int

		pdf.SetFont("Helvetica", "", g.fontBody)
		for i, f := range flights[startIdx:endIdx] {
			// Note: FAA PDF previously used bare "IPC"/"FR" suffixes while
			// FAA CSV used "[IPC]"/"[FR]". Centralised CombinedRemarks
			// produces "[IPC]"/"[FR]" — both formats now match.
			remarks := flightrules.CombinedRemarks(f, flightrules.FlagIPC, flightrules.FlagFlightReview)
			if len([]rune(remarks)) > 60 {
				remarks = string([]rune(remarks)[:57]) + "..."
			}

			cells := []string{
				f.Date.Format("01/02/06"),
				f.AircraftType, f.AircraftReg,
				safeStr(f.DepartureICAO), safeStr(f.ArrivalICAO),
				duration.FormatDecimal(f.SoloTime),
				duration.FormatDecimal(f.PICTime),
				duration.FormatDecimal(f.SICTime),
				duration.FormatDecimal(f.DualTime),
				duration.FormatDecimal(f.DualGivenTime),
				duration.FormatDecimal(f.ActualInstrumentTime),
				duration.FormatDecimal(f.SimulatedInstrumentTime),
				duration.FormatDecimal(f.CrossCountryTime),
				duration.FormatDecimal(f.NightTime),
				fmt.Sprintf("%d", f.LandingsDay),
				fmt.Sprintf("%d", f.LandingsNight),
				fmt.Sprintf("%d", f.ApproachesCount),
				fmt.Sprintf("%d", f.Holds),
				duration.FormatDecimal(f.TotalTime),
				remarks,
			}
			drawDataRow(pdf, g, colW, cells, faaAlign, i, tr)

			pTotal += f.TotalTime
			pPIC += f.PICTime
			pSIC += f.SICTime
			pSolo += f.SoloTime
			pDual += f.DualTime
			pInstr += f.DualGivenTime
			pAct += f.ActualInstrumentTime
			pSim += f.SimulatedInstrumentTime
			pXC += f.CrossCountryTime
			pNight += f.NightTime
			pLdgD += f.LandingsDay
			pLdgN += f.LandingsNight
			pAppr += f.ApproachesCount
			pHolds += f.Holds
		}

		drawTotalsRow(pdf, g, colW, []string{
			"", "", "", "", "TOTAL",
			duration.FormatDecimal(pSolo),
			duration.FormatDecimal(pPIC),
			duration.FormatDecimal(pSIC),
			duration.FormatDecimal(pDual),
			duration.FormatDecimal(pInstr),
			duration.FormatDecimal(pAct),
			duration.FormatDecimal(pSim),
			duration.FormatDecimal(pXC),
			duration.FormatDecimal(pNight),
			fmt.Sprintf("%d", pLdgD),
			fmt.Sprintf("%d", pLdgN),
			fmt.Sprintf("%d", pAppr),
			fmt.Sprintf("%d", pHolds),
			duration.FormatDecimal(pTotal),
			"",
		}, faaAlign, tr)
	}

	addGrandSummaryPage(pdf, flights, g, tr)
	return pdf
}

// ─────────────────────────────────────────────────────────────────────────────
// Summary PDF
// ─────────────────────────────────────────────────────────────────────────────

func generateSummaryPDF(flights []*models.Flight, g pageGeometry) *fpdf.Fpdf {
	pdf := newPDF(g)
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	addGrandSummaryPage(pdf, flights, g, tr)
	return pdf
}

func addGrandSummaryPage(pdf *fpdf.Fpdf, flights []*models.Flight, g pageGeometry, tr func(string) string) {
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", g.fontTitle+1)
	pdf.CellFormat(0, 8, tr("TOTALS SUMMARY"), "", 1, "C", false, 0, "")
	pdf.Ln(4)

	var grandTotal, grandPIC, grandDual, grandNight, grandIFR, grandSolo, grandXC int
	var grandSIC, grandDualGiven, grandMP int
	var grandLdgDay, grandLdgNight, grandFlights int
	for _, f := range flights {
		grandTotal += f.TotalTime
		grandPIC += f.PICTime
		grandDual += f.DualTime
		grandNight += f.NightTime
		grandIFR += flightrules.EffectiveIFRTime(f)
		grandSolo += f.SoloTime
		grandXC += f.CrossCountryTime
		grandSIC += f.SICTime
		grandDualGiven += f.DualGivenTime
		grandMP += f.MultiPilotTime
		grandLdgDay += f.LandingsDay
		grandLdgNight += f.LandingsNight
		grandFlights++
	}

	summaryW := g.usableWidth() * 0.55
	valW := g.usableWidth() * 0.25
	x0 := g.marginLR + (g.usableWidth()-summaryW-valW)/2

	rows := []struct{ label, value string }{
		{"Total Flights", fmt.Sprintf("%d", grandFlights)},
		{"Total Block Time", fmtDec(grandTotal)},
		{"PIC Time", fmtDec(grandPIC)},
		{"SIC / Co-Pilot Time", fmtDec(grandSIC)},
		{"Dual Received", fmtDec(grandDual)},
		{"Dual / Instruction Given", fmtDec(grandDualGiven)},
		{"Solo Time", fmtDec(grandSolo)},
		{"Night Time", fmtDec(grandNight)},
		{"IFR Time", fmtDec(grandIFR)},
		{"Cross-Country Time", fmtDec(grandXC)},
		{"Multi-Pilot Time", fmtDec(grandMP)},
		{"Day Landings", fmt.Sprintf("%d", grandLdgDay)},
		{"Night Landings", fmt.Sprintf("%d", grandLdgNight)},
		{"Total Landings", fmt.Sprintf("%d", grandLdgDay+grandLdgNight)},
	}
	for i, row := range rows {
		pdf.SetX(x0)
		// Zebra stripe summary too for consistency
		fill := i%2 == 1
		if fill {
			pdf.SetFillColor(245, 245, 245)
		}
		pdf.SetFont("Helvetica", "", g.fontTitle-1)
		pdf.CellFormat(summaryW, 7, tr(row.label), "1", 0, "L", fill, 0, "")
		pdf.SetFont("Helvetica", "B", g.fontTitle-1)
		pdf.CellFormat(valW, 7, tr(row.value), "1", 1, "R", fill, 0, "")
	}

	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", g.fontFoot)
	pdf.CellFormat(0, 4, tr(fmt.Sprintf("Generated by NinerLog on %s %s https://github.com/fjaeckel/ninerlog",
		time.Now().Format("02 Jan 2006 15:04 UTC"), emdash())), "", 1, "C", false, 0, "")
}

// ─────────────────────────────────────────────────────────────────────────────
// Drawing primitives
// ─────────────────────────────────────────────────────────────────────────────

func drawTitle(pdf *fpdf.Fpdf, g pageGeometry, tr func(string) string, title string) {
	pdf.SetFont("Helvetica", "B", g.fontTitle)
	pdf.SetTextColor(0, 0, 0)
	pdf.CellFormat(0, g.titleH-2, tr(title), "", 1, "C", false, 0, "")
	pdf.Ln(1)
}

// drawHeaderRow draws either a single header row (group=nil) or a two-row
// grouped header. Header background is mid-grey for visibility.
func drawHeaderRow(pdf *fpdf.Fpdf, g pageGeometry, widths []float64, group, cols []string) {
	pdf.SetFillColor(210, 215, 222)
	pdf.SetTextColor(20, 20, 20)
	pdf.SetDrawColor(120, 120, 120)
	pdf.SetLineWidth(0.2)

	if group != nil {
		pdf.SetFont("Helvetica", "B", g.fontHdr-0.5)
		for i, w := range widths {
			label := ""
			if i < len(group) {
				label = group[i]
			}
			pdf.CellFormat(w, g.headerH, label, "1", 0, "C", true, 0, "")
		}
		pdf.Ln(-1)
	}

	pdf.SetFont("Helvetica", "B", g.fontHdr)
	for i, w := range widths {
		label := ""
		if i < len(cols) {
			label = cols[i]
		}
		pdf.CellFormat(w, g.headerH, label, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)
}

// drawDataRow draws one zebra-striped data row.
func drawDataRow(pdf *fpdf.Fpdf, g pageGeometry, widths []float64, cells, align []string, rowIdx int, tr func(string) string) {
	zebra := rowIdx%2 == 1
	if zebra {
		pdf.SetFillColor(242, 244, 247)
	}
	pdf.SetTextColor(0, 0, 0)
	pdf.SetDrawColor(180, 180, 180)
	pdf.SetLineWidth(0.15)

	for i, w := range widths {
		val := ""
		if i < len(cells) {
			val = cells[i]
		}
		a := "C"
		if i < len(align) {
			a = align[i]
		}
		pdf.CellFormat(w, g.rowH, tr(val), "1", 0, a, zebra, 0, "")
	}
	pdf.Ln(-1)
}

// drawTotalsRow draws the bold "this page" totals row in a darker shade.
func drawTotalsRow(pdf *fpdf.Fpdf, g pageGeometry, widths []float64, cells, align []string, tr func(string) string) {
	pdf.SetFont("Helvetica", "B", g.fontBody)
	pdf.SetFillColor(220, 226, 235)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetDrawColor(120, 120, 120)
	pdf.SetLineWidth(0.2)
	for i, w := range widths {
		val := ""
		if i < len(cells) {
			val = cells[i]
		}
		a := "C"
		if i < len(align) {
			a = align[i]
		}
		pdf.CellFormat(w, g.rowH, tr(val), "1", 0, a, true, 0, "")
	}
	pdf.Ln(-1)
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// emdash returns the em-dash character. Centralised so we can swap to a plain
// hyphen if encoding ever proves problematic. The unicode translator in each
// generator converts this to CP1252 0x97 for fpdf core fonts.
func emdash() string { return "\u2014" }

func safeStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func fmtTime(s *string) string {
	if s == nil {
		return ""
	}
	v := *s
	if len(v) >= 5 {
		return v[:5]
	}
	return v
}

func fmtDec(v int) string {
	if v == 0 {
		return ""
	}
	h := v / 60
	m := v % 60
	return fmt.Sprintf("%d:%02d", h, m)
}
