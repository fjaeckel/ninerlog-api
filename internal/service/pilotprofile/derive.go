// Package pilotprofile derives which flying disciplines are relevant to a pilot and stores
// the pilot's intent for each.
package pilotprofile

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// RecencyWindowMonths is the look-back window for recent flight evidence.
const RecencyWindowMonths = 24

// Derive resolves every discipline, in models.AllDisciplines order, from the pilot's
// licences, class ratings, fleet, aggregated flights and stored settings at now.
func Derive(
	licences []*models.License,
	ratings []*models.ClassRating,
	fleet []*models.Aircraft,
	flights []models.DisciplineFlightGroup,
	settings *models.PilotProfile,
	now time.Time,
) []models.DisciplineState {
	d := newDerivation(now)
	d.addLicences(licences)
	d.addRatings(ratings, licences)
	d.addFleet(fleet)
	d.addFlights(flights)

	states := make([]models.DisciplineState, 0, len(models.AllDisciplines()))
	for _, disc := range models.AllDisciplines() {
		states = append(states, d.resolve(disc, settings.Setting(disc)))
	}
	return states
}

// PendingAcknowledgement returns the disciplines switched on by evidence alone (intent
// auto, status active or training) that the pilot has not acknowledged.
func PendingAcknowledgement(states []models.DisciplineState) []models.Discipline {
	pending := []models.Discipline{}
	for _, s := range states {
		if s.Intent != models.IntentAuto || s.AcknowledgedAt != nil {
			continue
		}
		if s.Status == models.StatusActive || s.Status == models.StatusTraining {
			pending = append(pending, s.Discipline)
		}
	}
	return pending
}

// aircraftDisciplines are the disciplines with a dual-only training signal.
var aircraftDisciplines = map[models.Discipline]bool{
	models.DisciplineAeroplane:  true,
	models.DisciplineTMG:        true,
	models.DisciplineSailplane:  true,
	models.DisciplineUltralight: true,
	models.DisciplineGyroplane:  true,
	models.DisciplineHelicopter: true,
}

// ulKindDisciplines maps an ultralight kind to the non-UL discipline its flights and
// aircraft are also evidence for.
var ulKindDisciplines = map[models.ULKind]models.Discipline{
	models.ULKindThreeAxis:            models.DisciplineAeroplane,
	models.ULKindThreeAxisMotorglider: models.DisciplineTMG,
	models.ULKindSailplane:            models.DisciplineSailplane,
	models.ULKindGyroplane:            models.DisciplineGyroplane,
	models.ULKindHelicopter:           models.DisciplineHelicopter,
}

// classDisciplines returns the disciplines an aircraft class (and UL kind) serves. With
// ulCredit false an ultralight serves ULTRALIGHT only.
func classDisciplines(class string, kind *models.ULKind, ulCredit bool) []models.Discipline {
	switch strings.ToUpper(strings.TrimSpace(class)) {
	case string(models.ClassTypeSEPLand), string(models.ClassTypeSEPSea),
		string(models.ClassTypeMEPLand), string(models.ClassTypeMEPSea),
		string(models.ClassTypeSETLand), string(models.ClassTypeSETSea):
		return []models.Discipline{models.DisciplineAeroplane}
	case string(models.ClassTypeTMG):
		return []models.Discipline{models.DisciplineTMG}
	case string(models.ClassTypeGlider):
		return []models.Discipline{models.DisciplineSailplane}
	case string(models.ClassTypeGyro):
		return []models.Discipline{models.DisciplineGyroplane}
	case string(models.ClassTypeUL):
		out := []models.Discipline{models.DisciplineUltralight}
		if kind != nil && ulCredit {
			if extra, ok := ulKindDisciplines[*kind]; ok {
				out = append(out, extra)
			}
		}
		return out
	}
	return nil
}

// ratingDiscipline returns the discipline a class rating is strong evidence for.
func ratingDiscipline(r *models.ClassRating) (models.Discipline, bool) {
	switch r.ClassType {
	case models.ClassTypeSEPLand, models.ClassTypeSEPSea, models.ClassTypeMEPLand,
		models.ClassTypeMEPSea, models.ClassTypeSETLand, models.ClassTypeSETSea:
		return models.DisciplineAeroplane, true
	case models.ClassTypeTMG:
		return models.DisciplineTMG, true
	case models.ClassTypeGlider:
		return models.DisciplineSailplane, true
	case models.ClassTypeUL:
		return models.DisciplineUltralight, true
	case models.ClassTypeGyro:
		return models.DisciplineGyroplane, true
	case models.ClassTypeIR:
		return models.DisciplineIFR, true
	case models.ClassTypeOther:
		if r.Notes != nil && models.ClassifyLicence(*r.Notes, "") == models.LicenceKindInstructor {
			return models.DisciplineInstructor, true
		}
	}
	return "", false
}

// licenceDisciplines returns the disciplines a licence kind is strong evidence for.
func licenceDisciplines(kind models.LicenceKind) []models.Discipline {
	var out []models.Discipline
	if kind.IsAeroplane() {
		out = append(out, models.DisciplineAeroplane)
	}
	if kind.IsSailplane() {
		out = append(out, models.DisciplineSailplane)
	}
	if kind.IsMultiCrew() {
		out = append(out, models.DisciplineMultiCrew)
	}
	switch kind {
	case models.LicenceKindUL:
		out = append(out, models.DisciplineUltralight)
	case models.LicenceKindGPL:
		out = append(out, models.DisciplineGyroplane)
	case models.LicenceKindHelicopter:
		out = append(out, models.DisciplineHelicopter)
	case models.LicenceKindIR:
		out = append(out, models.DisciplineIFR)
	case models.LicenceKindInstructor:
		out = append(out, models.DisciplineInstructor)
	}
	return out
}

// flightTally accumulates flight evidence for one discipline.
type flightTally struct {
	flights int
	dual    int
	last    *time.Time
}

func (t *flightTally) add(n, dual int, last *time.Time) {
	if n == 0 {
		return
	}
	t.flights += n
	t.dual += dual
	if last != nil && (t.last == nil || last.After(*t.last)) {
		v := *last
		t.last = &v
	}
}

type derivation struct {
	cutoff      time.Time
	evidence    map[models.Discipline][]models.DisciplineEvidence
	ulKinds     map[models.ULKind]bool
	flights     map[models.Discipline]*flightTally
	ifr         flightTally
	multiCrew   flightTally
	instructing flightTally
	simulator   flightTally
}

func newDerivation(now time.Time) *derivation {
	return &derivation{
		cutoff:   time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, -RecencyWindowMonths, 0),
		evidence: map[models.Discipline][]models.DisciplineEvidence{},
		ulKinds:  map[models.ULKind]bool{},
		flights:  map[models.Discipline]*flightTally{},
	}
}

func (d *derivation) add(disc models.Discipline, ev models.DisciplineEvidence) {
	d.evidence[disc] = append(d.evidence[disc], ev)
}

func licenceRef(l *models.License) string {
	return strings.TrimSpace(strings.TrimSpace(l.LicenseType) + " " + strings.TrimSpace(l.LicenseNumber))
}

func idRef(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	v := id
	return &v
}

func (d *derivation) addLicences(licences []*models.License) {
	for _, l := range licences {
		for _, disc := range licenceDisciplines(models.ClassifyLicence(l.LicenseType, l.RegulatoryAuthority)) {
			d.add(disc, models.DisciplineEvidence{
				Source: models.EvidenceLicence, Strength: models.StrengthStrong,
				Ref: licenceRef(l), RefID: idRef(l.ID),
			})
		}
	}
}

func (d *derivation) addRatings(ratings []*models.ClassRating, licences []*models.License) {
	byID := make(map[uuid.UUID]*models.License, len(licences))
	for _, l := range licences {
		byID[l.ID] = l
	}
	for _, r := range ratings {
		disc, ok := ratingDiscipline(r)
		if !ok {
			continue
		}
		ref := string(r.ClassType)
		if r.ULKind != nil {
			ref += " " + string(*r.ULKind)
			d.ulKinds[*r.ULKind] = true
		}
		if l, ok := byID[r.LicenseID]; ok {
			ref += " on " + licenceRef(l)
		}
		d.add(disc, models.DisciplineEvidence{
			Source: models.EvidenceRating, Strength: models.StrengthStrong,
			Ref: ref, RefID: idRef(r.ID),
		})
	}
}

func (d *derivation) addFleet(fleet []*models.Aircraft) {
	for _, a := range fleet {
		if !a.IsActive {
			continue
		}
		ev := models.DisciplineEvidence{
			Source: models.EvidenceAircraft, Strength: models.StrengthRecent,
			Ref: a.Registration, RefID: idRef(a.ID),
		}
		class := ""
		if a.AircraftClass != nil {
			class = *a.AircraftClass
		}
		for _, disc := range classDisciplines(class, a.ULKind, d.ulCredit()) {
			d.add(disc, ev)
		}
		if models.IsULClass(a.AircraftClass) && a.ULKind != nil {
			d.ulKinds[*a.ULKind] = true
		}
		if a.IsMultiPilot {
			d.add(models.DisciplineMultiCrew, ev)
		}
	}
}

// ulCredit reports whether ultralight aircraft and flights also feed the discipline of their
// kind: only for a pilot with no ULTRALIGHT licence or rating.
func (d *derivation) ulCredit() bool {
	for _, ev := range d.evidence[models.DisciplineUltralight] {
		if ev.Strength == models.StrengthStrong {
			return false
		}
	}
	return true
}

func (d *derivation) tally(disc models.Discipline) *flightTally {
	t, ok := d.flights[disc]
	if !ok {
		t = &flightTally{}
		d.flights[disc] = t
	}
	return t
}

func (d *derivation) addFlights(groups []models.DisciplineFlightGroup) {
	for _, g := range groups {
		discs := classDisciplines(g.AircraftClass, g.ULKind, d.ulCredit())
		for _, disc := range discs {
			d.tally(disc).add(g.Flights, g.DualReceivedFlights, g.LastFlight)
		}
		if g.TowedFlights > 0 {
			d.tally(models.DisciplineSailplane).add(g.TowedFlights, g.TowedDualReceivedFlights, g.LastTowed)
			for _, disc := range discs {
				if disc == models.DisciplineUltralight {
					d.tally(disc).add(g.TowedFlights, g.TowedDualReceivedFlights, g.LastTowed)
				}
			}
		}
		if g.ULKind != nil && strings.EqualFold(strings.TrimSpace(g.AircraftClass), string(models.ClassTypeUL)) &&
			g.Flights+g.TowedFlights > 0 {
			d.ulKinds[*g.ULKind] = true
		}
		d.ifr.add(g.IFRFlights, 0, g.LastIFR)
		d.multiCrew.add(g.MultiCrewFlights, 0, g.LastMultiCrew)
		d.instructing.add(g.InstructingFlights, 0, g.LastInstructing)
		d.simulator.add(g.SimulatorSessions, 0, g.LastSimulator)
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// flightEvidence builds the flight evidence entry for a tally.
func (d *derivation) flightEvidence(t *flightTally, source models.EvidenceSource, noun, nouns string) models.DisciplineEvidence {
	ev := models.DisciplineEvidence{Source: source, Strength: models.StrengthDormant, LastSeen: t.last}
	if t.last != nil && !t.last.Before(d.cutoff) {
		ev.Strength = models.StrengthRecent
	}
	ev.Ref = fmt.Sprintf("%d %s", t.flights, plural(t.flights, noun, nouns))
	if t.last != nil {
		ev.Ref += ", last " + t.last.Format("2006-01-02")
	}
	return ev
}

// sourceRank orders evidence licence, rating, aircraft, then flights.
var sourceRank = map[models.EvidenceSource]int{
	models.EvidenceLicence: 0, models.EvidenceRating: 1, models.EvidenceAircraft: 2,
	models.EvidenceFlights: 3, models.EvidenceFlightsDual: 3, models.EvidenceFlightsInstructing: 3,
}

func (d *derivation) resolve(disc models.Discipline, setting models.DisciplineSetting) models.DisciplineState {
	evidence := append([]models.DisciplineEvidence{}, d.evidence[disc]...)
	sort.SliceStable(evidence, func(i, j int) bool {
		if sourceRank[evidence[i].Source] != sourceRank[evidence[j].Source] {
			return sourceRank[evidence[i].Source] < sourceRank[evidence[j].Source]
		}
		return evidence[i].Ref < evidence[j].Ref
	})

	trainingSignal := false
	switch disc {
	case models.DisciplineIFR:
		evidence = d.appendTally(evidence, &d.ifr, models.EvidenceFlights, "flight", "flights")
	case models.DisciplineMultiCrew:
		evidence = d.appendTally(evidence, &d.multiCrew, models.EvidenceFlights, "flight", "flights")
	case models.DisciplineInstructor:
		evidence = d.appendTally(evidence, &d.instructing, models.EvidenceFlightsInstructing, "flight", "flights")
	case models.DisciplineSimulator:
		evidence = d.appendTally(evidence, &d.simulator, models.EvidenceFlights, "session", "sessions")
	default:
		if t, ok := d.flights[disc]; ok && t.flights > 0 {
			if t.dual == t.flights {
				ev := d.flightEvidence(t, models.EvidenceFlightsDual, "dual flight", "dual flights")
				evidence = append(evidence, ev)
				trainingSignal = aircraftDisciplines[disc] && ev.Strength == models.StrengthRecent
			} else {
				evidence = append(evidence, d.flightEvidence(t, models.EvidenceFlights, "flight", "flights"))
			}
		}
	}

	var strong, recent, dormant bool
	for _, ev := range evidence {
		switch ev.Strength {
		case models.StrengthStrong:
			strong = true
		case models.StrengthRecent:
			recent = true
		case models.StrengthDormant:
			dormant = true
		}
	}
	if strong {
		trainingSignal = false
	}

	intent := setting.EffectiveIntent()
	status := models.StatusOff
	switch {
	case intent == models.IntentOff:
		status = models.StatusOff
	case intent == models.IntentOn, recent && !trainingSignal, strong && !dormant:
		status = models.StatusActive
	case intent == models.IntentGoal, trainingSignal:
		status = models.StatusTraining
	case dormant:
		status = models.StatusDormant
	}

	state := models.DisciplineState{
		Discipline:     disc,
		Status:         status,
		Intent:         intent,
		Evidence:       evidence,
		ULKinds:        []models.ULKind{},
		AcknowledgedAt: setting.AcknowledgedAt,
	}
	if disc == models.DisciplineUltralight {
		for _, k := range models.ValidAircraftULKinds() {
			if d.ulKinds[k] {
				state.ULKinds = append(state.ULKinds, k)
			}
		}
	}
	return state
}

func (d *derivation) appendTally(evidence []models.DisciplineEvidence, t *flightTally, source models.EvidenceSource, noun, nouns string) []models.DisciplineEvidence {
	if t.flights == 0 {
		return evidence
	}
	return append(evidence, d.flightEvidence(t, source, noun, nouns))
}
