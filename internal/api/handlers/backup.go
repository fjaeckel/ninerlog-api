package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// requireBackupService returns the configured backup service or writes a 503
// to the response and returns nil.
func (h *APIHandler) requireBackupService(c *gin.Context) *cloudbackup.Service {
	if h.backupService == nil {
		h.sendError(c, http.StatusServiceUnavailable, "Cloud backups are not configured on this server")
		return nil
	}
	return h.backupService
}

// ListBackupProviders returns the static catalog of registered providers.
func (h *APIHandler) ListBackupProviders(c *gin.Context) {
	svc := h.requireBackupService(c)
	if svc == nil {
		return
	}
	providers := svc.ListProviders()
	out := make([]generated.BackupProvider, 0, len(providers))
	for _, p := range providers {
		out = append(out, providerToAPI(p))
	}
	c.JSON(http.StatusOK, out)
}

// ListBackupDestinations returns the user's configured destinations.
func (h *APIHandler) ListBackupDestinations(c *gin.Context) {
	svc := h.requireBackupService(c)
	if svc == nil {
		return
	}
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	dests, err := svc.ListDestinations(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to list backup destinations")
		return
	}
	out := make([]generated.BackupDestination, 0, len(dests))
	for _, d := range dests {
		out = append(out, destinationToAPI(d))
	}
	c.JSON(http.StatusOK, out)
}

// CreateBackupDestination wires up a new destination for the current user.
func (h *APIHandler) CreateBackupDestination(c *gin.Context) {
	svc := h.requireBackupService(c)
	if svc == nil {
		return
	}
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var body generated.BackupDestinationCreate
	if err := c.ShouldBindJSON(&body); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	in := cloudbackup.CreateDestinationInput{
		UserID:             userID,
		Provider:           body.Provider,
		DisplayName:        body.DisplayName,
		Config:             provider.Config(body.Config),
		Credentials:        provider.Credentials(body.Credentials),
		Schedule:           models.BackupSchedule(body.Schedule),
		ScheduleHourUTC:    intOrDefault(body.ScheduleHourUtc, 3),
		ScheduleDayOfWeek:  body.ScheduleDayOfWeek,
		ScheduleDayOfMonth: body.ScheduleDayOfMonth,
		RetentionCount:     intOrDefault(body.RetentionCount, 30),
		Enabled:            boolOrDefault(body.Enabled, true),
	}
	d, err := svc.CreateDestination(c.Request.Context(), in)
	if err != nil {
		h.respondBackupError(c, err)
		return
	}
	c.JSON(http.StatusCreated, destinationToAPI(d))
}

// GetBackupDestination returns one destination by ID.
func (h *APIHandler) GetBackupDestination(c *gin.Context, destinationID generated.BackupDestinationId) {
	svc := h.requireBackupService(c)
	if svc == nil {
		return
	}
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	d, err := svc.GetDestination(c.Request.Context(), uuid.UUID(destinationID), userID)
	if err != nil {
		h.respondBackupError(c, err)
		return
	}
	c.JSON(http.StatusOK, destinationToAPI(d))
}

// UpdateBackupDestination applies a partial update.
func (h *APIHandler) UpdateBackupDestination(c *gin.Context, destinationID generated.BackupDestinationId) {
	svc := h.requireBackupService(c)
	if svc == nil {
		return
	}
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var body generated.BackupDestinationUpdate
	if err := c.ShouldBindJSON(&body); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	var sched *models.BackupSchedule
	if body.Schedule != nil {
		s := models.BackupSchedule(*body.Schedule)
		sched = &s
	}
	d, err := svc.UpdateDestination(c.Request.Context(), uuid.UUID(destinationID), userID, cloudbackup.UpdateDestinationInput{
		DisplayName:        body.DisplayName,
		Schedule:           sched,
		ScheduleHourUTC:    body.ScheduleHourUtc,
		ScheduleDayOfWeek:  body.ScheduleDayOfWeek,
		ScheduleDayOfMonth: body.ScheduleDayOfMonth,
		RetentionCount:     body.RetentionCount,
		Enabled:            body.Enabled,
	})
	if err != nil {
		h.respondBackupError(c, err)
		return
	}
	c.JSON(http.StatusOK, destinationToAPI(d))
}

// DeleteBackupDestination removes a destination.
func (h *APIHandler) DeleteBackupDestination(c *gin.Context, destinationID generated.BackupDestinationId) {
	svc := h.requireBackupService(c)
	if svc == nil {
		return
	}
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if err := svc.DeleteDestination(c.Request.Context(), uuid.UUID(destinationID), userID); err != nil {
		h.respondBackupError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// TestBackupDestination re-runs the provider validation.
func (h *APIHandler) TestBackupDestination(c *gin.Context, destinationID generated.BackupDestinationId) {
	svc := h.requireBackupService(c)
	if svc == nil {
		return
	}
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	ok, msg, err := svc.TestDestination(c.Request.Context(), uuid.UUID(destinationID), userID)
	if err != nil {
		h.respondBackupError(c, err)
		return
	}
	res := generated.BackupTestResult{Success: ok}
	if msg != "" {
		res.Message = &msg
	}
	c.JSON(http.StatusOK, res)
}

// RunBackupNow starts a synchronous backup run.
func (h *APIHandler) RunBackupNow(c *gin.Context, destinationID generated.BackupDestinationId) {
	svc := h.requireBackupService(c)
	if svc == nil {
		return
	}
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	run, err := svc.RunOnce(c.Request.Context(), cloudbackup.RunRequest{
		DestinationID: uuid.UUID(destinationID),
		UserID:        userID,
		Trigger:       models.BackupRunTriggerManual,
	})
	if err != nil {
		if errors.Is(err, cloudbackup.ErrConcurrentRun) {
			h.sendError(c, http.StatusConflict, err.Error())
			return
		}
		// Even if RunOnce returned an error, the BackupRun record was
		// persisted with the failure details, so return 202-with-run-payload.
		if run != nil {
			c.JSON(http.StatusAccepted, runToAPI(run))
			return
		}
		h.respondBackupError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, runToAPI(run))
}

// ListBackupRuns returns paginated audit log entries.
func (h *APIHandler) ListBackupRuns(c *gin.Context, destinationID generated.BackupDestinationId, params generated.ListBackupRunsParams) {
	svc := h.requireBackupService(c)
	if svc == nil {
		return
	}
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	page := intPtrOrDefault(params.Page, 1)
	pageSize := intPtrOrDefault(params.PageSize, 20)
	runs, total, err := svc.ListRuns(c.Request.Context(), uuid.UUID(destinationID), userID, page, pageSize)
	if err != nil {
		h.respondBackupError(c, err)
		return
	}
	out := generated.PaginatedBackupRuns{
		Data: make([]generated.BackupRun, 0, len(runs)),
	}
	out.Pagination.Page = page
	out.Pagination.PageSize = pageSize
	out.Pagination.Total = total
	out.Pagination.TotalPages = (total + pageSize - 1) / pageSize
	for _, r := range runs {
		out.Data = append(out.Data, runToAPI(r))
	}
	c.JSON(http.StatusOK, out)
}

// GetBackupRun returns a single run scoped to the calling user.
func (h *APIHandler) GetBackupRun(c *gin.Context, runID openapi_types.UUID) {
	svc := h.requireBackupService(c)
	if svc == nil {
		return
	}
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	run, err := svc.GetRun(c.Request.Context(), uuid.UUID(runID), userID)
	if err != nil {
		h.respondBackupError(c, err)
		return
	}
	c.JSON(http.StatusOK, runToAPI(run))
}

// ----- helpers ------------------------------------------------------------

func (h *APIHandler) respondBackupError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, cloudbackup.ErrDestinationNotFound):
		h.sendError(c, http.StatusNotFound, "Backup destination not found")
	case errors.Is(err, cloudbackup.ErrUnauthorized):
		h.sendError(c, http.StatusForbidden, "Not authorized for this destination")
	case errors.Is(err, cloudbackup.ErrProviderUnknown):
		h.sendError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, cloudbackup.ErrInvalidSchedule):
		h.sendError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, provider.ErrInvalidConfig):
		h.sendError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, provider.ErrInvalidCredentials):
		h.sendError(c, http.StatusUnauthorized, "Invalid provider credentials")
	case errors.Is(err, provider.ErrPermissionDenied):
		h.sendError(c, http.StatusForbidden, "Provider rejected the request: permission denied")
	case errors.Is(err, provider.ErrNotFound):
		h.sendError(c, http.StatusBadRequest, "Provider resource not found (e.g. bucket)")
	case errors.Is(err, provider.ErrTransient):
		h.sendError(c, http.StatusBadGateway, "Provider returned a transient error; please retry")
	default:
		// Avoid leaking internal details; sanitised messages are already
		// stored on the destination's last_error field.
		msg := err.Error()
		if strings.Contains(strings.ToLower(msg), "missing required field") {
			h.sendError(c, http.StatusBadRequest, msg)
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Backup operation failed")
	}
}

func providerToAPI(p provider.Provider) generated.BackupProvider {
	desc := p.Description()
	return generated.BackupProvider{
		Name:             p.Name(),
		DisplayName:      p.DisplayName(),
		Description:      &desc,
		ConfigSchema:     generated.BackupFieldSchema{Fields: fieldsToAPI(p.ConfigSchema())},
		CredentialSchema: generated.BackupFieldSchema{Fields: fieldsToAPI(p.CredentialSchema())},
	}
}

func fieldsToAPI(in []provider.Field) []generated.BackupField {
	out := make([]generated.BackupField, 0, len(in))
	for _, f := range in {
		bf := generated.BackupField{
			Name:     f.Name,
			Label:    f.Label,
			Required: f.Required,
			Type:     generated.BackupFieldType(string(f.Type)),
		}
		if f.Help != "" {
			h := f.Help
			bf.Description = &h
		}
		if f.Placeholder != "" {
			p := f.Placeholder
			bf.Placeholder = &p
		}
		out = append(out, bf)
	}
	return out
}

func destinationToAPI(d *models.BackupDestination) generated.BackupDestination {
	out := generated.BackupDestination{
		Id:              openapi_types.UUID(d.ID),
		UserId:          openapi_types.UUID(d.UserID),
		Provider:        d.Provider,
		DisplayName:     d.DisplayName,
		Config:          d.Config,
		Schedule:        generated.BackupSchedule(string(d.Schedule)),
		RetentionCount:  d.RetentionCount,
		Status:          generated.BackupStatus(string(d.Status)),
		Enabled:         d.Enabled,
		CreatedAt:       d.CreatedAt,
		UpdatedAt:       d.UpdatedAt,
	}
	hour := d.ScheduleHourUTC
	out.ScheduleHourUtc = &hour
	if d.ScheduleDayOfWeek != nil {
		v := *d.ScheduleDayOfWeek
		out.ScheduleDayOfWeek = &v
	}
	if d.ScheduleDayOfMonth != nil {
		v := *d.ScheduleDayOfMonth
		out.ScheduleDayOfMonth = &v
	}
	if d.CredentialHint != "" {
		h := d.CredentialHint
		out.CredentialHint = &h
	}
	if d.LastError != "" {
		e := d.LastError
		out.LastError = &e
	}
	if d.LastRunAt != nil {
		t := *d.LastRunAt
		out.LastRunAt = &t
	}
	if d.LastSuccessAt != nil {
		t := *d.LastSuccessAt
		out.LastSuccessAt = &t
	}
	cf := d.ConsecutiveFailures
	out.ConsecutiveFailures = &cf
	return out
}

func runToAPI(r *models.BackupRun) generated.BackupRun {
	out := generated.BackupRun{
		Id:            openapi_types.UUID(r.ID),
		DestinationId: openapi_types.UUID(r.DestinationID),
		UserId:        openapi_types.UUID(r.UserID),
		Status:        generated.BackupRunStatus(string(r.Status)),
		StartedAt:     r.StartedAt,
		CreatedAt:     r.CreatedAt,
	}
	trig := generated.BackupRunTrigger(string(r.Trigger))
	out.Trigger = &trig
	if r.CompletedAt != nil {
		t := *r.CompletedAt
		out.CompletedAt = &t
	}
	if r.DurationMs != nil {
		v := *r.DurationMs
		out.DurationMs = &v
	}
	if r.SizeBytes != nil {
		v := *r.SizeBytes
		out.SizeBytes = &v
	}
	if r.SHA256 != "" {
		v := r.SHA256
		out.Sha256 = &v
	}
	if r.FlightCount != nil {
		v := *r.FlightCount
		out.FlightCount = &v
	}
	if r.AircraftCount != nil {
		v := *r.AircraftCount
		out.AircraftCount = &v
	}
	if r.LicenseCount != nil {
		v := *r.LicenseCount
		out.LicenseCount = &v
	}
	if r.CredentialCount != nil {
		v := *r.CredentialCount
		out.CredentialCount = &v
	}
	if r.RemotePath != "" {
		v := r.RemotePath
		out.RemotePath = &v
	}
	if r.ErrorMessage != "" {
		v := r.ErrorMessage
		out.ErrorMessage = &v
	}
	return out
}

func intOrDefault(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func intPtrOrDefault(p *int, def int) int {
	if p == nil || *p <= 0 {
		return def
	}
	return *p
}

func boolOrDefault(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}
