package cloudbackup

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/testutil/restorefake"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

func validRuleBody() models.CustomCurrencyRuleBody {
	return models.CustomCurrencyRuleBody{
		Window:       models.CurrencyWindow{Amount: 90, Unit: "days"},
		Requirements: []models.CurrencyRequirement{{Metric: "landings", Min: 3}},
	}
}

func invalidRuleBody() models.CustomCurrencyRuleBody {
	return models.CustomCurrencyRuleBody{
		Window:       models.CurrencyWindow{Amount: 90, Unit: "fortnights"},
		Requirements: []models.CurrencyRequirement{{Metric: "landings", Min: 3}},
	}
}

func validReportDef() models.CustomReportDefinition {
	return models.CustomReportDefinition{
		Window:  models.CustomReportWindow{Kind: models.ReportWindowAll},
		GroupBy: models.ReportGroupYear,
		Metric:  models.ReportMetrics[0],
	}
}

func sampleFlight(day int, reg string) *models.Flight {
	return &models.Flight{
		ID:           uuid.New(),
		Date:         time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC),
		AircraftReg:  reg,
		AircraftType: "C172",
		TotalTime:    60,
		PICTime:      60,
		IsPIC:        true,
		LandingsDay:  1,
		AllLandings:  1,
	}
}

func samplePayload() *Payload {
	licenseID := uuid.New()
	flight := sampleFlight(1, "D-EABC")
	flight.CrewMembers = []models.FlightCrewMember{{Name: "Sam Carter", Role: models.CrewRoleInstructor}}
	return &Payload{
		Format:  PayloadFormat,
		Version: PayloadVersion,
		Flights: []*models.Flight{flight, sampleFlight(2, "D-EABC")},
		Aircraft: []*models.Aircraft{{
			Registration: "D-EABC", Type: "C172", Make: "Cessna", Model: "172S", IsActive: true,
		}},
		Licenses: []LicenseWithRatings{{
			License: &models.License{
				ID: licenseID, RegulatoryAuthority: "EASA", LicenseType: "PPL", LicenseNumber: "PPL-1",
				IssueDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), IssuingAuthority: "EASA",
			},
			ClassRatings: []*models.ClassRating{{
				ClassType: models.ClassTypeSEPLand, IssueDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			}},
		}},
		Credentials: []*models.Credential{{
			CredentialType: models.CredentialTypeEASAClass2Medical,
			IssueDate:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), IssuingAuthority: "AME",
		}},
		Contacts: []*models.Contact{{ID: uuid.New(), Name: "Sam Carter"}},
		CustomCurrencyRules: []CustomCurrencyRule{
			{Name: "Night", Definition: validRuleBody(), Enabled: true, Notify: true},
		},
		CustomReports: []CustomReport{{Name: "By year", Definition: validReportDef()}},
		NotificationPreferences: &NotificationPreferences{
			EmailEnabled: true, EnabledCategories: []string{"rating_expiry"}, WarningDays: []int64{30}, CheckHour: 8,
		},
		FlightBaseline: &FlightBaseline{BaselineDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalMinutes: 600},
	}
}

func restoreSample(t *testing.T, store *restorefake.Store, p *Payload, mode RestoreMode) (*RestoreResult, error) {
	t.Helper()
	return NewRestorer(store).Restore(context.Background(), uuid.New(), p, mode)
}

func TestRestore_EmptyAccount(t *testing.T) {
	store := restorefake.New()
	userID := uuid.New()
	p := samplePayload()
	oldFlightID := p.Flights[0].ID
	signed := uuid.New()
	p.Flights[0].SignatureID = &signed

	res, err := NewRestorer(store).Restore(context.Background(), userID, p, "")
	require.NoError(t, err)

	assert.Equal(t, RestoreModeRestore, res.Mode)
	assert.Equal(t, 1, res.AircraftImported)
	assert.Equal(t, 1, res.LicensesImported)
	assert.Equal(t, 1, res.ClassRatingsImported)
	assert.Equal(t, 1, res.CredentialsImported)
	assert.Equal(t, 2, res.FlightsImported)
	assert.Equal(t, 1, res.CrewMembersImported)
	assert.Equal(t, 1, res.ContactsImported)
	assert.Equal(t, 0, res.ContactsCreated)
	assert.Equal(t, 1, res.CustomCurrencyRulesImported)
	assert.Equal(t, 1, res.CustomReportsImported)
	assert.True(t, res.NotificationPreferencesImported)
	assert.True(t, res.FlightBaselineImported)
	assert.Empty(t, res.Warnings)
	assert.NotNil(t, res.Warnings)
	assert.Equal(t, 1, store.Commits)

	st := store.State
	require.Len(t, st.Flights, 2)
	assert.Equal(t, oldFlightID, st.Flights[0].ID, "a flight keeps its id when nothing holds it")
	assert.Equal(t, userID, st.Flights[0].UserID)
	assert.Nil(t, st.Flights[0].SignatureID)
	assert.Nil(t, st.Flights[0].CrewMembers)
	require.Len(t, st.Crew, 1)
	assert.Equal(t, st.Flights[0].ID, st.Crew[0].FlightID)
	require.NotNil(t, st.Crew[0].ContactID)
	assert.Equal(t, st.Contacts[0].ID, *st.Crew[0].ContactID)
	assert.Equal(t, st.Licenses[0].ID, st.Ratings[0].LicenseID)
	assert.Equal(t, userID, st.Prefs.UserID)
	assert.Equal(t, userID, st.Baseline.UserID)
}

func TestRestore_NonEmptyAccountIsRefusedAndNothingIsWritten(t *testing.T) {
	seeds := map[string]func(*restorefake.State){
		"flights":     func(s *restorefake.State) { s.Flights = []*models.Flight{sampleFlight(1, "D-EXXX")} },
		"aircraft":    func(s *restorefake.State) { s.Aircraft = []*models.Aircraft{{Registration: "D-EXXX"}} },
		"licenses":    func(s *restorefake.State) { s.Licenses = []*models.License{{ID: uuid.New()}} },
		"credentials": func(s *restorefake.State) { s.Credentials = []*models.Credential{{ID: uuid.New()}} },
		"contacts":    func(s *restorefake.State) { s.Contacts = []*models.Contact{{ID: uuid.New(), Name: "X"}} },
		"rules":       func(s *restorefake.State) { s.Rules = []*models.CustomCurrencyRule{{ID: uuid.New()}} },
		"reports":     func(s *restorefake.State) { s.Reports = []*models.CustomReport{{ID: uuid.New()}} },
	}
	for name, seed := range seeds {
		t.Run(name, func(t *testing.T) {
			store := restorefake.New()
			seed(&store.State)
			before := fmt.Sprintf("%+v", store.State)

			_, err := restoreSample(t, store, samplePayload(), RestoreModeRestore)

			require.ErrorIs(t, err, ErrAccountNotEmpty)
			assert.Equal(t, before, fmt.Sprintf("%+v", store.State))
			assert.Equal(t, 0, store.Commits)
		})
	}
}

func TestRestore_SingleRowSettingsDoNotMakeAnAccountNonEmpty(t *testing.T) {
	store := restorefake.New()
	store.State.Prefs = &models.NotificationPreferences{CheckHour: 3}
	store.State.Baseline = &models.FlightBaseline{TotalMinutes: 1}

	res, err := restoreSample(t, store, samplePayload(), RestoreModeRestore)

	require.NoError(t, err)
	assert.True(t, res.NotificationPreferencesImported)
	assert.Equal(t, 8, store.State.Prefs.CheckHour)
	assert.Equal(t, 600, store.State.Baseline.TotalMinutes)
}

func TestRestore_ReplaceWipesAndRestoresAtomically(t *testing.T) {
	store := restorefake.New()
	store.State.Flights = []*models.Flight{sampleFlight(9, "D-EOLD")}
	store.State.Aircraft = []*models.Aircraft{{Registration: "D-EOLD"}}
	store.State.Contacts = []*models.Contact{{ID: uuid.New(), Name: "Old Contact"}}
	store.State.Rules = []*models.CustomCurrencyRule{{ID: uuid.New(), Name: "old"}}
	store.State.Reports = []*models.CustomReport{{ID: uuid.New(), Name: "old"}}

	res, err := restoreSample(t, store, samplePayload(), RestoreModeReplace)

	require.NoError(t, err)
	assert.Equal(t, RestoreModeReplace, res.Mode)
	require.Len(t, store.State.Flights, 2)
	require.Len(t, store.State.Aircraft, 1)
	assert.Equal(t, "D-EABC", store.State.Aircraft[0].Registration)
	require.Len(t, store.State.Contacts, 1)
	assert.Equal(t, "Sam Carter", store.State.Contacts[0].Name)
	require.Len(t, store.State.Rules, 1)
	assert.Equal(t, "Night", store.State.Rules[0].Name)
	require.Len(t, store.State.Reports, 1)
	assert.Equal(t, "By year", store.State.Reports[0].Name)
	assert.Equal(t, 1, store.Commits)
}

func TestRestore_ReplaceIsRefusedWhileAttachmentsExist(t *testing.T) {
	cases := map[string]repository.RestoreAttachments{
		"document files": {DocumentFiles: true},
		"signed flights": {SignedFlights: true},
	}
	for name, attachments := range cases {
		t.Run(name, func(t *testing.T) {
			store := restorefake.New()
			store.Attachments = attachments
			store.State.Flights = []*models.Flight{sampleFlight(9, "D-EOLD")}
			var wiped bool
			store.FailAt = func(op string, n int) error {
				wiped = wiped || op == restorefake.OpWipe
				return nil
			}

			_, err := restoreSample(t, store, samplePayload(), RestoreModeReplace)

			require.ErrorIs(t, err, ErrReplaceWouldDropAttachments)
			assert.False(t, wiped)
			assert.Len(t, store.State.Flights, 1)
			assert.Equal(t, "D-EOLD", store.State.Flights[0].AircraftReg)
		})
	}
}

func TestRestore_FailureRollsEverythingBack(t *testing.T) {
	ops := []string{
		restorefake.OpWipe, restorefake.OpAircraft, restorefake.OpLicense, restorefake.OpClassRating,
		restorefake.OpCredential, restorefake.OpContacts, restorefake.OpFlights, restorefake.OpCrew,
		restorefake.OpRule, restorefake.OpReport, restorefake.OpPrefs, restorefake.OpBaseline,
	}
	for _, op := range ops {
		t.Run(op, func(t *testing.T) {
			store := restorefake.New()
			store.State.Flights = []*models.Flight{sampleFlight(9, "D-EOLD")}
			store.State.Aircraft = []*models.Aircraft{{Registration: "D-EOLD"}}
			store.State.Prefs = &models.NotificationPreferences{CheckHour: 3}
			before := fmt.Sprintf("%+v", store.State)
			boom := errors.New("pq: connection reset by peer")
			store.FailAt = func(got string, n int) error {
				if got == op {
					return boom
				}
				return nil
			}

			_, err := restoreSample(t, store, samplePayload(), RestoreModeReplace)

			require.ErrorIs(t, err, boom)
			assert.Equal(t, before, fmt.Sprintf("%+v", store.State))
			assert.Equal(t, 1, store.Rollbacks)
			assert.Equal(t, 0, store.Commits)
		})
	}
}

func TestRestore_FailureOnLaterRowRollsBack(t *testing.T) {
	store := restorefake.New()
	p := samplePayload()
	p.Aircraft = nil
	for i := 0; i < 5; i++ {
		p.Aircraft = append(p.Aircraft, &models.Aircraft{
			Registration: fmt.Sprintf("D-E%03d", i), Type: "C172", Make: "Cessna", Model: "172",
		})
	}
	store.FailAt = func(op string, n int) error {
		if op == restorefake.OpAircraft && n == 3 {
			return errors.New("injected")
		}
		return nil
	}

	_, err := restoreSample(t, store, p, RestoreModeRestore)

	require.Error(t, err)
	assert.Empty(t, store.State.Aircraft)
	assert.Empty(t, store.State.Flights)
}

func TestRestore_SecondRestoreAfterReplaceYieldsOneCopy(t *testing.T) {
	store := restorefake.New()

	_, err := restoreSample(t, store, samplePayload(), RestoreModeRestore)
	require.NoError(t, err)

	_, err = restoreSample(t, store, samplePayload(), RestoreModeRestore)
	require.ErrorIs(t, err, ErrAccountNotEmpty)
	assert.Len(t, store.State.Flights, 2)

	_, err = restoreSample(t, store, samplePayload(), RestoreModeReplace)
	require.NoError(t, err)
	assert.Len(t, store.State.Flights, 2)
	assert.Len(t, store.State.Aircraft, 1)
	assert.Len(t, store.State.Licenses, 1)
	assert.Len(t, store.State.Contacts, 1)
	assert.Len(t, store.State.Rules, 1)
	assert.Len(t, store.State.Reports, 1)
}

func TestRestore_InvalidCustomRuleIsRestoredPausedWithWarning(t *testing.T) {
	store := restorefake.New()
	p := samplePayload()
	p.CustomCurrencyRules = []CustomCurrencyRule{
		{Name: "Good", Definition: validRuleBody(), Enabled: true, Notify: true},
		{Name: "Stale", Definition: invalidRuleBody(), Enabled: true, Notify: true},
	}

	res, err := restoreSample(t, store, p, RestoreModeRestore)

	require.NoError(t, err)
	assert.Equal(t, 2, res.CustomCurrencyRulesImported)
	require.Len(t, res.Warnings, 1)
	assert.Equal(t, "customCurrencyRules", res.Warnings[0].Section)
	assert.Equal(t, 1, res.Warnings[0].Index)
	assert.NotEmpty(t, res.Warnings[0].Message)

	good, stale := store.State.Rules[0], store.State.Rules[1]
	assert.True(t, good.Enabled)
	assert.True(t, good.Notify)
	assert.False(t, stale.Enabled)
	assert.False(t, stale.Notify)
	assert.Equal(t, "fortnights", stale.Definition.Window.Unit)
	assert.False(t, stale.IsShared)
	assert.Nil(t, stale.ShareToken)
}

func TestRestore_InvalidCustomReportIsRestoredWithWarning(t *testing.T) {
	store := restorefake.New()
	p := samplePayload()
	bad := validReportDef()
	bad.GroupBy = "fortnight"
	p.CustomReports = []CustomReport{{Name: "Stale", Definition: bad}}

	res, err := restoreSample(t, store, p, RestoreModeRestore)

	require.NoError(t, err)
	assert.Equal(t, 1, res.CustomReportsImported)
	require.Len(t, res.Warnings, 1)
	assert.Equal(t, "customReports", res.Warnings[0].Section)
	assert.Equal(t, "fortnight", store.State.Reports[0].Definition.GroupBy)
}

func TestRestore_NoPerAccountRuleLimitApplies(t *testing.T) {
	store := restorefake.New()
	p := samplePayload()
	p.CustomCurrencyRules = nil
	for i := 0; i < 300; i++ {
		p.CustomCurrencyRules = append(p.CustomCurrencyRules, CustomCurrencyRule{
			Name: fmt.Sprintf("Rule %d", i), Definition: validRuleBody(), Enabled: true,
		})
	}
	p.CustomReports = nil
	for i := 0; i < 150; i++ {
		p.CustomReports = append(p.CustomReports, CustomReport{Name: fmt.Sprintf("Report %d", i), Definition: validReportDef()})
	}

	res, err := restoreSample(t, store, p, RestoreModeRestore)

	require.NoError(t, err)
	assert.Equal(t, 300, res.CustomCurrencyRulesImported)
	assert.Equal(t, 150, res.CustomReportsImported)
	assert.Empty(t, res.Warnings)
}

func TestRestore_RuleThatCannotBeStoredStopsTheRestore(t *testing.T) {
	store := restorefake.New()
	p := samplePayload()
	p.CustomCurrencyRules = []CustomCurrencyRule{{Name: "   ", Definition: validRuleBody()}}

	_, err := restoreSample(t, store, p, RestoreModeRestore)

	var invalid *InvalidBackupError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, 0, store.Commits)
	assert.Empty(t, store.State.Flights)
}

func TestRestore_ReportLicenseScopeFollowsRestoredLicense(t *testing.T) {
	store := restorefake.New()
	p := samplePayload()
	known := p.Licenses[0].License.ID
	unknown := uuid.New()
	scoped, orphan := validReportDef(), validReportDef()
	scoped.Filter.LogbookLicenseID = &known
	orphan.Filter.LogbookLicenseID = &unknown
	p.CustomReports = []CustomReport{{Name: "Scoped", Definition: scoped}, {Name: "Orphan", Definition: orphan}}

	_, err := restoreSample(t, store, p, RestoreModeRestore)

	require.NoError(t, err)
	require.NotNil(t, store.State.Reports[0].Definition.Filter.LogbookLicenseID)
	assert.Equal(t, store.State.Licenses[0].ID, *store.State.Reports[0].Definition.Filter.LogbookLicenseID)
	assert.Nil(t, store.State.Reports[1].Definition.Filter.LogbookLicenseID)
}

func TestRestore_DuplicatesWithinTheBackupAreSkipped(t *testing.T) {
	store := restorefake.New()
	p := samplePayload()
	p.Aircraft = append(p.Aircraft, &models.Aircraft{Registration: "d-eabc", Type: "C172", Make: "Cessna", Model: "172"})
	p.Contacts = append(p.Contacts, &models.Contact{ID: uuid.New(), Name: "  SAM carter "})

	res, err := restoreSample(t, store, p, RestoreModeRestore)

	require.NoError(t, err)
	assert.Equal(t, 1, res.AircraftImported)
	assert.Equal(t, 1, res.AircraftSkipped)
	assert.Equal(t, 1, res.ContactsImported)
	assert.Equal(t, 1, res.ContactsSkipped)
}

func TestRestore_CrewNameWithoutContactCreatesOne(t *testing.T) {
	store := restorefake.New()
	p := samplePayload()
	p.Flights[1].CrewMembers = []models.FlightCrewMember{
		{Name: "New Person", Role: models.CrewRoleStudent},
		{Name: "new person", Role: models.CrewRoleSIC},
		{Name: "", Role: models.CrewRolePassenger},
	}

	res, err := restoreSample(t, store, p, RestoreModeRestore)

	require.NoError(t, err)
	assert.Equal(t, 1, res.ContactsCreated)
	assert.Equal(t, 4, res.CrewMembersImported)
	assert.Len(t, store.State.Contacts, 2)
	require.NotNil(t, store.State.Crew[1].ContactID)
	require.NotNil(t, store.State.Crew[2].ContactID)
	assert.Equal(t, *store.State.Crew[1].ContactID, *store.State.Crew[2].ContactID)
	assert.Nil(t, store.State.Crew[3].ContactID)
}

func TestRestore_FlightFailingRevalidationIsRestoredWithWarning(t *testing.T) {
	store := restorefake.New()
	p := samplePayload()
	p.Flights[1].IsDual = true

	res, err := restoreSample(t, store, p, RestoreModeRestore)

	require.NoError(t, err)
	assert.Equal(t, 2, res.FlightsImported)
	require.Len(t, res.Warnings, 1)
	assert.Equal(t, "flights", res.Warnings[0].Section)
	assert.Equal(t, 1, res.Warnings[0].Index)
}

func TestRestore_WarningsAreCapped(t *testing.T) {
	store := restorefake.New()
	p := samplePayload()
	p.CustomCurrencyRules = nil
	for i := 0; i < maxRestoreWarnings+5; i++ {
		p.CustomCurrencyRules = append(p.CustomCurrencyRules, CustomCurrencyRule{Name: fmt.Sprintf("r%d", i), Definition: invalidRuleBody()})
	}
	p.CustomCurrencyRules = p.CustomCurrencyRules[:maxRestoreWarnings+5]

	res, err := restoreSample(t, store, p, RestoreModeRestore)

	require.NoError(t, err)
	require.Len(t, res.Warnings, maxRestoreWarnings+1)
	assert.Equal(t, "warnings", res.Warnings[maxRestoreWarnings].Section)
	assert.Contains(t, res.Warnings[maxRestoreWarnings].Message, "5 further warnings")
}

func TestRestore_Version(t *testing.T) {
	cases := []struct {
		version string
		wantErr error
		invalid bool
	}{
		{version: ""},
		{version: "1"},
		{version: "1.0"},
		{version: "1.9"},
		{version: "2.0", wantErr: ErrUnsupportedBackupVersion},
		{version: "10.1", wantErr: ErrUnsupportedBackupVersion},
		{version: "v1", invalid: true},
		{version: "-1.0", invalid: true},
	}
	for _, tc := range cases {
		t.Run("version "+tc.version, func(t *testing.T) {
			store := restorefake.New()
			p := samplePayload()
			p.Version = tc.version

			_, err := restoreSample(t, store, p, RestoreModeRestore)

			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
				assert.Equal(t, 0, store.Commits)
			case tc.invalid:
				var invalid *InvalidBackupError
				require.ErrorAs(t, err, &invalid)
			default:
				require.NoError(t, err)
			}
		})
	}
}

func TestRestore_RejectsWrongFormatAndUnknownMode(t *testing.T) {
	store := restorefake.New()
	p := samplePayload()
	p.Format = "Something Else"

	_, err := restoreSample(t, store, p, RestoreModeRestore)
	var invalid *InvalidBackupError
	require.ErrorAs(t, err, &invalid)
	assert.Contains(t, invalid.Message, PayloadFormat)

	_, err = restoreSample(t, store, samplePayload(), RestoreMode("merge"))
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, 0, store.Commits)
}

func TestRestore_SectionCaps(t *testing.T) {
	cases := map[string]func(*Payload){
		"flights":     func(p *Payload) { p.Flights = make([]*models.Flight, MaxRestoreFlights+1) },
		"contacts":    func(p *Payload) { p.Contacts = make([]*models.Contact, MaxRestoreContacts+1) },
		"aircraft":    func(p *Payload) { p.Aircraft = make([]*models.Aircraft, MaxRestoreEntities+1) },
		"licenses":    func(p *Payload) { p.Licenses = make([]LicenseWithRatings, MaxRestoreEntities+1) },
		"credentials": func(p *Payload) { p.Credentials = make([]*models.Credential, MaxRestoreEntities+1) },
		"rules":       func(p *Payload) { p.CustomCurrencyRules = make([]CustomCurrencyRule, MaxRestoreEntities+1) },
		"reports":     func(p *Payload) { p.CustomReports = make([]CustomReport, MaxRestoreEntities+1) },
	}
	for name, grow := range cases {
		t.Run(name, func(t *testing.T) {
			store := restorefake.New()
			p := &Payload{Format: PayloadFormat, Version: PayloadVersion}
			grow(p)

			_, err := restoreSample(t, store, p, RestoreModeRestore)

			var invalid *InvalidBackupError
			require.ErrorAs(t, err, &invalid)
			assert.Contains(t, invalid.Message, "too many")
			assert.Equal(t, 0, store.Commits)
		})
	}
}

func TestRestore_FlightCapAllowsOneHundredThousand(t *testing.T) {
	assert.Equal(t, 100000, MaxRestoreFlights)
	store := restorefake.New()
	p := &Payload{Format: PayloadFormat, Version: PayloadVersion}
	p.Flights = make([]*models.Flight, MaxRestoreFlights)
	template := sampleFlight(1, "D-EABC")
	for i := range p.Flights {
		f := *template
		p.Flights[i] = &f
	}

	res, err := restoreSample(t, store, p, RestoreModeRestore)

	require.NoError(t, err)
	assert.Equal(t, MaxRestoreFlights, res.FlightsImported)
}

func TestRestore_InvalidEntriesNameTheSectionWithoutInternals(t *testing.T) {
	cases := map[string]func(*Payload){
		"null flight":      func(p *Payload) { p.Flights = append(p.Flights, nil) },
		"bad crew role":    func(p *Payload) { p.Flights[0].CrewMembers[0].Role = "Captain" },
		"unknown class":    func(p *Payload) { p.Licenses[0].ClassRatings[0].ClassType = "XYZ" },
		"license missing":  func(p *Payload) { p.Licenses[0].License = nil },
		"unnamed contact":  func(p *Payload) { p.Contacts[0].Name = " " },
		"overlong remarks": func(p *Payload) { p.Flights[0].Remarks = ptr(string(make([]byte, 100000))) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			store := restorefake.New()
			p := samplePayload()
			mutate(p)

			_, err := restoreSample(t, store, p, RestoreModeRestore)

			var invalid *InvalidBackupError
			require.ErrorAs(t, err, &invalid)
			assert.Contains(t, invalid.Message, "Invalid entry")
			assert.NotContains(t, invalid.Message, "pq:")
			assert.Equal(t, 0, store.Commits)
			assert.Empty(t, store.State.Flights)
		})
	}
}
