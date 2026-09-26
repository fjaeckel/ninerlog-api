package handlers

import (
	"errors"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/service/readiness"
	"github.com/gin-gonic/gin"
)

// GetAllCurrencyStatus returns currency status for all class ratings across all licenses
func (h *APIHandler) GetAllCurrencyStatus(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	result, err := h.currencyService.EvaluateAll(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to evaluate currency")
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetCurrencyReadiness answers readiness per rating, launch method, passengers
// and medical certificate on a date.
func (h *APIHandler) GetCurrencyReadiness(c *gin.Context, params generated.GetCurrencyReadinessParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	req := readiness.Request{}
	if params.Date != nil {
		d := params.Date.Time
		req.Date = &d
	}
	if params.AircraftReg != nil {
		req.AircraftReg = *params.AircraftReg
	}
	if params.Passengers != nil {
		req.Passengers = *params.Passengers
	}

	report, err := h.readinessService.Evaluate(c.Request.Context(), userID, req)
	switch {
	case errors.Is(err, readiness.ErrInvalidDate):
		h.sendError(c, http.StatusBadRequest, "date must be between today and 366 days ahead")
		return
	case errors.Is(err, readiness.ErrAircraftNotFound):
		h.sendError(c, http.StatusNotFound, "Aircraft not found")
		return
	case err != nil:
		h.sendError(c, http.StatusInternalServerError, "Failed to evaluate readiness")
		return
	}

	c.JSON(http.StatusOK, report)
}
