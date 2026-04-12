package handlers

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightcalc"
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

	// Build query options from params
	opts := &repository.FlightQueryOptions{
		Page:      1,
		PageSize:  20,
		SortBy:    "date",
		SortOrder: "desc",
	}
	if params.StartDate != nil {
		t := params.StartDate.Time
		opts.StartDate = &t
	}
	if params.EndDate != nil {
		t := params.EndDate.Time
		opts.EndDate = &t
	}
	if params.AircraftReg != nil {
		opts.AircraftReg = params.AircraftReg
	}
	if params.DepartureIcao != nil {
		opts.DepartureICAO = params.DepartureIcao
	}
	if params.ArrivalIcao != nil {
		opts.ArrivalICAO = params.ArrivalIcao
	}
	if params.IsPic != nil {
		opts.IsPIC = params.IsPic
	}
	if params.IsDual != nil {
		opts.IsDual = params.IsDual
	}
	if params.Search != nil {
		opts.Search = params.Search
	}
	if params.Page != nil && *params.Page > 0 {
		opts.Page = *params.Page
	}
	if params.PageSize != nil && *params.PageSize > 0 {
		opts.PageSize = *params.PageSize
		if opts.PageSize > 100 {
			opts.PageSize = 100
		}
	}
	if params.SortBy != nil {
		opts.SortBy = string(*params.SortBy)
	}
	if params.SortOrder != nil {
		opts.SortOrder = string(*params.SortOrder)
	}

	// Get total count for pagination
	total, err := h.flightService.CountFlights(c.Request.Context(), userID, opts)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to count flights")
		return
	}

	flights, err := h.flightService.ListFlights(c.Request.Context(), userID, opts)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve flights")
		return
	}

	// Logbook filtering: if logbookLicenseId is set, filter to flights on aircraft
	// whose class matches the license's class ratings
	if params.LogbookLicenseId != nil {
		licenseID := uuid.UUID(*params.LogbookLicenseId)
		classRatings, err := h.classRatingService.ListClassRatings(c.Request.Context(), licenseID, userID)
		if err == nil && len(classRatings) > 0 {
			// Build set of allowed class types
			allowedClasses := make(map[string]bool)
			for _, cr := range classRatings {
				allowedClasses[string(cr.ClassType)] = true
			}
			// Get user aircraft to build reg → class lookup
			aircraftList, _ := h.aircraftService.ListAircraft(c.Request.Context(), userID)
			regToClass := make(map[string]string)
			for _, ac := range aircraftList {
				if ac.AircraftClass != nil {
					regToClass[strings.ToUpper(ac.Registration)] = *ac.AircraftClass
				}
			}
			// Filter flights
			var filtered []*models.Flight
			for _, f := range flights {
				acClass := regToClass[strings.ToUpper(f.AircraftReg)]
				if acClass != "" && allowedClasses[acClass] {
					filtered = append(filtered, f)
				}
			}
			flights = filtered
			total = len(filtered)
		}
	}

	flightList := make([]generated.Flight, 0, len(flights))
	for _, f := range flights {
		flightList = append(flightList, convertToGeneratedFlight(f))
	}

	totalPages := (total + opts.PageSize - 1) / opts.PageSize

	response := generated.PaginatedFlights{
		Data: flightList,
		Pagination: struct {
			Page       int "json:\"page\""
			PageSize   int "json:\"pageSize\""
			Total      int "json:\"total\""
			TotalPages int "json:\"totalPages\""
		}{
			Page:       opts.Page,
			PageSize:   opts.PageSize,
			Total:      total,
			TotalPages: totalPages,
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
		h.sendError(c, http.StatusBadRequest, "Invalid block times format")
		return
	}

	// PIC/Dual will be auto-determined by ApplyAutoCalculations based on crew
	isPic := true
	isDual := false

	// Compute picTime/dualTime from booleans
	var picTime, dualTime int
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
		Date:          flightDate,
		AircraftReg:   req.AircraftReg,
		AircraftType:  req.AircraftType,
		DepartureICAO: &departureIcao,
		ArrivalICAO:   &arrivalIcao,
		OffBlockTime:  &offBlockTime,
		OnBlockTime:   &onBlockTime,
		DepartureTime: departureTime,
		ArrivalTime:   arrivalTime,
		TotalTime:     totalTime,
		IsPIC:         isPic,
		IsDual:        isDual,
		PICTime:       picTime,
		DualTime:      dualTime,
		NightTime:     0,
		IFRTime:       getIntOrDefault(req.IfrTime, 0),
		AllLandings:   req.Landings,
	}

	// Set route if provided
	if req.Route != nil {
		flight.Route = req.Route
	}

	// Set takeoffs with manual override flags if provided
	if req.TakeoffsDay != nil {
		flight.TakeoffsDay = *req.TakeoffsDay
		flight.TakeoffsDayOverride = true
	}
	if req.TakeoffsNight != nil {
		flight.TakeoffsNight = *req.TakeoffsNight
		flight.TakeoffsNightOverride = true
	}

	if req.Remarks != nil {
		flight.Remarks = req.Remarks
	}

	// New fields: instructor, comments, advanced times
	flight.InstructorName = req.InstructorName
	flight.InstructorComments = req.InstructorComments
	flight.SICTime = getIntOrDefault(req.SicTime, 0)
	flight.DualGivenTime = getIntOrDefault(req.DualGivenTime, 0)
	flight.SimulatedFlightTime = getIntOrDefault(req.SimulatedFlightTime, 0)
	flight.GroundTrainingTime = getIntOrDefault(req.GroundTrainingTime, 0)
	flight.ActualInstrumentTime = getIntOrDefault(req.ActualInstrumentTime, 0)
	flight.SimulatedInstrumentTime = getIntOrDefault(req.SimulatedInstrumentTime, 0)
	// Auto-calculate IFR time as actual + simulated instrument if not explicitly provided
	if req.IfrTime == nil || *req.IfrTime == 0 {
		flight.IFRTime = flight.ActualInstrumentTime + flight.SimulatedInstrumentTime
	}
	if req.Holds != nil {
		flight.Holds = *req.Holds
	}
	if req.ApproachesCount != nil {
		flight.ApproachesCount = *req.ApproachesCount
	}
	if req.IsIpc != nil {
		flight.IsIPC = *req.IsIpc
	}
	if req.IsFlightReview != nil {
		flight.IsFlightReview = *req.IsFlightReview
	}
	if req.IsProficiencyCheck != nil {
		flight.IsProficiencyCheck = *req.IsProficiencyCheck
	}
	if req.LaunchMethod != nil {
		if *req.LaunchMethod != "null" {
			lm := string(*req.LaunchMethod)
			flight.LaunchMethod = &lm
		}
	}

	// Phase 6c: PIC Name, Multi-Pilot Time, FSTD Type, Approaches, Endorsements
	flight.PICName = req.PicName
	flight.MultiPilotTime = getIntOrDefault(req.MultiPilotTime, 0)
	flight.FSTDType = req.FstdType
	flight.Endorsements = req.Endorsements

	// Parse structured approaches
	if req.Approaches != nil {
		for _, a := range *req.Approaches {
			entry := models.ApproachEntry{Type: string(a.Type)}
			if a.Airport != nil {
				entry.Airport = a.Airport
			}
			if a.Runway != nil {
				entry.Runway = a.Runway
			}
			flight.Approaches = append(flight.Approaches, entry)
		}
		flight.ApproachesCount = len(flight.Approaches)
	}

	// Auto-set PIC name
	if flight.PICName == nil {
		if flight.IsPIC {
			self := "Self"
			flight.PICName = &self
		} else if flight.IsDual && flight.InstructorName != nil {
			flight.PICName = flight.InstructorName
		}
	}

	// Parse crew members
	if req.CrewMembers != nil {
		for _, cm := range *req.CrewMembers {
			member := models.FlightCrewMember{
				Name: cm.Name,
				Role: models.CrewRole(cm.Role),
			}
			if cm.ContactId != nil {
				cid := uuid.UUID(*cm.ContactId)
				member.ContactID = &cid
			}
			flight.CrewMembers = append(flight.CrewMembers, member)
		}
	}

	// Apply auto-calculations (solo, cross-country, distance, takeoff/landing split, SIC, dual given)
	flightcalc.ApplyAutoCalculations(&flight)

	if err := h.flightService.CreateFlight(c.Request.Context(), &flight); err != nil {
		h.sendError(c, http.StatusBadRequest, "Failed to create flight")
		return
	}
	// Persist crew members
	if len(flight.CrewMembers) > 0 && h.flightCrewRepo != nil {
		if err := h.flightCrewRepo.SetCrewMembers(c.Request.Context(), flight.ID, flight.CrewMembers); err != nil {
			// Flight created but crew failed - log but don't fail the request
			fmt.Printf("Warning: failed to save crew members for flight %s: %v\n", flight.ID, err)
		}
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

	// Load crew members
	if h.flightCrewRepo != nil {
		crew, err := h.flightCrewRepo.GetByFlightID(c.Request.Context(), flight.ID)
		if err == nil {
			flight.CrewMembers = crew
		}
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
	if req.IfrTime != nil {
		flight.IFRTime = *req.IfrTime
	}
	if req.Landings != nil {
		flight.AllLandings = *req.Landings
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
	if req.Route != nil {
		flight.Route = req.Route
	}
	if req.TakeoffsDay != nil {
		flight.TakeoffsDay = *req.TakeoffsDay
		flight.TakeoffsDayOverride = true
	}
	if req.TakeoffsNight != nil {
		flight.TakeoffsNight = *req.TakeoffsNight
		flight.TakeoffsNightOverride = true
	}

	// New fields
	if req.InstructorName != nil {
		flight.InstructorName = req.InstructorName
	}
	if req.InstructorComments != nil {
		flight.InstructorComments = req.InstructorComments
	}
	if req.SicTime != nil {
		flight.SICTime = *req.SicTime
	}
	if req.DualGivenTime != nil {
		flight.DualGivenTime = *req.DualGivenTime
	}
	if req.SimulatedFlightTime != nil {
		flight.SimulatedFlightTime = *req.SimulatedFlightTime
	}
	if req.GroundTrainingTime != nil {
		flight.GroundTrainingTime = *req.GroundTrainingTime
	}
	if req.ActualInstrumentTime != nil {
		flight.ActualInstrumentTime = *req.ActualInstrumentTime
	}
	if req.SimulatedInstrumentTime != nil {
		flight.SimulatedInstrumentTime = *req.SimulatedInstrumentTime
	}
	if req.Holds != nil {
		flight.Holds = *req.Holds
	}
	if req.ApproachesCount != nil {
		flight.ApproachesCount = *req.ApproachesCount
	}
	if req.IsIpc != nil {
		flight.IsIPC = *req.IsIpc
	}
	if req.IsFlightReview != nil {
		flight.IsFlightReview = *req.IsFlightReview
	}
	if req.IsProficiencyCheck != nil {
		flight.IsProficiencyCheck = *req.IsProficiencyCheck
	}
	if req.LaunchMethod != nil {
		if *req.LaunchMethod != "null" {
			lm := string(*req.LaunchMethod)
			flight.LaunchMethod = &lm
		} else {
			flight.LaunchMethod = nil
		}
	}
	// Phase 6c fields
	if req.PicName != nil {
		flight.PICName = req.PicName
	}
	if req.MultiPilotTime != nil {
		flight.MultiPilotTime = *req.MultiPilotTime
	}
	if req.FstdType != nil {
		flight.FSTDType = req.FstdType
	}
	if req.Endorsements != nil {
		flight.Endorsements = req.Endorsements
	}
	if req.Approaches != nil {
		flight.Approaches = nil
		for _, a := range *req.Approaches {
			entry := models.ApproachEntry{Type: string(a.Type)}
			if a.Airport != nil {
				entry.Airport = a.Airport
			}
			if a.Runway != nil {
				entry.Runway = a.Runway
			}
			flight.Approaches = append(flight.Approaches, entry)
		}
		flight.ApproachesCount = len(flight.Approaches)
	}
	// Auto-set PIC name if not explicitly provided
	if req.PicName == nil && flight.PICName == nil {
		if flight.IsPIC {
			self := "Self"
			flight.PICName = &self
		} else if flight.IsDual && flight.InstructorName != nil {
			flight.PICName = flight.InstructorName
		}
	}
	if req.CrewMembers != nil {
		flight.CrewMembers = nil
		for _, cm := range *req.CrewMembers {
			member := models.FlightCrewMember{
				Name: cm.Name,
				Role: models.CrewRole(cm.Role),
			}
			if cm.ContactId != nil {
				cid := uuid.UUID(*cm.ContactId)
				member.ContactID = &cid
			}
			flight.CrewMembers = append(flight.CrewMembers, member)
		}
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
				h.sendError(c, http.StatusBadRequest, "Invalid block times format")
				return
			}
			flight.TotalTime = totalTime
		}
	} else if req.TotalTime != nil {
		// Allow direct totalTime override only if block times are not being updated
		flight.TotalTime = *req.TotalTime
	}

	// Apply auto-calculations (solo, cross-country, distance, takeoff/landing split)
	flightcalc.ApplyAutoCalculations(flight)

	if err := h.flightService.UpdateFlight(c.Request.Context(), flight, userID); err != nil {
		h.sendError(c, http.StatusBadRequest, "Failed to update flight")
		return
	}

	// Persist crew members if they were updated
	if req.CrewMembers != nil && h.flightCrewRepo != nil {
		if err := h.flightCrewRepo.SetCrewMembers(c.Request.Context(), flight.ID, flight.CrewMembers); err != nil {
			fmt.Printf("Warning: failed to save crew members for flight %s: %v\n", flight.ID, err)
		}
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
		Id:               openapi_types.UUID(f.ID),
		UserId:           openapi_types.UUID(f.UserID),
		Date:             openapi_types.Date{Time: f.Date},
		AircraftReg:      f.AircraftReg,
		AircraftType:     f.AircraftType,
		TotalTime:        f.TotalTime,
		IsPic:            f.IsPIC,
		IsDual:           f.IsDual,
		PicTime:          f.PICTime,
		DualTime:         f.DualTime,
		NightTime:        f.NightTime,
		IfrTime:          f.IFRTime,
		SoloTime:         f.SoloTime,
		CrossCountryTime: f.CrossCountryTime,
		Distance:         float32(f.Distance),
		LandingsDay:      f.LandingsDay,
		LandingsNight:    f.LandingsNight,
		AllLandings:      f.AllLandings,
		TakeoffsDay:      f.TakeoffsDay,
		TakeoffsNight:    f.TakeoffsNight,
		CreatedAt:        f.CreatedAt,
		UpdatedAt:        f.UpdatedAt,
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
	if f.Route != nil {
		flight.Route = f.Route
	}
	if f.Remarks != nil {
		flight.Remarks = f.Remarks
	}

	// New fields
	flight.InstructorName = f.InstructorName
	flight.InstructorComments = f.InstructorComments
	flight.SicTime = ptrInt(f.SICTime)
	flight.DualGivenTime = ptrInt(f.DualGivenTime)
	flight.SimulatedFlightTime = ptrInt(f.SimulatedFlightTime)
	flight.GroundTrainingTime = ptrInt(f.GroundTrainingTime)
	flight.ActualInstrumentTime = ptrInt(f.ActualInstrumentTime)
	flight.SimulatedInstrumentTime = ptrInt(f.SimulatedInstrumentTime)
	flight.Holds = ptrInt(f.Holds)
	flight.ApproachesCount = ptrInt(f.ApproachesCount)
	flight.IsIpc = ptrBool(f.IsIPC)
	flight.IsFlightReview = ptrBool(f.IsFlightReview)
	flight.IsProficiencyCheck = ptrBool(f.IsProficiencyCheck)
	if f.LaunchMethod != nil {
		lm := generated.FlightLaunchMethod(*f.LaunchMethod)
		flight.LaunchMethod = &lm
	}

	// Phase 6c regulatory compliance fields
	flight.PicName = f.PICName
	flight.MultiPilotTime = ptrInt(f.MultiPilotTime)
	flight.FstdType = f.FSTDType
	flight.Endorsements = f.Endorsements

	// Structured approaches
	if len(f.Approaches) > 0 {
		approaches := make([]generated.ApproachEntry, len(f.Approaches))
		for i, a := range f.Approaches {
			approaches[i] = generated.ApproachEntry{
				Type:    generated.ApproachType(a.Type),
				Airport: a.Airport,
				Runway:  a.Runway,
			}
		}
		flight.Approaches = &approaches
	}

	// Crew members
	if len(f.CrewMembers) > 0 {
		crew := make([]generated.FlightCrewMember, len(f.CrewMembers))
		for i, m := range f.CrewMembers {
			crew[i] = generated.FlightCrewMember{
				Id:       openapi_types.UUID(m.ID),
				FlightId: openapi_types.UUID(m.FlightID),
				Name:     m.Name,
				Role:     generated.CrewRole(m.Role),
			}
			if m.ContactID != nil {
				cid := openapi_types.UUID(*m.ContactID)
				crew[i].ContactId = &cid
			}
		}
		flight.CrewMembers = &crew
	}

	return flight
}

func getIntOrDefault(val *int, def int) int {
	if val != nil {
		return *val
	}
	return def
}

func ptrInt(v int) *int {
	return &v
}

func ptrBool(v bool) *bool {
	return &v
}

// calculateBlockTime computes total block time in minutes from off-block and on-block time strings (HH:MM:SS).
// Handles overnight flights (on-block before off-block crosses midnight).
func calculateBlockTime(offBlock, onBlock string) (int, error) {
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

	minutes := int(math.Round(duration.Minutes()))
	return minutes, nil
}
