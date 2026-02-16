package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

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
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=pilotlog_flights_%s.csv", time.Now().Format("2006-01-02")))

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
			fmt.Sprintf("%.1f", f.TotalTime),
			fmt.Sprintf("%.1f", f.PICTime),
			fmt.Sprintf("%.1f", f.SICTime),
			fmt.Sprintf("%.1f", f.NightTime),
			fmt.Sprintf("%.1f", f.SoloTime),
			fmt.Sprintf("%.1f", f.CrossCountryTime),
			fmt.Sprintf("%.1f", f.Distance),
			fmt.Sprintf("%d", f.TakeoffsDay),
			fmt.Sprintf("%d", f.LandingsDay),
			fmt.Sprintf("%d", f.TakeoffsNight),
			fmt.Sprintf("%d", f.LandingsNight),
			fmt.Sprintf("%d", f.AllLandings),
			fmt.Sprintf("%.1f", f.ActualInstrumentTime),
			fmt.Sprintf("%.1f", f.SimulatedInstrumentTime),
			fmt.Sprintf("%d", f.Holds),
			fmt.Sprintf("%d", f.ApproachesCount),
			fmt.Sprintf("%.1f", f.DualGivenTime),
			fmt.Sprintf("%.1f", f.DualTime),
			fmt.Sprintf("%.1f", f.SimulatedFlightTime),
			fmt.Sprintf("%.1f", f.GroundTrainingTime),
			instrName,
			instrComments,
			fmt.Sprintf("%t", f.IsFlightReview),
			fmt.Sprintf("%t", f.IsIPC),
			fmt.Sprintf("%.1f", f.IFRTime),
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
		"format":      "PilotLog JSON Backup",
		"flights":     flights,
		"aircraft":    aircraft,
		"licenses":    licensesWithRatings,
		"credentials": credentials,
	}

	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=pilotlog_backup_%s.json", time.Now().Format("2006-01-02")))

	encoder := json.NewEncoder(c.Writer)
	encoder.SetIndent("", "  ")
	encoder.Encode(backup)
}
