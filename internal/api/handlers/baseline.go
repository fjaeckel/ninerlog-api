package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// GetMyBaseline implements GET /users/me/baseline.
func (h *APIHandler) GetMyBaseline(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	baseline, err := h.flightService.GetBaseline(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.sendError(c, http.StatusNotFound, "No baseline configured")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to load baseline")
		return
	}

	c.JSON(http.StatusOK, baselineToGenerated(baseline))
}

// PutMyBaseline implements PUT /users/me/baseline.
func (h *APIHandler) PutMyBaseline(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.PutMyBaselineJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	baseline := &models.FlightBaseline{
		UserID:              userID,
		BaselineDate:        req.BaselineDate.Time,
		TotalFlights:        derefInt(req.TotalFlights),
		TotalMinutes:        derefInt(req.TotalMinutes),
		PICMinutes:          derefInt(req.PicMinutes),
		SICMinutes:          derefInt(req.SicMinutes),
		DualMinutes:         derefInt(req.DualMinutes),
		DualGivenMinutes:    derefInt(req.DualGivenMinutes),
		MultiPilotMinutes:   derefInt(req.MultiPilotMinutes),
		NightMinutes:        derefInt(req.NightMinutes),
		IFRMinutes:          derefInt(req.IfrMinutes),
		SoloMinutes:         derefInt(req.SoloMinutes),
		CrossCountryMinutes: derefInt(req.CrossCountryMinutes),
		LandingsDay:         derefInt(req.LandingsDay),
		LandingsNight:       derefInt(req.LandingsNight),
		Notes:               trimmedNotes(req.Notes),
	}

	if err := h.flightService.UpsertBaseline(c.Request.Context(), baseline); err != nil {
		if errors.Is(err, models.ErrInvalidFlightBaseline) {
			h.sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to save baseline")
		return
	}

	c.JSON(http.StatusOK, baselineToGenerated(baseline))
}

// DeleteMyBaseline implements DELETE /users/me/baseline.
func (h *APIHandler) DeleteMyBaseline(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := h.flightService.DeleteBaseline(c.Request.Context(), userID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.sendError(c, http.StatusNotFound, "No baseline configured")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to delete baseline")
		return
	}

	c.Status(http.StatusNoContent)
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func trimmedNotes(p *string) *string {
	if p == nil {
		return nil
	}
	s := strings.TrimSpace(*p)
	if s == "" {
		return nil
	}
	return &s
}

func baselineToGenerated(b *models.FlightBaseline) generated.FlightBaseline {
	totalFlights := b.TotalFlights
	totalMinutes := b.TotalMinutes
	picMinutes := b.PICMinutes
	sicMinutes := b.SICMinutes
	dualMinutes := b.DualMinutes
	dualGivenMinutes := b.DualGivenMinutes
	multiPilotMinutes := b.MultiPilotMinutes
	nightMinutes := b.NightMinutes
	ifrMinutes := b.IFRMinutes
	soloMinutes := b.SoloMinutes
	crossCountryMinutes := b.CrossCountryMinutes
	landingsDay := b.LandingsDay
	landingsNight := b.LandingsNight

	createdAt := b.CreatedAt
	updatedAt := b.UpdatedAt

	return generated.FlightBaseline{
		BaselineDate:        openapi_types.Date{Time: b.BaselineDate},
		TotalFlights:        &totalFlights,
		TotalMinutes:        &totalMinutes,
		PicMinutes:          &picMinutes,
		SicMinutes:          &sicMinutes,
		DualMinutes:         &dualMinutes,
		DualGivenMinutes:    &dualGivenMinutes,
		MultiPilotMinutes:   &multiPilotMinutes,
		NightMinutes:        &nightMinutes,
		IfrMinutes:          &ifrMinutes,
		SoloMinutes:         &soloMinutes,
		CrossCountryMinutes: &crossCountryMinutes,
		LandingsDay:         &landingsDay,
		LandingsNight:       &landingsNight,
		Notes:               b.Notes,
		CreatedAt:           &createdAt,
		UpdatedAt:           &updatedAt,
	}
}

// baselineContribution converts a baseline that was added to a Statistics
// response into the embedded breakdown the API exposes.
func baselineContribution(b *models.FlightBaseline) *generated.StatisticsBaselineContribution {
	if b == nil {
		return nil
	}
	sicMinutes := b.SICMinutes
	dualGivenMinutes := b.DualGivenMinutes
	multiPilotMinutes := b.MultiPilotMinutes
	soloMinutes := b.SoloMinutes
	crossCountryMinutes := b.CrossCountryMinutes
	return &generated.StatisticsBaselineContribution{
		BaselineDate:        openapi_types.Date{Time: b.BaselineDate},
		TotalFlights:        b.TotalFlights,
		TotalMinutes:        b.TotalMinutes,
		PicMinutes:          b.PICMinutes,
		SicMinutes:          &sicMinutes,
		DualMinutes:         b.DualMinutes,
		DualGivenMinutes:    &dualGivenMinutes,
		MultiPilotMinutes:   &multiPilotMinutes,
		NightMinutes:        b.NightMinutes,
		IfrMinutes:          b.IFRMinutes,
		SoloMinutes:         &soloMinutes,
		CrossCountryMinutes: &crossCountryMinutes,
		LandingsDay:         b.LandingsDay,
		LandingsNight:       b.LandingsNight,
	}
}
