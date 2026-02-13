package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// MonthlyTrend represents flight statistics for a single month
type MonthlyTrend struct {
	Month         string  `json:"month"`
	TotalFlights  int     `json:"totalFlights"`
	TotalHours    float64 `json:"totalHours"`
	PICHours      float64 `json:"picHours"`
	DualHours     float64 `json:"dualHours"`
	NightHours    float64 `json:"nightHours"`
	IFRHours      float64 `json:"ifrHours"`
	LandingsDay   int     `json:"landingsDay"`
	LandingsNight int     `json:"landingsNight"`
}

// AircraftBreakdown represents flight statistics per aircraft type
type AircraftBreakdown struct {
	AircraftType string  `json:"aircraftType"`
	TotalFlights int     `json:"totalFlights"`
	TotalHours   float64 `json:"totalHours"`
}

// TrendsResponse contains all reporting data
type TrendsResponse struct {
	Monthly        []MonthlyTrend      `json:"monthly"`
	ByAircraftType []AircraftBreakdown `json:"byAircraftType"`
}

// RegisterReportsRoutes adds the custom reports endpoints to the router
func RegisterReportsRoutes(api *gin.RouterGroup, h *APIHandler, db *sql.DB) {
	api.GET("/reports/trends", func(c *gin.Context) {
		userID, err := h.getUserIDFromContext(c)
		if err != nil {
			h.sendError(c, http.StatusUnauthorized, "Unauthorized")
			return
		}

		months := 12
		if m := c.Query("months"); m != "" {
			if parsed, err := strconv.Atoi(m); err == nil && parsed > 0 && parsed <= 60 {
				months = parsed
			}
		}

		// Monthly trends
		monthlyQuery := `
			SELECT
				TO_CHAR(date_trunc('month', date), 'YYYY-MM') AS month,
				COUNT(*) AS total_flights,
				COALESCE(SUM(total_time), 0) AS total_hours,
				COALESCE(SUM(pic_time), 0) AS pic_hours,
				COALESCE(SUM(dual_time), 0) AS dual_hours,
				COALESCE(SUM(night_time), 0) AS night_hours,
				COALESCE(SUM(ifr_time), 0) AS ifr_hours,
				COALESCE(SUM(landings_day), 0) AS landings_day,
				COALESCE(SUM(landings_night), 0) AS landings_night
			FROM flights
			WHERE user_id = $1
			  AND date >= date_trunc('month', CURRENT_DATE - ($2 || ' months')::interval)
			GROUP BY date_trunc('month', date)
			ORDER BY date_trunc('month', date) ASC
		`

		rows, err := db.QueryContext(c.Request.Context(), monthlyQuery, userID, months)
		if err != nil {
			h.sendError(c, http.StatusInternalServerError, "Failed to query monthly trends")
			return
		}
		defer rows.Close()

		var monthly []MonthlyTrend
		for rows.Next() {
			var t MonthlyTrend
			if err := rows.Scan(
				&t.Month, &t.TotalFlights, &t.TotalHours,
				&t.PICHours, &t.DualHours, &t.NightHours, &t.IFRHours,
				&t.LandingsDay, &t.LandingsNight,
			); err != nil {
				h.sendError(c, http.StatusInternalServerError, "Failed to scan monthly trends")
				return
			}
			monthly = append(monthly, t)
		}
		if monthly == nil {
			monthly = []MonthlyTrend{}
		}

		// By aircraft type
		aircraftQuery := `
			SELECT
				aircraft_type,
				COUNT(*) AS total_flights,
				COALESCE(SUM(total_time), 0) AS total_hours
			FROM flights
			WHERE user_id = $1
			GROUP BY aircraft_type
			ORDER BY total_hours DESC
		`

		rows2, err := db.QueryContext(c.Request.Context(), aircraftQuery, userID)
		if err != nil {
			h.sendError(c, http.StatusInternalServerError, "Failed to query aircraft breakdown")
			return
		}
		defer rows2.Close()

		var byAircraft []AircraftBreakdown
		for rows2.Next() {
			var ab AircraftBreakdown
			if err := rows2.Scan(&ab.AircraftType, &ab.TotalFlights, &ab.TotalHours); err != nil {
				h.sendError(c, http.StatusInternalServerError, "Failed to scan aircraft breakdown")
				return
			}
			byAircraft = append(byAircraft, ab)
		}
		if byAircraft == nil {
			byAircraft = []AircraftBreakdown{}
		}

		c.JSON(http.StatusOK, TrendsResponse{
			Monthly:        monthly,
			ByAircraftType: byAircraft,
		})
	})
}
