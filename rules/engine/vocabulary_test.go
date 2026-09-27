package engine

import (
	"slices"
	"sort"
	"testing"
)

// Each test below exercises part of the closed vocabulary with an inline rule, so that every
// entry the engine implements has at least one case even before a catalogue rule uses it.

const src = `source: { cite: X, url: "https://x", file: sources/faa/61.57.md, quote: x }`

func TestPassengersSubjectAndHolds(t *testing.T) {
	c := testCatalogue(t, `
id: test.x.passengers
support: supported
`+src+`
applies_to: { subject: passengers, authorities: [EASA], excludeClasses: [IR] }
window: { rolling_days: 90 }
filter: { classes: [$subject], roles: [pic, dual], soleManipulator: true }
requirements:
  all_of:
    - { id: tl, metric: takeoffs_and_landings, min: 3, unit: landings, nameKey: requirement.takeoffs_and_landings, remedyKey: remedy.fly_more }
    - { id: night, metric: landings.night, min: 1, unit: landings, nameKey: requirement.night_landings, when: { not: { holds: { classes: [IR], sameLicence: true, valid: true } } } }
stages:
  - { when: { holds: { classes: [IR], sameLicence: true, valid: true } }, status: current, messageKey: pax.current_day_night_ir_waived }
  - { id: ok, when: all_met, status: current, messageKey: pax.current_day_night }
  - { id: unknown, when: undetermined, status: unknown, messageKey: pax.evaluation_failed }
  - { id: short, when: { unmet: tl }, status: expired, messageKey: pax.not_current, params: { needed: { needed: tl } } }
  - { id: night, when: always, status: expired, messageKey: pax.day_current_night_not, params: { needed: { needed: night } } }
`)
	rec := `
licences:
  - { id: l1, authority: easa, type: PPL(A) }
  - { id: l2, authority: EASA, type: CPL }
ratings:
  - { id: r1, licenceId: l1, class: SEP_LAND }
  - { id: r2, licenceId: l1, class: SEP_LAND, notes: duplicate class on the same licence }
  - { id: r3, licenceId: l1, class: IR, expires: 2026-01-01 }
  - { id: r4, licenceId: l2, class: MEP_LAND }
  - { id: r5, licenceId: l2, class: IR, expires: 2026-07-01 }
flights:
  - { date: 2026-07-01, class: SEP_LAND, minutes: { total: 60, pic: 60 }, takeoffs: { day: 2 }, landings: { day: 1, night: 1 }, soleManipulator: true }
  - { date: 2026-07-02, class: SEP_LAND, minutes: { total: 60, pic: 60 }, takeoffs: { day: 5 }, landings: { day: 5 }, soleManipulator: false }
  - { date: 2026-07-03, class: SEP_LAND, minutes: { total: 60, pic: 60 }, takeoffs: { day: 1 }, landings: { day: 1 } }
  - { date: 2026-07-04, class: SEP_LAND, minutes: { total: 60 }, takeoffs: { day: 9 }, landings: { day: 9 }, soleManipulator: true }
  - { date: 2026-07-05, class: MEP_LAND, minutes: { total: 60, pic: 60 }, takeoffs: { day: 3 }, landings: { day: 3 } }
`
	evs, tags := evalOne(t, c, rec, "2026-08-01")
	if len(evs) != 2 {
		t.Fatalf("one passenger subject per class and licence, got %d", len(evs))
	}
	wantStatus(t, evs, 0, "expired", "pax.not_current")
	if evs[0].MessageParams["needed"] != 1 || evs[0].Subject.ID != "l1" {
		t.Errorf("needed/subject: %+v %+v", evs[0].MessageParams, evs[0].Subject)
	}
	if r := row(t, evs[0], "tl"); r.Current != 2 || !r.Tracked || r.RemedyParams["missing"] != 1 {
		t.Errorf("tl row: %+v", r)
	}
	wantStatus(t, evs, 1, "unknown", "pax.evaluation_failed")
	if !hasTag(tags, "unknown:soleManipulator") {
		t.Errorf("tags %v", tags)
	}
	evs, _ = evalOne(t, c, rec, "2026-06-01")
	wantStatus(t, evs, 1, "current", "pax.current_day_night_ir_waived")
}

func TestLicenceSubjectSinceIssueNOfResets(t *testing.T) {
	c := testCatalogue(t, `
id: test.x.licence
support: partial
`+src+`
applies_to: { subject: licence, licenceKinds: [GPL] }
window: { since_issue: licence }
resets: [{ event: refresher_training }]
requirements:
  id: two
  n_of:
    n: 2
    of:
      - { id: pic, metric: minutes.pic, min: 600, unit: minutes, nameKey: requirement.pic_time_since_issue }
      - { id: xc, metric: minutes.crossCountry, min: 60, unit: minutes, nameKey: requirement.cross_country_flights, filter: { withMinutes: [crossCountry], withoutMinutes: [dual] } }
      - { id: ifr, metric: minutes.ifr, min: 60, unit: minutes, nameKey: requirement.ifr_time, window: { lifetime: true } }
      - { id: competence, metric: not_recorded, min: 1, unit: flight, nameKey: requirement.pax_competence_flight, informational: true, messages: { untracked: requirement.untracked } }
      - { id: info, metric: minutes.night, min: 1, unit: minutes, nameKey: requirement.night_time, informational: true, messages: { met: training.met, unmet: training.not_met, untracked: requirement.untracked } }
stages:
  - { id: ok, when: all_met, status: current, messageKey: licence.recency_current }
  - { id: no, when: always, status: lapsed, messageKey: licence.recency_not_met }
`)
	rec := `
licences:
  - { id: g, authority: EASA, type: GPL, issued: 2025-01-10 }
  - { id: p, authority: EASA, type: PPL }
events:
  - { date: 2025-06-01, kind: refresher_training }
flights:
  - { date: 2025-01-09, class: GYROPLANE, minutes: { total: 600, pic: 600, crossCountry: 600, ifr: 60 } }
  - { date: 2025-06-01, class: GYROPLANE, minutes: { total: 300, pic: 300, crossCountry: 30 } }
  - { date: 2025-07-01, class: GYROPLANE, minutes: { total: 300, pic: 300, crossCountry: 30, dual: 10, ifr: 60 } }
`
	evs, tags := evalOne(t, c, rec, "2026-01-01")
	if len(evs) != 1 || evs[0].Subject.Detail != "GPL" {
		t.Fatalf("one GPL licence subject: %+v", evs)
	}
	wantStatus(t, evs, 0, "current", "licence.recency_current")
	if r := row(t, evs[0], "pic"); r.Current != 600 {
		t.Errorf("reset window counts from 2025-06-01: %+v", r)
	}
	if r := row(t, evs[0], "xc"); r.Current != 30 || r.Met {
		t.Errorf("xc counts solo cross-country only: %+v", r)
	}
	if r := row(t, evs[0], "competence"); r.Tracked || r.Met || r.MessageKey != "requirement.untracked" {
		t.Errorf("not_recorded row: %+v", r)
	}
	if r := row(t, evs[0], "info"); r.MessageKey != "training.not_met" {
		t.Errorf("informational row: %+v", r)
	}
	for _, tag := range []string{"n_of:two:met", "event:refresher_training", "window:edge-in"} {
		if !hasTag(tags, tag) {
			t.Errorf("missing %s in %v", tag, tags)
		}
	}
	evs, tags = evalOne(t, c, `
licences: [{ id: g, authority: EASA, type: GPL, issued: 2025-01-10 }]
flights: [{ date: 2025-01-10, class: GYROPLANE, minutes: { total: 60, pic: 60, night: 10 } }]
`, "2026-01-01")
	wantStatus(t, evs, 0, "lapsed", "licence.recency_not_met")
	if !hasTag(tags, "n_of:two:unmet") || row(t, evs[0], "info").MessageKey != "training.met" {
		t.Errorf("tags %v rows %+v", tags, evs[0].Requirements)
	}
	evs, _ = evalOne(t, c, `licences: [{ id: g, authority: EASA, type: GPL }]`, "2026-01-01")
	if r := row(t, evs[0], "pic"); r.Tracked {
		t.Errorf("no issue date means the window is unknown: %+v", r)
	}
}

func TestPrivilegeTowsProjectionAndParams(t *testing.T) {
	c := testCatalogue(t, `
id: test.x.towing
support: supported
`+src+`
applies_to: { subject: privilege, privilegeKinds: [SAILPLANE_TOWING], excludeAuthorities: [FAA] }
window: { rolling_months: 24 }
requirements:
  id: tows
  any_of:
    - { id: tows, metric: tows, min: 5, unit: tows, nameKey: requirement.tows, remedyKey: remedy.privilege_with_instructor, filter: { towKinds: [glider], flags: { accompanied: false } } }
    - { id: aerotows, metric: flights, min: 3, unit: flights, nameKey: requirement.towed_glider_flights, filter: { classes: [GLIDER], launchMethods: [aerotow], roles: [pic] } }
stages:
  - { id: expired, when: expired, status: expired, messageKey: privilege.expired, params: { date: expiry_date } }
  - { id: soon, when: { all: [all_met, { expires_within: { days: 30 } }] }, status: expiring, messageKey: privilege.expiring, params: { days: days_to_expiry, date: expiry_date } }
  - { id: ok, when: all_met, status: current, messageKey: privilege.recency_current, params: { date: valid_until } }
  - { id: no, when: always, status: lapsed, messageKey: privilege.recency_not_met, params: { date: { last_date: tows } } }
`)
	rec := `
licences: [{ id: l, authority: EASA, type: PPL(A) }, { id: f, authority: FAA, type: PRIVATE }]
privileges:
  - { id: p1, licenceId: l, kind: SAILPLANE_TOWING, expires: 2027-12-31 }
  - { id: p2, licenceId: f, kind: SAILPLANE_TOWING }
  - { id: p3, licenceId: l, kind: CLOUD_FLYING }
flights:
  - { date: 2024-08-31, class: SEP_LAND, minutes: { total: 60, pic: 60 }, towKind: glider, towedGliders: 3, flags: { towFlight: true } }
  - { date: 2025-01-31, class: SEP_LAND, minutes: { total: 60, pic: 60 }, towKind: glider, flags: { towFlight: true } }
  - { date: 2025-02-01, class: SEP_LAND, minutes: { total: 60, pic: 60 }, towKind: glider, flags: { towFlight: true } }
  - { date: 2025-03-01, class: SEP_LAND, minutes: { total: 60, pic: 60 }, towKind: banner, flags: { towFlight: true } }
  - { date: 2025-03-02, class: SEP_LAND, minutes: { total: 60, pic: 60 }, flags: { towFlight: true } }
  - { date: 2025-03-03, class: SEP_LAND, minutes: { total: 60, pic: 60 }, towKind: glider, flags: { towFlight: true, accompanied: true } }
  - { date: 2025-03-04, class: SEP_LAND, minutes: { total: 60, pic: 60 } }
  - { date: 2025-04-01, class: GLIDER, launchMethod: aerotow, minutes: { total: 60, pic: 60 } }
  - { date: 2025-04-02, class: GLIDER, launchMethod: winch, minutes: { total: 60, pic: 60 } }
  - { date: 2025-04-03, class: GLIDER, minutes: { total: 60, pic: 60 } }
`
	evs, tags := evalOne(t, c, rec, "2026-08-31")
	if len(evs) != 1 {
		t.Fatalf("FAA licence excluded: %+v", evs)
	}
	wantStatus(t, evs, 0, "current", "privilege.recency_current")
	if evs[0].ValidUntil == nil || evs[0].ValidUntil.String() != "2026-08-31" || evs[0].MessageParams["date"] != "2026-08-31" {
		t.Errorf("validUntil when the three-glider tow leaves: %v %v", evs[0].ValidUntil, evs[0].MessageParams)
	}
	if r := row(t, evs[0], "tows"); r.Current != 5 || r.ValidUntil.String() != "2026-08-31" {
		t.Errorf("tows row: %+v", r)
	}
	if !hasTag(tags, "unknown:towKinds") || !hasTag(tags, "any_of:tows:tows") || !hasTag(tags, "window:edge-in") {
		t.Errorf("tags %v", tags)
	}
	evs, tags = evalOne(t, c, rec, "2026-09-01")
	wantStatus(t, evs, 0, "lapsed", "privilege.recency_not_met")
	if evs[0].MessageParams["date"] != "2025-02-01" || !hasTag(tags, "window:edge-out") {
		t.Errorf("params %v tags %v", evs[0].MessageParams, tags)
	}
	if r := row(t, evs[0], "tows"); r.RemedyKey != "remedy.privilege_with_instructor" || r.RemedyParams["missing"] != 3 || r.RemedyParams["unit"] != "tows" {
		t.Errorf("remedy %+v", r)
	}
	evs, _ = evalOne(t, c, rec, "2027-12-15")
	wantStatus(t, evs, 0, "lapsed", "privilege.recency_not_met")
	evs, _ = evalOne(t, c, rec+"  - { date: 2027-12-01, class: SEP_LAND, minutes: { total: 60 }, towKind: glider, towedGliders: 5, flags: { towFlight: true } }\n", "2027-12-15")
	wantStatus(t, evs, 0, "expiring", "privilege.expiring")
	if evs[0].MessageParams["days"] != 16 || evs[0].MessageParams["date"] != "2027-12-31" {
		t.Errorf("params %v", evs[0].MessageParams)
	}
	evs, _ = evalOne(t, c, rec, "2028-01-01")
	wantStatus(t, evs, 0, "expired", "privilege.expired")
}

func TestCredentialValidity(t *testing.T) {
	c := testCatalogue(t, `
id: test.x.medical
support: supported
`+src+`
applies_to: { subject: credential, credentialTypes: [EASA_CLASS2_MEDICAL] }
validity:
  from: valid_from
  periods:
    - { when: { age_under: 40 }, months: 60, cap_at_age: 42 }
    - { when: { age_at_least: 40, age_under: 50 }, months: 24, cap_at_age: 51 }
    - { months: 12, end_of_month: true }
stages:
  - { id: dob, when: { all: [no_expiry, { missing: date_of_birth }] }, status: unknown, messageKey: credential.date_of_birth_required }
  - { id: none, when: no_expiry, status: unknown, messageKey: credential.no_expiry_date }
  - { id: expired, when: expired, status: expired, messageKey: credential.expired, params: { date: expiry_date } }
  - { id: window, when: { expires_within: { days: 45 } }, status: expiring, messageKey: credential.revalidation_window_open, params: { days: days_to_expiry, date: expiry_date } }
  - { id: ok, when: always, status: current, messageKey: credential.valid, params: { date: expiry_date } }
`)
	cases := []struct {
		name, rec, asOf, status, key, expires string
	}{
		{"young", "holder: { dateOfBirth: 1990-05-10 }\ncredentials: [{ id: m, type: EASA_CLASS2_MEDICAL, issued: 2025-01-15 }]", "2026-01-01", "current", "credential.valid", "2030-01-15"},
		{"cap at 42", "holder: { dateOfBirth: 1985-03-01 }\ncredentials: [{ id: m, type: EASA_CLASS2_MEDICAL, issued: 2024-06-01 }]", "2026-01-01", "current", "credential.valid", "2027-03-01"},
		{"forties", "holder: { dateOfBirth: 1980-03-01 }\ncredentials: [{ id: m, type: EASA_CLASS2_MEDICAL, issued: 2025-06-01 }]", "2027-05-01", "expiring", "credential.revalidation_window_open", "2027-06-01"},
		{"over fifty from previous expiry", "holder: { dateOfBirth: 1960-01-01 }\ncredentials: [{ id: m, type: EASA_CLASS2_MEDICAL, issued: 2025-05-20, validFrom: 2025-06-10 }]", "2026-07-01", "expired", "credential.expired", "2026-06-30"},
		{"recorded earlier expiry wins", "holder: { dateOfBirth: 1990-05-10 }\ncredentials: [{ id: m, type: EASA_CLASS2_MEDICAL, issued: 2025-01-15, expires: 2026-02-01 }]", "2026-01-01", "expiring", "credential.revalidation_window_open", "2026-02-01"},
		{"no dob", "credentials: [{ id: m, type: EASA_CLASS2_MEDICAL, issued: 2025-01-15 }]", "2026-01-01", "unknown", "credential.date_of_birth_required", ""},
		{"no issue date", "holder: { dateOfBirth: 1990-05-10 }\ncredentials: [{ id: m, type: EASA_CLASS2_MEDICAL }]", "2026-01-01", "unknown", "credential.no_expiry_date", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evs, _ := evalOne(t, c, tc.rec, tc.asOf)
			wantStatus(t, evs, 0, tc.status, tc.key)
			if dateStr(evs[0].ExpiresOn) != tc.expires {
				t.Errorf("expiresOn %s, want %s", dateStr(evs[0].ExpiresOn), tc.expires)
			}
		})
	}
	c2 := testCatalogue(t, `
id: test.x.medical-cap
support: supported
`+src+`
applies_to: { subject: credential }
validity: { from: issued, periods: [{ months: 60, cap_at_age: 42 }] }
stages: [{ when: always, status: current, messageKey: credential.valid }]
`)
	evs, _ := evalOne(t, c2, "credentials: [{ id: m, type: OTHER, issued: 2025-01-15 }]", "2026-01-01")
	if evs[0].ExpiresOn != nil {
		t.Errorf("cap without a date of birth leaves the expiry unknown: %v", evs[0].ExpiresOn)
	}
}

func TestFlightReviewEventsCalendarMonths(t *testing.T) {
	c := testCatalogue(t, `
id: test.x.review
support: supported
`+src+`
applies_to: { subject: flight_review, authorities: [FAA] }
window: { calendar_months: 24 }
requirements:
  id: review
  any_of:
    - { id: review, metric: events, min: 1, unit: review, nameKey: requirement.flight_review, filter: { eventKinds: [flight_review], simulator: include } }
    - { id: check, metric: events, min: 1, unit: check, nameKey: requirement.proficiency_check, filter: { eventKinds: [proficiency_check, practical_test], eventRatings: [PRIVATE, IR] } }
stages:
  - { id: ok, when: all_met, status: current, messageKey: flight_review.current, params: { date: { last_date: review } } }
  - { id: none, when: always, status: expired, messageKey: flight_review.none_on_record }
`)
	rec := `
licences: [{ id: e, authority: EASA, type: PPL }, { id: f, authority: FAA, type: PRIVATE }, { id: g, authority: FAA, type: COMMERCIAL }]
events:
  - { date: 2024-06-15, kind: practical_test, rating: IR }
  - { date: 2024-07-01, kind: proficiency_check }
flights:
  - { date: 2024-05-31, class: SEP_LAND, isSimulator: true, minutes: { total: 60 }, flags: { flightReview: true } }
`
	evs, tags := evalOne(t, c, rec, "2026-05-10")
	if len(evs) != 1 || evs[0].Subject.ID != "f" {
		t.Fatalf("one review subject, first FAA licence: %+v", evs)
	}
	wantStatus(t, evs, 0, "current", "flight_review.current")
	if evs[0].ValidUntil.String() != "2026-06-30" || evs[0].MessageParams["date"] != "2024-05-31" {
		t.Errorf("the June practical test keeps the review current through June 2026: %v %v", evs[0].ValidUntil, evs[0].MessageParams)
	}
	if r := row(t, evs[0], "review"); r.ValidUntil.String() != "2026-05-31" || hasTag(tags, "any_of:review:check") {
		t.Errorf("review row %+v tags %v", r, tags)
	}
	evs, _ = evalOne(t, c, rec, "2026-06-01")
	wantStatus(t, evs, 0, "current", "flight_review.current")
	evs, _ = evalOne(t, c, rec, "2026-07-01")
	wantStatus(t, evs, 0, "expired", "flight_review.none_on_record")
}

func TestTrainingHatchAndAnyFilter(t *testing.T) {
	c := testCatalogue(t, `
id: test.x.training
support: partial
`+src+`
applies_to: { subject: training, programme: SPL }
window: { lifetime: true }
requirements:
  all_of:
    - { id: instruction, metric: minutes.dualOrSpic, min: 900, unit: minutes, nameKey: requirement.instruction_given, filter: { classes: [GLIDER] } }
    - id: xc
      metric: flights
      min: 1
      unit: flights
      nameKey: requirement.cross_country_flights
      filter:
        classes: [GLIDER]
        withMinutes: [crossCountry]
        any:
          - { withoutMinutes: [dual], minDistanceKm: 50 }
          - { roles: [dual], minDistanceKm: 100 }
    - { id: credit, metric: minutes.pic, min: 420, unit: minutes, nameKey: requirement.pic_time, informational: true, escape_hatch: sfcl-130b-credit, filter: { excludeClasses: [GLIDER, TMG, ULTRALIGHT] } }
    - { id: bogus, metric: minutes.pic, min: 1, unit: minutes, nameKey: requirement.pic_time, informational: true, escape_hatch: no-such-hatch }
stages:
  - { id: ok, when: all_met, status: current, messageKey: training.all_met }
  - { id: open, when: always, status: lapsed, messageKey: training.in_progress }
`)
	rec := `
flights:
  - { date: 2025-01-01, class: GLIDER, minutes: { total: 600, dual: 600 } }
  - { date: 2025-02-01, class: GLIDER, minutes: { total: 300, spic: 300, crossCountry: 120 } }
  - { date: 2025-03-01, class: SEP_LAND, minutes: { total: 1200, pic: 1200 } }
`
	evs, tags := evalOne(t, c, rec, "2026-01-01")
	if len(evs) != 1 || evs[0].Subject.Detail != "SPL" {
		t.Fatalf("one training subject: %+v", evs)
	}
	wantStatus(t, evs, 0, "lapsed", "training.in_progress")
	if r := row(t, evs[0], "xc"); r.Tracked {
		t.Errorf("unknown distance leaves the cross-country row untracked: %+v", r)
	}
	if r := row(t, evs[0], "credit"); r.Current != 120 || r.Met {
		t.Errorf("credit: %+v", r)
	}
	if r := row(t, evs[0], "bogus"); r.Tracked {
		t.Errorf("an unknown hatch is untracked: %+v", r)
	}
	if !hasTag(tags, "unknown:minDistanceKm") {
		t.Errorf("tags %v", tags)
	}
	evs, _ = evalOne(t, c, rec+"  - { date: 2025-04-01, class: GLIDER, minutes: { total: 300, dual: 300, crossCountry: 200 }, distanceKm: 150 }\n", "2026-01-01")
	wantStatus(t, evs, 0, "current", "training.all_met")
	c2 := testCatalogue(t, `
id: test.x.training-licensed
support: partial
`+src+`
applies_to: { subject: training, programme: UL_THREE_AXIS, authorities: [DULV] }
window: { lifetime: true }
requirements: { all_of: [{ id: t, metric: minutes.picOrSpic, min: 1, unit: minutes, nameKey: requirement.pic_time }] }
stages: [{ when: always, status: current, messageKey: training.in_progress }]
`)
	if evs, _ := evalOne(t, c2, "licences: [{ id: e, authority: EASA, type: PPL }]", "2026-01-01"); len(evs) != 0 {
		t.Errorf("no DULV licence, no subject: %+v", evs)
	}
	if evs, _ := evalOne(t, c2, "licences: [{ id: d, authority: DULV, type: UL }]", "2026-01-01"); len(evs) != 1 || evs[0].Subject.ID != "d" {
		t.Errorf("DULV subject: %+v", evs)
	}
}

func TestTypeVariantRestoredByAnchoredWindow(t *testing.T) {
	c := testCatalogue(t, `
id: test.x.variant
support: supported
`+src+`
applies_to: { subject: type, classes: [SEP_LAND] }
window: { rolling_months: 24 }
requirements: { all_of: [{ id: flown, metric: flights, min: 1, unit: flights, nameKey: requirement.variant_flown, filter: { classes: [$subject], variants: [$subject] } }] }
restored_by: [{ event: differences_training, filter: { variants: [$subject] } }]
stages:
  - { id: ok, when: all_met, status: current, messageKey: type.current }
  - { id: unknown, when: undetermined, status: unknown, messageKey: type.unknown }
  - { id: no, when: always, status: lapsed, messageKey: type.not_flown }
`)
	rec := `
licences: [{ id: l, authority: EASA, type: PPL }]
ratings: [{ id: r, licenceId: l, class: SEP_LAND }, { id: m, licenceId: l, class: MEP_LAND }]
variants: [{ id: v1, ratingId: r, name: electric }, { id: v2, ratingId: r, name: diesel }, { id: v3, ratingId: m, name: turbo }]
events: [{ date: 2026-02-01, kind: differences_training, variant: diesel }]
flights:
  - { date: 2023-05-01, class: SEP_LAND, variant: electric, minutes: { total: 60 } }
  - { date: 2025-05-01, class: SEP_LAND, minutes: { total: 60 } }
`
	evs, tags := evalOne(t, c, rec, "2026-03-01")
	if len(evs) != 2 {
		t.Fatalf("two SEP variants: %+v", evs)
	}
	wantStatus(t, evs, 0, "unknown", "type.unknown")
	wantStatus(t, evs, 1, "current", "type.current")
	if evs[1].ValidUntil.String() != "2028-02-01" || !hasTag(tags, "event:differences_training") || !hasTag(tags, "unknown:variants") {
		t.Errorf("validUntil %v tags %v", evs[1].ValidUntil, tags)
	}
	c2 := testCatalogue(t, `
id: test.x.anchored
support: supported
`+src+`
applies_to: { subject: rating, classes: [SEP_LAND] }
window: { validity_period: true }
requirements: { all_of: [{ id: f, metric: flights, min: 2, unit: flights, nameKey: requirement.variant_flown }] }
restored_by: [{ event: proficiency_check }]
stages:
  - { id: before, when: before_window, status: current, messageKey: rating.window_not_open, params: { date: window_opens_at } }
  - { id: ok, when: all_met, status: current, messageKey: rating.revalidation_current }
  - { id: no, when: always, status: expiring, messageKey: rating.revalidation_not_met }
`)
	rec2 := `
licences: [{ id: l, authority: EASA, type: PPL }]
ratings: [{ id: r, licenceId: l, class: SEP_LAND, issued: 2020-01-01, validFrom: 2025-01-01, expires: 2027-01-01 }]
flights: [{ date: 2024-12-31, class: SEP_LAND, flags: { proficiencyCheck: true } }, { date: 2025-01-01, class: SEP_LAND }]
`
	evs, tags = evalOne(t, c2, rec2, "2026-01-01")
	wantStatus(t, evs, 0, "expiring", "rating.revalidation_not_met")
	if evs[0].WindowOpensAt.String() != "2025-01-01" || !hasTag(tags, "window:edge-in") || !hasTag(tags, "window:edge-out") {
		t.Errorf("windowOpensAt %v tags %v", evs[0].WindowOpensAt, tags)
	}
	evs, _ = evalOne(t, c2, rec2+"events: [{ date: 2025-06-01, kind: proficiency_check }]\n", "2026-01-01")
	wantStatus(t, evs, 0, "current", "rating.revalidation_current")
	evs, _ = evalOne(t, c2, "licences: [{ id: l, authority: EASA, type: PPL }]\nratings: [{ id: r, licenceId: l, class: SEP_LAND, validFrom: 2027-01-01, expires: 2028-01-01 }]", "2026-01-01")
	wantStatus(t, evs, 0, "current", "rating.window_not_open")
	evs, _ = evalOne(t, c2, "licences: [{ id: l, authority: EASA, type: PPL }]\nratings: [{ id: r, licenceId: l, class: SEP_LAND, issued: 2025-01-01 }]\nflights: [{ date: 2025-02-01, class: SEP_LAND }, { date: 2025-03-01, class: SEP_LAND }]", "2026-01-01")
	wantStatus(t, evs, 0, "current", "rating.revalidation_current")
	evs, _ = evalOne(t, c2, "licences: [{ id: l, authority: EASA, type: PPL }]\nratings: [{ id: r, licenceId: l, class: SEP_LAND }]", "2026-01-01")
	if row(t, evs[0], "f").Tracked {
		t.Error("no validity start: untracked")
	}
}

func TestLaunchMethodSubject(t *testing.T) {
	c := testCatalogue(t, `
id: test.x.launch
support: supported
`+src+`
applies_to: { subject: launch_method, classes: [GLIDER], launchMethods: [winch, aerotow, self-launch] }
window: { rolling_months: 24 }
requirements:
  all_of:
    - id: launches
      metric: launches
      min: 5
      unit: launches
      nameKey: requirement.launches
      remedyKey: remedy.launch_method_dual
      filter:
        any:
          - { classes: [$subject], launchMethods: [$subject] }
          - { classes: [TMG], excludeLaunchMethods: [winch, aerotow, car, bungee] }
stages:
  - { id: ok, when: all_met, status: current, messageKey: launch_method.current }
  - { id: no, when: always, status: lapsed, messageKey: launch_method.lapsed }
`)
	rec := `
licences: [{ id: l, authority: EASA, type: SPL }]
ratings: [{ id: g, licenceId: l, class: GLIDER }]
privileges: [{ id: p, licenceId: l, kind: LAUNCH_METHOD_TRAINED, detail: Self-Launch }]
flights:
  - { date: 2026-01-01, class: GLIDER, launchMethod: winch, launches: 6, minutes: { total: 60 } }
  - { date: 2026-01-02, class: GLIDER, launchMethod: bungee, launches: 2, minutes: { total: 60 } }
  - { date: 2026-01-03, class: TMG, launches: 4, minutes: { total: 60 } }
  - { date: 2026-01-04, class: TMG, launchMethod: aerotow, launches: 4, minutes: { total: 60 } }
  - { date: 2027-01-05, class: GLIDER, launchMethod: aerotow, launches: 1, minutes: { total: 60 } }
`
	evs, _ := evalOne(t, c, rec, "2026-06-01")
	var got []string
	for _, e := range evs {
		got = append(got, e.Subject.Detail+"="+e.Status)
	}
	sort.Strings(got)
	if !slices.Equal(got, []string{"self-launch=lapsed", "winch=current"}) {
		t.Fatalf("methods: %v", got)
	}
	for _, e := range evs {
		if e.Subject.Detail == "self-launch" {
			r := row(t, e, "launches")
			if r.Current != 4 || r.RemedyParams["method"] != "self-launch" || r.RemedyParams["missing"] != 1 {
				t.Errorf("self-launch row %+v", r)
			}
		}
	}
}

func TestRatingFilters(t *testing.T) {
	c := testCatalogue(t, `
id: test.x.filters
support: supported
effective_from: 2020-01-01
effective_to: 2030-12-31
`+src+`
applies_to: { subject: rating, classes: [ULTRALIGHT], ulKinds: [THREE_AXIS, none], licenceKinds: [UL] }
window: { rolling_days: 365 }
filter: { simulator: include, fstdTypes: [FFS] }
requirements:
  all_of:
    - { id: kind, metric: minutes.total, min: 60, unit: minutes, nameKey: requirement.total_time, filter: { ulKinds: [$subject] } }
    - { id: type, metric: flights, min: 1, unit: flights, nameKey: requirement.variant_flown, filter: { typeDesignators: [C42], minMinutes: 30, minLandings: 2 } }
    - { id: tail, metric: full_stop_landings, min: 1, unit: landings, nameKey: requirement.tailwheel_full_stop_landings, filter: { tailwheel: true, pilotFlying: true } }
    - { id: night, metric: full_stop_night_landings, min: 1, unit: landings, nameKey: requirement.full_stop_night_landings }
    - { id: sectors, metric: route_sectors, min: 2, unit: sectors, nameKey: requirement.route_sectors }
    - { id: dist, metric: distance_km, min: 100, unit: km, nameKey: requirement.ul_xc_distance }
    - { id: sim, metric: flights, min: 1, unit: flights, nameKey: requirement.flight_time, filter: { simulator: only } }
    - { id: longest, metric: longest_training_flight_minutes, min: 60, unit: minutes, nameKey: requirement.training_flight }
    - { id: training, metric: training_flights, min: 2, unit: flights, nameKey: requirement.training_flights }
    - { id: credit, metric: minutes.total, min: 1, unit: minutes, nameKey: requirement.total_time, filter: { classes: [SEP_LAND], ulCredit: [{ class: SEP_LAND, ulKinds: [GYROPLANE], minMtomKg: 450 }] } }
stages:
  - { id: ok, when: all_met, status: current, messageKey: rating.recency_current }
  - { id: no, when: always, status: lapsed, messageKey: rating.recency_not_met }
`)
	rec := `
licences: [{ id: l, authority: DAeC, type: Luftsportgeräteführer }, { id: p, authority: EASA, type: PPL }]
ratings:
  - { id: r1, licenceId: l, class: ULTRALIGHT, ulKind: THREE_AXIS }
  - { id: r2, licenceId: l, class: ULTRALIGHT }
  - { id: r3, licenceId: l, class: ULTRALIGHT, ulKind: GYROPLANE }
  - { id: r4, licenceId: p, class: ULTRALIGHT, ulKind: THREE_AXIS }
flights:
  - { date: 2026-01-10, class: ULTRALIGHT, ulKind: THREE_AXIS, typeDesignator: C42, tailwheel: true, pilotFlying: true, fullStopLandings: 2, fullStopNightLandings: 1, cruiseMinutes: 20, distanceKm: 120.5, minutes: { total: 90, dual: 90 }, landings: { day: 2 } }
  - { date: 2026-01-11, class: ULTRALIGHT, ulKind: THREE_AXIS, typeDesignator: C42, cruiseMinutes: 10, minutes: { total: 20, dual: 20 }, landings: { day: 3 } }
  - { date: 2026-01-12, class: ULTRALIGHT, minutes: { total: 30 }, cruiseMinutes: 30 }
  - { date: 2026-01-13, class: SEP_LAND, isSimulator: true, fstdType: FFS, minutes: { total: 60 } }
  - { date: 2026-01-14, class: SEP_LAND, isSimulator: true, minutes: { total: 60 } }
  - { date: 2026-01-15, class: ULTRALIGHT, ulKind: GYROPLANE, mtomKg: 500, minutes: { total: 30 } }
  - { date: 2026-01-16, class: ULTRALIGHT, ulKind: GYROPLANE, mtomKg: 300, minutes: { total: 30 } }
  - { date: 2026-01-17, class: ULTRALIGHT, ulKind: GYROPLANE, minutes: { total: 30 } }
  - { date: 2026-01-18, class: ULTRALIGHT, ulKind: THREE_AXIS, minutes: { total: 40 }, landings: { day: 2 } }
`
	evs, tags := evalOne(t, c, rec, "2026-06-01")
	if len(evs) != 2 {
		t.Fatalf("THREE_AXIS and kindless UL ratings on the UL licence: %+v", evs)
	}
	wantStatus(t, evs, 0, "current", "rating.recency_current")
	for _, want := range []struct {
		id  string
		cur float64
	}{{"kind", 150}, {"type", 1}, {"tail", 2}, {"night", 1}, {"sectors", 2}, {"dist", 120.5}, {"sim", 1}, {"longest", 90}, {"training", 2}, {"credit", 90}} {
		if r := row(t, evs[0], want.id); r.Current != want.cur {
			t.Errorf("%s: current %v, want %v", want.id, r.Current, want.cur)
		}
	}
	for _, tag := range []string{"unknown:ulKinds", "unknown:typeDesignators", "unknown:tailwheel", "unknown:fstdTypes", "unknown:ulCredit"} {
		if !hasTag(tags, tag) {
			t.Errorf("missing %s in %v", tag, tags)
		}
	}
	if evs, _ := evalOne(t, c, rec, "2019-06-01"); len(evs) != 0 {
		t.Error("before effective_from nothing is evaluated")
	}
	if evs, _ := evalOne(t, c, rec, "2031-06-01"); len(evs) != 0 {
		t.Error("after effective_to nothing is evaluated")
	}
}

func TestMetWithinAndNotSupported(t *testing.T) {
	for _, span := range []string{"rolling_months: 1", "rolling_days: 30", "calendar_months: 1"} {
		c := testCatalogue(t, `
id: test.x.grace
support: supported
`+src+`
applies_to: { subject: rating, classes: [SEP_LAND], holds: { credentials: [EASA_CLASS2_MEDICAL], valid: true } }
window: { rolling_days: 90 }
requirements: { all_of: [{ id: l, metric: landings.day, min: 3, unit: landings, nameKey: requirement.day_landings }, { id: n, metric: takeoffs.night, min: 0, unit: takeoffs, nameKey: requirement.night_takeoffs }, { id: d, metric: takeoffs.day, min: 0, unit: takeoffs, nameKey: requirement.takeoffs }] }
stages:
  - { id: ok, when: { met: l }, status: current, messageKey: rating.recency_current }
  - { id: grace, when: { met_within: { `+span+` } }, status: expiring, messageKey: rating.expiring, params: { days: days_to_expiry } }
  - { id: no, when: { any: [{ untracked: l }, always] }, status: lapsed, messageKey: rating.recency_not_met }
`)
		rec := `
licences: [{ id: l, authority: EASA, type: PPL }]
ratings: [{ id: r, licenceId: l, class: SEP_LAND }]
credentials: [{ id: m, type: EASA_CLASS2_MEDICAL, expires: 2030-01-01 }]
flights: [{ date: 2026-01-01, class: SEP_LAND, landings: { day: 3 } }]
`
		evs, _ := evalOne(t, c, rec, "2026-04-15")
		wantStatus(t, evs, 0, "expiring", "rating.expiring")
		if _, ok := evs[0].MessageParams["days"]; ok {
			t.Error("no expiry, no days param")
		}
		evs, _ = evalOne(t, c, rec, "2026-06-15")
		wantStatus(t, evs, 0, "lapsed", "rating.recency_not_met")
		if evs, _ := evalOne(t, c, "licences: [{ id: l, authority: EASA, type: PPL }]\nratings: [{ id: r, licenceId: l, class: SEP_LAND }]", "2026-04-15"); len(evs) != 0 {
			t.Error("applies_to.holds without a medical skips the subject")
		}
	}
	c := testCatalogue(t, "id: test.x.none\nsupport: not_supported\n"+src+"\napplies_to: { subject: rating }\n")
	if evs, _ := evalOne(t, c, "licences: [{ id: l, authority: EASA, type: PPL }]\nratings: [{ id: r, licenceId: l, class: SEP_LAND }]", "2026-01-01"); len(evs) != 0 {
		t.Error("not_supported rules evaluate nothing")
	}
}

func TestHoldsVariants(t *testing.T) {
	rec := record(t, `
licences: [{ id: a, authority: EASA, type: PPL, expires: 2025-01-01 }, { id: b, authority: EASA, type: SPL }]
ratings: [{ id: r1, licenceId: a, class: TMG }, { id: r2, licenceId: b, class: TMG }, { id: r3, licenceId: a, class: ULTRALIGHT, ulKind: THREE_AXIS }]
privileges: [{ id: p, licenceId: b, kind: TMG_NIGHT, expires: 2025-01-01 }]
credentials: [{ id: c, type: EASA_LAPL_MEDICAL }]
`)
	c := testCatalogue(t)
	e := &evalCtx{cat: c, v: c.Vocabulary, p: prepare(rec, c.Vocabulary), asOf: MustDate("2026-01-01")}
	e.subj = subjectRef{licence: &rec.Licences[1]}
	d := e.asOf
	for _, tc := range []struct {
		h    Holds
		want bool
	}{
		{Holds{Classes: []string{"TMG"}, LicenceKinds: []string{"PPL_A"}}, true},
		{Holds{Classes: []string{"TMG"}, LicenceKinds: []string{"LAPL_A"}}, false},
		{Holds{Classes: []string{"ULTRALIGHT"}, ULKinds: []string{"SAILPLANE"}}, false},
		{Holds{Classes: []string{"TMG", "ULTRALIGHT"}, Every: true}, true},
		{Holds{Classes: []string{"TMG", "GLIDER"}, Every: true}, false},
		{Holds{LicenceKinds: []string{"PPL_A"}, Valid: true}, false},
		{Holds{LicenceKinds: []string{"SPL"}, SameLicence: true}, true},
		{Holds{Privileges: []string{"TMG_NIGHT"}}, true},
		{Holds{Privileges: []string{"TMG_NIGHT"}, Valid: true}, false},
		{Holds{Credentials: []string{"EASA_LAPL_MEDICAL"}, Valid: true}, true},
		{Holds{}, false},
	} {
		if got := e.holds(&tc.h, d); got != tc.want {
			t.Errorf("holds %+v = %v, want %v", tc.h, got, tc.want)
		}
	}
	e.rule = &Rule{}
	if e.cond(&Condition{Op: "met", Ref: "nope"}, d, triMet) || e.cond(&Condition{Op: "bogus"}, d, triMet) {
		t.Error("unknown requirement or condition is false")
	}
	if !e.cond(&Condition{Op: "all", List: []*Condition{{Op: "always"}}}, d, triMet) || e.cond(&Condition{Op: "any", List: []*Condition{{Op: "no_expiry", Not: nil}, {Op: "all_met"}}}, d, triUnmet) != true {
		t.Error("all/any")
	}
}
