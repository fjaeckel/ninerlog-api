package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// DeleteAllFlights implements DELETE /flights/delete-all
func (h *APIHandler) DeleteAllFlights(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Delete import history first
	_, _ = h.db.ExecContext(c.Request.Context(),
		`DELETE FROM flight_imports WHERE user_id = $1`, userID)

	deleted, err := h.flightService.DeleteAllFlights(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to delete flights")
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

// DeleteAllUserData implements DELETE /users/me/data
func (h *APIHandler) DeleteAllUserData(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Delete in dependency order to respect FK constraints:
	// 1. flight_crew_members (depends on flights, contacts)
	// 2. flight_imports (depends on users)
	// 3. flights (depends on users)
	// 4. class_ratings (depends on licenses)
	// 5. licenses (depends on users)
	// 6. aircraft (depends on users)
	// 7. contacts (depends on users)
	// 8. credentials (depends on users)
	// 9. notification_log (depends on users)

	queries := []string{
		`DELETE FROM flight_crew_members WHERE flight_id IN (SELECT id FROM flights WHERE user_id = $1)`,
		`DELETE FROM flight_imports WHERE user_id = $1`,
		`DELETE FROM flights WHERE user_id = $1`,
		`DELETE FROM class_ratings WHERE license_id IN (SELECT id FROM licenses WHERE user_id = $1)`,
		`DELETE FROM licenses WHERE user_id = $1`,
		`DELETE FROM aircraft WHERE user_id = $1`,
		`DELETE FROM contacts WHERE user_id = $1`,
		`DELETE FROM credentials WHERE user_id = $1`,
		`DELETE FROM notification_log WHERE user_id = $1`,
	}

	for _, q := range queries {
		_, _ = h.db.ExecContext(c.Request.Context(), q, userID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "All user data deleted successfully"})
}
