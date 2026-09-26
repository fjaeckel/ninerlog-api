package handlers

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// SetAircraftReminderService wires the aircraft reminder service.
func (h *APIHandler) SetAircraftReminderService(s *service.AircraftReminderService) {
	h.aircraftReminderService = s
}

// sendAircraftReminderError maps reminder service errors to responses.
func (h *APIHandler) sendAircraftReminderError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAircraftNotFound), errors.Is(err, service.ErrUnauthorizedAircraft):
		h.sendError(c, http.StatusNotFound, "Aircraft not found")
	case errors.Is(err, service.ErrAircraftReminderNotFound):
		h.sendError(c, http.StatusNotFound, "Aircraft reminder not found")
	case errors.Is(err, models.ErrInvalidAircraftReminder),
		errors.Is(err, models.ErrFieldTooLong),
		errors.Is(err, service.ErrInvalidReminderQuery):
		h.sendError(c, http.StatusBadRequest, err.Error())
	default:
		h.sendError(c, http.StatusInternalServerError, "Failed to process aircraft reminder")
	}
}

func convertToGeneratedAircraftReminder(r *models.AircraftReminder, today time.Time) generated.AircraftReminder {
	out := generated.AircraftReminder{
		Id:                   openapi_types.UUID(r.ID),
		AircraftId:           openapi_types.UUID(r.AircraftID),
		AircraftRegistration: r.AircraftRegistration,
		Kind:                 generated.AircraftReminderKind(r.Kind),
		Label:                r.Label,
		DueDate:              openapi_types.Date{Time: r.DueDate},
		IntervalMonths:       r.IntervalMonths,
		Notes:                r.Notes,
		Status:               generated.AircraftReminderStatus(r.Status(today)),
		DaysUntilDue:         r.DaysUntilDue(today),
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
	if r.LastDoneOn != nil {
		d := openapi_types.Date{Time: *r.LastDoneOn}
		out.LastDoneOn = &d
	}
	return out
}

func (h *APIHandler) aircraftReminderList(reminders []*models.AircraftReminder) []generated.AircraftReminder {
	today := h.aircraftReminderService.Today()
	out := make([]generated.AircraftReminder, 0, len(reminders))
	for _, r := range reminders {
		out = append(out, convertToGeneratedAircraftReminder(r, today))
	}
	return out
}

// ListAircraftReminders implements GET /aircraft/{aircraftId}/reminders.
func (h *APIHandler) ListAircraftReminders(c *gin.Context, aircraftId generated.AircraftId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	reminders, err := h.aircraftReminderService.List(c.Request.Context(), userID, uuid.UUID(aircraftId))
	if err != nil {
		h.sendAircraftReminderError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.aircraftReminderList(reminders))
}

// ListAllAircraftReminders implements GET /aircraft-reminders.
func (h *APIHandler) ListAllAircraftReminders(c *gin.Context, params generated.ListAllAircraftRemindersParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	reminders, err := h.aircraftReminderService.ListAll(c.Request.Context(), userID, params.DueWithinDays)
	if err != nil {
		h.sendAircraftReminderError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.aircraftReminderList(reminders))
}

// CreateAircraftReminder implements POST /aircraft/{aircraftId}/reminders.
func (h *APIHandler) CreateAircraftReminder(c *gin.Context, aircraftId generated.AircraftId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var req generated.AircraftReminderCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	in := service.AircraftReminderInput{
		Kind:           models.AircraftReminderKind(req.Kind),
		Label:          req.Label,
		DueDate:        req.DueDate.Time,
		IntervalMonths: req.IntervalMonths,
		Notes:          req.Notes,
	}
	if req.LastDoneOn != nil {
		t := req.LastDoneOn.Time
		in.LastDoneOn = &t
	}
	rem, err := h.aircraftReminderService.Create(c.Request.Context(), userID, uuid.UUID(aircraftId), in)
	if err != nil {
		h.sendAircraftReminderError(c, err)
		return
	}
	c.JSON(http.StatusCreated, convertToGeneratedAircraftReminder(rem, h.aircraftReminderService.Today()))
}

// nullablePatch splits a nullable field into a value and a clear flag.
func nullablePatch[T any](n nullable.Nullable[T]) (*T, bool) {
	if !n.IsSpecified() {
		return nil, false
	}
	if n.IsNull() {
		return nil, true
	}
	v, _ := n.Get()
	return &v, false
}

// UpdateAircraftReminder implements PATCH /aircraft/{aircraftId}/reminders/{reminderId}.
func (h *APIHandler) UpdateAircraftReminder(c *gin.Context, aircraftId generated.AircraftId, reminderId generated.ReminderId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var req generated.AircraftReminderUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	var p service.AircraftReminderPatch
	if req.Kind != nil {
		k := models.AircraftReminderKind(*req.Kind)
		p.Kind = &k
	}
	if req.DueDate != nil {
		t := req.DueDate.Time
		p.DueDate = &t
	}
	p.Label, p.ClearLabel = nullablePatch(req.Label)
	p.IntervalMonths, p.ClearIntervalMonths = nullablePatch(req.IntervalMonths)
	p.Notes, p.ClearNotes = nullablePatch(req.Notes)
	var lastDone *openapi_types.Date
	lastDone, p.ClearLastDoneOn = nullablePatch(req.LastDoneOn)
	if lastDone != nil {
		t := lastDone.Time
		p.LastDoneOn = &t
	}
	rem, err := h.aircraftReminderService.Update(c.Request.Context(), userID, uuid.UUID(aircraftId), uuid.UUID(reminderId), p)
	if err != nil {
		h.sendAircraftReminderError(c, err)
		return
	}
	c.JSON(http.StatusOK, convertToGeneratedAircraftReminder(rem, h.aircraftReminderService.Today()))
}

// DeleteAircraftReminder implements DELETE /aircraft/{aircraftId}/reminders/{reminderId}.
func (h *APIHandler) DeleteAircraftReminder(c *gin.Context, aircraftId generated.AircraftId, reminderId generated.ReminderId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if err := h.aircraftReminderService.Delete(c.Request.Context(), userID, uuid.UUID(aircraftId), uuid.UUID(reminderId)); err != nil {
		h.sendAircraftReminderError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// CompleteAircraftReminder implements POST /aircraft/{aircraftId}/reminders/{reminderId}/complete.
func (h *APIHandler) CompleteAircraftReminder(c *gin.Context, aircraftId generated.AircraftId, reminderId generated.ReminderId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var req generated.AircraftReminderComplete
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	var doneOn *time.Time
	if req.DoneOn != nil {
		t := req.DoneOn.Time
		doneOn = &t
	}
	rem, err := h.aircraftReminderService.Complete(c.Request.Context(), userID, uuid.UUID(aircraftId), uuid.UUID(reminderId), doneOn)
	if err != nil {
		h.sendAircraftReminderError(c, err)
		return
	}
	c.JSON(http.StatusOK, convertToGeneratedAircraftReminder(rem, h.aircraftReminderService.Today()))
}
