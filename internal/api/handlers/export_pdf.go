package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
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

	pdf := fpdf.New("L", "mm", "A4", "") // Landscape A4
	pdf.SetAutoPageBreak(true, 10)

	// EASA FCL.050 logbook columns (simplified for A4 landscape)
	// The official EASA format has these column groups:
	// 1. Date | 2. Departure (Place/Time) | 3. Arrival (Place/Time) | 4. Aircraft (Type/Reg)
	// 5. PIC Name | 6. Single Pilot (SE/ME) | 7. Multi Pilot | 8. Total Time
	// 9. Landings (Day/Night) | 10. Night | 11. IFR | 12. PIC | 13. Co-pilot
	// 14. Dual | 15. Instructor | 16. Remarks/Endorsements

	colWidths := []float64{
		18, // Date
		16, // Dep ICAO
		12, // Dep Time
		16, // Arr ICAO
		12, // Arr Time
		20, // Aircraft Type
		22, // Aircraft Reg
		30, // PIC Name
		14, // Total Time
		10, // Ldg Day
		10, // Ldg Night
		14, // Night
		14, // IFR
		14, // PIC Time
		14, // Dual
		41, // Remarks
	}

	headers1 := []string{
		"", "DEPARTURE", "", "ARRIVAL", "", "AIRCRAFT", "", "",
		"SINGLE", "LANDINGS", "", "OPERATIONAL", "CONDITION", "PILOT", "FUNCTION", "",
	}
	headers2 := []string{
		"DATE", "PLACE", "TIME", "PLACE", "TIME", "TYPE", "REG", "PIC NAME",
		"TOTAL", "DAY", "NIGHT", "NIGHT", "IFR", "PIC", "DUAL", "REMARKS",
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
		pdf.SetFont("Helvetica", "B", 5)
		pdf.SetFillColor(230, 230, 230)
		for i, w := range colWidths {
			pdf.CellFormat(w, headerH, headers1[i], "1", 0, "C", true, 0, "")
		}
		pdf.Ln(-1)

		// Header row 2 (column names)
		pdf.SetFont("Helvetica", "B", 5.5)
		for i, w := range colWidths {
			pdf.CellFormat(w, headerH, headers2[i], "1", 0, "C", true, 0, "")
		}
		pdf.Ln(-1)

		// Data rows
		pdf.SetFont("Helvetica", "", 5.5)

		endIdx := startIdx + rowsPerPage
		if endIdx > len(flights) {
			endIdx = len(flights)
		}

		// Running totals for this page
		var pageTotalTime, pageNight, pageIFR, pagePIC, pageDual float64
		var pageLdgDay, pageLdgNight int

		for _, f := range flights[startIdx:endIdx] {
			dep := safeStr(f.DepartureICAO)
			arr := safeStr(f.ArrivalICAO)
			depTime := fmtTime(f.OffBlockTime)
			arrTime := fmtTime(f.OnBlockTime)
			remarks := safeStr(f.Remarks)
			if len(remarks) > 45 {
				remarks = remarks[:42] + "..."
			}

			picName := "SELF"
			if f.InstructorName != nil && *f.InstructorName != "" {
				picName = *f.InstructorName
			}

			cells := []string{
				f.Date.Format("02.01.06"),
				dep,
				depTime,
				arr,
				arrTime,
				f.AircraftType,
				f.AircraftReg,
				picName,
				fmtDec(f.TotalTime),
				fmt.Sprintf("%d", f.LandingsDay),
				fmt.Sprintf("%d", f.LandingsNight),
				fmtDec(f.NightTime),
				fmtDec(f.IFRTime),
				fmtDec(f.PICTime),
				fmtDec(f.DualTime),
				remarks,
			}

			for i, w := range colWidths {
				align := "C"
				if i == 7 || i == 15 { // PIC Name, Remarks
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
			pageLdgDay += f.LandingsDay
			pageLdgNight += f.LandingsNight
		}

		// Page totals row
		pdf.SetFont("Helvetica", "B", 5.5)
		pdf.SetFillColor(245, 245, 245)
		totalCells := []string{
			"", "", "", "", "", "", "", "PAGE TOTAL",
			fmtDec(pageTotalTime),
			fmt.Sprintf("%d", pageLdgDay),
			fmt.Sprintf("%d", pageLdgNight),
			fmtDec(pageNight),
			fmtDec(pageIFR),
			fmtDec(pagePIC),
			fmtDec(pageDual),
			"",
		}
		for i, w := range colWidths {
			align := "C"
			if i == 7 {
				align = "R"
			}
			pdf.CellFormat(w, rowH, totalCells[i], "1", 0, align, true, 0, "")
		}
		pdf.Ln(-1)
	}

	// Final summary page
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(0, 8, "TOTALS SUMMARY", "", 1, "C", false, 0, "")
	pdf.Ln(4)

	var grandTotal, grandPIC, grandDual, grandNight, grandIFR, grandSolo, grandXC float64
	var grandLdgDay, grandLdgNight, grandFlights int
	for _, f := range flights {
		grandTotal += f.TotalTime
		grandPIC += f.PICTime
		grandDual += f.DualTime
		grandNight += f.NightTime
		grandIFR += f.IFRTime
		grandSolo += f.SoloTime
		grandXC += f.CrossCountryTime
		grandLdgDay += f.LandingsDay
		grandLdgNight += f.LandingsNight
		grandFlights++
	}

	pdf.SetFont("Helvetica", "", 10)
	summaryW := 80.0
	valW := 40.0

	summaryRows := []struct{ label, value string }{
		{"Total Flights", fmt.Sprintf("%d", grandFlights)},
		{"Total Block Time", fmt.Sprintf("%.1f hours", grandTotal)},
		{"PIC Time", fmt.Sprintf("%.1f hours", grandPIC)},
		{"Dual Received", fmt.Sprintf("%.1f hours", grandDual)},
		{"Solo Time", fmt.Sprintf("%.1f hours", grandSolo)},
		{"Night Time", fmt.Sprintf("%.1f hours", grandNight)},
		{"IFR Time", fmt.Sprintf("%.1f hours", grandIFR)},
		{"Cross-Country Time", fmt.Sprintf("%.1f hours", grandXC)},
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

	// Send PDF
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=ninerlog_%s.pdf", time.Now().Format("2006-01-02")))
	pdf.Output(c.Writer)
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

func fmtDec(v float64) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprintf("%.1f", v)
}
