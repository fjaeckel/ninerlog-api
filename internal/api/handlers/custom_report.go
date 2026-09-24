package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/customreport"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SetCustomReportService wires the custom report service.
func (h *APIHandler) SetCustomReportService(s *customreport.Service) {
	h.customReportService = s
}

// customReportRequest is the create/update payload.
type customReportRequest struct {
	Name       string                        `json:"name"`
	Definition models.CustomReportDefinition `json:"definition"`
}

type customReportPreviewRequest struct {
	Definition models.CustomReportDefinition `json:"definition"`
}

type customReportOrderRequest struct {
	ReportIDs []uuid.UUID `json:"reportIds"`
}

// respondCustomReportError maps service errors to HTTP responses.
func (h *APIHandler) respondCustomReportError(c *gin.Context, err error) {
	switch {
	case customreport.IsValidationError(err):
		h.sendError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, repository.ErrNotFound):
		h.sendError(c, http.StatusNotFound, "Custom report not found")
	default:
		slog.Error("[custom-report] internal error", "error", err)
		h.sendError(c, http.StatusInternalServerError, "Failed to process custom report")
	}
}

func (h *APIHandler) customReportUser(c *gin.Context) (uuid.UUID, bool) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return uuid.Nil, false
	}
	return userID, true
}

// ListCustomReports implements GET /reports/custom.
func (h *APIHandler) ListCustomReports(c *gin.Context) {
	userID, ok := h.customReportUser(c)
	if !ok {
		return
	}
	reports, err := h.customReportService.List(c.Request.Context(), userID)
	if err != nil {
		h.respondCustomReportError(c, err)
		return
	}
	c.JSON(http.StatusOK, reports)
}

// CreateCustomReport implements POST /reports/custom.
func (h *APIHandler) CreateCustomReport(c *gin.Context) {
	userID, ok := h.customReportUser(c)
	if !ok {
		return
	}
	var req customReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	rep, err := h.customReportService.Create(c.Request.Context(), userID,
		customreport.Input{Name: req.Name, Definition: req.Definition})
	if err != nil {
		h.respondCustomReportError(c, err)
		return
	}
	c.JSON(http.StatusCreated, rep)
}

// PreviewCustomReport implements POST /reports/custom/preview.
func (h *APIHandler) PreviewCustomReport(c *gin.Context) {
	userID, ok := h.customReportUser(c)
	if !ok {
		return
	}
	var req customReportPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	res, err := h.customReportService.Evaluate(c.Request.Context(), userID, req.Definition)
	if err != nil {
		h.respondCustomReportError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// ReorderCustomReports implements PUT /reports/custom/order.
func (h *APIHandler) ReorderCustomReports(c *gin.Context) {
	userID, ok := h.customReportUser(c)
	if !ok {
		return
	}
	var req customReportOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	reports, err := h.customReportService.Reorder(c.Request.Context(), userID, req.ReportIDs)
	if err != nil {
		h.respondCustomReportError(c, err)
		return
	}
	c.JSON(http.StatusOK, reports)
}

// GetCustomReport implements GET /reports/custom/{reportId}.
func (h *APIHandler) GetCustomReport(c *gin.Context, reportID generated.CustomReportId) {
	userID, ok := h.customReportUser(c)
	if !ok {
		return
	}
	rep, err := h.customReportService.Get(c.Request.Context(), userID, reportID)
	if err != nil {
		h.respondCustomReportError(c, err)
		return
	}
	c.JSON(http.StatusOK, rep)
}

// UpdateCustomReport implements PUT /reports/custom/{reportId}.
func (h *APIHandler) UpdateCustomReport(c *gin.Context, reportID generated.CustomReportId) {
	userID, ok := h.customReportUser(c)
	if !ok {
		return
	}
	var req customReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	rep, err := h.customReportService.Update(c.Request.Context(), userID, reportID,
		customreport.Input{Name: req.Name, Definition: req.Definition})
	if err != nil {
		h.respondCustomReportError(c, err)
		return
	}
	c.JSON(http.StatusOK, rep)
}

// DeleteCustomReport implements DELETE /reports/custom/{reportId}.
func (h *APIHandler) DeleteCustomReport(c *gin.Context, reportID generated.CustomReportId) {
	userID, ok := h.customReportUser(c)
	if !ok {
		return
	}
	if err := h.customReportService.Delete(c.Request.Context(), userID, reportID); err != nil {
		h.respondCustomReportError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// GetCustomReportResult implements GET /reports/custom/{reportId}/result.
func (h *APIHandler) GetCustomReportResult(c *gin.Context, reportID generated.CustomReportId) {
	userID, ok := h.customReportUser(c)
	if !ok {
		return
	}
	_, res, err := h.customReportService.Result(c.Request.Context(), userID, reportID)
	if err != nil {
		h.respondCustomReportError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// ExportCustomReport implements GET /reports/custom/{reportId}/export.
func (h *APIHandler) ExportCustomReport(c *gin.Context, reportID generated.CustomReportId, params generated.ExportCustomReportParams) {
	userID, ok := h.customReportUser(c)
	if !ok {
		return
	}
	if !params.Format.Valid() {
		h.sendError(c, http.StatusBadRequest, "format must be csv or pdf")
		return
	}
	rep, res, err := h.customReportService.Result(c.Request.Context(), userID, reportID)
	if err != nil {
		h.respondCustomReportError(c, err)
		return
	}

	var (
		body        []byte
		contentType string
	)
	if params.Format == generated.Csv {
		body, err = renderCustomReportCSV(res)
		contentType = "text/csv"
	} else {
		body, err = renderCustomReportPDF(rep, res)
		contentType = "application/pdf"
	}
	if err != nil {
		slog.Error("[custom-report] export failed", "format", params.Format, "error", err)
		h.sendError(c, http.StatusInternalServerError, "Failed to export custom report")
		return
	}
	c.Header("Content-Disposition", "attachment; filename="+customReportFilename(rep, res, string(params.Format)))
	c.Data(http.StatusOK, contentType, body)
}
