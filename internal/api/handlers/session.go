package handlers

import (
	"errors"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// getSessionIDFromContext reads the session the access token belongs to.
// Returns uuid.Nil for a token minted before sessions existed.
func getSessionIDFromContext(c *gin.Context) uuid.UUID {
	if sessionID, exists := c.Get("sessionID"); exists {
		if id, ok := sessionID.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.Nil
}

// ListSessions implements GET /auth/sessions
// (GET /auth/sessions)
func (h *APIHandler) ListSessions(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Authentication required")
		return
	}

	sessions, err := h.authService.ListSessions(c.Request.Context(), userID, getSessionIDFromContext(c))
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to list sessions")
		return
	}

	out := make([]generated.Session, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, convertSession(s))
	}

	c.JSON(http.StatusOK, gin.H{
		"sessions":    out,
		"maxSessions": h.authService.SessionPolicy().MaxPerUser,
	})
}

// RevokeSession implements DELETE /auth/sessions/{sessionId}
// (DELETE /auth/sessions/{sessionId})
func (h *APIHandler) RevokeSession(c *gin.Context, sessionId openapi_types.UUID) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Authentication required")
		return
	}

	if err := h.authService.RevokeSession(c.Request.Context(), userID, sessionId); err != nil {
		if errors.Is(err, service.ErrSessionNotFound) {
			h.sendError(c, http.StatusNotFound, "Session not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to revoke session")
		return
	}

	c.Status(http.StatusNoContent)
}

// RevokeOtherSessions implements DELETE /auth/sessions
// (DELETE /auth/sessions)
func (h *APIHandler) RevokeOtherSessions(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Authentication required")
		return
	}

	revoked, err := h.authService.RevokeOtherSessions(c.Request.Context(), userID, getSessionIDFromContext(c))
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to revoke sessions")
		return
	}

	c.JSON(http.StatusOK, gin.H{"revoked": revoked})
}

func convertSession(s *models.Session) generated.Session {
	out := generated.Session{
		Id:          s.ID,
		DeviceLabel: s.DeviceLabel,
		CreatedAt:   s.CreatedAt,
		LastUsedAt:  s.LastUsedAt,
		ExpiresAt:   s.ExpiresAt,
		Current:     s.Current,
	}
	if s.IPAddress != "" {
		ip := s.IPAddress
		out.IpAddress = &ip
	}
	return out
}
