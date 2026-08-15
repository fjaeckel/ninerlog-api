package handlers

import (
	"bytes"

	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/service/portability"
	"github.com/gin-gonic/gin"
)

// Leaving is a supported operation.
//
// These three endpoints exist so a pilot can take a career's worth of logbook
// to another product without asking anyone's permission and without
// hand-mapping columns in a spreadsheet. The handlers stay thin — every format
// decision lives in internal/service/portability.

// portabilityGatherer builds the data-gathering component from the services
// this handler already holds. It is constructed per request rather than stored
// so that a service wired in later is picked up without another constructor
// parameter.
func (h *APIHandler) portabilityGatherer() *portability.Gatherer {
	return &portability.Gatherer{
		Flights:     h.flightService,
		Aircraft:    h.aircraftService,
		Licenses:    h.licenseService,
		ClassRating: h.classRatingService,
		Credentials: h.credentialService,
		Contacts:    h.contactService,
		Signatures:  h.flightSignatureService,
		Users:       h.authService,
		AttachCrew:  h.attachCrewMembers,
	}
}

// ListExportTargets implements GET /exports/targets
func (h *APIHandler) ListExportTargets(c *gin.Context) {
	if _, err := h.getUserIDFromContext(c); err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	descriptors := portability.Targets()
	targets := make([]generated.ExportTarget, 0, len(descriptors))
	for _, d := range descriptors {
		targets = append(targets, generated.ExportTarget{
			Id:          generated.ExportTargetId(d.Target),
			Product:     d.Product,
			ContentType: d.ContentType,
			Extension:   d.Extension,
			Notes:       d.Notes,
			Verified:    d.Verified,
		})
	}

	c.JSON(http.StatusOK, gin.H{"targets": targets})
}

// ExportLogbookForTarget implements GET /exports/logbook
func (h *APIHandler) ExportLogbookForTarget(c *gin.Context, params generated.ExportLogbookForTargetParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	target := portability.Target(params.Target)
	descriptor, err := portability.Lookup(target)
	if err != nil {
		h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Unsupported export target %q", params.Target))
		return
	}

	label := string(target)
	started := time.Now()

	bundle, err := h.portabilityGatherer().Gather(c.Request.Context(), userID)
	if err != nil {
		portability.ExportsTotal.WithLabelValues(label, portability.ResultError).Inc()
		slog.Error("portability export failed to gather data", "target", label, "error", err)
		h.sendError(c, http.StatusInternalServerError, "Failed to assemble your logbook for export")
		return
	}

	// Render into a buffer before touching the response. A pilot who asks for
	// their logbook must get either the whole file or an honest error — never a
	// 200 that turns out to be a truncated logbook halfway down.
	var buf bytes.Buffer
	if err := portability.Write(target, &buf, bundle); err != nil {
		portability.ExportsTotal.WithLabelValues(label, portability.ResultError).Inc()
		slog.Error("portability export failed to render", "target", label, "error", err)
		h.sendError(c, http.StatusInternalServerError, "Failed to render the export")
		return
	}

	portability.ExportsTotal.WithLabelValues(label, portability.ResultSuccess).Inc()
	portability.ExportDurationSeconds.WithLabelValues(label).Observe(time.Since(started).Seconds())
	portability.ExportFlightRows.Observe(float64(len(bundle.Flights)))

	filename := descriptor.Filename(bundle.ExportedAt)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, descriptor.ContentType, buf.Bytes())
}

// ExportPortabilityArchive implements GET /exports/archive
func (h *APIHandler) ExportPortabilityArchive(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	label := portability.ArchiveTargetLabel
	started := time.Now()

	bundle, err := h.portabilityGatherer().Gather(c.Request.Context(), userID)
	if err != nil {
		portability.ExportsTotal.WithLabelValues(label, portability.ResultError).Inc()
		slog.Error("portability archive failed to gather data", "error", err)
		h.sendError(c, http.StatusInternalServerError, "Failed to assemble your logbook for export")
		return
	}

	// The ZIP central directory is written on Close, so a mid-stream failure
	// would otherwise leave the pilot with a corrupt archive and a 200.
	var buf bytes.Buffer
	if err := portability.WriteArchive(&buf, bundle); err != nil {
		portability.ExportsTotal.WithLabelValues(label, portability.ResultError).Inc()
		slog.Error("portability archive failed to render", "error", err)
		h.sendError(c, http.StatusInternalServerError, "Failed to build the archive")
		return
	}

	portability.ExportsTotal.WithLabelValues(label, portability.ResultSuccess).Inc()
	portability.ExportDurationSeconds.WithLabelValues(label).Observe(time.Since(started).Seconds())
	portability.ExportFlightRows.Observe(float64(len(bundle.Flights)))

	filename := portability.ArchiveFilename(bundle.ExportedAt)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}
