package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/pkg/duration"
	"github.com/gin-gonic/gin"
)

// ExportFlightsCSV implements GET /exports/csv
func (h *APIHandler) ExportFlightsCSV(c *gin.Context) {
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
		}
		w.Write(row)
	}

	w.Flush()
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
