package handlers

import (
	"bytes"
	"compress/zlib"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type logbookLicenseRepo struct {
	licenses map[uuid.UUID]*models.License
}

func (m *logbookLicenseRepo) Create(_ context.Context, l *models.License) error {
	l.ID = uuid.New()
	m.licenses[l.ID] = l
	return nil
}
func (m *logbookLicenseRepo) GetByID(_ context.Context, id uuid.UUID) (*models.License, error) {
	if l, ok := m.licenses[id]; ok {
		return l, nil
	}
	return nil, repository.ErrNotFound
}
func (m *logbookLicenseRepo) GetByUserID(_ context.Context, _ uuid.UUID, _ *time.Time) ([]*models.License, error) {
	return nil, nil
}
func (m *logbookLicenseRepo) Update(_ context.Context, _ *models.License) error { return nil }
func (m *logbookLicenseRepo) Delete(_ context.Context, _ uuid.UUID) error       { return nil }

type logbookRatingRepo struct {
	ratings []*models.ClassRating
}

func (m *logbookRatingRepo) Create(_ context.Context, cr *models.ClassRating) error {
	cr.ID = uuid.New()
	m.ratings = append(m.ratings, cr)
	return nil
}
func (m *logbookRatingRepo) GetByID(_ context.Context, _ uuid.UUID) (*models.ClassRating, error) {
	return nil, repository.ErrNotFound
}
func (m *logbookRatingRepo) GetByLicenseID(_ context.Context, id uuid.UUID) ([]*models.ClassRating, error) {
	var out []*models.ClassRating
	for _, cr := range m.ratings {
		if cr.LicenseID == id {
			out = append(out, cr)
		}
	}
	return out, nil
}
func (m *logbookRatingRepo) Update(_ context.Context, _ *models.ClassRating) error { return nil }
func (m *logbookRatingRepo) Delete(_ context.Context, _ uuid.UUID) error           { return nil }

type logbookPDFFixture struct {
	h        *APIHandler
	userID   uuid.UUID
	licenses *logbookLicenseRepo
	ratings  *logbookRatingRepo
	aircraft *mockAircraftRepo
	flights  *mockFlightRepo
}

func newLogbookPDFFixture() *logbookPDFFixture {
	gin.SetMode(gin.TestMode)
	registry := currency.NewRegistry()
	registry.Register(currency.NewEASAEvaluator())
	registry.Register(currency.NewOtherEvaluator())
	ul := currency.NewGermanULEvaluator()
	registry.RegisterMulti(ul, ul.Authorities()...)

	fx := &logbookPDFFixture{
		userID:   uuid.New(),
		licenses: &logbookLicenseRepo{licenses: map[uuid.UUID]*models.License{}},
		ratings:  &logbookRatingRepo{},
		aircraft: newMockAircraftRepo(),
		flights:  newMockFlightRepo(),
	}
	fx.h, _ = setupTestHandler()
	fx.h.licenseService = service.NewLicenseService(fx.licenses)
	fx.h.classRatingService = service.NewClassRatingService(fx.ratings, fx.licenses)
	fx.h.aircraftService = service.NewAircraftService(fx.aircraft)
	fx.h.flightService = service.NewFlightService(fx.flights, nil)
	fx.h.currencyService = currency.NewService(registry, nil, nil, nil)
	return fx
}

func (fx *logbookPDFFixture) license(authority, typ string, ratings ...*models.ClassRating) uuid.UUID {
	lic := &models.License{UserID: fx.userID, RegulatoryAuthority: authority, LicenseType: typ}
	_ = fx.licenses.Create(context.Background(), lic)
	for _, cr := range ratings {
		cr.LicenseID = lic.ID
		_ = fx.ratings.Create(context.Background(), cr)
	}
	return lic.ID
}

func (fx *logbookPDFFixture) addAircraft(reg, class string, kind *models.ULKind) {
	ac := &models.Aircraft{UserID: fx.userID, Registration: reg, Type: "T", Make: "M", Model: "M", AircraftClass: &class, ULKind: kind}
	_ = fx.aircraft.Create(context.Background(), ac)
}

func (fx *logbookPDFFixture) addFlight(reg, typ, remarks, launch string) {
	f := &models.Flight{
		UserID: fx.userID, Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		AircraftReg: reg, AircraftType: typ, TotalTime: 60, PICTime: 60, LandingsDay: 1,
	}
	if remarks != "" {
		f.Remarks = &remarks
	}
	if launch != "" {
		f.LaunchMethod = &launch
	}
	_ = fx.flights.Create(context.Background(), f)
}

func (fx *logbookPDFFixture) export(t *testing.T, licenseID uuid.UUID) string {
	t.Helper()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, fx.userID)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/exports/pdf", nil)
	id := openapi_types.UUID(licenseID)
	layout := generated.ExportFlightsPDFParamsLayout(layoutSingle)
	fx.h.ExportFlightsPDF(c, generated.ExportFlightsPDFParams{LogbookLicenseId: &id, Layout: &layout})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	return pdfPlainText(t, w.Body.Bytes())
}

var pdfStreamRe = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)

// pdfPlainText returns the PDF with every Flate stream inflated.
func pdfPlainText(t *testing.T, pdf []byte) string {
	t.Helper()
	var out strings.Builder
	for _, m := range pdfStreamRe.FindAllSubmatch(pdf, -1) {
		r, err := zlib.NewReader(bytes.NewReader(m[1]))
		if err != nil {
			out.Write(m[1])
			continue
		}
		b, _ := io.ReadAll(r)
		out.Write(b)
	}
	return out.String()
}

func TestExportFlightsPDF_LicenceLogbook(t *testing.T) {
	kind := func(k models.ULKind) *models.ULKind { return &k }

	t.Run("M2 German three-axis licence includes credited SEP flight marked credited", func(t *testing.T) {
		fx := newLogbookPDFFixture()
		fx.addAircraft("D-MXYZ", "ULTRALIGHT", kind(models.ULKindThreeAxis))
		fx.addAircraft("D-EABC", "sep_land", nil)
		fx.addAircraft("D-MTRK", "ULTRALIGHT", kind(models.ULKindWeightShift))
		fx.addAircraft("D-KTMG", "TMG", nil)
		fx.addFlight("D-MXYZ", "C42", "Platzrunden", "")
		fx.addFlight("D-EABC", "C172", "Rundflug", "")
		fx.addFlight("D-MTRK", "TRIKE", "", "")
		fx.addFlight("D-KTMG", "SF25", "", "winch")
		licID := fx.license("DULV", "UL", &models.ClassRating{ClassType: models.ClassTypeUL, ULKind: kind(models.ULKindThreeAxis)})

		text := fx.export(t, licID)
		for _, want := range []string{"D-MXYZ", "D-EABC", flightrules.CreditedLabel + " Rundflug"} {
			if !strings.Contains(text, want) {
				t.Errorf("PDF lacks %q", want)
			}
		}
		for _, absent := range []string{"D-MTRK", "D-KTMG", flightrules.CreditedLabel + " Platzrunden"} {
			if strings.Contains(text, absent) {
				t.Errorf("PDF contains %q", absent)
			}
		}
	})

	t.Run("S Sabine trike licence excludes three-axis flights", func(t *testing.T) {
		fx := newLogbookPDFFixture()
		fx.addAircraft("D-MXYZ", "ULTRALIGHT", kind(models.ULKindThreeAxis))
		fx.addAircraft("D-MTRK", "ultralight ", kind(models.ULKindWeightShift))
		fx.addFlight("D-MXYZ", "C42", "", "")
		fx.addFlight("D-MTRK", "TRIKE", "", "")
		licID := fx.license("DULV", "UL", &models.ClassRating{ClassType: models.ClassTypeUL, ULKind: kind(models.ULKindWeightShift)})

		text := fx.export(t, licID)
		if !strings.Contains(text, "D-MTRK") {
			t.Error("PDF lacks the trike flight")
		}
		if strings.Contains(text, "D-MXYZ") {
			t.Error("PDF contains the three-axis flight")
		}
		if strings.Contains(text, flightrules.CreditedLabel) {
			t.Error("PDF marks a flight credited")
		}
	})

	t.Run("N2 towed glider launch never credited to a PPL", func(t *testing.T) {
		fx := newLogbookPDFFixture()
		fx.addAircraft("D-EABC", "SEP_LAND", nil)
		fx.addAircraft("D-1234", "GLIDER", nil)
		fx.addFlight("D-EABC", "C172", "", "")
		fx.addFlight("D-1234", "ASK21", "", "winch")
		licID := fx.license("EASA", "LAPL(A)", &models.ClassRating{ClassType: models.ClassTypeSEPLand})

		text := fx.export(t, licID)
		if !strings.Contains(text, "D-EABC") {
			t.Error("PDF lacks the SEP flight")
		}
		if strings.Contains(text, "D-1234") {
			t.Error("PDF contains the winch-launched glider flight")
		}
	})
}

func TestFilterLogbookFlights_MarksCreditedCopies(t *testing.T) {
	fx := newLogbookPDFFixture()
	fx.addAircraft("D-MXYZ", "ULTRALIGHT", func() *models.ULKind { k := models.ULKindThreeAxis; return &k }())
	fx.addAircraft("D-EABC", "SEP_LAND", nil)
	licID := fx.license("DULV", "UL", &models.ClassRating{ClassType: models.ClassTypeUL, ULKind: func() *models.ULKind { k := models.ULKindThreeAxis; return &k }()})
	lb, err := fx.h.logbookScope().Resolve(context.Background(), fx.userID, licID)
	if err != nil || lb == nil {
		t.Fatalf("Resolve = (%v, %v)", lb, err)
	}
	remark := "Rundflug"
	native := &models.Flight{ID: uuid.New(), AircraftReg: "D-MXYZ"}
	credited := &models.Flight{ID: uuid.New(), AircraftReg: "D-EABC", Remarks: &remark}
	bare := &models.Flight{ID: uuid.New(), AircraftReg: "D-EABC"}

	cases := []struct {
		name string
		in   *models.Flight
		want string
	}{
		{"native keeps remarks", native, ""},
		{"credited prefixes remarks", credited, flightrules.CreditedLabel + " Rundflug"},
		{"credited without remarks", bare, flightrules.CreditedLabel},
	}
	out := filterLogbookFlights([]*models.Flight{native, credited, bare}, lb)
	if len(out) != len(cases) {
		t.Fatalf("got %d flights, want %d", len(out), len(cases))
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ""
			if out[i].Remarks != nil {
				got = *out[i].Remarks
			}
			if got != tc.want {
				t.Errorf("remarks = %q, want %q", got, tc.want)
			}
			if out[i].ID != tc.in.ID {
				t.Error("flight ID changed")
			}
		})
	}
	if *credited.Remarks != "Rundflug" {
		t.Errorf("source flight remarks mutated to %q", *credited.Remarks)
	}
}
