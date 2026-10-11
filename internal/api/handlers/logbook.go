package handlers

import (
	"errors"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/errcode"
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/logbookrules"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// SetLogbookService stores the strict logbook mode service.
func (h *APIHandler) SetLogbookService(s *service.LogbookService) {
	h.logbookService = s
}

func (h *APIHandler) logbookUnavailable(c *gin.Context) bool {
	if h.logbookService == nil {
		h.sendError(c, http.StatusServiceUnavailable, "Logbook mode is not available")
		return true
	}
	return false
}

func convertLogbookMode(v *service.ModeView) generated.LogbookMode {
	out := generated.LogbookMode{
		Strict:                v.Strict,
		Since:                 v.Since,
		OverridesDownwardOnly: v.OverridesDownwardOnly,
		RequiredBy:            convertModeLicences(v.Required),
		SuggestedBy:           convertModeLicences(v.Suggested),
		Events:                make([]generated.LogbookModeEvent, 0, len(v.Events)),
	}
	if v.DraftWindow != "" {
		w := generated.LogbookModeDraftWindow(v.DraftWindow)
		out.DraftWindow = &w
	}
	if v.LatestCertifiedThrough != nil {
		out.LatestCertifiedThrough = &openapi_types.Date{Time: *v.LatestCertifiedThrough}
	}
	for _, e := range v.Events {
		ev := generated.LogbookModeEvent{
			Strict:    e.Strict,
			Trigger:   generated.LogbookModeEventTrigger(e.Trigger),
			CreatedAt: e.CreatedAt,
		}
		if e.LicenseID != nil {
			id := openapi_types.UUID(*e.LicenseID)
			ev.LicenseId = &id
		}
		out.Events = append(out.Events, ev)
	}
	return out
}

func convertModeLicences(matches []logbookrules.Match) []generated.LogbookModeLicence {
	out := make([]generated.LogbookModeLicence, 0, len(matches))
	for _, m := range matches {
		l := generated.LogbookModeLicence{
			LicenseId:           openapi_types.UUID(m.Licence.ID),
			RegulatoryAuthority: m.Licence.RegulatoryAuthority,
			LicenseType:         m.Licence.LicenseType,
			IssuingCountry:      m.Licence.IssuingCountry,
			Basis:               m.Rule.Basis,
		}
		if m.Rule.Window != "" {
			w := generated.LogbookModeLicenceDraftWindow(m.Rule.Window)
			l.DraftWindow = &w
		}
		out = append(out, l)
	}
	return out
}

// GetLogbookMode implements GET /logbook/mode
func (h *APIHandler) GetLogbookMode(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.logbookUnavailable(c) {
		return
	}
	view, err := h.logbookService.GetMode(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve logbook mode")
		return
	}
	c.JSON(http.StatusOK, convertLogbookMode(view))
}

// UpdateLogbookMode implements PUT /logbook/mode
func (h *APIHandler) UpdateLogbookMode(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.logbookUnavailable(c) {
		return
	}
	var req struct {
		Strict *bool `json:"strict"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Strict == nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	view, err := h.logbookService.SetStrict(c.Request.Context(), userID, *req.Strict)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to update logbook mode")
		return
	}
	c.JSON(http.StatusOK, convertLogbookMode(view))
}

func convertCertification(c models.LogbookCertification) generated.LogbookCertification {
	return generated.LogbookCertification{
		Id:               openapi_types.UUID(c.ID),
		CertifiedThrough: openapi_types.Date{Time: c.CertifiedThrough},
		CertifiedAt:      c.CertifiedAt,
	}
}

// ListLogbookCertifications implements GET /logbook/certifications
func (h *APIHandler) ListLogbookCertifications(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.logbookUnavailable(c) {
		return
	}
	certs, err := h.logbookService.ListCertifications(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve certifications")
		return
	}
	out := make([]generated.LogbookCertification, 0, len(certs))
	for _, cert := range certs {
		out = append(out, convertCertification(cert))
	}
	c.JSON(http.StatusOK, out)
}

// CreateLogbookCertification implements POST /logbook/certifications
func (h *APIHandler) CreateLogbookCertification(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.logbookUnavailable(c) {
		return
	}
	var req generated.CreateLogbookCertificationJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	cert, err := h.logbookService.CreateCertification(c.Request.Context(), userID, req.CertifiedThrough.Time)
	switch {
	case err == nil:
		c.JSON(http.StatusCreated, convertCertification(*cert))
	case errors.Is(err, service.ErrStrictModeRequired):
		h.sendCodedError(c, http.StatusConflict, errcode.StrictModeRequired, "Strict logbook mode must be on to certify the logbook")
	case errors.Is(err, service.ErrCertificationInFuture):
		h.sendCodedError(c, http.StatusUnprocessableEntity, errcode.CertificationInFuture, "The certification date is in the future")
	case errors.Is(err, service.ErrCertificationBeforeLatest):
		h.sendCodedError(c, http.StatusUnprocessableEntity, errcode.CertificationBeforeLatest, "The certification date is earlier than the latest certification")
	default:
		h.sendError(c, http.StatusInternalServerError, "Failed to certify the logbook")
	}
}

// ListFlightRevisions implements GET /flights/{flightId}/revisions
func (h *APIHandler) ListFlightRevisions(c *gin.Context, flightId generated.FlightId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.logbookUnavailable(c) {
		return
	}
	revs, err := h.logbookService.ListRevisions(c.Request.Context(), userID, uuid.UUID(flightId))
	if err != nil {
		if errors.Is(err, service.ErrFlightNotFound) {
			h.sendError(c, http.StatusNotFound, "Flight not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve revisions")
		return
	}
	out := make([]generated.FlightRevision, 0, len(revs))
	for _, r := range revs {
		rev := generated.FlightRevision{
			Id:           openapi_types.UUID(r.ID),
			FlightId:     openapi_types.UUID(r.FlightID),
			RevisionNo:   r.RevisionNo,
			Kind:         generated.FlightRevisionKind(r.Kind),
			Source:       generated.FlightRevisionSource(r.Source),
			Reason:       r.Reason,
			IsCorrection: r.IsCorrection,
			ChangedAt:    r.ChangedAt,
			Changes:      make([]generated.FlightRevisionChange, 0, len(r.Changes)),
		}
		for _, ch := range r.Changes {
			rev.Changes = append(rev.Changes, generated.FlightRevisionChange{
				Field: ch.Field, OldValue: ch.OldValue, NewValue: ch.NewValue,
			})
		}
		out = append(out, rev)
	}
	c.JSON(http.StatusOK, out)
}

// ListVoidedFlights implements GET /flights/voided
func (h *APIHandler) ListVoidedFlights(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.logbookUnavailable(c) {
		return
	}
	voided, err := h.logbookService.ListVoided(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve voided flights")
		return
	}
	out := make([]generated.VoidedFlight, 0, len(voided))
	for _, v := range voided {
		out = append(out, generated.VoidedFlight{
			FlightId:      openapi_types.UUID(v.FlightID),
			RevisionId:    openapi_types.UUID(v.RevisionID),
			DeletedAt:     v.DeletedAt,
			Reason:        v.Reason,
			Source:        generated.VoidedFlightSource(v.Source),
			IsCorrection:  v.IsCorrection,
			Date:          openapi_types.Date{Time: v.Date},
			AircraftReg:   v.AircraftReg,
			AircraftType:  v.AircraftType,
			DepartureIcao: v.DepartureICAO,
			ArrivalIcao:   v.ArrivalICAO,
			TotalTime:     v.TotalTime,
			Remarks:       v.Remarks,
		})
	}
	c.JSON(http.StatusOK, out)
}
