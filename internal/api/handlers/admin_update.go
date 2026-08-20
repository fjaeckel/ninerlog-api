package handlers

import (
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/updatecheck"
	"github.com/gin-gonic/gin"
)

// SetUpdateChecker attaches the release checker.
func (h *APIHandler) SetUpdateChecker(c *updatecheck.Checker) {
	h.updateChecker = c
}

// GetUpdateStatus implements GET /admin/update
func (h *APIHandler) GetUpdateStatus(c *gin.Context, params generated.GetUpdateStatusParams) {
	_, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	if h.updateChecker == nil {
		c.JSON(http.StatusOK, generated.UpdateStatus{
			CheckEnabled: false,
			Components:   []generated.UpdateComponent{},
		})
		return
	}

	var frontendVersion string
	if params.FrontendVersion != nil {
		frontendVersion = *params.FrontendVersion
	}

	c.JSON(http.StatusOK, toUpdateStatus(h.updateChecker.Status(frontendVersion)))
}

// toUpdateStatus maps the checker's status onto the generated response.
func toUpdateStatus(status updatecheck.Status) generated.UpdateStatus {
	out := generated.UpdateStatus{
		CheckEnabled:    status.Enabled,
		UpdateAvailable: status.UpdateAvailable,
		LastCheckedAt:   status.LastCheckedAt,
		Components:      make([]generated.UpdateComponent, 0, len(status.Components)),
	}

	if status.LastError != "" {
		reason := generated.UpdateStatusLastError(status.LastError)
		if !reason.Valid() {
			reason = generated.UpdateStatusLastErrorError
		}
		out.LastError = &reason
	}

	for _, component := range status.Components {
		entry := generated.UpdateComponent{
			Name:           generated.UpdateComponentName(component.Name),
			CurrentVersion: component.CurrentVersion,
			State:          generated.UpdateComponentState(component.State),
			PublishedAt:    component.PublishedAt,
		}
		if component.LatestVersion != "" {
			latest := component.LatestVersion
			entry.LatestVersion = &latest
		}
		if component.ReleaseURL != "" {
			url := component.ReleaseURL
			entry.ReleaseUrl = &url
		}
		out.Components = append(out.Components, entry)
	}

	return out
}
