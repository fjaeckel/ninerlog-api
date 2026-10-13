package currency

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// actFlight is a flight in the fake activity provider.
type actFlight struct {
	date         time.Time
	class        models.ClassType
	ulKind       models.ULKind
	tow          bool
	pic          int
	ifr          int
	dualGiven    int
	dual         int
	xc           int
	launches     int
	landings     int
	distanceNM   float64
	launchMethod string
}

// activityProvider is mockFlightDataProvider plus GetActivityDays over flights.
type activityProvider struct {
	*mockFlightDataProvider
	flights []actFlight
	err     error
}

func newActivityProvider(flights ...actFlight) *activityProvider {
	return &activityProvider{mockFlightDataProvider: newMockFlightDataProvider(), flights: flights}
}

func (a *activityProvider) matches(f actFlight, q ActivityQuery) bool {
	classOK := len(q.Classes) == 0 && len(q.ULKinds) == 0
	for _, c := range q.Classes {
		classOK = classOK || f.class == c
	}
	for _, k := range q.ULKinds {
		classOK = classOK || (f.class == models.ClassTypeUL && f.ulKind == k)
	}
	switch {
	case !classOK,
		q.ExcludeUL && f.class == models.ClassTypeUL,
		q.TowOnly && !f.tow,
		q.IFROnly && f.ifr == 0,
		q.PICOnly && f.pic == 0,
		q.DualGivenOnly && f.dualGiven == 0,
		q.DualReceivedOnly && f.dual == 0,
		q.CrossCountryOnly && f.xc == 0,
		q.LaunchMethod != "" && f.launchMethod != q.LaunchMethod:
		return false
	}
	return true
}

func (a *activityProvider) GetActivityDays(_ context.Context, _ uuid.UUID, q ActivityQuery, since time.Time) ([]ActivityDay, error) {
	if a.err != nil {
		return nil, a.err
	}
	byDate := map[time.Time]*ActivityDay{}
	for _, f := range a.flights {
		if midnightUTC(f.date).Before(since) || !a.matches(f, q) {
			continue
		}
		d := midnightUTC(f.date)
		day, ok := byDate[d]
		if !ok {
			day = &ActivityDay{Date: d}
			byDate[d] = day
		}
		launches := f.launches
		if launches == 0 {
			launches = 1
		}
		day.Flights++
		day.PICMinutes += f.pic
		day.IFRMinutes += f.ifr
		day.DualGivenMinutes += f.dualGiven
		day.Launches += launches
		if f.landings >= 2 {
			day.MultiLandingFlights++
		}
		day.DistanceNM += f.distanceNM
	}
	out := make([]ActivityDay, 0, len(byDate))
	for _, d := range byDate {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out, nil
}

var privNow = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

func daysAgo(n int) time.Time { return privNow.AddDate(0, 0, -n) }

func privCtx() context.Context { return withNow(context.Background(), privNow) }

func strp(s string) *string { return &s }

func tows(n int, class models.ClassType, age int) []actFlight {
	var out []actFlight
	for i := 0; i < n; i++ {
		out = append(out, actFlight{date: daysAgo(age + i), class: class, tow: true, pic: 10})
	}
	return out
}

func TestEvaluatePrivilege_Recency(t *testing.T) {
	easaPPL := &models.License{UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL(A)"}
	spl := &models.License{UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "SPL"}
	faa := &models.License{UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "Private"}
	dulv := &models.License{UserID: uuid.New(), RegulatoryAuthority: "DULV", LicenseType: "UL"}
	expired := daysAgo(1)
	future := daysAgo(-200)

	tests := []struct {
		name        string
		privilege   models.LicencePrivilege
		license     *models.License
		flights     []actFlight
		wantStatus  Status
		wantKey     string
		wantRule    string
		check       func(t *testing.T, r PrivilegeCurrency)
		providerErr bool
	}{
		{
			name:       "P job 3 Petra towing 5 tows current",
			privilege:  models.LicencePrivilege{Kind: models.PrivilegeSailplaneTowing},
			license:    easaPPL,
			flights:    tows(5, models.ClassTypeSEPLand, 30),
			wantStatus: StatusCurrent, wantKey: MsgPrivilegeRecencyCurrent, wantRule: RuleSFCL205Towing,
			check: func(t *testing.T, r PrivilegeCurrency) {
				row := reqByKey(r.Requirements, ReqKeyTows)
				if row == nil || row.Current != 5 || row.Required != 5 || !row.Met {
					t.Fatalf("tows row = %+v", row)
				}
				want := midnightUTC(daysAgo(34)).AddDate(2, 0, -1).Format("2006-01-02")
				if row.ValidUntil == nil || *row.ValidUntil != want {
					t.Errorf("validUntil = %v, want %s", row.ValidUntil, want)
				}
			},
		},
		{
			name:       "P job 3 Petra towing 4 tows lapsed, missing tows with an instructor",
			privilege:  models.LicencePrivilege{Kind: models.PrivilegeSailplaneTowing},
			license:    easaPPL,
			flights:    append(tows(4, models.ClassTypeSEPLand, 30), tows(3, models.ClassTypeSEPLand, 800)...),
			wantStatus: StatusLapsed, wantKey: MsgPrivilegeRecencyNotMet, wantRule: RuleSFCL205Towing,
			check: func(t *testing.T, r PrivilegeCurrency) {
				row := reqByKey(r.Requirements, ReqKeyTows)
				if row.Current != 4 || row.RemedyKey != RemedyPrivilegeWithInstructor || *row.RemedyParams.Missing != 1 || *row.RemedyParams.Unit != "tows" {
					t.Errorf("tows row = %+v", row)
				}
			},
		},
		{
			name:       "a series tow row counts its take-offs",
			privilege:  models.LicencePrivilege{Kind: models.PrivilegeSailplaneTowing},
			license:    easaPPL,
			flights:    []actFlight{{date: daysAgo(3), class: models.ClassTypeSEPLand, tow: true, pic: 60, launches: 6}},
			wantStatus: StatusCurrent, wantKey: MsgPrivilegeRecencyCurrent, wantRule: RuleSFCL205Towing,
		},
		{
			name:       "tows in an ultralight tug do not count toward SFCL.205",
			privilege:  models.LicencePrivilege{Kind: models.PrivilegeSailplaneTowing},
			license:    easaPPL,
			flights:    tows(5, models.ClassTypeUL, 10),
			wantStatus: StatusLapsed, wantKey: MsgPrivilegeRecencyNotMet, wantRule: RuleSFCL205Towing,
		},
		{
			name:       "banner towing counts the same tow flag",
			privilege:  models.LicencePrivilege{Kind: models.PrivilegeBannerTowing},
			license:    easaPPL,
			flights:    tows(5, models.ClassTypeSEPLand, 10),
			wantStatus: StatusCurrent, wantKey: MsgPrivilegeRecencyCurrent, wantRule: RuleSFCL205BannerTowing,
		},
		{
			name:       "FAA 61.69 three tows current",
			privilege:  models.LicencePrivilege{Kind: models.PrivilegeSailplaneTowing},
			license:    faa,
			flights:    tows(3, models.ClassTypeSEPLand, 40),
			wantStatus: StatusCurrent, wantKey: MsgPrivilegeRecencyCurrent, wantRule: RuleFAA6169Towing,
			check: func(t *testing.T, r PrivilegeCurrency) {
				row := reqByKey(r.Requirements, ReqKeyTows)
				d := midnightUTC(daysAgo(42))
				want := time.Date(d.Year(), d.Month()+25, 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
				if row.ValidUntil == nil || *row.ValidUntil != want {
					t.Errorf("validUntil = %v, want end of the 24th calendar month %s", row.ValidUntil, want)
				}
			},
		},
		{
			name:      "FAA 61.69 three aerotows as glider PIC stand in for tows",
			privilege: models.LicencePrivilege{Kind: models.PrivilegeSailplaneTowing},
			license:   faa,
			flights: []actFlight{
				{date: daysAgo(5), class: models.ClassTypeGlider, pic: 30, launchMethod: "aerotow"},
				{date: daysAgo(6), class: models.ClassTypeGlider, pic: 30, launchMethod: "aerotow"},
				{date: daysAgo(7), class: models.ClassTypeGlider, pic: 30, launchMethod: "aerotow"},
				{date: daysAgo(8), class: models.ClassTypeGlider, pic: 30, launchMethod: "winch"},
			},
			wantStatus: StatusCurrent, wantKey: MsgPrivilegeRecencyCurrent, wantRule: RuleFAA6169Towing,
			check: func(t *testing.T, r PrivilegeCurrency) {
				if row := reqByKey(r.Requirements, ReqKeyTowedGliderFlights); row == nil || row.Current != 3 {
					t.Errorf("towed glider row = %+v", row)
				}
			},
		},
		{
			name:      "Petra cloud flying from IFR time on gliders",
			privilege: models.LicencePrivilege{Kind: models.PrivilegeCloudFlying},
			license:   spl,
			flights: []actFlight{
				{date: daysAgo(20), class: models.ClassTypeGlider, pic: 180, ifr: 35},
				{date: daysAgo(90), class: models.ClassTypeGlider, pic: 240, ifr: 30},
				{date: daysAgo(10), class: models.ClassTypeSEPLand, pic: 60, ifr: 60},
				{date: daysAgo(12), class: models.ClassTypeGlider, dual: 60, ifr: 60},
				{date: daysAgo(900), class: models.ClassTypeGlider, pic: 60, ifr: 60},
			},
			wantStatus: StatusCurrent, wantKey: MsgPrivilegeRecencyCurrent, wantRule: RuleSFCL215CloudFlying,
			check: func(t *testing.T, r PrivilegeCurrency) {
				tm := reqByKey(r.Requirements, ReqKeyCloudFlyingTime)
				fl := reqByKey(r.Requirements, ReqKeyCloudFlyingFlights)
				if tm.Current != 65 || !tm.Met || fl.Current != 2 || fl.Met {
					t.Errorf("time = %+v, flights = %+v", tm, fl)
				}
				if fl.RemedyKey != RemedyPrivilegeWithInstructor || *fl.RemedyParams.Missing != 3 {
					t.Errorf("flights remedy = %s %+v", fl.RemedyKey, fl.RemedyParams)
				}
			},
		},
		{
			name:       "cloud flying without IFR time on gliders lapses",
			privilege:  models.LicencePrivilege{Kind: models.PrivilegeCloudFlying},
			license:    spl,
			flights:    []actFlight{{date: daysAgo(20), class: models.ClassTypeGlider, pic: 180}},
			wantStatus: StatusLapsed, wantKey: MsgPrivilegeRecencyNotMet, wantRule: RuleSFCL215CloudFlying,
		},
		{
			name:      "P job 3 Petra FI(S) with 60 launches as instructor current",
			privilege: models.LicencePrivilege{Kind: models.PrivilegeFIS, ExpiresOn: &future},
			license:   spl,
			flights: []actFlight{
				{date: daysAgo(100), class: models.ClassTypeGlider, dualGiven: 300, launches: 40},
				{date: daysAgo(400), class: models.ClassTypeTMG, dualGiven: 120, launches: 20},
				{date: daysAgo(50), class: models.ClassTypeGlider, pic: 300, launches: 30},
			},
			wantStatus: StatusCurrent, wantKey: MsgPrivilegeRecencyCurrent, wantRule: RuleSFCL360FIS,
			check: func(t *testing.T, r PrivilegeCurrency) {
				if l := reqByKey(r.Requirements, ReqKeyInstructionLaunches); l.Current != 60 || !l.Met {
					t.Errorf("launches row = %+v", l)
				}
				if tm := reqByKey(r.Requirements, ReqKeyInstructionTime); tm.Current != 420 || tm.Met {
					t.Errorf("time row = %+v", tm)
				}
				ref := reqByKey(r.Requirements, ReqKeyFIRefresher)
				if ref == nil || ref.MessageKey != MsgRequirementUntracked || ref.RemedyKey != "" {
					t.Errorf("refresher row = %+v", ref)
				}
				if r.MessageParams == nil || *r.MessageParams.Date != future.Format("2006-01-02") {
					t.Errorf("messageParams = %+v, want the expiry date", r.MessageParams)
				}
			},
		},
		{
			name:       "FI(S) with 20 h of instruction lapses",
			privilege:  models.LicencePrivilege{Kind: models.PrivilegeFIS},
			license:    spl,
			flights:    []actFlight{{date: daysAgo(100), class: models.ClassTypeGlider, dualGiven: 1200, launches: 30}},
			wantStatus: StatusLapsed, wantKey: MsgPrivilegeRecencyNotMet, wantRule: RuleSFCL360FIS,
		},
		{
			name:       "FI(S) past its expiry date is expired",
			privilege:  models.LicencePrivilege{Kind: models.PrivilegeFIS, ExpiresOn: &expired},
			license:    spl,
			flights:    []actFlight{{date: daysAgo(100), class: models.ClassTypeGlider, dualGiven: 2000, launches: 80}},
			wantStatus: StatusExpired, wantKey: MsgPrivilegeExpired, wantRule: RuleSFCL360FIS,
		},
		{
			name:      "DULV UL towing 10 tows on three-axis current",
			privilege: models.LicencePrivilege{Kind: models.PrivilegeULTowing, Detail: strp("THREE_AXIS")},
			license:   dulv,
			flights: func() []actFlight {
				var out []actFlight
				for i := 0; i < 10; i++ {
					out = append(out, actFlight{date: daysAgo(10 + i), class: models.ClassTypeUL, ulKind: models.ULKindThreeAxis, tow: true, pic: 15})
				}
				return out
			}(),
			wantStatus: StatusCurrent, wantKey: MsgPrivilegeRecencyCurrent, wantRule: RuleDULVULTowing,
		},
		{
			name:      "DULV UL towing does not count weight-shift or aeroplane tows",
			privilege: models.LicencePrivilege{Kind: models.PrivilegeULTowing, Detail: strp("THREE_AXIS")},
			license:   dulv,
			flights: append(tows(10, models.ClassTypeSEPLand, 5),
				actFlight{date: daysAgo(3), class: models.ClassTypeUL, ulKind: models.ULKindWeightShift, tow: true, pic: 10}),
			wantStatus: StatusLapsed, wantKey: MsgPrivilegeRecencyNotMet, wantRule: RuleDULVULTowing,
			check: func(t *testing.T, r PrivilegeCurrency) {
				if row := reqByKey(r.Requirements, ReqKeyTows); row.Current != 0 || row.Required != 10 {
					t.Errorf("tows row = %+v", row)
				}
			},
		},
		{
			name:       "aerobatics without expiry is valid",
			privilege:  models.LicencePrivilege{Kind: models.PrivilegeAerobaticBasic},
			license:    spl,
			wantStatus: StatusCurrent, wantKey: MsgPrivilegeValid, wantRule: RulePrivilegeExpiry,
		},
		{
			name:       "TMG night past its expiry is expired",
			privilege:  models.LicencePrivilege{Kind: models.PrivilegeTMGNight, ExpiresOn: &expired},
			license:    spl,
			wantStatus: StatusExpired, wantKey: MsgPrivilegeExpired, wantRule: RulePrivilegeExpiry,
		},
		{
			name:       "launch method trained is valid",
			privilege:  models.LicencePrivilege{Kind: models.PrivilegeLaunchMethodTrained, Detail: strp("aerotow")},
			license:    spl,
			wantStatus: StatusCurrent, wantKey: MsgPrivilegeValid, wantRule: RuleSFCL155LaunchTrained,
		},
		{
			name:        "unreadable flight data is unknown",
			privilege:   models.LicencePrivilege{Kind: models.PrivilegeSailplaneTowing},
			license:     easaPPL,
			providerErr: true,
			wantStatus:  StatusUnknown, wantKey: MsgPrivilegeEvaluationFailed, wantRule: RuleSFCL205Towing,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dp := newActivityProvider(tc.flights...)
			if tc.providerErr {
				dp.err = context.DeadlineExceeded
			}
			p := tc.privilege
			p.ID = uuid.New()
			r := EvaluatePrivilege(privCtx(), &p, tc.license, dp)
			if r.Status != tc.wantStatus || r.MessageKey != tc.wantKey || r.RuleDescriptionKey != tc.wantRule {
				t.Fatalf("got %s/%s/%s, want %s/%s/%s (requirements %+v)", r.Status, r.MessageKey, r.RuleDescriptionKey,
					tc.wantStatus, tc.wantKey, tc.wantRule, r.Requirements)
			}
			if r.PrivilegeID != p.ID {
				t.Errorf("privilegeId = %s, want %s", r.PrivilegeID, p.ID)
			}
			if tc.check != nil {
				tc.check(t, r)
			}
		})
	}
}

type fakePrivilegeLister struct {
	privileges []*models.LicencePrivilege
	err        error
}

func (f *fakePrivilegeLister) ListByUser(_ context.Context, userID uuid.UUID) ([]*models.LicencePrivilege, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []*models.LicencePrivilege
	for _, p := range f.privileges {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	return out, nil
}

// privilegeService builds a service with one licence holding ratings and privileges.
func privilegeService(authority, licenceType string, issued time.Time, ratings []*models.ClassRating, kinds []models.LicencePrivilege, dp *activityProvider) (*Service, uuid.UUID, *models.License) {
	userID := uuid.New()
	licRepo := newMockLicenseRepo()
	lic := &models.License{UserID: userID, RegulatoryAuthority: authority, LicenseType: licenceType, IssueDate: issued}
	_ = licRepo.Create(context.Background(), lic)
	crRepo := newMockCRRepo()
	for _, r := range ratings {
		r.ID = uuid.New()
		r.LicenseID = lic.ID
	}
	crRepo.ratings[lic.ID] = ratings
	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())
	reg.Register(NewFAAEvaluator())
	ul := NewGermanULEvaluator()
	reg.RegisterMulti(ul, ul.Authorities()...)
	svc := NewService(reg, licRepo, crRepo, dp)
	lister := &fakePrivilegeLister{}
	for _, k := range kinds {
		p := k
		p.ID = uuid.New()
		p.UserID = userID
		p.LicenseID = lic.ID
		lister.privileges = append(lister.privileges, &p)
	}
	svc.SetPrivilegeSource(lister)
	return svc, userID, lic
}

func paxFor(resp *CurrencyStatusResponse, class models.ClassType) *PassengerCurrency {
	for i := range resp.PassengerCurrency {
		if resp.PassengerCurrency[i].ClassType == class {
			return &resp.PassengerCurrency[i]
		}
	}
	return nil
}

func threeAxis() *models.ULKind { k := models.ULKindThreeAxis; return &k }

func TestService_ULPassengerAuthorisation(t *testing.T) {
	xc := func(age, landings int, nm float64) actFlight {
		return actFlight{date: daysAgo(age), class: models.ClassTypeUL, ulKind: models.ULKindThreeAxis, dual: 60, xc: 60, landings: landings, distanceNM: nm}
	}
	paxDays := []LandingDay{{Date: midnightUTC(daysAgo(5)), DayLandings: 3, Takeoffs: 3}}

	t.Run("M §45a passengers need authorisation", func(t *testing.T) {
		dp := newActivityProvider(xc(400, 1, 30), xc(390, 2, 40), xc(380, 1, 20))
		dp.landingDaysByUL = map[models.ULKind][]LandingDay{models.ULKindThreeAxis: paxDays}
		svc, userID, _ := privilegeService("DULV", "UL", daysAgo(3000),
			[]*models.ClassRating{{ClassType: models.ClassTypeUL, ULKind: threeAxis()}}, nil, dp)
		resp, err := svc.EvaluateAsOf(context.Background(), userID, privNow)
		if err != nil {
			t.Fatal(err)
		}
		pax := paxFor(resp, models.ClassTypeUL)
		if pax == nil || pax.DayStatus != StatusUnknown || pax.MessageKey != MsgPaxULAuthorisationMissing {
			t.Fatalf("pax = %+v, want unknown with %s", pax, MsgPaxULAuthorisationMissing)
		}
		if pax.DayLandings != 3 {
			t.Errorf("dayLandings = %d, want 3", pax.DayLandings)
		}
		want := map[string]float64{ReqKeyULXCFlights: 3, ReqKeyULXCLandingFlights: 1, ReqKeyULXCDistance: 167}
		for key, cur := range want {
			r := reqByKey(pax.Requirements, key)
			if r == nil || r.Current != cur || r.Met {
				t.Errorf("%s = %+v, want %v unmet", key, r, cur)
			}
		}
		if len(resp.Privileges) != 0 {
			t.Errorf("privileges = %+v, want none", resp.Privileges)
		}
	})

	t.Run("M §45a with passenger authorisation is current", func(t *testing.T) {
		dp := newActivityProvider()
		dp.landingDaysByUL = map[models.ULKind][]LandingDay{models.ULKindThreeAxis: paxDays}
		svc, userID, _ := privilegeService("DULV", "UL", daysAgo(3000),
			[]*models.ClassRating{{ClassType: models.ClassTypeUL, ULKind: threeAxis()}},
			[]models.LicencePrivilege{{Kind: models.PrivilegeULPassengerAuth}}, dp)
		resp, _ := svc.EvaluateAsOf(context.Background(), userID, privNow)
		pax := paxFor(resp, models.ClassTypeUL)
		if pax.DayStatus != StatusCurrent || pax.MessageKey != MsgPaxCurrentDayNoNight || pax.Requirements != nil {
			t.Errorf("pax = %+v, want current without progress rows", pax)
		}
		if len(resp.Privileges) != 1 || resp.Privileges[0].Status != StatusCurrent || resp.Privileges[0].MessageKey != MsgPrivilegeValid {
			t.Errorf("privileges = %+v", resp.Privileges)
		}
	})

	t.Run("M §45a short of take-offs stays not current without authorisation", func(t *testing.T) {
		dp := newActivityProvider()
		dp.landingDaysByUL = map[models.ULKind][]LandingDay{models.ULKindThreeAxis: {{Date: midnightUTC(daysAgo(5)), DayLandings: 1, Takeoffs: 1}}}
		svc, userID, _ := privilegeService("DULV", "UL", daysAgo(3000),
			[]*models.ClassRating{{ClassType: models.ClassTypeUL, ULKind: threeAxis()}}, nil, dp)
		resp, _ := svc.EvaluateAsOf(context.Background(), userID, privNow)
		pax := paxFor(resp, models.ClassTypeUL)
		if pax.DayStatus != StatusExpired || pax.MessageKey != MsgPaxNotCurrent {
			t.Errorf("pax = %+v, want expired with %s", pax, MsgPaxNotCurrent)
		}
	})

	t.Run("an expired authorisation counts as missing", func(t *testing.T) {
		exp := daysAgo(2)
		dp := newActivityProvider()
		dp.landingDaysByUL = map[models.ULKind][]LandingDay{models.ULKindThreeAxis: paxDays}
		svc, userID, _ := privilegeService("DULV", "UL", daysAgo(3000),
			[]*models.ClassRating{{ClassType: models.ClassTypeUL, ULKind: threeAxis()}},
			[]models.LicencePrivilege{{Kind: models.PrivilegeULPassengerAuth, ExpiresOn: &exp}}, dp)
		resp, _ := svc.EvaluateAsOf(context.Background(), userID, privNow)
		if pax := paxFor(resp, models.ClassTypeUL); pax.MessageKey != MsgPaxULAuthorisationMissing {
			t.Errorf("messageKey = %s, want authorisation missing", pax.MessageKey)
		}
	})

	t.Run("without a privilege source the §45a result is unchanged", func(t *testing.T) {
		dp := newActivityProvider()
		dp.landingDaysByUL = map[models.ULKind][]LandingDay{models.ULKindThreeAxis: paxDays}
		svc, userID, _ := privilegeService("DULV", "UL", daysAgo(3000),
			[]*models.ClassRating{{ClassType: models.ClassTypeUL, ULKind: threeAxis()}}, nil, dp)
		svc.SetPrivilegeSource(nil)
		resp, _ := svc.EvaluateAsOf(context.Background(), userID, privNow)
		if pax := paxFor(resp, models.ClassTypeUL); pax.DayStatus != StatusCurrent || pax.MessageKey != MsgPaxCurrentPrivilegeSeparat {
			t.Errorf("pax = %+v", pax)
		}
	})

	t.Run("a failed privilege read leaves the §45a result unchanged", func(t *testing.T) {
		dp := newActivityProvider()
		dp.landingDaysByUL = map[models.ULKind][]LandingDay{models.ULKindThreeAxis: paxDays}
		svc, userID, _ := privilegeService("DULV", "UL", daysAgo(3000),
			[]*models.ClassRating{{ClassType: models.ClassTypeUL, ULKind: threeAxis()}}, nil, dp)
		svc.SetPrivilegeSource(&fakePrivilegeLister{err: context.DeadlineExceeded})
		resp, _ := svc.EvaluateAsOf(context.Background(), userID, privNow)
		if pax := paxFor(resp, models.ClassTypeUL); pax.DayStatus != StatusCurrent || resp.Privileges != nil {
			t.Errorf("pax = %+v, privileges = %+v", pax, resp.Privileges)
		}
	})
}

func TestService_SPLPassengerPrivileges(t *testing.T) {
	t.Run("K SFCL.160(e)(2) night passengers in a TMG with TMG night", func(t *testing.T) {
		dp := newActivityProvider()
		dp.landingDays = map[models.ClassType][]LandingDay{
			models.ClassTypeTMG: {
				{Date: midnightUTC(daysAgo(10)), DayLandings: 2, NightLandings: 1},
			},
		}
		svc, userID, _ := privilegeService("EASA", "SPL", daysAgo(3000),
			[]*models.ClassRating{{ClassType: models.ClassTypeTMG}},
			[]models.LicencePrivilege{{Kind: models.PrivilegeTMGNight}}, dp)
		resp, _ := svc.EvaluateAsOf(context.Background(), userID, privNow)
		pax := paxFor(resp, models.ClassTypeTMG)
		if !pax.NightPrivilege || pax.NightStatus != StatusCurrent || pax.MessageKey != MsgPaxCurrentDayNight {
			t.Fatalf("pax = %+v, want night current", pax)
		}
		want := midnightUTC(daysAgo(10)).AddDate(0, 0, 90).Format("2006-01-02")
		if pax.NightExpiresOn == nil || *pax.NightExpiresOn != want {
			t.Errorf("nightExpiresOn = %v, want %s", pax.NightExpiresOn, want)
		}
	})

	t.Run("K TMG night privilege without a night landing", func(t *testing.T) {
		dp := newActivityProvider()
		dp.landingDays = map[models.ClassType][]LandingDay{
			models.ClassTypeTMG: {{Date: midnightUTC(daysAgo(10)), DayLandings: 3}},
		}
		svc, userID, _ := privilegeService("EASA", "SPL", daysAgo(3000),
			[]*models.ClassRating{{ClassType: models.ClassTypeTMG}},
			[]models.LicencePrivilege{{Kind: models.PrivilegeTMGNight}}, dp)
		resp, _ := svc.EvaluateAsOf(context.Background(), userID, privNow)
		pax := paxFor(resp, models.ClassTypeTMG)
		if pax.NightStatus != StatusExpired || pax.MessageKey != MsgPaxDayCurrentNightNot {
			t.Errorf("pax = %+v, want night not current", pax)
		}
	})

	t.Run("K SPL TMG passengers without TMG night keep no night privilege", func(t *testing.T) {
		dp := newActivityProvider()
		dp.landingDays = map[models.ClassType][]LandingDay{
			models.ClassTypeTMG: {{Date: midnightUTC(daysAgo(10)), DayLandings: 2, NightLandings: 1}},
		}
		svc, userID, _ := privilegeService("EASA", "SPL", daysAgo(3000),
			[]*models.ClassRating{{ClassType: models.ClassTypeTMG}}, nil, dp)
		resp, _ := svc.EvaluateAsOf(context.Background(), userID, privNow)
		if pax := paxFor(resp, models.ClassTypeTMG); pax.NightPrivilege || pax.MessageKey != MsgPaxCurrentDayNoNight {
			t.Errorf("pax = %+v", pax)
		}
	})

	t.Run("L SFCL.115(a)(2) passenger prerequisites since licence issue", func(t *testing.T) {
		var flights []actFlight
		for i := 0; i < 32; i++ {
			flights = append(flights, actFlight{date: daysAgo(100 + i), class: models.ClassTypeGlider, pic: 10})
		}
		flights = append(flights, actFlight{date: daysAgo(5000), class: models.ClassTypeGlider, pic: 600, launches: 40})
		dp := newActivityProvider(flights...)
		dp.landingDays = map[models.ClassType][]LandingDay{
			models.ClassTypeGlider: {{Date: midnightUTC(daysAgo(10)), DayLandings: 3}},
		}
		svc, userID, _ := privilegeService("EASA", "SPL", daysAgo(1000),
			[]*models.ClassRating{{ClassType: models.ClassTypeGlider}}, nil, dp)
		resp, _ := svc.EvaluateAsOf(context.Background(), userID, privNow)
		pax := paxFor(resp, models.ClassTypeGlider)
		tm := reqByKey(pax.Requirements, ReqKeyPaxPrerequisiteTime)
		ln := reqByKey(pax.Requirements, ReqKeyPaxPrerequisiteLaunches)
		cf := reqByKey(pax.Requirements, ReqKeyPaxCompetenceFlight)
		if tm.Current != 320 || tm.Met || ln.Current != 32 || !ln.Met || cf == nil || cf.MessageKey != MsgRequirementUntracked {
			t.Errorf("rows = %+v", pax.Requirements)
		}
		if pax.DayStatus != StatusCurrent {
			t.Errorf("dayStatus = %s, want current (prerequisites are informational)", pax.DayStatus)
		}
	})
}

func TestService_TrainedLaunchMethods(t *testing.T) {
	dp := newActivityProvider()
	dp.launchCountsAllTime = map[string]int{"winch": 40}
	dp.launchCounts = map[string]int{"winch": 12}
	dp.progressByClass[models.ClassTypeTMG] = &Progress{Launches: 2}
	svc, userID, _ := privilegeService("EASA", "SPL", daysAgo(3000),
		[]*models.ClassRating{{ClassType: models.ClassTypeGlider}},
		[]models.LicencePrivilege{
			{Kind: models.PrivilegeLaunchMethodTrained, Detail: strp("winch")},
			{Kind: models.PrivilegeLaunchMethodTrained, Detail: strp("aerotow")},
			{Kind: models.PrivilegeLaunchMethodTrained, Detail: strp("self-launch")},
		}, dp)
	resp, err := svc.EvaluateAsOf(context.Background(), userID, privNow)
	if err != nil {
		t.Fatal(err)
	}
	var glider *ClassRatingCurrency
	for i := range resp.Ratings {
		if resp.Ratings[i].ClassType == models.ClassTypeGlider {
			glider = &resp.Ratings[i]
		}
	}
	type row struct {
		method   string
		launches int
		met      bool
		trained  bool
	}
	want := []row{{"winch", 12, true, true}, {"aerotow", 0, false, true}, {"self-launch", 2, false, true}}
	if len(glider.LaunchMethodCurrency) != len(want) {
		t.Fatalf("launch methods = %+v", glider.LaunchMethodCurrency)
	}
	for i, w := range want {
		lm := glider.LaunchMethodCurrency[i]
		if lm.Method != w.method || lm.Launches != w.launches || lm.Met != w.met || lm.Trained != w.trained {
			t.Errorf("L4 row %d = %+v, want %+v", i, lm, w)
		}
	}
	aerotow := glider.LaunchMethodCurrency[1]
	if aerotow.RemedyKey != RemedyLaunchMethodDual || *aerotow.RemedyParams.Missing != 5 {
		t.Errorf("aerotow remedy = %s %+v", aerotow.RemedyKey, aerotow.RemedyParams)
	}
	if len(resp.Privileges) != 3 {
		t.Errorf("privileges = %d, want 3", len(resp.Privileges))
	}
}

func TestEveryPrivilegeResultCarriesAKey(t *testing.T) {
	expired := daysAgo(1)
	for _, authority := range []string{"EASA", "FAA", "DULV"} {
		lic := &models.License{UserID: uuid.New(), RegulatoryAuthority: authority, LicenseType: "SPL"}
		for _, kind := range models.ValidLicencePrivilegeKinds() {
			for _, variant := range []string{"no flights", "expired", "provider error"} {
				t.Run(authority+" "+string(kind)+" "+variant, func(t *testing.T) {
					p := models.LicencePrivilege{ID: uuid.New(), Kind: kind}
					switch kind {
					case models.PrivilegeLaunchMethodTrained:
						p.Detail = strp("winch")
					case models.PrivilegeULTowing:
						p.Detail = strp("THREE_AXIS")
					case models.PrivilegeULTypeBriefing:
						p.Detail = strp("C42")
					}
					dp := newActivityProvider()
					switch variant {
					case "expired":
						p.ExpiresOn = &expired
					case "provider error":
						dp.err = context.Canceled
					}
					r := EvaluatePrivilege(privCtx(), &p, lic, dp)
					if !knownMessageKeys[r.MessageKey] {
						t.Errorf("messageKey %q not in the catalogue", r.MessageKey)
					}
					if r.RuleDescriptionKey == "" {
						t.Error("no ruleDescriptionKey")
					}
					for _, req := range r.Requirements {
						if !knownNameKeys[req.NameKey] || !knownMessageKeys[req.MessageKey] {
							t.Errorf("requirement %+v has an unknown key", req)
						}
						if req.RemedyKey != "" && !knownMessageKeys[req.RemedyKey] {
							t.Errorf("remedy %q not in the catalogue", req.RemedyKey)
						}
					}
				})
			}
		}
	}
}
