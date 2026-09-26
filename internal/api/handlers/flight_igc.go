package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/registration"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// SetFlightFileService wires up flight recorder files.
func (h *APIHandler) SetFlightFileService(s *service.FlightFileService) {
	h.flightFileService = s
}

// PreviewIgcFlight implements POST /flights/igc/preview.
func (h *APIHandler) PreviewIgcFlight(c *gin.Context) {
	userID, ok := h.flightFileCaller(c)
	if !ok {
		return
	}
	data, _, ok := h.readIgcUpload(c)
	if !ok {
		return
	}
	preview, err := h.flightFileService.Preview(c.Request.Context(), userID, data)
	if err != nil {
		h.sendFlightFileError(c, err)
		return
	}
	c.JSON(http.StatusOK, convertIgcPreview(preview))
}

// ImportIgcFlight implements POST /flights/igc.
func (h *APIHandler) ImportIgcFlight(c *gin.Context) {
	userID, ok := h.flightFileCaller(c)
	if !ok {
		return
	}
	data, filename, ok := h.readIgcUpload(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	var flightID uuid.UUID
	if raw := c.PostForm("flightId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			h.sendError(c, http.StatusBadRequest, "flightId must be a UUID")
			return
		}
		flightID = id
	}

	preview, err := h.flightFileService.Preview(ctx, userID, data)
	if err != nil {
		h.sendFlightFileError(c, err)
		return
	}

	var flight *models.Flight
	if flightID != uuid.Nil {
		file, err := h.flightFileService.Attach(ctx, userID, flightID, filename, data)
		if err != nil {
			h.sendFlightFileError(c, err)
			return
		}
		flight, err = h.flightService.GetFlight(ctx, flightID, userID)
		if err != nil {
			h.sendFlightFileError(c, err)
			return
		}
		h.respondIgcImport(c, flight, file, preview)
		return
	}

	if preview.GliderRegistration == nil {
		h.sendFlightFileError(c, service.ErrIGCNoRegistration)
		return
	}
	stored, err := h.flightFileService.StoredFlightFor(ctx, userID, data)
	if err != nil {
		h.sendFlightFileError(c, err)
		return
	}
	if stored != nil {
		h.sendFlightFileError(c, &service.IGCDuplicateError{FlightID: *stored})
		return
	}

	aircraft := h.ensureIgcAircraft(ctx, userID, *preview.GliderRegistration, preview.GliderType)
	req := igcFlightCreate(preview, aircraft)
	flight, errMsg := h.flightFromCreate(c, userID, &req)
	if errMsg != "" {
		h.sendError(c, http.StatusBadRequest, "The IGC file does not describe a valid flight: "+errMsg)
		return
	}
	if err := h.flightService.CreateFlight(ctx, flight); err != nil {
		h.sendError(c, http.StatusBadRequest, "The IGC file does not describe a valid flight")
		return
	}
	file, err := h.flightFileService.Attach(ctx, userID, flight.ID, filename, data)
	if err != nil {
		if derr := h.flightService.DeleteFlight(ctx, flight.ID, userID); derr != nil {
			slog.Error("igc import: failed to remove flight after file store failed", "flightId", flight.ID, "error", derr)
		}
		h.sendFlightFileError(c, err)
		return
	}
	h.respondIgcImport(c, flight, file, preview)
}

func (h *APIHandler) respondIgcImport(c *gin.Context, flight *models.Flight, file *models.FlightFile, preview *service.IGCPreview) {
	c.JSON(http.StatusCreated, generated.IgcImportResult{
		Flight:  convertToGeneratedFlight(flight),
		FileId:  openapi_types.UUID(file.ID),
		Summary: convertIgcPreview(preview),
	})
}

// ensureIgcAircraft returns the user's aircraft with this registration,
// creating it as the importers do when absent. Returns nil when it can
// neither be found nor created.
func (h *APIHandler) ensureIgcAircraft(ctx context.Context, userID uuid.UUID, reg string, gliderType *string) *models.Aircraft {
	fleet, err := h.aircraftService.ListAircraft(ctx, userID)
	if err == nil {
		for _, a := range fleet {
			if registration.Canonical(a.Registration) == reg {
				return a
			}
		}
	}
	typeCode := reg
	if gliderType != nil && *gliderType != "" {
		typeCode = *gliderType
	}
	a := &models.Aircraft{
		UserID:        userID,
		Registration:  reg,
		Type:          typeCode,
		Make:          typeCode,
		Model:         typeCode,
		IsActive:      true,
		AircraftClass: models.InferImportedAircraftClass("", reg, true),
	}
	if err := h.aircraftService.CreateAircraft(ctx, a); err != nil {
		slog.Warn("igc import: failed to create aircraft", "registration", reg, "error", err)
		return nil
	}
	return a
}

// igcFlightCreate builds the POST /flights body an IGC preview describes.
func igcFlightCreate(p *service.IGCPreview, aircraft *models.Aircraft) generated.FlightCreate {
	reg := *p.GliderRegistration
	aircraftType := reg
	if aircraft != nil && aircraft.Type != "" {
		aircraftType = aircraft.Type
	} else if p.GliderType != nil {
		aircraftType = *p.GliderType
	}
	dep, arr := p.Departure.Label(), p.Arrival.Label()
	takeoff, landing := p.TakeoffTime, p.LandingTime
	landings := 1
	outlanding := p.Outlanding
	req := generated.FlightCreate{
		Date:          openapi_types.Date{Time: p.Date},
		AircraftReg:   &reg,
		AircraftType:  aircraftType,
		DepartureIcao: &dep,
		ArrivalIcao:   &arr,
		DepartureTime: &takeoff,
		ArrivalTime:   &landing,
		Landings:      &landings,
		IsOutlanding:  &outlanding,
	}
	var class *string
	var kind *models.ULKind
	if aircraft != nil {
		class, kind = aircraft.AircraftClass, aircraft.ULKind
	}
	if p.LaunchMethod != "unknown" && models.IsValidLaunchMethod(p.LaunchMethod) && models.LaunchMethodApplies(class, kind) {
		lm := generated.FlightCreateLaunchMethod(p.LaunchMethod)
		req.LaunchMethod = &lm
		req.ReleaseHeightM = p.ReleaseHeightM
	}
	return req
}

// ListFlightFiles implements GET /flights/{flightId}/files.
func (h *APIHandler) ListFlightFiles(c *gin.Context, flightId generated.FlightId) {
	userID, ok := h.flightFileCaller(c)
	if !ok {
		return
	}
	files, err := h.flightFileService.List(c.Request.Context(), userID, uuid.UUID(flightId))
	if err != nil {
		h.sendFlightFileError(c, err)
		return
	}
	out := make([]generated.FlightFile, 0, len(files))
	for _, f := range files {
		out = append(out, convertFlightFile(f))
	}
	c.JSON(http.StatusOK, out)
}

// GetFlightFile implements GET /flights/{flightId}/files/{fileId}.
func (h *APIHandler) GetFlightFile(c *gin.Context, flightId generated.FlightId, fileId generated.FlightFileId) {
	userID, ok := h.flightFileCaller(c)
	if !ok {
		return
	}
	file, err := h.flightFileService.Get(c.Request.Context(), userID, uuid.UUID(flightId), uuid.UUID(fileId))
	if err != nil {
		h.sendFlightFileError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", contentDispositionSafe(file.Filename)))
	c.Data(http.StatusOK, models.FlightFileContentTypeIGC, file.Content)
}

// DeleteFlightFile implements DELETE /flights/{flightId}/files/{fileId}.
func (h *APIHandler) DeleteFlightFile(c *gin.Context, flightId generated.FlightId, fileId generated.FlightFileId) {
	userID, ok := h.flightFileCaller(c)
	if !ok {
		return
	}
	if err := h.flightFileService.Delete(c.Request.Context(), userID, uuid.UUID(flightId), uuid.UUID(fileId)); err != nil {
		h.sendFlightFileError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// flightFileCaller resolves the authenticated user and confirms the service
// is wired, writing the error response itself otherwise.
func (h *APIHandler) flightFileCaller(c *gin.Context) (uuid.UUID, bool) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return uuid.Nil, false
	}
	if h.flightFileService == nil {
		h.sendError(c, http.StatusServiceUnavailable, "Flight files are not available")
		return uuid.Nil, false
	}
	return userID, true
}

// readIgcUpload reads the "file" part, at most models.MaxFlightFileBytes.
func (h *APIHandler) readIgcUpload(c *gin.Context) ([]byte, string, bool) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			h.sendFlightFileError(c, service.ErrFlightFileTooLarge)
			return nil, "", false
		}
		h.sendError(c, http.StatusBadRequest, "A file field is required")
		return nil, "", false
	}
	defer func() { _ = file.Close() }()
	if header.Size > models.MaxFlightFileBytes {
		h.sendFlightFileError(c, service.ErrFlightFileTooLarge)
		return nil, "", false
	}
	data, err := io.ReadAll(io.LimitReader(file, models.MaxFlightFileBytes+1))
	if err != nil {
		h.sendError(c, http.StatusBadRequest, "Could not read the uploaded file")
		return nil, "", false
	}
	if len(data) > models.MaxFlightFileBytes {
		h.sendFlightFileError(c, service.ErrFlightFileTooLarge)
		return nil, "", false
	}
	return data, header.Filename, true
}

// sendFlightFileError maps the service's sentinel errors onto status codes.
// A missing and a foreign flight share the 404.
func (h *APIHandler) sendFlightFileError(c *gin.Context, err error) {
	var dup *service.IGCDuplicateError
	switch {
	case errors.Is(err, service.ErrFlightNotFound), errors.Is(err, service.ErrUnauthorizedFlight),
		errors.Is(err, service.ErrFlightFileNotFound):
		h.sendError(c, http.StatusNotFound, "Not found")
	case errors.Is(err, service.ErrFlightFileTooLarge):
		h.sendError(c, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("The file exceeds the maximum size of %d MB", models.MaxFlightFileBytes/(1024*1024)))
	case errors.As(err, &dup):
		h.sendError(c, http.StatusConflict, dup.Error())
	case errors.Is(err, service.ErrFlightFileDuplicate):
		h.sendError(c, http.StatusConflict, "This IGC file is already stored on this flight")
	case errors.Is(err, service.ErrFlightFileLimitReached):
		h.sendError(c, http.StatusConflict,
			fmt.Sprintf("This flight already has the maximum of %d files", models.MaxFlightFilesPerFlight))
	case errors.Is(err, service.ErrInvalidIGC), errors.Is(err, service.ErrFlightFileEmpty),
		errors.Is(err, service.ErrIGCNoFlight), errors.Is(err, service.ErrIGCNoRegistration):
		h.sendError(c, http.StatusBadRequest, err.Error())
	default:
		slog.Error("flight file request failed", "error", err)
		h.sendError(c, http.StatusInternalServerError, "Failed to process flight file")
	}
}

func convertIgcPreview(p *service.IGCPreview) generated.IgcFlightPreview {
	out := generated.IgcFlightPreview{
		Date:                   openapi_types.Date{Time: p.Date},
		TakeoffTime:            p.TakeoffTime,
		LandingTime:            p.LandingTime,
		LandingDetected:        p.LandingDetected,
		DurationMinutes:        p.DurationMinutes,
		LaunchMethod:           generated.IgcFlightPreviewLaunchMethod(p.LaunchMethod),
		LaunchMethodConfidence: p.LaunchConfidence,
		ReleaseHeightM:         p.ReleaseHeightM,
		MaxAltitudeM:           p.MaxAltitudeM,
		FreeDistanceKm:         p.FreeDistanceKm,
		OutAndReturnDistanceKm: p.OutAndReturnKm,
		Outlanding:             p.Outlanding,
		Departure:              convertIgcPlace(p.Departure),
		Arrival:                convertIgcPlace(p.Arrival),
		GliderRegistration:     p.GliderRegistration,
		GliderType:             p.GliderType,
		Pilot:                  p.Pilot,
	}
	if p.MatchingFlightID != nil {
		id := openapi_types.UUID(*p.MatchingFlightID)
		out.MatchingFlightId = &id
	}
	return out
}

func convertIgcPlace(p service.IGCPlace) generated.IgcPlace {
	return generated.IgcPlace{Icao: p.ICAO, Name: p.Name, Lat: p.Lat, Lon: p.Lon}
}

func convertFlightFile(f *models.FlightFile) generated.FlightFile {
	return generated.FlightFile{
		Id:        openapi_types.UUID(f.ID),
		FlightId:  openapi_types.UUID(f.FlightID),
		Kind:      generated.FlightFileKind(f.Kind),
		Filename:  f.Filename,
		SizeBytes: f.SizeBytes,
		Sha256:    f.SHA256,
		CreatedAt: f.CreatedAt,
	}
}
