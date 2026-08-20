package handlers

import (
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/gin-gonic/gin"
)

// MonthlyTrend represents flight statistics for a single month
type MonthlyTrend struct {
	Month         string `json:"month"`
	TotalFlights  int    `json:"totalFlights"`
	TotalMinutes  int    `json:"totalMinutes"`
	PICMinutes    int    `json:"picMinutes"`
	DualMinutes   int    `json:"dualMinutes"`
	NightMinutes  int    `json:"nightMinutes"`
	IFRMinutes    int    `json:"ifrMinutes"`
	LandingsDay   int    `json:"landingsDay"`
	LandingsNight int    `json:"landingsNight"`
}

// AircraftBreakdown represents flight statistics per aircraft type
type AircraftBreakdown struct {
	AircraftType string `json:"aircraftType"`
	TotalFlights int    `json:"totalFlights"`
	TotalMinutes int    `json:"totalMinutes"`
}

// TrendsResponse contains all reporting data
type TrendsResponse struct {
	Monthly        []MonthlyTrend      `json:"monthly"`
	ByAircraftType []AircraftBreakdown `json:"byAircraftType"`
}

// GetFlightTrends implements GET /reports/trends
func (h *APIHandler) GetFlightTrends(c *gin.Context, params generated.GetFlightTrendsParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	months := 12
	if params.Months != nil && *params.Months >= 0 && *params.Months <= 600 {
		months = *params.Months
	}

	monthlyRows, err := h.reportsRepo.MonthlyTrends(c.Request.Context(), userID, months)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to query monthly trends")
		return
	}
	var monthly []MonthlyTrend
	for _, row := range monthlyRows {
		monthly = append(monthly, MonthlyTrend{
			Month:         row.Month,
			TotalFlights:  row.TotalFlights,
			TotalMinutes:  row.TotalMinutes,
			PICMinutes:    row.PICMinutes,
			DualMinutes:   row.DualMinutes,
			NightMinutes:  row.NightMinutes,
			IFRMinutes:    row.IFRMinutes,
			LandingsDay:   row.LandingsDay,
			LandingsNight: row.LandingsNight,
		})
	}
	if monthly == nil {
		monthly = []MonthlyTrend{}
	}

	aircraftRows, err := h.reportsRepo.AircraftTypeTrends(c.Request.Context(), userID, months)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to query aircraft breakdown")
		return
	}
	var byAircraft []AircraftBreakdown
	for _, row := range aircraftRows {
		byAircraft = append(byAircraft, AircraftBreakdown{
			AircraftType: row.AircraftType,
			TotalFlights: row.TotalFlights,
			TotalMinutes: row.TotalMinutes,
		})
	}
	if byAircraft == nil {
		byAircraft = []AircraftBreakdown{}
	}

	c.JSON(http.StatusOK, TrendsResponse{
		Monthly:        monthly,
		ByAircraftType: byAircraft,
	})
}

// GetStatsByClass implements GET /reports/stats-by-class
func (h *APIHandler) GetStatsByClass(c *gin.Context, params generated.GetStatsByClassParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	months := 12
	if params.Months != nil && *params.Months >= 0 && *params.Months <= 600 {
		months = *params.Months
	}

	classRows, err := h.reportsRepo.StatsByClass(c.Request.Context(), userID, months)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to query class stats")
		return
	}

	type ClassStat struct {
		Class       string `json:"class"`
		Flights     int    `json:"flights"`
		Minutes     int    `json:"minutes"`
		PICMinutes  int    `json:"picMinutes"`
		DualMinutes int    `json:"dualMinutes"`
		Landings    int    `json:"landings"`
	}
	var byClass []ClassStat
	for _, row := range classRows {
		byClass = append(byClass, ClassStat{
			Class:       row.Class,
			Flights:     row.Flights,
			Minutes:     row.Minutes,
			PICMinutes:  row.PICMinutes,
			DualMinutes: row.DualMinutes,
			Landings:    row.Landings,
		})
	}
	if byClass == nil {
		byClass = []ClassStat{}
	}

	// Time by aircraft category (tailwheel, complex, high-performance)
	categoryRows, err := h.reportsRepo.StatsByCategory(c.Request.Context(), userID, months)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to query category stats")
		return
	}

	type CategoryStat struct {
		Category    string `json:"category"`
		Flights     int    `json:"flights"`
		PICMinutes  int    `json:"picMinutes"`
		DualMinutes int    `json:"dualMinutes"`
	}
	var byCategory []CategoryStat
	for _, row := range categoryRows {
		byCategory = append(byCategory, CategoryStat{
			Category:    row.Category,
			Flights:     row.Flights,
			PICMinutes:  row.PICMinutes,
			DualMinutes: row.DualMinutes,
		})
	}
	if byCategory == nil {
		byCategory = []CategoryStat{}
	}

	licenses, _ := h.licenseService.ListLicenses(c.Request.Context(), userID)
	type AuthorityStat struct {
		Authority   string `json:"authority"`
		LicenseType string `json:"licenseType"`
		Flights     int    `json:"flights"`
		Minutes     int    `json:"minutes"`
	}
	var byAuthority []AuthorityStat
	authorityMap := make(map[string]*AuthorityStat)
	for _, lic := range licenses {
		key := lic.RegulatoryAuthority + "|" + lic.LicenseType
		if _, exists := authorityMap[key]; !exists {
			authorityMap[key] = &AuthorityStat{
				Authority:   lic.RegulatoryAuthority,
				LicenseType: lic.LicenseType,
			}
		}
	}
	var overallFlights int
	var overallMinutes int
	for _, cs := range byClass {
		overallFlights += cs.Flights
		overallMinutes += cs.Minutes
	}
	for _, stat := range authorityMap {
		stat.Flights = overallFlights
		stat.Minutes = overallMinutes
		byAuthority = append(byAuthority, *stat)
	}
	if byAuthority == nil {
		byAuthority = []AuthorityStat{}
	}

	c.JSON(http.StatusOK, gin.H{
		"byClass":     byClass,
		"byCategory":  byCategory,
		"byAuthority": byAuthority,
	})
}
