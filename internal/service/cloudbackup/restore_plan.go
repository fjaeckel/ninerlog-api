package cloudbackup

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/fjaeckel/ninerlog-api/internal/service/customreport"
	"github.com/fjaeckel/ninerlog-api/pkg/registration"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// maxCrewNameLen matches the flight_crew_members.name column.
const maxCrewNameLen = 255

type plannedLicense struct {
	oldID      uuid.UUID
	license    *models.License
	ratings    []*models.ClassRating
	privileges []*models.LicencePrivilege
}

// restorePlan is a payload turned into rows ready to insert for one user.
type restorePlan struct {
	userID uuid.UUID

	aircraft        []*models.Aircraft
	aircraftSkipped int
	licenses        []plannedLicense
	credentials     []*models.Credential

	contacts         []*models.Contact
	contactsImported int
	contactsSkipped  int
	contactsCreated  int

	flights []*models.Flight
	// flightBackupIDs holds each planned flight's id in the backup, in the
	// order of flights; uuid.Nil when the backup gave none.
	flightBackupIDs []uuid.UUID
	crew            []models.FlightCrewMember

	rules    []*models.CustomCurrencyRule
	reports  []*models.CustomReport
	prefs    *models.NotificationPreferences
	baseline *models.FlightBaseline
	profile  *models.PilotProfile

	logbook *service.PreparedLogbook

	warnings warningList
}

// warningList collects warnings up to a cap.
type warningList struct {
	items   []RestoreWarning
	dropped int
}

func (w *warningList) add(section string, index int, format string, args ...any) {
	if len(w.items) >= maxRestoreWarnings {
		w.dropped++
		return
	}
	w.items = append(w.items, RestoreWarning{Section: section, Index: index, Message: fmt.Sprintf(format, args...)})
}

func (w *warningList) list() []RestoreWarning {
	out := append([]RestoreWarning{}, w.items...)
	if w.dropped > 0 {
		out = append(out, RestoreWarning{
			Section: "warnings",
			Index:   len(w.items),
			Message: fmt.Sprintf("%d further warnings omitted", w.dropped),
		})
	}
	return out
}

// buildRestorePlan converts the payload into insertable rows without touching
// storage. Identifiers are minted here; timestamps are left to the database.
func buildRestorePlan(userID uuid.UUID, p *Payload) (*restorePlan, error) {
	pl := &restorePlan{userID: userID}

	if err := pl.planAircraft(p); err != nil {
		return nil, err
	}
	if err := pl.planLicenses(p); err != nil {
		return nil, err
	}
	if err := pl.planCredentials(p); err != nil {
		return nil, err
	}
	contactIDs, err := pl.planContacts(p)
	if err != nil {
		return nil, err
	}
	if err := pl.planFlights(p, contactIDs); err != nil {
		return nil, err
	}
	if err := pl.planRules(p); err != nil {
		return nil, err
	}
	if err := pl.planReports(p); err != nil {
		return nil, err
	}
	pl.planSettings(p)
	if err := pl.planPilotProfile(p); err != nil {
		return nil, err
	}
	if err := pl.planLogbook(p); err != nil {
		return nil, err
	}
	return pl, nil
}

func (pl *restorePlan) planAircraft(p *Payload) error {
	seen := make(map[string]bool, len(p.Aircraft))
	for i, src := range p.Aircraft {
		if src == nil {
			return invalidEntry("aircraft", i, nil)
		}
		a := *src
		a.ID = uuid.Nil
		a.UserID = pl.userID
		a.Registration = registration.Canonical(a.Registration)
		if seen[a.Registration] {
			pl.aircraftSkipped++
			continue
		}
		if err := models.ValidateAircraftTextFields(&a); err != nil {
			return invalidEntry("aircraft", i, err)
		}
		if err := a.NormalizeULKind(); err != nil {
			return invalidEntry("aircraft", i, err)
		}
		if err := a.Validate(); err != nil {
			pl.warnings.add("aircraft", i, "restored although it no longer passes validation: %s", err)
		}
		seen[a.Registration] = true
		pl.aircraft = append(pl.aircraft, &a)
	}
	return nil
}

func (pl *restorePlan) planLicenses(p *Payload) error {
	for i, bundle := range p.Licenses {
		if bundle.License == nil {
			return invalidEntry("licenses", i, nil)
		}
		lic := *bundle.License
		oldID := lic.ID
		lic.ID = uuid.Nil
		lic.UserID = pl.userID
		if err := models.ValidateLicenseTextFields(&lic); err != nil {
			return invalidEntry("licenses", i, err)
		}
		country, err := service.NormalizeIssuingCountry(lic.IssuingCountry)
		if err != nil {
			return invalidEntry("licenses", i, err)
		}
		lic.IssuingCountry = country
		if err := lic.Validate(); err != nil {
			pl.warnings.add("licenses", i, "restored although it no longer passes validation: %s", err)
		}
		planned := plannedLicense{oldID: oldID, license: &lic}
		for j, src := range bundle.ClassRatings {
			if src == nil {
				return invalidEntry("licenses", i, fmt.Errorf("class rating %d is empty", j))
			}
			cr := *src
			cr.ID = uuid.Nil
			if !models.IsValidClassType(cr.ClassType) {
				return invalidEntry("licenses", i, fmt.Errorf("class rating %d: unknown class type", j))
			}
			if err := cr.NormalizeULKind(); err != nil {
				return invalidEntry("licenses", i, fmt.Errorf("class rating %d: %w", j, err))
			}
			planned.ratings = append(planned.ratings, &cr)
		}
		for j, src := range bundle.Privileges {
			if src == nil {
				return invalidEntry("licenses", i, fmt.Errorf("privilege %d is empty", j))
			}
			lp := *src
			lp.ID = uuid.Nil
			lp.UserID = pl.userID
			if err := lp.Validate(); err != nil {
				return invalidEntry("licenses", i, fmt.Errorf("privilege %d: %w", j, err))
			}
			planned.privileges = append(planned.privileges, &lp)
		}
		pl.licenses = append(pl.licenses, planned)
	}
	return nil
}

func (pl *restorePlan) planCredentials(p *Payload) error {
	for i, src := range p.Credentials {
		if src == nil {
			return invalidEntry("credentials", i, nil)
		}
		c := *src
		c.ID = uuid.Nil
		c.UserID = pl.userID
		if err := models.ValidateCredentialTextFields(&c); err != nil {
			return invalidEntry("credentials", i, err)
		}
		if c.ExpiryDate != nil && c.ExpiryDate.Before(c.IssueDate) {
			pl.warnings.add("credentials", i, "restored although it expires before it was issued")
		}
		pl.credentials = append(pl.credentials, &c)
	}
	return nil
}

func contactKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// planContacts plans the payload's own contacts and returns their identifiers
// by lower-cased name.
func (pl *restorePlan) planContacts(p *Payload) (map[string]uuid.UUID, error) {
	ids := make(map[string]uuid.UUID, len(p.Contacts))
	for i, src := range p.Contacts {
		if src == nil {
			return nil, invalidEntry("contacts", i, nil)
		}
		c := *src
		c.Name = strings.TrimSpace(c.Name)
		if c.Name == "" {
			return nil, invalidEntry("contacts", i, nil)
		}
		key := contactKey(c.Name)
		if _, dup := ids[key]; dup {
			pl.contactsSkipped++
			continue
		}
		if err := models.ValidateContactTextFields(&c); err != nil {
			return nil, invalidEntry("contacts", i, err)
		}
		c.ID = uuid.New()
		c.UserID = pl.userID
		ids[key] = c.ID
		pl.contacts = append(pl.contacts, &c)
		pl.contactsImported++
	}
	return ids, nil
}

// planFlights plans flights and their crew. A crew name matching a contact
// links to it; any other valid name gets a contact of its own.
func (pl *restorePlan) planFlights(p *Payload, contactIDs map[string]uuid.UUID) error {
	now := time.Now()
	pl.flights = make([]*models.Flight, 0, len(p.Flights))
	pl.flightBackupIDs = make([]uuid.UUID, 0, len(p.Flights))
	for i, src := range p.Flights {
		if src == nil {
			return invalidEntry("flights", i, nil)
		}
		f := *src
		f.ID = uuid.New()
		f.CreatedAt = restoredCreatedAt(src.CreatedAt, src.Date, now)
		pl.flightBackupIDs = append(pl.flightBackupIDs, src.ID)
		f.UserID = pl.userID
		f.SignatureID = nil
		f.AircraftReg = registration.Canonical(f.AircraftReg)
		models.NormalizeLaunchMethod(&f)
		crew := f.CrewMembers
		f.CrewMembers = nil

		if err := models.ValidateFlightTextFields(&f); err != nil {
			return invalidEntry("flights", i, err)
		}
		if !f.IsValid() {
			pl.warnings.add("flights", i, "restored although it no longer passes validation: required fields are missing")
		} else if err := f.ValidateTimeDistribution(); err != nil {
			pl.warnings.add("flights", i, "restored although it no longer passes validation: %s", err)
		}

		for j, m := range crew {
			member, err := pl.planCrewMember(m, f.ID, contactIDs)
			if err != nil {
				return invalidEntry("flights", i, fmt.Errorf("crew member %d: %w", j, err))
			}
			pl.crew = append(pl.crew, member)
		}
		pl.flights = append(pl.flights, &f)
	}
	return nil
}

func (pl *restorePlan) planCrewMember(m models.FlightCrewMember, flightID uuid.UUID, contactIDs map[string]uuid.UUID) (models.FlightCrewMember, error) {
	name := strings.TrimSpace(m.Name)
	if len(name) > maxCrewNameLen {
		return models.FlightCrewMember{}, fmt.Errorf("name exceeds %d characters", maxCrewNameLen)
	}
	if !models.IsValidCrewRole(m.Role) {
		return models.FlightCrewMember{}, fmt.Errorf("unknown role %q", m.Role)
	}
	member := models.FlightCrewMember{ID: uuid.New(), FlightID: flightID, Name: name, Role: m.Role}
	if name == "" {
		return member, nil
	}
	key := contactKey(name)
	id, ok := contactIDs[key]
	if !ok {
		c := &models.Contact{Name: name}
		if models.ValidateContactTextFields(c) != nil {
			return member, nil //nolint:nilerr // crew member kept without a contact link
		}
		c.ID = uuid.New()
		c.UserID = pl.userID
		contactIDs[key] = c.ID
		pl.contacts = append(pl.contacts, c)
		pl.contactsCreated++
		id = c.ID
	}
	member.ContactID = &id
	return member, nil
}

func (pl *restorePlan) planRules(p *Payload) error {
	for i, src := range p.CustomCurrencyRules {
		in := currency.CustomRuleInput{
			Name:        src.Name,
			Description: src.Description,
			Emoji:       src.Emoji,
			Definition:  src.Definition,
		}
		if err := currency.CheckRuleStorable(in); err != nil {
			return invalidEntry("customCurrencyRules", i, err)
		}
		rule := &models.CustomCurrencyRule{
			UserID:      pl.userID,
			Name:        strings.TrimSpace(src.Name),
			Description: src.Description,
			Emoji:       src.Emoji,
			Definition:  src.Definition,
			Enabled:     src.Enabled,
			Notify:      src.Notify,
		}
		if err := currency.ValidateRuleInput(&in); err != nil {
			rule.Enabled = false
			rule.Notify = false
			pl.warnings.add("customCurrencyRules", i, "restored paused because it no longer passes validation: %s", err)
		} else {
			rule.Name = in.Name
			rule.Definition = in.Definition
		}
		pl.rules = append(pl.rules, rule)
	}
	return nil
}

func (pl *restorePlan) planReports(p *Payload) error {
	for i, src := range p.CustomReports {
		in := customreport.Input{Name: src.Name, Definition: src.Definition}
		if err := customreport.CheckStorable(in); err != nil {
			return invalidEntry("customReports", i, err)
		}
		report := &models.CustomReport{
			UserID:     pl.userID,
			Name:       strings.TrimSpace(src.Name),
			Definition: src.Definition,
		}
		if err := customreport.ValidateInput(&in); err != nil {
			pl.warnings.add("customReports", i, "restored although it no longer passes validation: %s", err)
		} else {
			report.Name = in.Name
			report.Definition = in.Definition
		}
		pl.reports = append(pl.reports, report)
	}
	return nil
}

func (pl *restorePlan) planSettings(p *Payload) {
	if prefs := p.NotificationPreferences; prefs != nil {
		pl.prefs = &models.NotificationPreferences{
			UserID:            pl.userID,
			EmailEnabled:      prefs.EmailEnabled,
			EnabledCategories: pq.StringArray(prefs.EnabledCategories),
			WarningDays:       pq.Int64Array(prefs.WarningDays),
			CheckHour:         prefs.CheckHour,
		}
	}
	if b := p.FlightBaseline; b != nil {
		pl.baseline = b.ToModel(pl.userID)
		if err := pl.baseline.Validate(); err != nil {
			pl.warnings.add("flightBaseline", 0, "restored although it no longer passes validation: %s", err)
		}
	}
}

// planPilotProfile plans the pilot profile, rejecting an unknown mode or
// intent.
func (pl *restorePlan) planPilotProfile(p *Payload) error {
	if p.PilotProfile == nil {
		return nil
	}
	profile := p.PilotProfile.ToModel(pl.userID)
	if profile.Mode == "" {
		profile.Mode = models.ModeAdaptive
	}
	if err := profile.Validate(); err != nil {
		return invalidEntry("pilotProfile", 0, err)
	}
	profile.Compact()
	pl.profile = profile
	return nil
}

// planLogbook plans strict logbook mode, its switch events, the flight
// revisions and the certifications. Licence ids are mapped when applied.
func (pl *restorePlan) planLogbook(p *Payload) error {
	var in service.LogbookImport
	if m := p.LogbookMode; m != nil {
		in.Strict = m.Strict
		in.Since = m.Since
		for _, e := range m.Events {
			in.Events = append(in.Events, models.LogbookModeEvent{
				Strict: e.Strict, Trigger: e.Trigger, LicenseID: e.LicenseID, CreatedAt: e.CreatedAt,
			})
		}
	}
	for _, r := range p.FlightRevisions {
		in.Revisions = append(in.Revisions, models.FlightRevision{
			FlightID: r.FlightID, RevisionNo: r.RevisionNo, Kind: r.Kind,
			Snapshot: r.Snapshot, CrewSnapshot: r.CrewSnapshot, Source: r.Source,
			Reason: r.Reason, IsCorrection: r.IsCorrection, ChangedAt: r.ChangedAt,
		})
	}
	for i, c := range p.LogbookCertifications {
		through, err := time.Parse("2006-01-02", c.CertifiedThrough)
		if err != nil {
			return invalidEntry("logbookCertifications", i, err)
		}
		in.Certifications = append(in.Certifications, models.LogbookCertification{
			CertifiedThrough: through, CertifiedAt: c.CertifiedAt,
		})
	}
	prepared, err := in.Prepare()
	if err != nil {
		var entry *service.LogbookImportError
		if errors.As(err, &entry) {
			return invalidEntry(entry.Section, entry.Index, err)
		}
		return &InvalidBackupError{Message: "Invalid logbook history in backup", Detail: err.Error()}
	}
	pl.logbook = prepared
	return nil
}

// restoredCreatedAt returns the creation time a restored flight keeps: the
// backup's value, or the flight date at 00:00 UTC when it has none, never
// later than now.
func restoredCreatedAt(backup, flightDate, now time.Time) time.Time {
	if backup.IsZero() && flightDate.IsZero() {
		return now
	}
	if backup.IsZero() {
		y, m, d := flightDate.Date()
		backup = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	if backup.After(now) {
		return now
	}
	return backup
}
