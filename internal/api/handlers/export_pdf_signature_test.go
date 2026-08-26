package handlers

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"
)

// signatureInstructors are the signers used by the fixtures below; the third
// carries no credential number so the fallback text path is exercised too.
var signatureInstructors = []struct{ name, cred string }{
	{"Katrin Vogelsang", "DE.FCL.FI.20481"},
	{"Alan R. Whitfield", "CFI 3184771"},
	{"Sofia Marchetti", ""},
}

// buildSampleSignatures signs every `every`-th flight, returning the map the
// renderers take.
func buildSampleSignatures(flights []*models.Flight, every int) map[uuid.UUID]*models.FlightSignature {
	out := make(map[uuid.UUID]*models.FlightSignature)
	for i, f := range flights {
		if every <= 0 || i%every != 0 {
			continue
		}
		in := signatureInstructors[i%len(signatureInstructors)]
		name, cred := in.name, in.cred
		signedAt := f.Date.Add(26 * time.Hour)
		sig := &models.FlightSignature{
			ID:             uuid.New(),
			FlightID:       f.ID,
			Method:         models.SignatureMethodLive,
			Status:         models.SignatureStatusCompleted,
			InstructorName: &name,
			SignatureImage: fakeSignaturePNG(i),
			SignedAt:       &signedAt,
		}
		if cred != "" {
			sig.InstructorCredentialNo = &cred
		}
		f.SignatureID = &sig.ID
		out[f.ID] = sig
	}
	return out
}

// fakeSignaturePNG draws a handwriting-like stroke on a transparent canvas so
// the rendered block can be judged the way a real signature would look.
func fakeSignaturePNG(seed int) []byte {
	const w, h = 620, 200
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	ink := color.NRGBA{18, 24, 44, 255}
	phase := float64(seed) * 0.7
	for x := 30; x < w-30; x++ {
		t := float64(x-30) / float64(w-60)
		y := float64(h)/2 +
			52*math.Sin(t*10+phase) +
			26*math.Sin(t*23+phase*1.7) -
			34*t
		thick := 2 + int(2*math.Abs(math.Cos(t*8+phase)))
		for dy := -thick; dy <= thick; dy++ {
			py := int(y) + dy
			if py >= 0 && py < h {
				img.SetNRGBA(x, py, ink)
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// renderedBytes emits the document uncompressed, so its content streams can
// be searched for literal text and object types.
func renderedBytes(t *testing.T, doc *fpdf.Fpdf) []byte {
	t.Helper()
	doc.SetCompression(false)
	var buf bytes.Buffer
	if err := doc.Output(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestPDFEndorsementRendersSignerDetails asserts a signed flight prints the
// instructor's name and credential number in its remarks column, and that the
// signature raster reaches the document, for every format × layout that has a
// remarks column.
func TestPDFEndorsementRendersSignerDetails(t *testing.T) {
	g := geometryFor("a4")
	cases := []struct {
		name string
		// wantName is the abbreviated form where the block is too narrow for
		// the full name; wantCred is false where it is too narrow for the
		// credential line at all and degrades to ink + name.
		wantName string
		wantCred bool
		render   func([]*models.Flight, map[uuid.UUID]*models.FlightSignature) []byte
	}{
		{"easa spread", "Katrin Vogelsang", true, func(f []*models.Flight, s map[uuid.UUID]*models.FlightSignature) []byte {
			return renderedBytes(t, renderEASA(f, g, nil, "Test Pilot", layoutSpread, nil, s))
		}},
		{"easa single", "K. Vogelsang", false, func(f []*models.Flight, s map[uuid.UUID]*models.FlightSignature) []byte {
			return renderedBytes(t, renderEASA(f, g, nil, "Test Pilot", layoutSingle, nil, s))
		}},
		{"faa spread", "Katrin Vogelsang", true, func(f []*models.Flight, s map[uuid.UUID]*models.FlightSignature) []byte {
			return renderedBytes(t, generateFAAPDF(f, g, "Test Pilot", layoutSpread, nil, s))
		}},
		{"faa single", "Katrin Vogelsang", true, func(f []*models.Flight, s map[uuid.UUID]*models.FlightSignature) []byte {
			return renderedBytes(t, generateFAAPDF(f, g, "Test Pilot", layoutSingle, nil, s))
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flights := buildSamplePDFFlights(6)
			sigs := buildSampleSignatures(flights, 3)
			signed := tc.render(flights, sigs)

			want := []string{tc.wantName}
			if tc.wantCred {
				want = append(want, "DE.FCL.FI.20481")
			}
			for _, w := range want {
				if !bytes.Contains(signed, []byte(w)) {
					t.Errorf("signed document does not print %q", w)
				}
			}
			// The raster reaches the document as an embedded image object.
			if !bytes.Contains(signed, []byte("/Subtype /Image")) {
				t.Error("signed document embeds no signature image")
			}

			// Nothing of the sort appears without the signatures.
			if bytes.Contains(tc.render(buildSamplePDFFlights(6), nil), []byte("Vogelsang")) {
				t.Error("unsigned document prints an instructor name")
			}
		})
	}
}

// TestPDFEndorsementDeduplicatesRaster asserts one signature reused across
// rows is embedded once, however many rows reference it.
func TestPDFEndorsementDeduplicatesRaster(t *testing.T) {
	g := geometryFor("a4")
	countImages := func(sigs map[uuid.UUID]*models.FlightSignature, flights []*models.Flight) int {
		doc := renderedBytes(t, renderEASA(flights, g, nil, "Test Pilot", layoutSpread, nil, sigs))
		return bytes.Count(doc, []byte("/Subtype /Image"))
	}

	distinctFlights := buildSamplePDFFlights(9)
	distinct := buildSampleSignatures(distinctFlights, 3)
	if len(distinct) != 3 {
		t.Fatalf("fixture signed %d flights, want 3", len(distinct))
	}

	// The same signature record attesting all three rows.
	sharedFlights := buildSamplePDFFlights(9)
	shared := buildSampleSignatures(sharedFlights, 3)
	var one *models.FlightSignature
	for _, s := range shared {
		one = s
		break
	}
	for id := range shared {
		shared[id] = one
	}

	// And a single signed row, as the reference for "embedded once".
	oneFlights := buildSamplePDFFlights(9)
	single := buildSampleSignatures(oneFlights, 9)
	if len(single) != 1 {
		t.Fatalf("fixture signed %d flights, want 1", len(single))
	}

	nDistinct := countImages(distinct, distinctFlights)
	nShared := countImages(shared, sharedFlights)
	nSingle := countImages(single, oneFlights)
	if nSingle == 0 || nDistinct != 3*nSingle {
		t.Fatalf("three distinct signatures embedded %d image objects, one embedded %d", nDistinct, nSingle)
	}
	if nShared != nSingle {
		t.Errorf("one signature across three rows embedded %d image objects, want %d", nShared, nSingle)
	}
}

// TestPDFEndorsementSurvivesBadRaster asserts an undecodable or oversized
// signature costs the ink, not the document: the row still prints, the signer
// details still appear, and the export still produces a valid PDF.
func TestPDFEndorsementSurvivesBadRaster(t *testing.T) {
	cases := map[string][]byte{
		"garbage":    []byte("this is not an image"),
		"empty":      {},
		"truncated":  fakeSignaturePNG(1)[:64],
		"pixel bomb": pixelBombPNG(),
	}
	for name, raster := range cases {
		t.Run(name, func(t *testing.T) {
			flights := buildSamplePDFFlights(4)
			sigs := buildSampleSignatures(flights, 2)
			for _, s := range sigs {
				s.SignatureImage = raster
			}

			doc := renderedBytes(t, renderEASA(flights, geometryFor("a4"), nil, "Test Pilot", layoutSpread, nil, sigs))
			if !bytes.HasPrefix(doc, []byte("%PDF-")) {
				t.Fatal("output is not a PDF")
			}
			if !bytes.Contains(doc, []byte("Katrin Vogelsang")) {
				t.Error("signer details dropped along with the unusable raster")
			}
			if bytes.Contains(doc, []byte("/Subtype /Image")) {
				t.Error("unusable raster was embedded anyway")
			}
		})
	}
}

// pixelBombPNG returns a small PNG whose decoded dimensions exceed
// maxSignatureSourcePixels.
func pixelBombPNG() []byte {
	img := image.NewGray(image.Rect(0, 0, 4200, 4200))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// TestSigZoneWidth asserts the endorsement block claims a share of the
// remarks column, stops growing past its cap, always leaves room for the
// remark text, and declines a column too narrow to carry it.
func TestSigZoneWidth(t *testing.T) {
	cases := []struct {
		colW float64
		want func(float64) bool
		desc string
	}{
		{6, func(z float64) bool { return z == 0 }, "too narrow to endorse"},
		{12, func(z float64) bool { return z == 0 }, "below the minimum block width"},
		{30, func(z float64) bool { return z > 9 && z <= 30-4 }, "shares a narrow column"},
		{85, func(z float64) bool { return z == sigZoneMaxW }, "capped on a wide column"},
	}
	for _, tc := range cases {
		if z := sigZoneWidth(tc.colW); !tc.want(z) {
			t.Errorf("sigZoneWidth(%.0f) = %.2f — %s", tc.colW, z, tc.desc)
		}
	}
}

// TestNormalizeSignatureImage asserts a raster is bounded to the print
// resolution while its aspect ratio is preserved.
func TestNormalizeSignatureImage(t *testing.T) {
	raw := fakeSignaturePNG(0)
	out, aspect, ok := normalizeSignatureImage(raw)
	if !ok {
		t.Fatal("normalizeSignatureImage rejected a valid PNG")
	}
	if want := 620.0 / 200.0; math.Abs(aspect-want) > 0.001 {
		t.Errorf("aspect = %.4f, want %.4f", aspect, want)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("re-encoded raster does not decode: %v", err)
	}
	if cfg.Width > maxSignatureWidthPx || cfg.Height > maxSignatureHeightPx {
		t.Errorf("raster %dx%d exceeds bounds %dx%d",
			cfg.Width, cfg.Height, maxSignatureWidthPx, maxSignatureHeightPx)
	}
	if len(out) >= len(raw) {
		t.Errorf("normalised raster (%d bytes) not smaller than source (%d bytes)", len(out), len(raw))
	}
}
