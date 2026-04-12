package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/pkg/duration"
	"github.com/gin-gonic/gin"
	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"
)

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

	// Logbook filtering — if logbookLicenseId is set, filter flights to matching aircraft classes
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

	// Sort flights by date ascending (oldest first, like a physical logbook)
	sort.Slice(flights, func(i, j int) bool {
		return flights[i].Date.Before(flights[j].Date)
	})

	// Determine format
	format := "easa"
	if params.Format != nil {
		format = string(*params.Format)
	}

	var pdf *fpdf.Fpdf
	switch format {
	case "faa":
		pdf = generateFAAPDF(flights)
	case "summary":
		pdf = generateSummaryPDF(flights)
	default:
		pdf = generateEASAPDF(flights, h, c, userID)
	}

	// Send PDF
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=ninerlog_%s_%s.pdf", format, time.Now().Format("2006-01-02")))
	pdf.Output(c.Writer)
}

// generateEASAPDF creates an EASA AMC1 FCL.050 compliant two-page-spread layout
func generateEASAPDF(flights []*models.Flight, h *APIHandler, c *gin.Context, userID uuid.UUID) *fpdf.Fpdf {
	// Get aircraft for class lookup (to derive SP-SE / SP-ME)
	aircraftList, _ := h.aircraftService.ListAircraft(c.Request.Context(), userID)
	regToClass := make(map[string]string)
	for _, ac := range aircraftList {
		if ac.AircraftClass != nil {
			regToClass[strings.ToUpper(ac.Registration)] = *ac.AircraftClass
		}
	}

	pdf := fpdf.New("L", "mm", "A4", "") // Landscape A4
	pdf.SetAutoPageBreak(true, 10)

	// EASA AMC1 FCL.050 logbook columns — full 24-column layout
	// Left page: Cols 1-12, Right page: Cols 13-24
	colWidths := []float64{
		16, // 1. Date
		14, // 2. Dep Place
		10, // 3. Dep Time
		14, // 4. Arr Place
		10, // 5. Arr Time
		16, // 6. A/C Type
		16, // 7. A/C Reg
		12, // 8. SP-SE
		12, // 9. SP-ME
		12, // 10. Multi-Pilot
		12, // 11. Total Time
		22, // 12. PIC Name
		8,  // 13. Ldg Day
		8,  // 14. Ldg Night
		12, // 15. Night
		12, // 16. IFR
		12, // 17. PIC
		12, // 18. Co-Pilot
		12, // 19. Dual
		12, // 20. Instructor
		12, // 21. FSTD Date (narrower)
		14, // 22. FSTD Type
		10, // 23. FSTD Time
		23, // 24. Remarks
	}

	headers1 := []string{
		"", "DEPARTURE", "", "ARRIVAL", "", "AIRCRAFT", "",
		"SINGLE PILOT", "", "MULTI", "TOTAL", "",
		"LANDINGS", "", "OPERATIONAL", "CONDITION", "PILOT FUNCTION", "", "", "",
		"FSTD", "", "", "",
	}
	headers2 := []string{
		"DATE", "PLACE", "TIME", "PLACE", "TIME", "TYPE", "REG",
		"SE", "ME", "PILOT", "TIME", "PIC NAME",
		"DAY", "NIGHT", "NIGHT", "IFR", "PIC", "CO-PLT", "DUAL", "INSTR",
		"DATE", "TYPE", "TIME", "REMARKS",
	}

	rowH := 5.0
	headerH := 4.5

	pageNum := 0
	rowsPerPage := 28

	for startIdx := 0; startIdx < len(flights); startIdx += rowsPerPage {
		pdf.AddPage()
		pageNum++

		// Title
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(0, 6, fmt.Sprintf("PILOT LOGBOOK — Page %d", pageNum), "", 1, "C", false, 0, "")
		pdf.Ln(2)

		// Header row 1 (group headers)
		pdf.SetFont("Helvetica", "B", 4)
		pdf.SetFillColor(230, 230, 230)
		for i, w := range colWidths {
			pdf.CellFormat(w, headerH, headers1[i], "1", 0, "C", true, 0, "")
		}
		pdf.Ln(-1)

		// Header row 2 (column names)
		pdf.SetFont("Helvetica", "B", 4.5)
		for i, w := range colWidths {
			pdf.CellFormat(w, headerH, headers2[i], "1", 0, "C", true, 0, "")
		}
		pdf.Ln(-1)

		// Data rows
		pdf.SetFont("Helvetica", "", 4.5)

		endIdx := startIdx + rowsPerPage
		if endIdx > len(flights) {
			endIdx = len(flights)
		}

		// Running totals for this page
		var pageTotalTime, pageNight, pageIFR, pagePIC, pageDual int
		var pageSIC, pageDualGiven, pageSPSE, pageSPME, pageMP, pageFSTD int
		var pageLdgDay, pageLdgNight int

		for _, f := range flights[startIdx:endIdx] {
			dep := safeStr(f.DepartureICAO)
			arr := safeStr(f.ArrivalICAO)
			depTime := fmtTime(f.OffBlockTime)
			arrTime := fmtTime(f.OnBlockTime)
			remarks := safeStr(f.Remarks)
			if f.Endorsements != nil && *f.Endorsements != "" {
				if remarks != "" {
					remarks += " | "
				}
				remarks += *f.Endorsements
			}
			if len(remarks) > 30 {
				remarks = remarks[:27] + "..."
			}

			picName := "SELF"
			if f.PICName != nil && *f.PICName != "" {
				picName = *f.PICName
			} else if f.InstructorName != nil && *f.InstructorName != "" {
				picName = *f.InstructorName
			}

			// Derive SP-SE / SP-ME / MP from aircraft class
			spSE, spME, mp := 0, 0, 0
			if f.MultiPilotTime > 0 {
				mp = f.MultiPilotTime
			} else {
				acClass := regToClass[strings.ToUpper(f.AircraftReg)]
				if strings.HasPrefix(acClass, "MEP") || strings.HasPrefix(acClass, "SET") {
					spME = f.TotalTime
				} else {
					spSE = f.TotalTime
				}
			}

			// FSTD columns
			fstdDate, fstdType, fstdTime := "", "", ""
			if f.FSTDType != nil && *f.FSTDType != "" && f.SimulatedFlightTime > 0 {
				fstdDate = f.Date.Format("02.01")
				fstdType = *f.FSTDType
				fstdTime = fmtDec(f.SimulatedFlightTime)
			}

			cells := []string{
				f.Date.Format("02.01.06"),
				dep, depTime, arr, arrTime,
				f.AircraftType, f.AircraftReg,
				fmtDec(spSE), fmtDec(spME), fmtDec(mp),
				fmtDec(f.TotalTime),
				picName,
				fmt.Sprintf("%d", f.LandingsDay), fmt.Sprintf("%d", f.LandingsNight),
				fmtDec(f.NightTime), fmtDec(f.IFRTime),
				fmtDec(f.PICTime), fmtDec(f.SICTime), fmtDec(f.DualTime), fmtDec(f.DualGivenTime),
				fstdDate, fstdType, fstdTime,
				remarks,
			}

			for i, w := range colWidths {
				align := "C"
				if i == 11 || i == 23 { // PIC Name, Remarks
					align = "L"
				}
				pdf.CellFormat(w, rowH, cells[i], "1", 0, align, false, 0, "")
			}
			pdf.Ln(-1)

			pageTotalTime += f.TotalTime
			pageNight += f.NightTime
			pageIFR += f.IFRTime
			pagePIC += f.PICTime
			pageDual += f.DualTime
			pageSIC += f.SICTime
			pageDualGiven += f.DualGivenTime
			pageLdgDay += f.LandingsDay
			pageLdgNight += f.LandingsNight
			pageSPSE += spSE
			pageSPME += spME
			pageMP += mp
			if f.FSTDType != nil && *f.FSTDType != "" {
				pageFSTD += f.SimulatedFlightTime
			}
		}

		// Page totals row (Row A)
		pdf.SetFont("Helvetica", "B", 4.5)
		pdf.SetFillColor(245, 245, 245)
		totalCells := []string{
			"", "", "", "", "", "", "THIS PAGE",
			fmtDec(pageSPSE), fmtDec(pageSPME), fmtDec(pageMP),
			fmtDec(pageTotalTime), "",
			fmt.Sprintf("%d", pageLdgDay), fmt.Sprintf("%d", pageLdgNight),
			fmtDec(pageNight), fmtDec(pageIFR),
			fmtDec(pagePIC), fmtDec(pageSIC), fmtDec(pageDual), fmtDec(pageDualGiven),
			"", "", fmtDec(pageFSTD), "",
		}
		for i, w := range colWidths {
			align := "C"
			if i == 6 {
				align = "R"
			}
			pdf.CellFormat(w, rowH, totalCells[i], "1", 0, align, true, 0, "")
		}
		pdf.Ln(-1)

		// Cumulative totals rows (B and C) could be added here for full compliance
		// For now, the grand summary page covers this
	}

	// Final summary page
	addGrandSummaryPage(pdf, flights)

	return pdf
}

// generateFAAPDF creates a FAA-style PDF logbook (ASA/Jeppesen standard layout)
func generateFAAPDF(flights []*models.Flight) *fpdf.Fpdf {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 10)

	colWidths := []float64{
		16, // Date
		16, // A/C Type
		18, // A/C Ident
		14, // From
		14, // To
		12, // Solo
		12, // PIC
		12, // SIC
		12, // Dual Rcvd
		12, // Instr Given
		12, // Act Inst
		12, // Sim Inst
		12, // XC
		12, // Night
		9,  // Day Ldg
		9,  // Ngt Ldg
		9,  // Appr
		8,  // Holds
		12, // Total
		44, // Remarks
	}

	headers := []string{
		"DATE", "A/C TYPE", "A/C IDENT", "FROM", "TO",
		"SOLO", "PIC", "SIC", "DUAL", "INSTR",
		"ACT INST", "SIM INST", "XC", "NIGHT",
		"D LDG", "N LDG", "APPR", "HOLD",
		"TOTAL", "REMARKS/ENDORSEMENTS",
	}

	rowH := 5.0
	headerH := 4.5
	pageNum := 0
	rowsPerPage := 28

	for startIdx := 0; startIdx < len(flights); startIdx += rowsPerPage {
		pdf.AddPage()
		pageNum++

		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(0, 6, fmt.Sprintf("PILOT LOGBOOK (FAA) — Page %d", pageNum), "", 1, "C", false, 0, "")
		pdf.Ln(2)

		pdf.SetFont("Helvetica", "B", 4.5)
		pdf.SetFillColor(230, 230, 230)
		for i, w := range colWidths {
			pdf.CellFormat(w, headerH, headers[i], "1", 0, "C", true, 0, "")
		}
		pdf.Ln(-1)

		pdf.SetFont("Helvetica", "", 4.5)

		endIdx := startIdx + rowsPerPage
		if endIdx > len(flights) {
			endIdx = len(flights)
		}

		var pageTotal, pagePIC, pageSIC, pageSolo, pageDual, pageInstr int
		var pageActInst, pageSimInst, pageXC, pageNight int
		var pageLdgDay, pageLdgNight, pageAppr, pageHolds int

		for _, f := range flights[startIdx:endIdx] {
			dep := safeStr(f.DepartureICAO)
			arr := safeStr(f.ArrivalICAO)
			remarks := safeStr(f.Remarks)
			if f.Endorsements != nil && *f.Endorsements != "" {
				if remarks != "" {
					remarks += " | "
				}
				remarks += *f.Endorsements
			}
			if f.IsIPC {
				remarks += " ✓IPC"
			}
			if f.IsFlightReview {
				remarks += " ✓FR"
			}
			if len(remarks) > 55 {
				remarks = remarks[:52] + "..."
			}

			fmtD := func(v int) string {
				return duration.FormatDecimal(v)
			}

			cells := []string{
				f.Date.Format("01/02/06"),
				f.AircraftType, f.AircraftReg,
				dep, arr,
				fmtD(f.SoloTime), fmtD(f.PICTime), fmtD(f.SICTime),
				fmtD(f.DualTime), fmtD(f.DualGivenTime),
				fmtD(f.ActualInstrumentTime), fmtD(f.SimulatedInstrumentTime),
				fmtD(f.CrossCountryTime), fmtD(f.NightTime),
				fmt.Sprintf("%d", f.LandingsDay), fmt.Sprintf("%d", f.LandingsNight),
				fmt.Sprintf("%d", f.ApproachesCount), fmt.Sprintf("%d", f.Holds),
				fmtD(f.TotalTime),
				remarks,
			}

			for i, w := range colWidths {
				align := "C"
				if i == 19 {
					align = "L"
				}
				pdf.CellFormat(w, rowH, cells[i], "1", 0, align, false, 0, "")
			}
			pdf.Ln(-1)

			pageTotal += f.TotalTime
			pagePIC += f.PICTime
			pageSIC += f.SICTime
			pageSolo += f.SoloTime
			pageDual += f.DualTime
			pageInstr += f.DualGivenTime
			pageActInst += f.ActualInstrumentTime
			pageSimInst += f.SimulatedInstrumentTime
			pageXC += f.CrossCountryTime
			pageNight += f.NightTime
			pageLdgDay += f.LandingsDay
			pageLdgNight += f.LandingsNight
			pageAppr += f.ApproachesCount
			pageHolds += f.Holds
		}

		// Page totals
		pdf.SetFont("Helvetica", "B", 4.5)
		pdf.SetFillColor(245, 245, 245)
		fmtD := func(v int) string { return duration.FormatDecimal(v) }
		totalCells := []string{
			"", "", "", "", "TOTAL",
			fmtD(pageSolo), fmtD(pagePIC), fmtD(pageSIC),
			fmtD(pageDual), fmtD(pageInstr),
			fmtD(pageActInst), fmtD(pageSimInst),
			fmtD(pageXC), fmtD(pageNight),
			fmt.Sprintf("%d", pageLdgDay), fmt.Sprintf("%d", pageLdgNight),
			fmt.Sprintf("%d", pageAppr), fmt.Sprintf("%d", pageHolds),
			fmtD(pageTotal), "",
		}
		for i, w := range colWidths {
			align := "C"
			if i == 4 {
				align = "R"
			}
			pdf.CellFormat(w, rowH, totalCells[i], "1", 0, align, true, 0, "")
		}
		pdf.Ln(-1)
	}

	addGrandSummaryPage(pdf, flights)

	return pdf
}

// generateSummaryPDF creates a simple summary PDF (legacy format)
func generateSummaryPDF(flights []*models.Flight) *fpdf.Fpdf {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 10)
	addGrandSummaryPage(pdf, flights)
	return pdf
}

// addGrandSummaryPage adds a final summary page with grand totals
func addGrandSummaryPage(pdf *fpdf.Fpdf, flights []*models.Flight) {
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(0, 8, "TOTALS SUMMARY", "", 1, "C", false, 0, "")
	pdf.Ln(4)

	var grandTotal, grandPIC, grandDual, grandNight, grandIFR, grandSolo, grandXC int
	var grandSIC, grandDualGiven, grandMP int
	var grandLdgDay, grandLdgNight, grandFlights int
	for _, f := range flights {
		grandTotal += f.TotalTime
		grandPIC += f.PICTime
		grandDual += f.DualTime
		grandNight += f.NightTime
		grandIFR += f.IFRTime
		grandSolo += f.SoloTime
		grandXC += f.CrossCountryTime
		grandSIC += f.SICTime
		grandDualGiven += f.DualGivenTime
		grandMP += f.MultiPilotTime
		grandLdgDay += f.LandingsDay
		grandLdgNight += f.LandingsNight
		grandFlights++
	}

	pdf.SetFont("Helvetica", "", 10)
	summaryW := 80.0
	valW := 40.0

	summaryRows := []struct{ label, value string }{
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

	x0 := (297 - summaryW - valW) / 2 // center on landscape A4
	for _, row := range summaryRows {
		pdf.SetX(x0)
		pdf.SetFont("Helvetica", "", 10)
		pdf.CellFormat(summaryW, 7, row.label, "1", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(valW, 7, row.value, "1", 1, "R", false, 0, "")
	}

	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 7)
	pdf.CellFormat(0, 4, fmt.Sprintf("Generated by NinerLog on %s — https://github.com/fjaeckel/ninerlog", time.Now().Format("02 Jan 2006 15:04 UTC")), "", 1, "C", false, 0, "")
}

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
		return v[:5] // HH:MM from HH:MM:SS
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
