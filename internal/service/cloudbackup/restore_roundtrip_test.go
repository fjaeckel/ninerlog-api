package cloudbackup

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/testutil/restorefake"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	fillTime = time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	timeType = reflect.TypeOf(time.Time{})
)

// fill sets every exported field reachable from v to a non-zero value, so a
// field a restore forgets shows up as a difference.
func fill(v reflect.Value, name string) {
	switch v.Kind() {
	case reflect.String:
		v.SetString("v-" + name)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(7)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(2.5)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fill(v.Elem(), name)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fill(s.Index(0), name)
		v.Set(s)
	case reflect.Struct:
		if v.Type() == timeType {
			v.Set(reflect.ValueOf(fillTime))
			return
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				fill(v.Field(i), v.Type().Field(i).Name)
			}
		}
	}
}

func filled[T any](name string) *T {
	v := new(T)
	fill(reflect.ValueOf(v).Elem(), name)
	return v
}

// fullPayload populates every section and every field of every row, override
// flags included.
func fullPayload() *Payload {
	contact := filled[models.Contact]("Contact")
	contact.Name = "Crew Person"
	contact.ID = uuid.New()

	flight := filled[models.Flight]("Flight")
	flight.ID = uuid.New()
	flight.AircraftReg = "D-EABC"
	flight.SignatureID = nil
	flight.Approaches = []models.ApproachEntry{{Type: "ILS", Airport: ptr("EDDF"), Runway: ptr("25C")}}
	flight.LaunchMethod = ptr("winch")
	flight.OffBlockTime, flight.OnBlockTime = ptr("08:00:00"), ptr("09:00:00")
	flight.DepartureTime, flight.ArrivalTime = ptr("08:05:00"), ptr("08:55:00")
	flight.CrewMembers = []models.FlightCrewMember{
		{ID: uuid.New(), FlightID: flight.ID, ContactID: &contact.ID, Name: "Crew Person", Role: models.CrewRoleInstructor},
		{ID: uuid.New(), FlightID: flight.ID, Name: "", Role: models.CrewRolePassenger},
	}
	second := sampleFlight(5, "D-EABC")

	aircraft := filled[models.Aircraft]("Aircraft")
	aircraft.Registration = "D-EABC"
	aircraft.AircraftClass = ptr("ULTRALIGHT")
	aircraft.ULKind = ptr(models.ULKindThreeAxis)
	aircraft.MTOMKg = ptr(472)

	license := filled[models.License]("License")
	license.ID = uuid.New()
	license.IssuingCountry = ptr("CH")
	rating := filled[models.ClassRating]("ClassRating")
	rating.ClassType = models.ClassTypeUL
	rating.ULKind = ptr(models.ULKindThreeAxis)

	credential := filled[models.Credential]("Credential")
	credential.CredentialType = models.CredentialTypeEASAClass2Medical
	credential.ExpiryDate = ptr(fillTime.AddDate(1, 0, 0))

	report := validReportDef()
	report.GroupBy = models.ReportGroupRoute
	report.Limit = ptr(10)
	report.Filter = models.CustomReportFilter{
		Q: ptr("reg:D-EABC"), AircraftReg: ptr("D-EABC"), DepartureICAO: ptr("EDDF"),
		ArrivalICAO: ptr("EDDM"), Role: ptr("pic"), LogbookLicenseID: &license.ID,
	}

	return &Payload{
		ExportedAt:  fillTime.Format(time.RFC3339),
		Version:     PayloadVersion,
		Format:      PayloadFormat,
		Flights:     []*models.Flight{flight, second},
		Aircraft:    []*models.Aircraft{aircraft},
		Licenses:    []LicenseWithRatings{{License: license, ClassRatings: []*models.ClassRating{rating}}},
		Credentials: []*models.Credential{credential},
		Contacts:    []*models.Contact{contact},
		CustomCurrencyRules: []CustomCurrencyRule{
			{Name: "Night", Description: ptr("90 day night"), Emoji: ptr("N"), Definition: models.CustomCurrencyRuleBody{
				Window:       models.CurrencyWindow{Amount: 90, Unit: "days"},
				Filters:      []models.CurrencyFilter{{Field: "is_pic", Op: "is_true"}},
				Requirements: []models.CurrencyRequirement{{Metric: "night_landings", Min: 3, Label: "Landings"}},
			}, Enabled: false, Notify: true},
			{Name: "Hours", Definition: models.CustomCurrencyRuleBody{
				Window:       models.CurrencyWindow{Amount: 12, Unit: "months"},
				Requirements: []models.CurrencyRequirement{{Metric: "total_time", Min: 12, Unit: "hours"}},
			}, Enabled: true},
		},
		CustomReports: []CustomReport{{Name: "Scoped", Definition: report}, {Name: "Plain", Definition: validReportDef()}},
		NotificationPreferences: &NotificationPreferences{
			EmailEnabled: true, EnabledCategories: []string{"rating_expiry", "currency_night"}, WarningDays: []int64{30, 7}, CheckHour: 6,
		},
		FlightBaseline: filled[FlightBaseline]("Baseline"),
		LogbookMode: &LogbookMode{Strict: false, Events: []LogbookModeEvent{
			{Strict: true, Trigger: models.LogbookTriggerManual, CreatedAt: fillTime},
			{Strict: false, Trigger: models.LogbookTriggerManual, CreatedAt: fillTime.Add(time.Hour)},
		}},
		FlightRevisions: []FlightRevision{{
			FlightID: flight.ID, RevisionNo: 1, Kind: models.RevisionKindUpdate,
			Snapshot:     json.RawMessage(`{"id":"` + flight.ID.String() + `","date":"2026-02-03","aircraft_reg":"D-EABC","total_time":7}`),
			CrewSnapshot: json.RawMessage(`[{"name":"Crew Person","role":"Instructor"}]`),
			Source:       "user_edit", Reason: ptr("typo"), IsCorrection: true, ChangedAt: fillTime,
		}},
		LogbookCertifications: []LogbookCertification{{CertifiedThrough: "2026-02-01", CertifiedAt: fillTime}},
	}
}

// payloadFromState projects the stored rows the way an export reads them.
func payloadFromState(st restorefake.State) *Payload {
	p := &Payload{
		ExportedAt:  fillTime.Format(time.RFC3339),
		Version:     PayloadVersion,
		Format:      PayloadFormat,
		Aircraft:    st.Aircraft,
		Credentials: st.Credentials,
		Contacts:    st.Contacts,
	}
	crew := map[uuid.UUID][]models.FlightCrewMember{}
	for _, m := range st.Crew {
		crew[m.FlightID] = append(crew[m.FlightID], m)
	}
	for _, f := range st.Flights {
		out := *f
		out.CrewMembers = crew[f.ID]
		p.Flights = append(p.Flights, &out)
	}
	for _, l := range st.Licenses {
		bundle := LicenseWithRatings{License: l}
		for _, r := range st.Ratings {
			if r.LicenseID == l.ID {
				bundle.ClassRatings = append(bundle.ClassRatings, r)
			}
		}
		p.Licenses = append(p.Licenses, bundle)
	}
	for _, r := range st.Rules {
		p.CustomCurrencyRules = append(p.CustomCurrencyRules, NewCustomCurrencyRule(r))
	}
	for _, r := range st.Reports {
		p.CustomReports = append(p.CustomReports, CustomReport{Name: r.Name, Definition: r.Definition})
	}
	p.NotificationPreferences = NewNotificationPreferences(st.Prefs)
	p.FlightBaseline = NewFlightBaseline(st.Baseline)
	p.LogbookMode = &LogbookMode{Strict: st.Strict, Since: st.StrictSince, Events: []LogbookModeEvent{}}
	for _, e := range st.ModeEvents {
		p.LogbookMode.Events = append(p.LogbookMode.Events, LogbookModeEvent{
			Strict: e.Strict, Trigger: e.Trigger, LicenseID: e.LicenseID, CreatedAt: e.CreatedAt,
		})
	}
	for _, r := range st.Revisions {
		p.FlightRevisions = append(p.FlightRevisions, FlightRevision{
			FlightID: r.FlightID, RevisionNo: r.RevisionNo, Kind: r.Kind, Snapshot: r.Snapshot,
			CrewSnapshot: r.CrewSnapshot, Source: r.Source, Reason: r.Reason,
			IsCorrection: r.IsCorrection, ChangedAt: r.ChangedAt,
		})
	}
	for _, c := range st.Certifications {
		p.LogbookCertifications = append(p.LogbookCertifications, LogbookCertification{
			CertifiedThrough: c.CertifiedThrough.Format("2006-01-02"), CertifiedAt: c.CertifiedAt,
		})
	}
	return p
}

var identityKeys = map[string]bool{
	"id": true, "userId": true, "createdAt": true, "updatedAt": true,
	"flightId": true, "licenseId": true, "exportedAt": true, "signatureId": true,
	"user_id": true,
}

// normalize reduces an exported payload to what must survive a restore:
// identifiers and timestamps are dropped, links are reduced to their presence.
func normalize(t *testing.T, p *Payload) any {
	t.Helper()
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	var doc any
	require.NoError(t, json.Unmarshal(raw, &doc))
	return scrub(doc)
}

func scrub(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range x {
			switch {
			case identityKeys[k]:
			case k == "contactId" || k == "logbookLicenseId":
				out[k+"Present"] = true
			default:
				out[k] = scrub(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = scrub(val)
		}
		return out
	}
	return v
}

func TestRestore_ExportRestoreExportRoundTrip(t *testing.T) {
	source := fullPayload()
	want := normalize(t, source)

	store := restorefake.New()
	res, err := NewRestorer(store).Restore(context.Background(), uuid.New(), source, RestoreModeRestore)
	require.NoError(t, err)
	require.Equal(t, 0, res.ContactsCreated)

	got := normalize(t, payloadFromState(store.State))

	assert.Equal(t, want, got)
}

func TestRestore_EveryFlightFieldSurvives(t *testing.T) {
	source := fullPayload()
	store := restorefake.New()
	_, err := NewRestorer(store).Restore(context.Background(), uuid.New(), source, RestoreModeRestore)
	require.NoError(t, err)

	want, got := *source.Flights[0], *store.State.Flights[0]
	want.ID, got.ID = uuid.Nil, uuid.Nil
	want.UserID, got.UserID = uuid.Nil, uuid.Nil
	want.CrewMembers, got.CrewMembers = nil, nil
	assert.Equal(t, want, got)
	assert.True(t, got.TakeoffsDayOverride && got.NightTimeOverride && got.CrossCountryTimeOverride && got.SICTimeOverride)
}
