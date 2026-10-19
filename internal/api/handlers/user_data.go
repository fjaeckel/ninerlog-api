package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/errcode"
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// UserDataRequestBudget is the time a backup export or restore request may run.
const UserDataRequestBudget = 5 * time.Minute

// extendDeadlines lifts the server's read and write deadlines for this request
// to UserDataRequestBudget. Writers that cannot be extended are left alone.
func extendDeadlines(c *gin.Context) {
	rc := http.NewResponseController(c.Writer)
	deadline := time.Now().Add(UserDataRequestBudget)
	_ = rc.SetReadDeadline(deadline)
	_ = rc.SetWriteDeadline(deadline)
}

// ExportUserData implements GET /users/me/data.
func (h *APIHandler) ExportUserData(c *gin.Context) {
	h.exportBackup(c)
}

// ExportDataJSON implements GET /exports/json, the deprecated alias of
// GET /users/me/data.
func (h *APIHandler) ExportDataJSON(c *gin.Context) {
	h.exportBackup(c)
}

func (h *APIHandler) exportBackup(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	extendDeadlines(c)

	backup, err := h.BackupPayloadBuilder().Gather(c.Request.Context(), userID)
	if err != nil {
		slog.Error("json export gather error", "error", err)
		h.sendError(c, http.StatusInternalServerError, "Failed to build export")
		return
	}

	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=ninerlog_backup_%s.json", time.Now().Format("2006-01-02")))

	encoder := json.NewEncoder(c.Writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(backup); err != nil {
		slog.Error("json export encode error", "error", err)
	}
}

// RestoreUserData implements POST /users/me/data.
func (h *APIHandler) RestoreUserData(c *gin.Context, params generated.RestoreUserDataParams) {
	mode := ""
	if params.Mode != nil {
		mode = string(*params.Mode)
	}
	h.restoreBackup(c, mode)
}

// ImportDataJSON implements POST /imports/json, the deprecated alias of
// POST /users/me/data.
func (h *APIHandler) ImportDataJSON(c *gin.Context, params generated.ImportDataJSONParams) {
	mode := ""
	if params.Mode != nil {
		mode = string(*params.Mode)
	}
	h.restoreBackup(c, mode)
}

func (h *APIHandler) restoreBackup(c *gin.Context, modeParam string) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.restorer == nil {
		h.sendError(c, http.StatusServiceUnavailable, "Restore is not available")
		return
	}
	mode := cloudbackup.RestoreMode(modeParam)
	if mode == "" {
		mode = cloudbackup.RestoreModeRestore
	}
	if !mode.Valid() {
		h.sendError(c, http.StatusBadRequest, "Invalid mode (expected 'restore' or 'replace')")
		return
	}
	extendDeadlines(c)

	var body cloudbackup.Payload
	if err := json.NewDecoder(c.Request.Body).Decode(&body); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			h.sendError(c, http.StatusRequestEntityTooLarge, "Request body too large")
			return
		}
		slog.Warn("backup restore: undecodable body", "user_id", userID, "error", err)
		h.sendError(c, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	result, err := h.restorer.Restore(c.Request.Context(), userID, &body, mode)
	if err != nil {
		h.respondRestoreError(c, userID, err)
		return
	}
	slog.Info("backup restored", "user_id", userID, "mode", result.Mode,
		"flights", result.FlightsImported, "warnings", len(result.Warnings))
	c.JSON(http.StatusOK, result)
}

func (h *APIHandler) respondRestoreError(c *gin.Context, userID uuid.UUID, err error) {
	var invalid *cloudbackup.InvalidBackupError
	switch {
	case errors.Is(err, cloudbackup.ErrAccountNotEmpty):
		h.sendCodedError(c, http.StatusConflict, errcode.AccountNotEmpty,
			"The account already holds data; restore into an empty account or use mode=replace")
	case errors.Is(err, cloudbackup.ErrReplaceWouldDropAttachments):
		h.sendCodedError(c, http.StatusConflict, errcode.ReplaceWouldDropAttachments,
			"Replacing would delete document files or signed flights that a backup does not carry")
	case errors.Is(err, cloudbackup.ErrUnsupportedBackupVersion):
		h.sendCodedError(c, http.StatusUnprocessableEntity, errcode.BackupVersionUnsupported,
			"This backup was written by a newer version of NinerLog")
	case errors.As(err, &invalid):
		slog.Warn("backup restore: invalid backup", "user_id", userID, "error", invalid.Error())
		h.sendError(c, http.StatusBadRequest, invalid.Message)
	default:
		slog.Error("backup restore failed", "user_id", userID, "error", err)
		h.sendError(c, http.StatusInternalServerError, "Failed to restore backup")
	}
}

// DeleteAllUserData implements DELETE /users/me/data.
func (h *APIHandler) DeleteAllUserData(c *gin.Context, params generated.DeleteAllUserDataParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	scope := generated.DeleteAllUserDataParamsScopeAll
	if params.Scope != nil {
		scope = *params.Scope
	}
	switch scope {
	case generated.DeleteAllUserDataParamsScopeAll:
		if err := h.userDataService.DeleteAll(c.Request.Context(), userID); err != nil {
			slog.Error("delete all user data failed", "user_id", userID, "error", err)
			h.sendError(c, http.StatusInternalServerError, "Failed to delete user data")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "All user data deleted successfully"})
	case generated.DeleteAllUserDataParamsScopeFlights:
		deleted, err := h.userDataService.DeleteFlights(c.Request.Context(), userID)
		if err != nil {
			slog.Error("delete all flights failed", "user_id", userID, "error", err)
			h.sendError(c, http.StatusInternalServerError, "Failed to delete flights")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "All flights deleted successfully", "deletedFlights": deleted})
	default:
		h.sendError(c, http.StatusBadRequest, "Invalid scope (expected 'all' or 'flights')")
	}
}

// DeleteAllFlights implements DELETE /flights/delete-all, the deprecated alias
// of DELETE /users/me/data?scope=flights.
func (h *APIHandler) DeleteAllFlights(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	deleted, err := h.userDataService.DeleteFlights(c.Request.Context(), userID)
	if err != nil {
		slog.Error("delete all flights failed", "user_id", userID, "error", err)
		h.sendError(c, http.StatusInternalServerError, "Failed to delete flights")
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}
