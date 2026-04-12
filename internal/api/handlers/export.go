package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/pkg/duration"
	"github.com/gin-gonic/gin"
)

// ExportFlightsCSV implements GET /exports/csv
func (h *APIHandler) ExportFlightsCSV(c *gin.Context, params generated.ExportFlightsCSVParams) {
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

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=ninerlog_flights_%s.csv", time.Now().Format("2006-01-02")))

	w := csv.NewWriter(c.Writer)

	format := "standard"
	if params.Format != nil {
		format = string(*params.Format)
	}

	switch format {
	case "easa":
		writeEASACSV(w, flights)
	case "faa":
		writeFAACSV(w, flights)
	default:
		writeStandardCSV(w, flights)
	}

	w.Flush()
}

func writeStandardCSV(w *csv.Writer, flights []*models.Flight) {
	// Header row — compatible with ForeFlight import format
	headers := []string{
		"Date", "AircraftID", "AircraftType", "From", "To", "Route",
		"TimeOut", "TimeOff", "TimeOn", "TimeIn",
		"TotalTime", "PIC", "SIC", "Night", "Solo", "CrossCountry",
		"Distance", "DayTakeoffs", "DayLandingsFullStop",
		"NightTakeoffs", "NightLandingsFullStop", "AllLandings",
		"ActualInstrument", "SimulatedInstrument",
		"Holds", "ApproachesCount",
		"DualGiven", "DualReceived", "SimulatedFlight", "GroundTraining",
		"InstructorName", "InstructorComments",
		"FlightReview", "IPC",
		"IFRTime", "Remarks",
		"PICName", "MultiPilotTime", "FSTDType", "Endorsements",
	}
	w.Write(headers)

	for _, f := range flights {
		dep, arr := "", ""
		if f.DepartureICAO != nil {
			dep = *f.DepartureICAO
		}
		if f.ArrivalICAO != nil {
			arr = *f.ArrivalICAO
		}
		offBlock, onBlock, depTime, arrTime := "", "", "", ""
		if f.OffBlockTime != nil {
			offBlock = *f.OffBlockTime
		}
		if f.OnBlockTime != nil {
			onBlock = *f.OnBlockTime
		}
		if f.DepartureTime != nil {
			depTime = *f.DepartureTime
		}
		if f.ArrivalTime != nil {
			arrTime = *f.ArrivalTime
		}
		route := ""
		if f.Route != nil {
			route = *f.Route
		}
		remarks := ""
		if f.Remarks != nil {
			remarks = *f.Remarks
		}
		instrName, instrComments := "", ""
		if f.InstructorName != nil {
			instrName = *f.InstructorName
		}
		if f.InstructorComments != nil {
			instrComments = *f.InstructorComments
		}
		picName := ""
		if f.PICName != nil {
			picName = *f.PICName
		}
		fstdType := ""
		if f.FSTDType != nil {
			fstdType = *f.FSTDType
		}
		endorsements := ""
		if f.Endorsements != nil {
			endorsements = *f.Endorsements
		}

		row := []string{
			f.Date.Format("2006-01-02"),
			f.AircraftReg,
			f.AircraftType,
			dep,
			arr,
			route,
			offBlock,
			depTime,
			arrTime,
			onBlock,
			duration.FormatDecimal(f.TotalTime),
			duration.FormatDecimal(f.PICTime),
			duration.FormatDecimal(f.SICTime),
			duration.FormatDecimal(f.NightTime),
			duration.FormatDecimal(f.SoloTime),
			duration.FormatDecimal(f.CrossCountryTime),
			fmt.Sprintf("%.1f", f.Distance),
			fmt.Sprintf("%d", f.TakeoffsDay),
			fmt.Sprintf("%d", f.LandingsDay),
			fmt.Sprintf("%d", f.TakeoffsNight),
			fmt.Sprintf("%d", f.LandingsNight),
			fmt.Sprintf("%d", f.AllLandings),
			duration.FormatDecimal(f.ActualInstrumentTime),
			duration.FormatDecimal(f.SimulatedInstrumentTime),
			fmt.Sprintf("%d", f.Holds),
			fmt.Sprintf("%d", f.ApproachesCount),
			duration.FormatDecimal(f.DualGivenTime),
			duration.FormatDecimal(f.DualTime),
			duration.FormatDecimal(f.SimulatedFlightTime),
			duration.FormatDecimal(f.GroundTrainingTime),
			instrName,
			instrComments,
			fmt.Sprintf("%t", f.IsFlightReview),
			fmt.Sprintf("%t", f.IsIPC),
			duration.FormatDecimal(f.IFRTime),
			remarks,
			picName,
			duration.FormatDecimal(f.MultiPilotTime),
			fstdType,
			endorsements,
		}
		w.Write(row)
	}
}

func writeEASACSV(w *csv.Writer, flights []*models.Flight) {
	// EASA AMC1 FCL.050 columns (24 cols)
	headers := []string{
		"Date", "Dep Place", "Dep Time", "Arr Place", "Arr Time",
		"A/C Type", "A/C Reg",
		"SP-SE", "SP-ME", "Multi-Pilot",
		"Total Time",
		"PIC Name",
		"Ldg Day", "Ldg Night",
		"Night", "IFR",
		"PIC", "Co-Pilot", "Dual", "Instructor",
		"FSTD Date", "FSTD Type", "FSTD Time",
		"Remarks & Endorsements",
	}
	w.Write(headers)

	for _, f := range flights {
		dep := safeStrCSV(f.DepartureICAO)
		arr := safeStrCSV(f.ArrivalICAO)
		depTime := fmtTimeCSV(f.OffBlockTime)
		arrTime := fmtTimeCSV(f.OnBlockTime)
		picName := "SELF"
		if f.PICName != nil && *f.PICName != "" {
			picName = *f.PICName
		}

		// SP-SE / SP-ME derived (simplified: if not multi-pilot, it's single-pilot)
		spSE, spME, mp := "", "", ""
		if f.MultiPilotTime > 0 {
			mp = fmtHM(f.MultiPilotTime)
		} else {
			// Default to SP-SE for single-engine (most common)
			spSE = fmtHM(f.TotalTime)
		}

		// FSTD columns
		fstdDate, fstdType, fstdTime := "", "", ""
		if f.FSTDType != nil && *f.FSTDType != "" && f.SimulatedFlightTime > 0 {
			fstdDate = f.Date.Format("02.01.2006")
			fstdType = *f.FSTDType
			fstdTime = fmtHM(f.SimulatedFlightTime)
		}

		remarksAndEndorsements := safeStrCSV(f.Remarks)
		if f.Endorsements != nil && *f.Endorsements != "" {
			if remarksAndEndorsements != "" {
				remarksAndEndorsements += " | "
			}
			remarksAndEndorsements += *f.Endorsements
		}

		row := []string{
			f.Date.Format("02.01.2006"),
			dep, depTime, arr, arrTime,
			f.AircraftType, f.AircraftReg,
			spSE, spME, mp,
			fmtHM(f.TotalTime),
			picName,
			fmt.Sprintf("%d", f.LandingsDay), fmt.Sprintf("%d", f.LandingsNight),
			fmtHM(f.NightTime), fmtHM(f.IFRTime),
			fmtHM(f.PICTime), fmtHM(f.SICTime), fmtHM(f.DualTime), fmtHM(f.DualGivenTime),
			fstdDate, fstdType, fstdTime,
			remarksAndEndorsements,
		}
		w.Write(row)
	}
}

func writeFAACSV(w *csv.Writer, flights []*models.Flight) {
	// FAA standard logbook columns (ASA/Jeppesen layout)
	headers := []string{
		"Date", "A/C Type", "A/C Ident", "From", "To",
		"Solo", "PIC", "SIC", "Dual Rcvd", "Instr Given",
		"Actual Inst", "Sim Inst", "XC", "Night",
		"Day Ldg", "Night Ldg",
		"Approaches", "Holds",
		"Total",
		"Remarks/Endorsements",
	}
	w.Write(headers)

	for _, f := range flights {
		dep := safeStrCSV(f.DepartureICAO)
		arr := safeStrCSV(f.ArrivalICAO)

		remarks := safeStrCSV(f.Remarks)
		if f.Endorsements != nil && *f.Endorsements != "" {
			if remarks != "" {
				remarks += " | "
			}
			remarks += *f.Endorsements
		}
		if f.IsIPC {
			remarks += " [IPC]"
		}
		if f.IsFlightReview {
			remarks += " [FR]"
		}

		row := []string{
			f.Date.Format("01/02/2006"),
			f.AircraftType, f.AircraftReg,
			dep, arr,
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
		w.Write(row)
	}
}

func safeStrCSV(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func fmtTimeCSV(s *string) string {
	if s == nil {
		return ""
	}
	v := *s
	if len(v) >= 5 {
		return v[:5]
	}
	return v
}

func fmtHM(v int) string {
	if v == 0 {
		return ""
	}
	h := v / 60
	m := v % 60
	return fmt.Sprintf("%d:%02d", h, m)
}

// ExportDataJSON implements GET /exports/json
func (h *APIHandler) ExportDataJSON(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Gather all user data
	flights, _ := h.flightService.ListFlights(c.Request.Context(), userID, nil)
	aircraft, _ := h.aircraftService.ListAircraft(c.Request.Context(), userID)
	licenses, _ := h.licenseService.ListLicenses(c.Request.Context(), userID)
	credentials, _ := h.credentialService.ListCredentials(c.Request.Context(), userID)

	// Class ratings per license
	type licenseWithRatings struct {
		License      interface{}   `json:"license"`
		ClassRatings []interface{} `json:"classRatings"`
	}
	var licensesWithRatings []licenseWithRatings
	for _, lic := range licenses {
		ratings, _ := h.classRatingService.ListClassRatings(c.Request.Context(), lic.ID, userID)
		var ratingInterfaces []interface{}
		for _, r := range ratings {
			ratingInterfaces = append(ratingInterfaces, r)
		}
		licensesWithRatings = append(licensesWithRatings, licenseWithRatings{
			License:      lic,
			ClassRatings: ratingInterfaces,
		})
	}

	backup := map[string]interface{}{
		"exportedAt":  time.Now().UTC().Format(time.RFC3339),
		"version":     "1.0",
		"format":      "NinerLog JSON Backup",
		"flights":     flights,
		"aircraft":    aircraft,
		"licenses":    licensesWithRatings,
		"credentials": credentials,
	}

	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=ninerlog_backup_%s.json", time.Now().Format("2006-01-02")))

	encoder := json.NewEncoder(c.Writer)
	encoder.SetIndent("", "  ")
	encoder.Encode(backup)
}
