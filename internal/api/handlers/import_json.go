package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// importJSONBackup is the wire shape produced by GET /exports/json. It mirrors
// the in-memory map written by ExportDataJSON: a top-level metadata header
// plus per-entity arrays. License entries embed their class ratings so the
// importer can wire them up to the freshly-minted license IDs.
type importJSONBackup struct {
	Format      string                `json:"format"`
	Version     string                `json:"version"`
	ExportedAt  string                `json:"exportedAt"`
	Flights     []models.Flight       `json:"flights"`
	Aircraft    []models.Aircraft     `json:"aircraft"`
	Licenses    []importLicenseBundle `json:"licenses"`
	Credentials []models.Credential   `json:"credentials"`
}

type importLicenseBundle struct {
	License      models.License       `json:"license"`
	ClassRatings []models.ClassRating `json:"classRatings"`
}

type importJSONSummary struct {
	AircraftImported     int `json:"aircraftImported"`
	AircraftSkipped      int `json:"aircraftSkipped"`
	LicensesImported     int `json:"licensesImported"`
	ClassRatingsImported int `json:"classRatingsImported"`
	CredentialsImported  int `json:"credentialsImported"`
	FlightsImported      int `json:"flightsImported"`
	CrewMembersImported  int `json:"crewMembersImported"`
}

// ImportDataJSON implements POST /imports/json. It restores a NinerLog JSON
// backup (as produced by GET /exports/json) into the authenticated user's
// account. All entity IDs are regenerated so a backup can be restored into
// any installation, including the one it was exported from. The operation is
// additive: existing user data is never touched, and aircraft whose
// registration already exists for the user are skipped (their existing IDs
// are referenced by imported flights via aircraftReg, no remapping needed).
func (h *APIHandler) ImportDataJSON(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var body importJSONBackup
	// Allow unknown fields so older API versions can restore backups produced
	// by newer ones (forward-compat for additive schema changes).
	if err := json.NewDecoder(c.Request.Body).Decode(&body); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}

	if body.Format != "NinerLog JSON Backup" {
		h.sendError(c, http.StatusBadRequest, "Unsupported backup format (expected 'NinerLog JSON Backup')")
		return
	}

	ctx := c.Request.Context()
	summary := importJSONSummary{}

	// --- Aircraft ---
	// Build registration → existing ID map so duplicates are skipped and
	// flights can still reference the (already-owned) aircraft by reg.
	existingAircraft, err := h.aircraftService.ListAircraft(ctx, userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to load existing aircraft")
		return
	}
	existingRegs := make(map[string]bool, len(existingAircraft))
	for _, a := range existingAircraft {
		existingRegs[a.Registration] = true
	}
	for _, ac := range body.Aircraft {
		if existingRegs[ac.Registration] {
			summary.AircraftSkipped++
			continue
		}
		newAC := ac
		newAC.ID = uuid.New()
		newAC.UserID = userID
		newAC.CreatedAt = time.Time{}
		newAC.UpdatedAt = time.Time{}
		if err := h.aircraftService.CreateAircraft(ctx, &newAC); err != nil {
			h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Failed to import aircraft %q: %v", ac.Registration, err))
			return
		}
		existingRegs[newAC.Registration] = true
		summary.AircraftImported++
	}

	// --- Licenses + class ratings ---
	// Class ratings reference licenses by ID; we remap old → new on the fly.
	for _, bundle := range body.Licenses {
		lic := bundle.License
		lic.ID = uuid.New()
		lic.UserID = userID
		lic.CreatedAt = time.Time{}
		lic.UpdatedAt = time.Time{}
		if err := h.licenseService.CreateLicense(ctx, &lic); err != nil {
			h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Failed to import license %q: %v", bundle.License.LicenseNumber, err))
			return
		}
		summary.LicensesImported++

		for _, cr := range bundle.ClassRatings {
			newCR := cr
			newCR.ID = uuid.New()
			newCR.LicenseID = lic.ID
			newCR.CreatedAt = time.Time{}
			newCR.UpdatedAt = time.Time{}
			if err := h.classRatingService.CreateClassRating(ctx, &newCR, userID); err != nil {
				h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Failed to import class rating %q: %v", cr.ClassType, err))
				return
			}
			summary.ClassRatingsImported++
		}
	}

	// --- Credentials ---
	for _, cred := range body.Credentials {
		newC := cred
		newC.ID = uuid.New()
		newC.UserID = userID
		newC.CreatedAt = time.Time{}
		newC.UpdatedAt = time.Time{}
		if err := h.credentialService.CreateCredential(ctx, &newC); err != nil {
			h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Failed to import credential %q: %v", cred.CredentialType, err))
			return
		}
		summary.CredentialsImported++
	}

	// --- Flights (+ crew members) ---
	for _, f := range body.Flights {
		newF := f
		newF.ID = uuid.New()
		newF.UserID = userID
		newF.CreatedAt = time.Time{}
		newF.UpdatedAt = time.Time{}
		// Crew members are persisted separately via flightCrewRepo; the
		// FlightService.CreateFlight path doesn't write the join table.
		crew := f.CrewMembers
		newF.CrewMembers = nil

		if err := h.flightService.CreateFlight(ctx, &newF); err != nil {
			h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Failed to import flight on %s (%s): %v", f.Date.Format("2006-01-02"), f.AircraftReg, err))
			return
		}
		summary.FlightsImported++

		if len(crew) == 0 || h.flightCrewRepo == nil {
			continue
		}
		members := make([]models.FlightCrewMember, 0, len(crew))
		for _, m := range crew {
			members = append(members, models.FlightCrewMember{
				// ID + FlightID are assigned by SetCrewMembers.
				Name: m.Name,
				Role: m.Role,
				// ContactID intentionally nil: contacts are NOT part of the
				// backup format, so we cannot re-link to a contact row in
				// the destination installation. The crew member's name is
				// still preserved, which is what exports/PIC-resolution use.
				ContactID: nil,
			})
		}
		if err := h.flightCrewRepo.SetCrewMembers(ctx, newF.ID, members); err != nil {
			h.sendError(c, http.StatusInternalServerError, fmt.Sprintf("Failed to import crew for flight on %s: %v", f.Date.Format("2006-01-02"), err))
			return
		}
		summary.CrewMembersImported += len(members)
	}

	c.JSON(http.StatusOK, summary)
}
