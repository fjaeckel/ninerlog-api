package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/fjaeckel/ninerlog-api/internal/service/customreport"
	"github.com/fjaeckel/ninerlog-api/pkg/registration"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// importJSONBackup is the wire shape produced by GET /exports/json. It mirrors
// the in-memory map written by ExportDataJSON: a top-level metadata header
// plus per-entity arrays. License entries embed their class ratings so the
// importer can wire them up to the freshly-minted license IDs.
type importJSONBackup struct {
	Format                  string                               `json:"format"`
	Version                 string                               `json:"version"`
	ExportedAt              string                               `json:"exportedAt"`
	Flights                 []models.Flight                      `json:"flights"`
	Aircraft                []models.Aircraft                    `json:"aircraft"`
	Licenses                []importLicenseBundle                `json:"licenses"`
	Credentials             []models.Credential                  `json:"credentials"`
	Contacts                []models.Contact                     `json:"contacts"`
	CustomCurrencyRules     []cloudbackup.CustomCurrencyRule     `json:"customCurrencyRules"`
	CustomReports           []cloudbackup.CustomReport           `json:"customReports"`
	AircraftReminders       []models.AircraftReminder            `json:"aircraftReminders"`
	NotificationPreferences *cloudbackup.NotificationPreferences `json:"notificationPreferences"`
	FlightBaseline          *cloudbackup.FlightBaseline          `json:"flightBaseline"`
	PilotProfile            *cloudbackup.PilotProfile            `json:"pilotProfile"`
}

type importLicenseBundle struct {
	License      models.License            `json:"license"`
	ClassRatings []models.ClassRating      `json:"classRatings"`
	Privileges   []models.LicencePrivilege `json:"privileges"`
}

// Caps on a single restore. A logbook far beyond these sizes is not a realistic
// backup, and the request/statement timeouts would abort such a restore partway
// regardless -- better to refuse it up front than to half-apply it.
const (
	maxRestoreFlights  = 20000
	maxRestoreEntities = 2000
)

type importJSONSummary struct {
	AircraftImported     int `json:"aircraftImported"`
	AircraftSkipped      int `json:"aircraftSkipped"`
	LicensesImported     int `json:"licensesImported"`
	ClassRatingsImported int `json:"classRatingsImported"`
	CredentialsImported  int `json:"credentialsImported"`
	FlightsImported      int `json:"flightsImported"`
	CrewMembersImported  int `json:"crewMembersImported"`
	// ContactsImported counts address-book entries restored from the backup's
	// own contacts section; ContactsSkipped counts those whose name the
	// destination account already holds.
	ContactsImported int `json:"contactsImported"`
	ContactsSkipped  int `json:"contactsSkipped"`
	// ContactsCreated counts address-book entries created for a crew name in
	// the backup matching none of the destination account's contacts.
	ContactsCreated int `json:"contactsCreated"`
	// CustomCurrencyRulesImported counts user-authored currency rules
	// restored; sharing state is not carried in the backup.
	CustomCurrencyRulesImported int `json:"customCurrencyRulesImported"`
	// CustomReportsImported counts saved custom reports restored.
	CustomReportsImported int `json:"customReportsImported"`
	// AircraftRemindersImported counts aircraft reminders restored;
	// AircraftRemindersSkipped those whose aircraft is not in the backup or
	// that duplicate a reminder already on the aircraft.
	AircraftRemindersImported int `json:"aircraftRemindersImported"`
	AircraftRemindersSkipped  int `json:"aircraftRemindersSkipped"`
	// LicencePrivilegesImported counts licence privileges restored.
	LicencePrivilegesImported int `json:"licencePrivilegesImported"`
	// NotificationPreferencesImported and FlightBaselineImported report
	// whether those single-row settings were present and restored.
	NotificationPreferencesImported bool `json:"notificationPreferencesImported"`
	FlightBaselineImported          bool `json:"flightBaselineImported"`
	// PilotProfileImported reports whether a pilot profile was present and restored.
	PilotProfileImported bool `json:"pilotProfileImported"`
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

	// Bound the restore; the loops below are not transactional.
	if n := len(body.Flights); n > maxRestoreFlights {
		h.sendError(c, http.StatusBadRequest,
			fmt.Sprintf("Backup contains too many flights (%d, max %d)", n, maxRestoreFlights))
		return
	}
	if n := len(body.Aircraft); n > maxRestoreEntities {
		h.sendError(c, http.StatusBadRequest,
			fmt.Sprintf("Backup contains too many aircraft (%d, max %d)", n, maxRestoreEntities))
		return
	}
	if n := len(body.Licenses); n > maxRestoreEntities {
		h.sendError(c, http.StatusBadRequest,
			fmt.Sprintf("Backup contains too many licenses (%d, max %d)", n, maxRestoreEntities))
		return
	}
	privilegeCount := 0
	for _, bundle := range body.Licenses {
		privilegeCount += len(bundle.Privileges)
	}
	if privilegeCount > maxRestoreEntities {
		h.sendError(c, http.StatusBadRequest,
			fmt.Sprintf("Backup contains too many licence privileges (%d, max %d)", privilegeCount, maxRestoreEntities))
		return
	}
	if n := len(body.Credentials); n > maxRestoreEntities {
		h.sendError(c, http.StatusBadRequest,
			fmt.Sprintf("Backup contains too many credentials (%d, max %d)", n, maxRestoreEntities))
		return
	}
	if n := len(body.Contacts); n > maxRestoreEntities {
		h.sendError(c, http.StatusBadRequest,
			fmt.Sprintf("Backup contains too many contacts (%d, max %d)", n, maxRestoreEntities))
		return
	}
	if n := len(body.CustomCurrencyRules); n > maxRestoreEntities {
		h.sendError(c, http.StatusBadRequest,
			fmt.Sprintf("Backup contains too many custom currency rules (%d, max %d)", n, maxRestoreEntities))
		return
	}
	if n := len(body.AircraftReminders); n > maxRestoreEntities {
		h.sendError(c, http.StatusBadRequest,
			fmt.Sprintf("Backup contains too many aircraft reminders (%d, max %d)", n, maxRestoreEntities))
		return
	}
	if n := len(body.CustomReports); n > maxRestoreEntities {
		h.sendError(c, http.StatusBadRequest,
			fmt.Sprintf("Backup contains too many custom reports (%d, max %d)", n, maxRestoreEntities))
		return
	}

	ctx := c.Request.Context()
	summary := importJSONSummary{}
	// One linker for the whole restore so a crew name is looked up once, not
	// once per flight it appears on.
	crewLinker := h.contactService.NewCrewLinker(userID)

	// --- Contacts ---
	// Restored before flights so the crew linker below matches a crew name
	// against the contact this backup carries rather than creating a bare one.
	for _, contact := range body.Contacts {
		newC := contact
		newC.ID = uuid.New()
		newC.UserID = userID
		newC.CreatedAt = time.Time{}
		newC.UpdatedAt = time.Time{}
		if err := h.contactService.CreateContact(ctx, &newC); err != nil {
			if errors.Is(err, service.ErrContactNameExists) {
				summary.ContactsSkipped++
				continue
			}
			h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Failed to import contact %q: %v", contact.Name, err))
			return
		}
		summary.ContactsImported++
	}

	// --- Aircraft ---
	// Build registration → existing ID map so duplicates are skipped and
	// flights can still reference the (already-owned) aircraft by reg.
	existingAircraft, err := h.aircraftService.ListAircraft(ctx, userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to load existing aircraft")
		return
	}
	// Keyed in canonical notation.
	existingRegs := make(map[string]bool, len(existingAircraft))
	aircraftIDByReg := make(map[string]uuid.UUID, len(existingAircraft)+len(body.Aircraft))
	for _, a := range existingAircraft {
		existingRegs[registration.Canonical(a.Registration)] = true
		aircraftIDByReg[registration.Canonical(a.Registration)] = a.ID
	}
	backupAircraftReg := make(map[uuid.UUID]string, len(body.Aircraft))
	for _, ac := range body.Aircraft {
		backupAircraftReg[ac.ID] = registration.Canonical(ac.Registration)
	}
	for _, ac := range body.Aircraft {
		if existingRegs[registration.Canonical(ac.Registration)] {
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
		aircraftIDByReg[newAC.Registration] = newAC.ID
		summary.AircraftImported++
	}

	// --- Aircraft reminders ---
	if h.aircraftReminderService != nil {
		imported, skipped, err := h.restoreAircraftReminders(ctx, userID, body.AircraftReminders, backupAircraftReg, aircraftIDByReg)
		if err != nil {
			h.sendError(c, http.StatusBadRequest, "Failed to "+err.Error())
			return
		}
		summary.AircraftRemindersImported = imported
		summary.AircraftRemindersSkipped = skipped
	}

	// --- Licenses + class ratings ---
	// Class ratings reference licenses by ID; we remap old → new on the fly.
	licenseIDs := make(map[uuid.UUID]uuid.UUID, len(body.Licenses))
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
		licenseIDs[bundle.License.ID] = lic.ID
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

		if h.licencePrivilegeService != nil {
			for _, p := range bundle.Privileges {
				if _, err := h.licencePrivilegeService.Create(ctx, userID, lic.ID, service.LicencePrivilegeInput{
					Kind: p.Kind, Detail: p.Detail, IssuedOn: p.IssuedOn, ExpiresOn: p.ExpiresOn, Notes: p.Notes,
				}); err != nil {
					if errors.Is(err, models.ErrInvalidLicencePrivilege) || errors.Is(err, models.ErrFieldTooLong) {
						h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Failed to import licence privilege %q: %v", p.Kind, err))
						return
					}
					h.sendError(c, http.StatusInternalServerError, "Failed to import licence privilege")
					return
				}
				summary.LicencePrivilegesImported++
			}
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
				// ContactID starts nil; crewLinker re-establishes the link by
				// name against the destination account's contacts.
				ContactID: nil,
			})
		}
		if err := crewLinker.Link(ctx, members); err != nil {
			slog.Warn("restore: failed to link crew members to contacts",
				"flightId", newF.ID, "error", err)
		}
		if err := h.flightCrewRepo.SetCrewMembers(ctx, newF.ID, members); err != nil {
			h.sendError(c, http.StatusInternalServerError, fmt.Sprintf("Failed to import crew for flight on %s: %v", f.Date.Format("2006-01-02"), err))
			return
		}
		summary.CrewMembersImported += len(members)
	}

	summary.ContactsCreated = crewLinker.Created()

	// --- Custom currency rules ---
	// Recreated through the service so each definition is revalidated and the
	// per-account quota applies. Sharing is never carried over.
	for _, rule := range body.CustomCurrencyRules {
		created, err := h.customCurrencyService.Create(ctx, userID, currency.CustomRuleInput{
			Name:        rule.Name,
			Description: rule.Description,
			Emoji:       rule.Emoji,
			Definition:  rule.Definition,
		})
		if err != nil {
			h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Failed to import custom currency rule %q: %v", rule.Name, err))
			return
		}
		if !rule.Enabled {
			if _, err := h.customCurrencyService.SetEnabled(ctx, userID, created.Rule.ID, false); err != nil {
				h.sendError(c, http.StatusInternalServerError, fmt.Sprintf("Failed to restore paused state for rule %q: %v", rule.Name, err))
				return
			}
		}
		if rule.Notify {
			if _, err := h.customCurrencyService.SetNotify(ctx, userID, created.Rule.ID, true); err != nil {
				h.sendError(c, http.StatusInternalServerError, fmt.Sprintf("Failed to restore notification opt-in for rule %q: %v", rule.Name, err))
				return
			}
		}
		summary.CustomCurrencyRulesImported++
	}

	// --- Custom reports ---
	// Recreated through the service, in backup order; licence scopes follow
	// the restored licence or are dropped.
	for _, rep := range body.CustomReports {
		def := rep.Definition
		if id := def.Filter.LogbookLicenseID; id != nil {
			if newID, ok := licenseIDs[*id]; ok {
				def.Filter.LogbookLicenseID = &newID
			} else {
				def.Filter.LogbookLicenseID = nil
			}
		}
		if _, err := h.customReportService.Create(ctx, userID, customreport.Input{Name: rep.Name, Definition: def}); err != nil {
			h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Failed to import custom report %q: %v", rep.Name, err))
			return
		}
		summary.CustomReportsImported++
	}

	// --- Notification preferences ---
	if p := body.NotificationPreferences; p != nil {
		prefs := &models.NotificationPreferences{
			UserID:            userID,
			EmailEnabled:      p.EmailEnabled,
			EnabledCategories: pq.StringArray(p.EnabledCategories),
			WarningDays:       pq.Int64Array(p.WarningDays),
			CheckHour:         p.CheckHour,
		}
		if err := h.notificationService.UpdatePreferences(ctx, prefs); err != nil {
			h.sendError(c, http.StatusInternalServerError, "Failed to import notification preferences")
			return
		}
		summary.NotificationPreferencesImported = true
	}

	// --- Flight baseline ---
	if b := body.FlightBaseline; b != nil {
		if err := h.flightService.UpsertBaseline(ctx, b.ToModel(userID)); err != nil {
			h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Failed to import flight baseline: %v", err))
			return
		}
		summary.FlightBaselineImported = true
	}

	// --- Pilot profile ---
	if p := body.PilotProfile; p != nil && h.pilotProfileService != nil {
		if err := h.pilotProfileService.Replace(ctx, userID, p.ToModel(userID)); err != nil {
			if errors.Is(err, models.ErrInvalidPilotProfile) {
				h.sendError(c, http.StatusBadRequest, "Failed to import pilot profile: unknown mode or intent")
				return
			}
			h.sendError(c, http.StatusInternalServerError, "Failed to import pilot profile")
			return
		}
		summary.PilotProfileImported = true
	}

	c.JSON(http.StatusOK, summary)
}

// restoreAircraftReminders recreates backup reminders on the restored or
// existing aircraft with the backup aircraft's registration. Reminders whose
// aircraft cannot be resolved, or identical in kind, label and due date to one
// already on the aircraft, are skipped.
func (h *APIHandler) restoreAircraftReminders(
	ctx context.Context, userID uuid.UUID, reminders []models.AircraftReminder,
	backupAircraftReg map[uuid.UUID]string, aircraftIDByReg map[string]uuid.UUID,
) (imported, skipped int, err error) {
	existing := map[uuid.UUID]map[string]bool{}
	reminderKey := func(r *models.AircraftReminder) string {
		label := ""
		if r.Label != nil {
			label = strings.TrimSpace(*r.Label)
		}
		return string(r.Kind) + "|" + label + "|" + models.DateOnly(r.DueDate).Format("2006-01-02")
	}
	for i := range reminders {
		rem := reminders[i]
		reg, ok := backupAircraftReg[rem.AircraftID]
		if !ok {
			reg = registration.Canonical(rem.AircraftRegistration)
		}
		aircraftID, ok := aircraftIDByReg[reg]
		if !ok {
			skipped++
			continue
		}
		keys, loaded := existing[aircraftID]
		if !loaded {
			current, lerr := h.aircraftReminderService.List(ctx, userID, aircraftID)
			if lerr != nil {
				return imported, skipped, fmt.Errorf("load aircraft reminders for %q", reg)
			}
			keys = make(map[string]bool, len(current))
			for _, c := range current {
				keys[reminderKey(c)] = true
			}
			existing[aircraftID] = keys
		}
		if keys[reminderKey(&rem)] {
			skipped++
			continue
		}
		created, cerr := h.aircraftReminderService.Create(ctx, userID, aircraftID, service.AircraftReminderInput{
			Kind:           rem.Kind,
			Label:          rem.Label,
			DueDate:        rem.DueDate,
			IntervalMonths: rem.IntervalMonths,
			LastDoneOn:     rem.LastDoneOn,
			Notes:          rem.Notes,
		})
		if cerr != nil {
			return imported, skipped, fmt.Errorf("import aircraft reminder %q on %q: %w", rem.Kind, reg, cerr)
		}
		keys[reminderKey(created)] = true
		imported++
	}
	return imported, skipped, nil
}
