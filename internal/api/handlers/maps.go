package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup"
	"github.com/fjaeckel/ninerlog-api/pkg/email"
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

	routeCounts, err := h.reportsRepo.RouteCounts(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to query routes")
		return
	}

	airportSet := make(map[string]*airports.AirportInfo)
	var routes []generated.FlightRoute

	for _, rc := range routeCounts {
		dep, arr, count := rc.DepartureICAO, rc.ArrivalICAO, rc.FlightCount
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

	airportCounts, err := h.reportsRepo.AirportCounts(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to query airport stats")
		return
	}

	type counts struct {
		departures int
		arrivals   int
	}
	statsMap := make(map[string]*counts)

	for _, ac := range airportCounts {
		upper := strings.ToUpper(ac.ICAO)
		if _, ok := statsMap[upper]; !ok {
			statsMap[upper] = &counts{}
		}
		if ac.Direction == "dep" {
			statsMap[upper].departures = ac.Count
		} else {
			statsMap[upper].arrivals = ac.Count
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

// SetAdminRepository stores the repository behind the admin console (stats,
// audit log, user listing, maintenance sweeps).
func (h *APIHandler) SetAdminRepository(repo repository.AdminRepository) {
	h.adminRepo = repo
}

// SetAnnouncementRepository stores the system announcement repository.
func (h *APIHandler) SetAnnouncementRepository(repo repository.AnnouncementRepository) {
	h.announcementRepo = repo
}

// SetFlightImportRepository stores the import-history repository.
func (h *APIHandler) SetFlightImportRepository(repo repository.FlightImportRepository) {
	h.flightImportRepo = repo
}

// SetReportsRepository stores the reports/analytics read repository.
func (h *APIHandler) SetReportsRepository(repo repository.ReportsRepository) {
	h.reportsRepo = repo
}

// SetUserContentRepository stores the account-content wipe repository.
func (h *APIHandler) SetUserContentRepository(repo repository.UserContentRepository) {
	h.userContentRepo = repo
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

// SetBackupService stores the cloud backup service (optional — only set when
// BACKUP_CREDENTIALS_KEY is configured at startup).
func (h *APIHandler) SetBackupService(s *cloudbackup.Service) {
	h.backupService = s
}

// SetOIDCService stores the OIDC service (optional — only set when
// OIDC_ISSUER is configured). A non-nil service also disables every local
// credential endpoint; see requireLocalAuth.
func (h *APIHandler) SetOIDCService(s *service.OIDCService) {
	h.oidcService = s
}

// SetFlightSessionService stores the tap-to-log flight session service
func (h *APIHandler) SetFlightSessionService(s *service.FlightSessionService) {
	h.flightSessionService = s
}

// SetFlightSignatureService stores the instructor sign-off service
func (h *APIHandler) SetFlightSignatureService(s *service.FlightSignatureService) {
	h.flightSignatureService = s
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
