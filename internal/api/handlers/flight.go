package handlers

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/api/generated"
	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// ListFlights implements GET /flights
// (GET /flights)
func (h *APIHandler) ListFlights(c *gin.Context, params generated.ListFlightsParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// TODO: Implement filtering, pagination, and sorting based on params
	// For now, return all flights for the user
	flights, err := h.flightService.ListFlights(c.Request.Context(), userID, nil)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve flights")
		return
	}

	// Convert to paginated response as per OpenAPI spec
	flightList := make([]generated.Flight, 0, len(flights))
	for _, f := range flights {
		flightList = append(flightList, convertToGeneratedFlight(f))
	}

	response := generated.PaginatedFlights{
		Data: flightList,
		Pagination: struct {
			Page       int "json:\"page\""
			PageSize   int "json:\"pageSize\""
			Total      int "json:\"total\""
			TotalPages int "json:\"totalPages\""
		}{
			Page:       1,
			PageSize:   len(flightList),
			Total:      len(flightList),
			TotalPages: 1,
		},
	}

	c.JSON(http.StatusOK, response)
}

// CreateFlight implements POST /flights
// (POST /flights)
func (h *APIHandler) CreateFlight(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.CreateFlightJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Parse date
	flightDate, err := time.Parse("2006-01-02", req.Date.String())
	if err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid date format")
		return
	}

	// Compute totalTime from off-block and on-block times
	totalTime, err := calculateBlockTime(req.OffBlockTime, req.OnBlockTime)
	if err != nil {
		h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Invalid block times: %v", err))
		return
	}

	// Determine PIC/Dual from booleans (default to PIC if neither specified)
	isPic := true
	isDual := false
	if req.IsPic != nil {
		isPic = *req.IsPic
	}
	if req.IsDual != nil {
		isDual = *req.IsDual
	}
	if isPic && isDual {
		h.sendError(c, http.StatusBadRequest, "isPic and isDual are mutually exclusive")
		return
	}

	// Compute picTime/dualTime from booleans
	var picTime, dualTime float64
	if isPic {
		picTime = totalTime
	}
	if isDual {
		dualTime = totalTime
	}

	departureIcao := req.DepartureIcao
	arrivalIcao := req.ArrivalIcao
	offBlockTime := req.OffBlockTime
	onBlockTime := req.OnBlockTime
	departureTime := req.DepartureTime
	arrivalTime := req.ArrivalTime

	flight := models.Flight{
		UserID:        userID,
		LicenseID:     uuid.UUID(req.LicenseId),
		Date:          flightDate,
		AircraftReg:   req.AircraftReg,
		AircraftType:  req.AircraftType,
		DepartureICAO: &departureIcao,
		ArrivalICAO:   &arrivalIcao,
		OffBlockTime:  &offBlockTime,
		OnBlockTime:   &onBlockTime,
		DepartureTime: &departureTime,
		ArrivalTime:   &arrivalTime,
		TotalTime:     totalTime,
		IsPIC:         isPic,
		IsDual:        isDual,
		PICTime:       picTime,
		DualTime:      dualTime,
		SoloTime:      float64(getFloat32OrDefault(req.SoloTime, 0)),
		NightTime:     float64(getFloat32OrDefault(req.NightTime, 0)),
		IFRTime:       float64(getFloat32OrDefault(req.IfrTime, 0)),
		LandingsDay:   req.LandingsDay,
		LandingsNight: req.LandingsNight,
	}

	if req.Remarks != nil {
		flight.Remarks = req.Remarks
	}

	if err := h.flightService.CreateFlight(c.Request.Context(), &flight); err != nil {
		h.sendError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusCreated, convertToGeneratedFlight(&flight))
}

// GetFlight implements GET /flights/{flightId}
// (GET /flights/{flightId})
func (h *APIHandler) GetFlight(c *gin.Context, flightId generated.FlightId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	flight, err := h.flightService.GetFlight(c.Request.Context(), uuid.UUID(flightId), userID)
	if err != nil {
		if errors.Is(err, service.ErrFlightNotFound) || errors.Is(err, service.ErrUnauthorizedFlight) {
			h.sendError(c, http.StatusNotFound, "Flight not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve flight")
		return
	}

	c.JSON(http.StatusOK, convertToGeneratedFlight(flight))
}

// UpdateFlight implements PUT /flights/{flightId}
// (PUT /flights/{flightId})
func (h *APIHandler) UpdateFlight(c *gin.Context, flightId generated.FlightId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.UpdateFlightJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Get existing flight
	flight, err := h.flightService.GetFlight(c.Request.Context(), uuid.UUID(flightId), userID)
	if err != nil {
		if errors.Is(err, service.ErrFlightNotFound) || errors.Is(err, service.ErrUnauthorizedFlight) {
			h.sendError(c, http.StatusNotFound, "Flight not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve flight")
		return
	}

	// Apply updates
	if req.Date != nil {
		flightDate, err := time.Parse("2006-01-02", req.Date.String())
		if err != nil {
			h.sendError(c, http.StatusBadRequest, "Invalid date format")
			return
		}
		flight.Date = flightDate
	}
	if req.AircraftReg != nil {
		flight.AircraftReg = *req.AircraftReg
	}
	if req.AircraftType != nil {
		flight.AircraftType = *req.AircraftType
	}
	if req.IsPic != nil {
		flight.IsPIC = *req.IsPic
	}
	if req.IsDual != nil {
		flight.IsDual = *req.IsDual
	}
	// Validate mutual exclusivity
	if flight.IsPIC && flight.IsDual {
		h.sendError(c, http.StatusBadRequest, "isPic and isDual are mutually exclusive")
		return
	}
	if req.SoloTime != nil {
		flight.SoloTime = float64(*req.SoloTime)
	}
	if req.NightTime != nil {
		flight.NightTime = float64(*req.NightTime)
	}
	if req.IfrTime != nil {
		flight.IFRTime = float64(*req.IfrTime)
	}
	if req.LandingsDay != nil {
		flight.LandingsDay = *req.LandingsDay
	}
	if req.LandingsNight != nil {
		flight.LandingsNight = *req.LandingsNight
	}
	if req.DepartureIcao != nil {
		flight.DepartureICAO = req.DepartureIcao
	}
	if req.ArrivalIcao != nil {
		flight.ArrivalICAO = req.ArrivalIcao
	}
	if req.OffBlockTime != nil {
		flight.OffBlockTime = req.OffBlockTime
	}
	if req.OnBlockTime != nil {
		flight.OnBlockTime = req.OnBlockTime
	}
	if req.DepartureTime != nil {
		flight.DepartureTime = req.DepartureTime
	}
	if req.ArrivalTime != nil {
		flight.ArrivalTime = req.ArrivalTime
	}
	if req.Remarks != nil {
		flight.Remarks = req.Remarks
	}

	// Recalculate totalTime from block times if either was updated
	if req.OffBlockTime != nil || req.OnBlockTime != nil {
		offBlock := ""
		onBlock := ""
		if flight.OffBlockTime != nil {
			offBlock = *flight.OffBlockTime
		}
		if flight.OnBlockTime != nil {
			onBlock = *flight.OnBlockTime
		}
		if offBlock != "" && onBlock != "" {
			totalTime, err := calculateBlockTime(offBlock, onBlock)
			if err != nil {
				h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Invalid block times: %v", err))
				return
			}
			flight.TotalTime = totalTime
		}
	} else if req.TotalTime != nil {
		// Allow direct totalTime override only if block times are not being updated
		flight.TotalTime = float64(*req.TotalTime)
	}

	// Recompute picTime/dualTime from booleans and totalTime
	if flight.IsPIC {
		flight.PICTime = flight.TotalTime
		flight.DualTime = 0
	} else if flight.IsDual {
		flight.DualTime = flight.TotalTime
		flight.PICTime = 0
	} else {
		flight.PICTime = 0
		flight.DualTime = 0
	}

	if err := h.flightService.UpdateFlight(c.Request.Context(), flight, userID); err != nil {
		h.sendError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, convertToGeneratedFlight(flight))
}

// DeleteFlight implements DELETE /flights/{flightId}
// (DELETE /flights/{flightId})
func (h *APIHandler) DeleteFlight(c *gin.Context, flightId generated.FlightId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := h.flightService.DeleteFlight(c.Request.Context(), uuid.UUID(flightId), userID); err != nil {
		if errors.Is(err, service.ErrFlightNotFound) || errors.Is(err, service.ErrUnauthorizedFlight) {
			h.sendError(c, http.StatusNotFound, "Flight not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to delete flight")
		return
	}

	// OpenAPI spec requires 204 No Content for successful DELETE
	c.Status(http.StatusNoContent)
}

// Helper functions

func convertToGeneratedFlight(f *models.Flight) generated.Flight {
	flight := generated.Flight{
		Id:            openapi_types.UUID(f.ID),
		UserId:        openapi_types.UUID(f.UserID),
		LicenseId:     openapi_types.UUID(f.LicenseID),
		Date:          openapi_types.Date{Time: f.Date},
		AircraftReg:   f.AircraftReg,
		AircraftType:  f.AircraftType,
		TotalTime:     float32(f.TotalTime),
		IsPic:         f.IsPIC,
		IsDual:        f.IsDual,
		PicTime:       float32(f.PICTime),
		DualTime:      float32(f.DualTime),
		SoloTime:      float32(f.SoloTime),
		NightTime:     float32(f.NightTime),
		IfrTime:       float32(f.IFRTime),
		LandingsDay:   f.LandingsDay,
		LandingsNight: f.LandingsNight,
		CreatedAt:     f.CreatedAt,
		UpdatedAt:     f.UpdatedAt,
	}

	if f.DepartureICAO != nil {
		flight.DepartureIcao = f.DepartureICAO
	}
	if f.ArrivalICAO != nil {
		flight.ArrivalIcao = f.ArrivalICAO
	}
	if f.OffBlockTime != nil {
		flight.OffBlockTime = f.OffBlockTime
	}
	if f.OnBlockTime != nil {
		flight.OnBlockTime = f.OnBlockTime
	}
	if f.DepartureTime != nil {
		flight.DepartureTime = f.DepartureTime
	}
	if f.ArrivalTime != nil {
		flight.ArrivalTime = f.ArrivalTime
	}
	if f.Remarks != nil {
		flight.Remarks = f.Remarks
	}

	return flight
}

func getFloat32OrDefault(val *float32, def float32) float32 {
	if val != nil {
		return *val
	}
	return def
}

// calculateBlockTime computes total block time in hours from off-block and on-block time strings (HH:MM:SS).
// Handles overnight flights (on-block before off-block crosses midnight).
func calculateBlockTime(offBlock, onBlock string) (float64, error) {
	offT, err := time.Parse("15:04:05", offBlock)
	if err != nil {
		// Try HH:MM format as well
		offT, err = time.Parse("15:04", offBlock)
		if err != nil {
			return 0, fmt.Errorf("invalid off-block time format: %s", offBlock)
		}
	}
	onT, err := time.Parse("15:04:05", onBlock)
	if err != nil {
		onT, err = time.Parse("15:04", onBlock)
		if err != nil {
			return 0, fmt.Errorf("invalid on-block time format: %s", onBlock)
		}
	}

	duration := onT.Sub(offT)
	if duration < 0 {
		// Overnight: add 24 hours
		duration += 24 * time.Hour
	}
	if duration == 0 {
		return 0, fmt.Errorf("off-block and on-block times cannot be identical")
	}

	hours := duration.Hours()
	// Round to 1 decimal place (standard logbook precision)
	return math.Round(hours*10) / 10, nil
}
