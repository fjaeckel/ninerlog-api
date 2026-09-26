package readiness

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/google/uuid"
)

type fakeCurrency struct {
	resp  *currency.CurrencyStatusResponse
	err   error
	asked time.Time
}

func (f *fakeCurrency) EvaluateAsOf(_ context.Context, _ uuid.UUID, date time.Time) (*currency.CurrencyStatusResponse, error) {
	f.asked = date
	return f.resp, f.err
}

type fakeAircraft []*models.Aircraft

func (f fakeAircraft) ListAircraft(context.Context, uuid.UUID) ([]*models.Aircraft, error) {
	return f, nil
}

type fakeCredentials []*models.Credential

func (f fakeCredentials) ListCredentials(context.Context, uuid.UUID) ([]*models.Credential, error) {
	return f, nil
}

var (
	userID   = uuid.New()
	now      = time.Date(2026, 9, 26, 9, 30, 0, 0, time.UTC)
	today    = time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	saturday = time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
)

func aircraft(reg string, class models.ClassType, kind *models.ULKind) *models.Aircraft {
	c := string(class)
	return &models.Aircraft{ID: uuid.New(), UserID: userID, Registration: reg, AircraftClass: &c, ULKind: kind}
}

func kind(k models.ULKind) *models.ULKind { return &k }

func rating(class models.ClassType, status currency.Status, ul *models.ULKind) currency.ClassRatingCurrency {
	return currency.ClassRatingCurrency{
		ClassRatingID: uuid.New(), LicenseID: uuid.New(), ClassType: class, ULKind: ul,
		Status: status, MessageKey: currency.MsgRatingRecencyCurrent,
	}
}

// fleet is Lena's glider, Mark's SEP, Mehmet's three-axis UL and Sabine's trike.
var fleet = fakeAircraft{
	aircraft("D-1234", models.ClassTypeGlider, nil),
	aircraft("D-EABC", models.ClassTypeSEPLand, nil),
	aircraft("D-MXYZ", models.ClassTypeUL, kind(models.ULKindThreeAxis)),
	aircraft("D-MTRK", models.ClassTypeUL, kind(models.ULKindWeightShift)),
}

func status() *currency.CurrencyStatusResponse {
	glider := rating(models.ClassTypeGlider, currency.StatusCurrent, nil)
	winchUntil := "2027-04-01"
	glider.LaunchMethodCurrency = []currency.LaunchMethodCurrency{
		{Method: "winch", Launches: 8, Required: 5, Met: true, ValidUntil: &winchUntil},
		{Method: "aerotow", Launches: 3, Required: 5, RemedyKey: currency.RemedyLaunchMethodDual,
			RemedyParams: &currency.MessageParams{Method: ptr("aerotow"), Missing: ptr(2)}},
	}
	sep := rating(models.ClassTypeSEPLand, currency.StatusExpiring, nil)
	sep.MessageKey = currency.MsgRatingRevalidationNotMet
	ir := rating(models.ClassTypeIR, currency.StatusCurrent, nil)
	ul := rating(models.ClassTypeUL, currency.StatusCurrent, kind(models.ULKindThreeAxis))
	trike := rating(models.ClassTypeUL, currency.StatusLapsed, kind(models.ULKindWeightShift))
	trike.MessageKey = currency.MsgRatingRecencyNotMet
	trike.Requirements = []currency.Requirement{
		{NameKey: currency.ReqKeyProficiencyCheck, RemedyKey: currency.RemedyProficiencyCheck},
		{NameKey: currency.ReqKeyPICTime, RemedyKey: currency.RemedyFlyMore,
			RemedyParams: &currency.MessageParams{Missing: ptr(120), Unit: ptr("minutes")}},
	}
	return &currency.CurrencyStatusResponse{
		Ratings: []currency.ClassRatingCurrency{glider, sep, ir, ul, trike},
		PassengerCurrency: []currency.PassengerCurrency{
			{ClassType: models.ClassTypeGlider, DayStatus: currency.StatusCurrent, MessageKey: currency.MsgPaxCurrentDayNoNight},
			{ClassType: models.ClassTypeSEPLand, DayStatus: currency.StatusExpired, MessageKey: currency.MsgPaxNotCurrent,
				MessageParams: &currency.MessageParams{Needed: ptr(2)}},
			{ClassType: models.ClassTypeUL, ULKind: kind(models.ULKindThreeAxis), DayStatus: currency.StatusCurrent,
				MessageKey: currency.MsgPaxCurrentPrivilegeSeparat},
		},
	}
}

func newService(cur *fakeCurrency, creds fakeCredentials) *Service {
	s := NewService(cur, fleet, creds)
	s.now = func() time.Time { return now }
	return s
}

type itemKey struct {
	kind   Kind
	class  models.ClassType
	method string
}

func keys(items []Item) []itemKey {
	var out []itemKey
	for _, it := range items {
		k := itemKey{kind: it.Kind}
		if it.ClassType != nil {
			k.class = *it.ClassType
		}
		if it.LaunchMethod != nil {
			k.method = *it.LaunchMethod
		}
		out = append(out, k)
	}
	return out
}

func TestEvaluate_AircraftFilter(t *testing.T) {
	tests := []struct {
		name       string
		reg        string
		passengers bool
		want       []itemKey
	}{
		{
			name: "L4 glider selects the glider rating and its launch methods", reg: "D-1234", passengers: true,
			want: []itemKey{
				{KindRating, models.ClassTypeGlider, ""},
				{KindLaunchMethod, models.ClassTypeGlider, "winch"},
				{KindLaunchMethod, models.ClassTypeGlider, "aerotow"},
				{KindPassengers, models.ClassTypeGlider, ""},
			},
		},
		{
			name: "A1 SEP selects the SEP rating only", reg: "d-eabc",
			want: []itemKey{{KindRating, models.ClassTypeSEPLand, ""}},
		},
		{
			name: "M1 three-axis UL selects the THREE_AXIS rating, not the trike", reg: "D-MXYZ", passengers: true,
			want: []itemKey{
				{KindRating, models.ClassTypeUL, ""},
				{KindPassengers, models.ClassTypeUL, ""},
			},
		},
		{
			name: "no aircraft answers every rating", passengers: false,
			want: []itemKey{
				{KindRating, models.ClassTypeGlider, ""},
				{KindRating, models.ClassTypeSEPLand, ""},
				{KindRating, models.ClassTypeIR, ""},
				{KindRating, models.ClassTypeUL, ""},
				{KindRating, models.ClassTypeUL, ""},
				{KindLaunchMethod, models.ClassTypeGlider, "winch"},
				{KindLaunchMethod, models.ClassTypeGlider, "aerotow"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newService(&fakeCurrency{resp: status()}, nil)
			rep, err := svc.Evaluate(context.Background(), userID, Request{AircraftReg: tt.reg, Passengers: tt.passengers})
			if err != nil {
				t.Fatal(err)
			}
			if got := keys(rep.Items); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("items = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEvaluate_Passengers(t *testing.T) {
	svc := newService(&fakeCurrency{resp: status()}, nil)
	rep, err := svc.Evaluate(context.Background(), userID, Request{Passengers: true})
	if err != nil {
		t.Fatal(err)
	}
	var pax []Item
	for _, it := range rep.Items {
		if it.Kind == KindPassengers {
			pax = append(pax, it)
		}
	}
	if len(pax) != 3 {
		t.Fatalf("passenger items = %d, want 3", len(pax))
	}
	if !pax[0].Ready || pax[0].ReasonKey != currency.MsgPaxCurrentDayNoNight {
		t.Errorf("glider passengers = %+v", pax[0])
	}
	if pax[1].Ready || pax[1].Status != "expired" || pax[1].ReasonKey != currency.MsgPaxNotCurrent || *pax[1].Params.Needed != 2 {
		t.Errorf("SEP passengers = %+v", pax[1])
	}
	if *pax[2].ULKind != models.ULKindThreeAxis {
		t.Errorf("UL passengers kind = %v", *pax[2].ULKind)
	}
}

func TestEvaluate_Items(t *testing.T) {
	svc := newService(&fakeCurrency{resp: status()}, nil)
	rep, err := svc.Evaluate(context.Background(), userID, Request{})
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]Item{}
	for _, it := range rep.Items {
		switch {
		case it.LaunchMethod != nil:
			byKey[*it.LaunchMethod] = it
		case it.ULKind != nil:
			byKey[string(*it.ULKind)] = it
		default:
			byKey[string(*it.ClassType)] = it
		}
	}

	t.Run("expiring rating is ready", func(t *testing.T) {
		it := byKey["SEP_LAND"]
		if !it.Ready || it.Status != "expiring" || it.ReasonKey != currency.MsgRatingRevalidationNotMet {
			t.Errorf("SEP = %+v", it)
		}
	})
	t.Run("lapsed rating gives its first experience remedy", func(t *testing.T) {
		it := byKey["WEIGHT_SHIFT"]
		if it.Ready || it.Status != "lapsed" || it.ReasonKey != currency.RemedyFlyMore || *it.Params.Missing != 120 {
			t.Errorf("trike = %+v", it)
		}
	})
	t.Run("met launch method names its validUntil", func(t *testing.T) {
		it := byKey["winch"]
		if !it.Ready || it.Status != "current" || it.ReasonKey != currency.MsgReadinessLaunchMethodCurrent || *it.Params.Date != "2027-04-01" {
			t.Errorf("winch = %+v", it)
		}
	})
	t.Run("L4 unmet launch method gives SFCL.155(d) remedy", func(t *testing.T) {
		it := byKey["aerotow"]
		if it.Ready || it.Status != "lapsed" || it.ReasonKey != currency.RemedyLaunchMethodDual ||
			*it.Params.Method != "aerotow" || *it.Params.Missing != 2 {
			t.Errorf("aerotow = %+v", it)
		}
	})
}

func TestEvaluate_Date(t *testing.T) {
	tests := []struct {
		name    string
		date    *time.Time
		wantErr error
		want    time.Time
	}{
		{"default today", nil, nil, today},
		{"today", ptr(today), nil, today},
		{"next Saturday", ptr(saturday), nil, saturday},
		{"366 days ahead", ptr(today.AddDate(0, 0, 366)), nil, today.AddDate(0, 0, 366)},
		{"367 days ahead", ptr(today.AddDate(0, 0, 367)), ErrInvalidDate, time.Time{}},
		{"yesterday", ptr(today.AddDate(0, 0, -1)), ErrInvalidDate, time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cur := &fakeCurrency{resp: status()}
			rep, err := newService(cur, nil).Evaluate(context.Background(), userID, Request{Date: tt.date})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if !cur.asked.Equal(tt.want) || rep.Date != tt.want.Format("2006-01-02") {
				t.Errorf("asked %v, report %s, want %v", cur.asked, rep.Date, tt.want)
			}
		})
	}
}

func TestEvaluate_Errors(t *testing.T) {
	t.Run("unknown aircraft", func(t *testing.T) {
		_, err := newService(&fakeCurrency{resp: status()}, nil).Evaluate(context.Background(), userID, Request{AircraftReg: "D-NONE"})
		if !errors.Is(err, ErrAircraftNotFound) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("another user's aircraft", func(t *testing.T) {
		other := aircraft("D-OTHR", models.ClassTypeGlider, nil)
		other.UserID = uuid.New()
		s := NewService(&fakeCurrency{resp: status()}, fakeAircraft{other}, nil)
		_, err := s.Evaluate(context.Background(), userID, Request{AircraftReg: "D-OTHR"})
		if !errors.Is(err, ErrAircraftNotFound) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("currency failure", func(t *testing.T) {
		_, err := newService(&fakeCurrency{err: errors.New("db")}, nil).Evaluate(context.Background(), userID, Request{})
		if err == nil {
			t.Error("want error")
		}
	})
}

func TestEvaluate_Medicals(t *testing.T) {
	d := func(y int, m time.Month, day int) *time.Time {
		v := time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
		return &v
	}
	old := &models.Credential{ID: uuid.New(), CredentialType: models.CredentialTypeEASALAPLMedical, ExpiryDate: d(2025, 5, 1)}
	lapl := &models.Credential{ID: uuid.New(), CredentialType: models.CredentialTypeEASALAPLMedical, ExpiryDate: d(2026, 10, 2)}
	class2 := &models.Credential{ID: uuid.New(), CredentialType: models.CredentialTypeEASAClass2Medical, ExpiryDate: d(2027, 6, 1)}
	lang := &models.Credential{ID: uuid.New(), CredentialType: models.CredentialTypeLangICAOLevel4, ExpiryDate: d(2020, 1, 1)}

	rep, err := newService(&fakeCurrency{resp: &currency.CurrencyStatusResponse{}}, fakeCredentials{old, lapl, class2, lang}).
		Evaluate(context.Background(), userID, Request{Date: &saturday})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Items) != 2 {
		t.Fatalf("items = %+v, want the latest LAPL medical and the class 2", rep.Items)
	}
	laplItem, class2Item := rep.Items[0], rep.Items[1]
	if *laplItem.CredentialID != lapl.ID || laplItem.Ready || laplItem.Status != StatusExpired ||
		laplItem.ReasonKey != currency.MsgReadinessCredentialExpired || *laplItem.Params.Date != "2026-10-02" {
		t.Errorf("LAPL medical expired before Saturday = %+v", laplItem)
	}
	if *class2Item.CredentialID != class2.ID || !class2Item.Ready || class2Item.Status != StatusValid ||
		class2Item.ReasonKey != currency.MsgReadinessCredentialValid {
		t.Errorf("class 2 medical = %+v", class2Item)
	}
}
