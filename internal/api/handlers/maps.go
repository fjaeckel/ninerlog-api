package handlers

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/airports"
	"github.com/fjaeckel/pilotlog-api/internal/api/generated"
	"github.com/fjaeckel/pilotlog-api/pkg/email"
	"github.com/gin-gonic/gin"
)

// GetAirport implements GET /airports/{icaoCode}
func (h *APIHandler) GetAirport(c *gin.Context, icaoCode string) {
	code := strings.ToUpper(icaoCode)
	ap := airports.Lookup(code)
	if ap == nil {
		h.sendError(c, http.StatusNotFound, "Airport not found")
		return
	}
	c.JSON(http.StatusOK, toGeneratedAirport(ap))
}

// SearchAirports implements GET /airports/search
func (h *APIHandler) SearchAirports(c *gin.Context, params generated.SearchAirportsParams) {
	limit := 10
	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
		if limit > 50 {
			limit = 50
		}
	}
	results := airports.Search(params.Q, limit)
	out := make([]generated.Airport, 0, len(results))
	for _, a := range results {
		out = append(out, toGeneratedAirport(&a))
	}
	c.JSON(http.StatusOK, out)
}

// GetFlightRoutes implements GET /reports/routes
func (h *APIHandler) GetFlightRoutes(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	query := `
		SELECT departure_icao, arrival_icao, COUNT(*) AS flight_count
		FROM flights
		WHERE user_id = $1
		  AND departure_icao IS NOT NULL AND departure_icao != ''
		  AND arrival_icao IS NOT NULL AND arrival_icao != ''
		GROUP BY departure_icao, arrival_icao
		ORDER BY flight_count DESC
	`

	rows, err := h.db.QueryContext(c.Request.Context(), query, userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to query routes")
		return
	}
	defer rows.Close()

	airportSet := make(map[string]*airports.AirportInfo)
	var routes []generated.FlightRoute

	for rows.Next() {
		var dep, arr string
		var count int
		if err := rows.Scan(&dep, &arr, &count); err != nil {
			continue
		}
		depAP := airports.Lookup(strings.ToUpper(dep))
		arrAP := airports.Lookup(strings.ToUpper(arr))
		if depAP == nil || arrAP == nil {
			continue
		}

		airportSet[depAP.ICAO] = depAP
		airportSet[arrAP.ICAO] = arrAP

		routes = append(routes, generated.FlightRoute{
			DepartureIcao: dep,
			ArrivalIcao:   arr,
			DepartureCoords: struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			}{Lat: depAP.Latitude, Lng: depAP.Longitude},
			ArrivalCoords: struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			}{Lat: arrAP.Latitude, Lng: arrAP.Longitude},
			FlightCount: count,
		})
	}

	if routes == nil {
		routes = []generated.FlightRoute{}
	}

	uniqueAirports := make([]generated.Airport, 0, len(airportSet))
	for _, a := range airportSet {
		uniqueAirports = append(uniqueAirports, toGeneratedAirport(a))
	}

	c.JSON(http.StatusOK, generated.FlightRoutesResponse{
		Routes:   routes,
		Airports: uniqueAirports,
	})
}

// GetAirportStats implements GET /reports/airport-stats
func (h *APIHandler) GetAirportStats(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	query := `
		SELECT icao, direction, COUNT(*) AS cnt FROM (
			SELECT departure_icao AS icao, 'dep' AS direction FROM flights
			WHERE user_id = $1 AND departure_icao IS NOT NULL AND departure_icao != ''
			UNION ALL
			SELECT arrival_icao AS icao, 'arr' AS direction FROM flights
			WHERE user_id = $1 AND arrival_icao IS NOT NULL AND arrival_icao != ''
		) sub
		GROUP BY icao, direction
		ORDER BY icao
	`

	rows, err := h.db.QueryContext(c.Request.Context(), query, userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to query airport stats")
		return
	}
	defer rows.Close()

	type counts struct {
		departures int
		arrivals   int
	}
	statsMap := make(map[string]*counts)

	for rows.Next() {
		var icao, dir string
		var cnt int
		if err := rows.Scan(&icao, &dir, &cnt); err != nil {
			continue
		}
		upper := strings.ToUpper(icao)
		if _, ok := statsMap[upper]; !ok {
			statsMap[upper] = &counts{}
		}
		if dir == "dep" {
			statsMap[upper].departures = cnt
		} else {
			statsMap[upper].arrivals = cnt
		}
	}

	var result []generated.AirportStats
	for icao, c := range statsMap {
		ap := airports.Lookup(icao)
		name := icao
		var lat, lng float64
		if ap != nil {
			name = ap.Name
			lat = ap.Latitude
			lng = ap.Longitude
		}
		result = append(result, generated.AirportStats{
			Icao:         icao,
			Name:         name,
			Latitude:     lat,
			Longitude:    lng,
			Departures:   c.departures,
			Arrivals:     c.arrivals,
			TotalFlights: c.departures + c.arrivals,
		})
	}
	if result == nil {
		result = []generated.AirportStats{}
	}

	c.JSON(http.StatusOK, result)
}

// SetDB stores the database reference for direct queries in map handlers
func (h *APIHandler) SetDB(db *sql.DB) {
	h.db = db
}

// SetEmailSender stores the email sender for admin SMTP test
func (h *APIHandler) SetEmailSender(sender *email.Sender) {
	h.emailSender = sender
}

// SetStartedAt records the server start time for uptime calculation
func (h *APIHandler) SetStartedAt(t time.Time) {
	h.startedAt = t
}

// SetCORSOrigins stores the configured CORS origins for the config endpoint
func (h *APIHandler) SetCORSOrigins(origins []string) {
	h.corsOrigins = origins
}

func toGeneratedAirport(a *airports.AirportInfo) generated.Airport {
	ap := generated.Airport{
		Icao:      a.ICAO,
		Name:      a.Name,
		Latitude:  a.Latitude,
		Longitude: a.Longitude,
	}
	if a.Elevation != 0 {
		elev := a.Elevation
		ap.Elevation = &elev
	}
	if a.Country != "" {
		ap.Country = &a.Country
	}
	return ap
}
