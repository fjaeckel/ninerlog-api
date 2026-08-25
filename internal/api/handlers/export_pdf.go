package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/gin-gonic/gin"
	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"
)

// maxPDFExportFlights caps how many flights a single PDF export will render.
const maxPDFExportFlights = 10000

// ─────────────────────────────────────────────────────────────────────────────
// Page geometry
// ─────────────────────────────────────────────────────────────────────────────

// pageGeometry describes the printable area of a landscape page. All values in mm.
// Vertical anatomy of every logbook page, top to bottom:
//
//	title band (bandH) → subtitle line (subH) → column grid (headers, data
//	rows, three totals rows) → signature strip (sigH) → footer line (footerH)
type pageGeometry struct {
	sizeName                                                 string  // "A4", "A5", "Letter"
	width                                                    float64 // landscape width (long edge)
	height                                                   float64 // landscape height (short edge)
	marginLR                                                 float64 // left/right margin
	bandH                                                    float64 // full-bleed title band height
	subH                                                     float64 // subtitle line height
	footerH                                                  float64 // footer line height
	sigH                                                     float64 // signature strip height
	rowH                                                     float64 // data row height
	headerH                                                  float64 // one header row height (grid headers are two rows)
	fontTitle, fontSub, fontHdr, fontBody, fontFoot, fontSig float64
}

func (g pageGeometry) usableWidth() float64 { return g.width - 2*g.marginLR }

// gridTop is the Y where the column grid starts, below the title band and
// subtitle line.
func (g pageGeometry) gridTop() float64 { return g.bandH + g.subH + 2.8 }

// footerY is the Y where the footer line is drawn.
func (g pageGeometry) footerY() float64 { return g.height - g.footerH - 1.5 }

// gridBottom is the lowest Y the grid (including the signature strip) may reach.
func (g pageGeometry) gridBottom() float64 { return g.footerY() - 1 }

func (g pageGeometry) usableHeight() float64 { return g.gridBottom() - g.gridTop() }

// logRowsPerPage returns how many data rows fit on a logbook page, leaving
// room for the two header rows, the three totals rows (this page / previous
// pages / total time) and the per-page signature strip.
func (g pageGeometry) logRowsPerPage() int {
	avail := g.usableHeight() - 2*g.headerH - 3*g.rowH - g.sigH
	n := int((avail + 0.001) / g.rowH) // epsilon absorbs float error from withRowsPerPage
	if n < 5 {
		n = 5
	}
	return n
}

// Legibility bounds for dynamically scaled rows. Below minDynRowH the core
// fonts stop being readable in print; above maxDynRowH rows look detached
// from their grid.
const (
	minDynRowH = 2.6
	maxDynRowH = 9.5
)

// withRowsPerPage returns a geometry whose row height is scaled so exactly n
// data rows (plus the three totals rows) fill the page. Row height is
// clamped to stay legible; dense layouts also scale the body font down.
func (g pageGeometry) withRowsPerPage(n int) pageGeometry {
	if n < 5 {
		n = 5
	}
	avail := g.usableHeight() - 2*g.headerH - g.sigH
	rowH := avail / float64(n+3)
	if rowH < minDynRowH {
		rowH = minDynRowH
	}
	if rowH > maxDynRowH {
		rowH = maxDynRowH
	}
	g.rowH = rowH
	if f := rowH * 1.35; f < g.fontBody {
		g.fontBody = f
	}
	return g
}

func geometryFor(sizeName string) pageGeometry {
	// Base A4-landscape geometry.
	base := pageGeometry{
		sizeName: "A4",
		width:    297, height: 210,
		marginLR: 10,
		bandH:    8, subH: 4.5, footerH: 4, sigH: 11,
		rowH: 5, headerH: 4.5,
		fontTitle: 12, fontSub: 6.5, fontHdr: 5, fontBody: 5, fontFoot: 6, fontSig: 6.5,
	}
	switch strings.ToLower(sizeName) {
	case "a5":
		// A5 landscape: 210 × 148 mm. Scale fonts/rows down proportionally.
		s := 210.0 / 297.0
		return pageGeometry{
			sizeName: "A5",
			width:    210, height: 148,
			marginLR: 7,
			bandH:    6, subH: 3.6, footerH: 3.2, sigH: 8.5,
			rowH: base.rowH * s, headerH: base.headerH * s,
			fontTitle: 9.5, fontSub: 5.5, fontHdr: 4, fontBody: 4, fontFoot: 5, fontSig: 5.5,
		}
	case "letter":
		// US Letter landscape: 279.4 × 215.9 mm.
		return pageGeometry{
			sizeName: "Letter",
			width:    279.4, height: 215.9,
			marginLR: 10,
			bandH:    8, subH: 4.5, footerH: 4, sigH: 11,
			rowH: 5, headerH: 4.5,
			fontTitle: 12, fontSub: 6.5, fontHdr: 5, fontBody: 5, fontFoot: 6, fontSig: 6.5,
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

// ─────────────────────────────────────────────────────────────────────────────
// Palette
// ─────────────────────────────────────────────────────────────────────────────

// Print-friendly palette: a deep navy identity band with a gold hairline,
// cool grey-blue fills for headers and totals, and light zebra striping.
var (
	colBand         = [3]int{28, 38, 56}    // deep navy — title band, grand-total text
	colAccent       = [3]int{198, 161, 91}  // aviation gold — hairline under the band
	colBandRegText  = [3]int{222, 199, 154} // gold-tinted regulation label in the band
	colGroupFill    = [3]int{223, 228, 237} // grouped header row
	colHdrFill      = [3]int{236, 240, 246} // column header row
	colZebra        = [3]int{246, 248, 251} // odd data rows
	colTotFill      = [3]int{229, 234, 242} // "this page" / "previous pages" rows
	colGrandFill    = [3]int{207, 216, 230} // "total time" row
	colBorder       = [3]int{187, 194, 205} // data cell borders
	colBorderStrong = [3]int{112, 122, 140} // header/totals borders
	colMuted        = [3]int{110, 118, 132} // footer, signature labels
)

func setFill(pdf *fpdf.Fpdf, c [3]int) { pdf.SetFillColor(c[0], c[1], c[2]) }
func setText(pdf *fpdf.Fpdf, c [3]int) { pdf.SetTextColor(c[0], c[1], c[2]) }
func setDraw(pdf *fpdf.Fpdf, c [3]int) { pdf.SetDrawColor(c[0], c[1], c[2]) }

// ─────────────────────────────────────────────────────────────────────────────
// Document scaffolding
// ─────────────────────────────────────────────────────────────────────────────

// pdfDoc bundles the fpdf handle with everything the per-page chrome needs.
type pdfDoc struct {
	pdf        *fpdf.Fpdf
	g          pageGeometry
	tr         func(string) string
	regulation string // e.g. "EASA Part-FCL · AMC1 FCL.050"
	holder     string // logbook holder (user display name)
	generated  string // render timestamp for the footer
	cert       string // certification wording for the signature blocks
	note       string // footer disclosure, e.g. carried-forward prior experience
}

func newDoc(g pageGeometry, regulation, holder, cert string) *pdfDoc {
	pdf := fpdf.NewCustom(&fpdf.InitType{
		OrientationStr: "L",
		UnitStr:        "mm",
		Size:           g.fpdfSize(),
	})
	pdf.SetMargins(g.marginLR, g.gridTop(), g.marginLR)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AliasNbPages("{nb}")
	return &pdfDoc{
		pdf:        pdf,
		g:          g,
		tr:         pdf.UnicodeTranslatorFromDescriptor(""), // CP1252 mapper for §, · , em-dash
		regulation: regulation,
		holder:     holder,
		generated:  time.Now().UTC().Format("02 Jan 2006 15:04 UTC"),
		cert:       cert,
	}
}

// startPage adds a page and draws the shared chrome: full-bleed navy title
// band with a gold hairline, the subtitle line (holder + page context) and
// the footer. Leaves the cursor at the top of the column grid.
func (d *pdfDoc) startPage(context string) {
	g, pdf := d.g, d.pdf
	pdf.AddPage()

	// Title band, full bleed.
	setFill(pdf, colBand)
	pdf.Rect(0, 0, g.width, g.bandH, "F")
	setFill(pdf, colAccent)
	pdf.Rect(0, g.bandH, g.width, 0.5, "F")

	pdf.SetFont("Helvetica", "B", g.fontTitle)
	setText(pdf, [3]int{255, 255, 255})
	pdf.SetXY(g.marginLR, 0)
	pdf.CellFormat(g.usableWidth()*0.5, g.bandH, d.tr("PILOT LOGBOOK"), "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", g.fontSub)
	setText(pdf, colBandRegText)
	pdf.SetXY(g.marginLR+g.usableWidth()*0.5, 0)
	pdf.CellFormat(g.usableWidth()*0.5, g.bandH, d.tr(d.regulation), "", 0, "R", false, 0, "")

	// Subtitle line: holder left, page context right.
	pdf.SetFont("Helvetica", "", g.fontSub)
	setText(pdf, colBand)
	pdf.SetXY(g.marginLR, g.bandH+1.2)
	holder := ""
	if d.holder != "" {
		holder = "Holder: " + d.holder
	}
	pdf.CellFormat(g.usableWidth()*0.5, g.subH, d.tr(holder), "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "B", g.fontSub)
	pdf.SetXY(g.marginLR+g.usableWidth()*0.5, g.bandH+1.2)
	pdf.CellFormat(g.usableWidth()*0.5, g.subH, d.tr(context), "", 0, "R", false, 0, "")

	d.drawFooter()

	pdf.SetXY(g.marginLR, g.gridTop())
	setText(pdf, [3]int{0, 0, 0})
}

// drawFooter draws the footer line: generation info left, page number right.
// When the document's balances are not purely logged time, the disclosure note
// rides along on the left, shrunk to fit rather than allowed to bleed.
func (d *pdfDoc) drawFooter() {
	g, pdf := d.g, d.pdf
	left := fmt.Sprintf("Generated by NinerLog %s %s", emdash(), d.generated)
	if d.note != "" {
		left += " · " + d.note
	}
	left = d.tr(left)
	leftW := g.usableWidth() * 0.6
	d.setFontFit("", g.fontFoot, left, leftW)
	setText(pdf, colMuted)
	pdf.SetXY(g.marginLR, g.footerY())
	pdf.CellFormat(leftW, g.footerH, left, "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", g.fontFoot)
	pdf.SetXY(g.marginLR+g.usableWidth()*0.6, g.footerY())
	pdf.CellFormat(g.usableWidth()*0.4, g.footerH,
		fmt.Sprintf("Page %d of {nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
}

// addBlankPage inserts an intentionally-blank filler page, before the first
// spread and before the totals summary of a spread document.
func (d *pdfDoc) addBlankPage() {
	g, pdf := d.g, d.pdf
	pdf.AddPage()
	pdf.SetFont("Helvetica", "I", g.fontSub+1)
	setText(pdf, colMuted)
	pdf.SetXY(g.marginLR, g.height/2-4)
	pdf.CellFormat(g.usableWidth(), 8, d.tr("This page was intentionally left blank."), "", 0, "C", false, 0, "")
	d.drawFooter()
	setText(pdf, [3]int{0, 0, 0})
}

// ─────────────────────────────────────────────────────────────────────────────
// Drawing primitives
// ─────────────────────────────────────────────────────────────────────────────

// setFontFit sets the font at `size`, shrinking it until `text` fits into
// maxW so labels never bleed out of narrow columns.
func (d *pdfDoc) setFontFit(style string, size float64, text string, maxW float64) {
	pdf := d.pdf
	pdf.SetFont("Helvetica", style, size)
	for size > 3 && pdf.GetStringWidth(text) > maxW {
		size -= 0.25
		pdf.SetFont("Helvetica", style, size)
	}
}

// colGroup is one merged cell in the grouped (upper) header row spanning
// `span` consecutive columns.
type colGroup struct {
	label string
	span  int
}

// drawHeader draws the two-row column header: a grouped row of merged cells
// above the per-column row. A span-1 group whose label (or whose sub label)
// is empty is drawn as a single full-height cell, which is how DATE, TOTAL
// TIME etc. get their vertically merged look.
func (d *pdfDoc) drawHeader(widths []float64, groups []colGroup, sub []string) {
	g, pdf := d.g, d.pdf
	x0 := g.marginLR
	y0 := pdf.GetY()

	setText(pdf, colBand)
	setDraw(pdf, colBorderStrong)
	pdf.SetLineWidth(0.2)

	x := x0
	col := 0
	merged := make([]bool, len(widths))
	for _, gr := range groups {
		w := 0.0
		for i := 0; i < gr.span && col+i < len(widths); i++ {
			w += widths[col+i]
		}
		label := gr.label
		h := g.headerH
		fill := colGroupFill
		if gr.span == 1 && (label == "" || sub[col] == "") {
			// Vertically merged single column.
			if label == "" {
				label = sub[col]
			}
			h = 2 * g.headerH
			merged[col] = true
			fill = colHdrFill
		}
		setFill(pdf, fill)
		d.setFontFit("B", g.fontHdr, d.tr(label), w-1.2)
		pdf.SetXY(x, y0)
		pdf.CellFormat(w, h, d.tr(label), "1", 0, "CM", true, 0, "")
		x += w
		col += gr.span
	}

	// Per-column sub row.
	setFill(pdf, colHdrFill)
	x = x0
	for i, w := range widths {
		if !merged[i] {
			d.setFontFit("B", g.fontHdr, d.tr(sub[i]), w-1.2)
			pdf.SetXY(x, y0+g.headerH)
			pdf.CellFormat(w, g.headerH, d.tr(sub[i]), "1", 0, "CM", true, 0, "")
		}
		x += w
	}
	pdf.SetXY(x0, y0+2*g.headerH)
}

// drawDataRow draws one zebra-striped data row.
func (d *pdfDoc) drawDataRow(widths []float64, cells, align []string, rowIdx int) {
	g, pdf := d.g, d.pdf
	zebra := rowIdx%2 == 1
	if zebra {
		setFill(pdf, colZebra)
	}
	setText(pdf, [3]int{0, 0, 0})
	setDraw(pdf, colBorder)
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
		pdf.CellFormat(w, g.rowH, d.tr(val), "1", 0, a, zebra, 0, "")
	}
	pdf.Ln(-1)
}

// drawTotalsRow draws one bold totals row. The first `span` columns merge
// into a single right-aligned label cell; `cells` supplies the values for
// the remaining columns. `grand` selects the stronger "TOTAL TIME" styling.
func (d *pdfDoc) drawTotalsRow(widths []float64, span int, label string, cells, align []string, grand bool) {
	g, pdf := d.g, d.pdf
	pdf.SetFont("Helvetica", "B", g.fontBody)
	if grand {
		setFill(pdf, colGrandFill)
		setText(pdf, colBand)
	} else {
		setFill(pdf, colTotFill)
		setText(pdf, [3]int{0, 0, 0})
	}
	setDraw(pdf, colBorderStrong)
	pdf.SetLineWidth(0.2)

	if span < 1 {
		span = 1
	}
	labelW := 0.0
	for i := 0; i < span && i < len(widths); i++ {
		labelW += widths[i]
	}
	d.setFontFit("B", g.fontBody, d.tr(label+"  "), labelW-0.8)
	pdf.CellFormat(labelW, g.rowH, d.tr(label+"  "), "1", 0, "R", true, 0, "")
	pdf.SetFont("Helvetica", "B", g.fontBody)
	for i := span; i < len(widths); i++ {
		val := ""
		if i-span < len(cells) {
			val = cells[i-span]
		}
		a := "C"
		if i < len(align) {
			a = align[i]
		}
		pdf.CellFormat(widths[i], g.rowH, d.tr(val), "1", 0, a, true, 0, "")
	}
	pdf.Ln(-1)
}

// drawSignatureBlock pins the certification text and the signature/date
// rules to the bottom of the page, just above the footer.
func (d *pdfDoc) drawSignatureBlock() {
	cert := d.cert
	g, pdf := d.g, d.pdf
	top := g.gridBottom() - g.sigH

	pdf.SetFont("Helvetica", "I", g.fontSig)
	setText(pdf, [3]int{40, 46, 60})
	certW := g.usableWidth() * 0.46
	lineY := top + g.sigH*0.62
	pdf.SetXY(g.marginLR, lineY-g.sigH*0.38)
	pdf.CellFormat(certW, g.sigH*0.38, d.tr(cert), "", 0, "L", false, 0, "")

	sx1 := g.marginLR + g.usableWidth()*0.50
	sx2 := g.marginLR + g.usableWidth()*0.80
	dx1 := g.marginLR + g.usableWidth()*0.84
	dx2 := g.marginLR + g.usableWidth()
	setDraw(pdf, [3]int{60, 66, 80})
	pdf.SetLineWidth(0.3)
	pdf.Line(sx1, lineY, sx2, lineY)
	pdf.Line(dx1, lineY, dx2, lineY)

	pdf.SetFont("Helvetica", "", g.fontFoot-1.5)
	setText(pdf, colMuted)
	pdf.SetXY(sx1, lineY+0.8)
	pdf.CellFormat(sx2-sx1, 3, "PILOT'S SIGNATURE", "", 0, "C", false, 0, "")
	pdf.SetXY(dx1, lineY+0.8)
	pdf.CellFormat(dx2-dx1, 3, "DATE", "", 0, "C", false, 0, "")
	setText(pdf, [3]int{0, 0, 0})
}

// ─────────────────────────────────────────────────────────────────────────────
// Handler
// ─────────────────────────────────────────────────────────────────────────────

// dropEmptyRows returns flights with the entries that print nothing removed.
func dropEmptyRows(flights []*models.Flight) []*models.Flight {
	out := make([]*models.Flight, 0, len(flights))
	for _, f := range flights {
		if flightrules.RendersLogbookRow(f) {
			out = append(out, f)
		}
	}
	return out
}

// ExportFlightsPDF implements GET /exports/pdf
func (h *APIHandler) ExportFlightsPDF(c *gin.Context, params generated.ExportFlightsPDFParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	count, err := h.flightService.CountFlights(c.Request.Context(), userID, nil)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve flights")
		return
	}
	if count > maxPDFExportFlights {
		h.sendError(c, http.StatusBadRequest, fmt.Sprintf(
			"Too many flights to export as a single PDF (%d, max %d). Narrow the range with startDate/endDate.",
			count, maxPDFExportFlights))
		return
	}

	flights, err := h.flightService.ListFlights(c.Request.Context(), userID, &repository.FlightQueryOptions{
		Page:     1,
		PageSize: maxPDFExportFlights,
	})
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve flights")
		return
	}
	flights = dropEmptyRows(flights)
	h.attachCrewMembers(c.Request.Context(), flights)

	classFiltered := false
	if params.LogbookLicenseId != nil {
		licenseID := uuid.UUID(*params.LogbookLicenseId)
		classRatings, err := h.classRatingService.ListClassRatings(c.Request.Context(), licenseID, userID)
		if err == nil && len(classRatings) > 0 {
			classFiltered = true
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

	sortFlightsChronological(flights)

	// Prior experience opens the balance on every sheet; omitted for a
	// class-rating-filtered export.
	var baseline *models.FlightBaseline
	if !classFiltered {
		baseline, err = h.analyticsBaseline(c.Request.Context(), userID)
		if err != nil {
			h.sendError(c, http.StatusInternalServerError, "Failed to retrieve prior experience totals")
			return
		}
	}

	format := "easa"
	if params.Format != nil {
		format = string(*params.Format)
	}
	pageSize := "a4"
	if params.PageSize != nil {
		pageSize = string(*params.PageSize)
	}
	layout := layoutSpread
	if params.Layout != nil && string(*params.Layout) == layoutSingle {
		layout = layoutSingle
	}
	geom := geometryFor(pageSize)
	if params.RowsPerPage != nil {
		geom = geom.withRowsPerPage(*params.RowsPerPage)
	}
	userName := h.getUserNameFromContext(c)

	var pdf *fpdf.Fpdf
	switch format {
	case "faa":
		pdf = generateFAAPDF(flights, geom, userName, layout, baseline)
	case "summary":
		pdf = generateSummaryPDF(flights, geom, userName, baseline)
	default:
		pdf = generateEASAPDF(flights, geom, h, c, userID, layout, baseline)
	}
	name := fmt.Sprintf("ninerlog_%s_%s_%s_%s.pdf",
		format, layout, strings.ToLower(geom.sizeName), time.Now().Format("2006-01-02"))
	if format == "summary" {
		name = fmt.Sprintf("ninerlog_summary_%s_%s.pdf",
			strings.ToLower(geom.sizeName), time.Now().Format("2006-01-02"))
	}
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", "attachment; filename="+name)
	if err := pdf.Output(c.Writer); err != nil {
		slog.Error("pdf export output error", "error", err)
	}
}

// Layout identifiers for the `layout` query parameter.
const (
	layoutSpread = "spread"
	layoutSingle = "single"
)

// ─────────────────────────────────────────────────────────────────────────────
// Summary PDF
// ─────────────────────────────────────────────────────────────────────────────

func generateSummaryPDF(flights []*models.Flight, g pageGeometry, userName string, b *models.FlightBaseline) *fpdf.Fpdf {
	d := newDoc(g, "Logbook Totals Summary", userName, certEASA)
	d.note = baselineFooterNote(b)
	addGrandSummaryPage(d, flights, b)
	return d.pdf
}

// summaryTotals are the career figures reported on the totals summary page.
type summaryTotals struct {
	flights                       int
	total, pic, sic               int
	dual, dualGiven, solo         int
	night, ifr, xc, multiPilot    int
	picus, spic, examiner, relief int
	ldgDay, ldgNight              int
}

// computeSummaryTotals aggregates every logged flight plus the prior
// experience baseline into a career total. See export_pdf_baseline.go for the
// baseline's field coverage.
func computeSummaryTotals(flights []*models.Flight, b *models.FlightBaseline) summaryTotals {
	var t summaryTotals
	for _, f := range flights {
		t.flights++
		t.total += f.TotalTime
		t.pic += f.PICTime
		t.sic += f.SICTime
		t.dual += f.DualTime
		t.dualGiven += f.DualGivenTime
		t.solo += f.SoloTime
		t.night += f.NightTime
		t.ifr += flightrules.EffectiveIFRTime(f)
		t.xc += f.CrossCountryTime
		t.multiPilot += f.MultiPilotTime
		t.picus += f.PICUSTime
		t.spic += f.SPICTime
		t.examiner += f.ExaminerTime
		t.relief += f.ReliefTime
		t.ldgDay += f.LandingsDay
		t.ldgNight += f.LandingsNight
	}
	if baselineApplies(b) {
		t.flights += b.TotalFlights
		t.total += b.TotalMinutes
		t.pic += b.PICMinutes
		t.sic += b.SICMinutes
		t.dual += b.DualMinutes
		t.dualGiven += b.DualGivenMinutes
		t.solo += b.SoloMinutes
		t.night += b.NightMinutes
		t.ifr += b.IFRMinutes
		t.xc += b.CrossCountryMinutes
		t.multiPilot += b.MultiPilotMinutes
		t.picus += b.PICUSMinutes
		t.spic += b.SPICMinutes
		t.examiner += b.ExaminerMinutes
		t.relief += b.ReliefMinutes
		t.ldgDay += b.LandingsDay
		t.ldgNight += b.LandingsNight
	}
	return t
}

func addGrandSummaryPage(d *pdfDoc, flights []*models.Flight, b *models.FlightBaseline) {
	g, pdf := d.g, d.pdf
	d.startPage("Totals Summary")

	t := computeSummaryTotals(flights, b)

	rowH := 7.0
	if g.sizeName == "A5" {
		rowH = 5.2
	}

	summaryW := g.usableWidth() * 0.5
	valW := g.usableWidth() * 0.22
	x0 := g.marginLR + (g.usableWidth()-summaryW-valW)/2

	rows := []struct{ label, value string }{
		{"Total Flights", fmt.Sprintf("%d", t.flights)},
		{"Total Block Time", fmtDec(t.total)},
		{"PIC Time", fmtDec(t.pic)},
		{"SIC / Co-Pilot Time", fmtDec(t.sic)},
		{"Dual Received", fmtDec(t.dual)},
		{"Dual / Instruction Given", fmtDec(t.dualGiven)},
		{"Solo Time", fmtDec(t.solo)},
	}
	// Declared function times appear only when the pilot has logged any.
	for _, extra := range []struct {
		minutes int
		label   string
	}{
		{t.picus, "PICUS (PIC under Supervision)"},
		{t.spic, "SPIC (Student PIC)"},
		{t.examiner, "Examiner Time"},
		{t.relief, "Cruise Relief Time"},
	} {
		if extra.minutes > 0 {
			rows = append(rows, struct{ label, value string }{extra.label, fmtDec(extra.minutes)})
		}
	}
	rows = append(rows, []struct{ label, value string }{
		{"Night Time", fmtDec(t.night)},
		{"IFR Time", fmtDec(t.ifr)},
		{"Cross-Country Time", fmtDec(t.xc)},
		{"Multi-Pilot Time", fmtDec(t.multiPilot)},
		{"Day Landings", fmt.Sprintf("%d", t.ldgDay)},
		{"Night Landings", fmt.Sprintf("%d", t.ldgNight)},
		{"Total Landings", fmt.Sprintf("%d", t.ldgDay+t.ldgNight)},
	}...)
	// Lead with the carried-forward block.
	if baselineApplies(b) {
		rows = append([]struct{ label, value string }{
			{"Brought Forward (prior logbooks)", fmtDecTotal(b.TotalMinutes)},
		}, rows...)
	}

	// Measure the disclosure note so the table stays vertically centred.
	noteLineH := 3.4
	if g.sizeName == "A5" {
		noteLineH = 2.8
	}
	note := d.tr(baselineSummaryNote(b))
	noteH := 0.0
	if note != "" {
		pdf.SetFont("Helvetica", "I", g.fontFoot)
		noteH = 3 + float64(len(pdf.SplitText(note, summaryW+valW)))*noteLineH
	}
	tableH := 9 + float64(len(rows))*rowH + noteH
	y0 := g.gridTop() + (g.usableHeight()-g.sigH-tableH)/2
	if y0 < g.gridTop() {
		y0 = g.gridTop()
	}
	pdf.SetY(y0)
	pdf.SetFont("Helvetica", "B", g.fontTitle)
	setText(pdf, colBand)
	pdf.CellFormat(0, 8, d.tr("TOTALS SUMMARY"), "", 1, "C", false, 0, "")
	pdf.Ln(1)

	setDraw(pdf, colBorder)
	pdf.SetLineWidth(0.15)
	for i, row := range rows {
		pdf.SetX(x0)
		fill := i%2 == 1
		if fill {
			setFill(pdf, colZebra)
		}
		setText(pdf, [3]int{0, 0, 0})
		pdf.SetFont("Helvetica", "", g.fontSub)
		pdf.CellFormat(summaryW, rowH, d.tr(row.label), "1", 0, "L", fill, 0, "")
		pdf.SetFont("Helvetica", "B", g.fontSub)
		pdf.CellFormat(valW, rowH, d.tr(row.value), "1", 1, "R", fill, 0, "")
	}

	if note != "" {
		pdf.Ln(3)
		pdf.SetX(x0)
		pdf.SetFont("Helvetica", "I", g.fontFoot)
		setText(pdf, colMuted)
		pdf.MultiCell(summaryW+valW, noteLineH, note, "", "L", false)
		setText(pdf, [3]int{0, 0, 0})
	}

	d.drawSignatureBlock()
}

// Certification wording for the per-page signature blocks.
const (
	certEASA = "I certify that the entries in this log are true."
	certFAA  = "I certify that the entries in this log are true and correct."
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// emdash returns the em-dash character.
func emdash() string { return "—" }

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

// fmtDec renders minutes as h:mm, leaving zero durations blank so data rows
// stay quiet.
func fmtDec(v int) string {
	if v == 0 {
		return ""
	}
	return fmtDecTotal(v)
}

// fmtDecTotal always prints, including "0:00" — used where a blank would read
// as "not recorded" rather than "none".
func fmtDecTotal(v int) string {
	return fmt.Sprintf("%d:%02d", v/60, v%60)
}

// truncRunes shortens s to at most max runes, ellipsising with "..." when cut.
func truncRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}
