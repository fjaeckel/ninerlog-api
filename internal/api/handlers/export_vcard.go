package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ExportContactsVCard handles GET /exports/vcard
func (h *APIHandler) ExportContactsVCard(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	card, err := h.contactService.ExportVCard(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to export contacts")
		return
	}

	c.Header("Content-Type", "text/vcard; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=ninerlog_contacts_%s.vcf", time.Now().Format("2006-01-02")))
	c.Data(http.StatusOK, "text/vcard; charset=utf-8", card)
}
