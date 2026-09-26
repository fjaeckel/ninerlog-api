package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SetWeGlideService wires the WeGlide link; syncInterval is the scheduled
// sync interval, zero when the scheduler is off.
func (h *APIHandler) SetWeGlideService(s *service.WeGlideService, syncInterval time.Duration) {
	h.weglideService = s
	h.weglideSyncInterval = syncInterval
}

// GetWeGlideLink implements GET /integrations/weglide.
func (h *APIHandler) GetWeGlideLink(c *gin.Context) {
	userID, ok := h.weglideCaller(c)
	if !ok {
		return
	}
	st, err := h.weglideService.Status(c.Request.Context(), userID)
	if err != nil {
		h.sendWeGlideError(c, err)
		return
	}
	c.JSON(http.StatusOK, convertWeGlideStatus(st))
}

// LinkWeGlide implements PUT /integrations/weglide.
func (h *APIHandler) LinkWeGlide(c *gin.Context) {
	userID, ok := h.weglideCaller(c)
	if !ok {
		return
	}
	var req generated.LinkWeGlideJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	st, err := h.weglideService.Link(c.Request.Context(), userID, req.ApiKey)
	if err != nil {
		h.sendWeGlideError(c, err)
		return
	}
	c.JSON(http.StatusOK, convertWeGlideStatus(st))
}

// UnlinkWeGlide implements DELETE /integrations/weglide.
func (h *APIHandler) UnlinkWeGlide(c *gin.Context) {
	userID, ok := h.weglideCaller(c)
	if !ok {
		return
	}
	if err := h.weglideService.Unlink(c.Request.Context(), userID); err != nil {
		h.sendWeGlideError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// SyncWeGlide implements POST /integrations/weglide/sync.
func (h *APIHandler) SyncWeGlide(c *gin.Context) {
	userID, ok := h.weglideCaller(c)
	if !ok {
		return
	}
	res, err := h.weglideService.Sync(c.Request.Context(), userID)
	if err != nil {
		h.sendWeGlideError(c, err)
		return
	}
	status := http.StatusOK
	if !res.Complete {
		status = http.StatusAccepted
	}
	c.JSON(status, generated.WeGlideSyncResult{
		Imported:          res.Imported,
		Skipped:           res.Skipped,
		Remaining:         res.Remaining,
		RequestsUsedToday: res.RequestsUsedToday,
	})
}

// weglideCaller resolves the authenticated user and confirms the service is
// wired, writing the error response itself otherwise.
func (h *APIHandler) weglideCaller(c *gin.Context) (uuid.UUID, bool) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return uuid.Nil, false
	}
	if h.weglideService == nil {
		h.sendError(c, http.StatusServiceUnavailable, "The WeGlide link is not available on this server")
		return uuid.Nil, false
	}
	return userID, true
}

// sendWeGlideError maps the service's sentinel errors onto status codes.
func (h *APIHandler) sendWeGlideError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrWeGlideInvalidKey):
		h.sendError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrWeGlideNotLinked):
		h.sendError(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrWeGlideSyncInProgress):
		h.sendError(c, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrWeGlideBudgetUsed):
		h.sendError(c, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, service.ErrWeGlideUnavailable):
		h.sendError(c, http.StatusBadGateway, err.Error())
	default:
		slog.Error("weglide request failed", "error", err)
		h.sendError(c, http.StatusInternalServerError, "The WeGlide request failed")
	}
}

func convertWeGlideStatus(st *service.WeGlideStatus) generated.WeGlideLinkStatus {
	out := generated.WeGlideLinkStatus{
		Linked:            st.Linked,
		WeglideUserId:     st.WeGlideUserID,
		LastSyncAt:        st.LastSyncAt,
		LastSyncError:     st.LastSyncError,
		RequestsUsedToday: st.RequestsUsedToday,
		RequestsPerDay:    st.RequestsPerDay,
	}
	if st.LastSyncStatus != nil {
		s := generated.WeGlideLinkStatusLastSyncStatus(*st.LastSyncStatus)
		out.LastSyncStatus = &s
	}
	return out
}
