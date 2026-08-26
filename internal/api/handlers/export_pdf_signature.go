package handlers

import (
	"bytes"
	"image"
	"image/png"
	"strings"

	_ "image/jpeg" // signature rasters are PNG by contract, JPEG by tolerance

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"
)

// ─────────────────────────────────────────────────────────────────────────────
// Instructor endorsement block
// ─────────────────────────────────────────────────────────────────────────────

// A signed flight's remarks cell is split in two: the remark text keeps the
// left, and an attested block on the right carries the instructor's ink, name
// and credential number. Layout ladder, widest to narrowest:
//
//	ink + name + "No. <credential> · <signed date>"   (two text lines)
//	ink + name                                        (one text line)
//	name + credential across the full zone            (no room for both,
//	                                                   or no usable raster)
const (
	sigZoneFrac      = 0.55 // share of the remarks column the block claims
	sigZoneMaxW      = 46.0 // mm; beyond this the block stops growing
	sigZoneMinW      = 9.0  // mm; below this the column carries no block
	sigInkFrac       = 0.42 // share of the block given to the ink
	sigGap           = 1.0  // mm between ink and text
	sigTwoLineMinW   = 12.0 // mm of text width needed for the credential line
	sigTwoLineMinRow = 3.4  // mm of row height needed for the credential line
	sigOneLineMinW   = 5.5  // mm of text width needed for the name
)

// colSigFill is the wash behind an attested block, marking the cell as
// carrying an instructor's sign-off rather than free-text remarks.
var colSigFill = [3]int{237, 242, 249}

// Raster bounds for an embedded signature. maxSignatureSourcePixels rejects a
// decompression bomb before it is decoded; the width/height caps hold the
// normalised raster at roughly twice the resolution the ink slot prints at
// 300 dpi.
const (
	maxSignatureSourcePixels = 16 << 20
	maxSignatureWidthPx      = 480
	maxSignatureHeightPx     = 200
)

// maxPDFSignatureBytes caps the total normalised raster embedded in one
// document. Past it rows fall back to the text-only endorsement.
const maxPDFSignatureBytes = 16 << 20

// sigInk is a signature raster registered with the document.
type sigInk struct {
	name   string  // fpdf image name
	aspect float64 // source width / height
	ok     bool
}

// sigZoneWidth returns the width reserved for the endorsement block inside a
// remarks column of colW, or 0 when the column is too narrow to carry one.
func sigZoneWidth(colW float64) float64 {
	z := colW * sigZoneFrac
	if z > sigZoneMaxW {
		z = sigZoneMaxW
	}
	if z > colW-4 {
		z = colW - 4
	}
	if z < sigZoneMinW {
		return 0
	}
	return z
}

// signatureInk normalises and registers sig's raster with the document,
// once per signature. Undecodable rasters, and everything past the
// document's embedding budget, come back with ok=false.
func (d *pdfDoc) signatureInk(sig *models.FlightSignature) sigInk {
	if d.sigInks == nil {
		d.sigInks = make(map[uuid.UUID]sigInk)
	}
	if ink, seen := d.sigInks[sig.ID]; seen {
		return ink
	}
	ink := sigInk{}
	defer func() { d.sigInks[sig.ID] = ink }()

	if len(sig.SignatureImage) == 0 || d.sigBytes >= maxPDFSignatureBytes {
		return ink
	}
	raster, aspect, ok := normalizeSignatureImage(sig.SignatureImage)
	if !ok || d.sigBytes+len(raster) > maxPDFSignatureBytes {
		return ink
	}
	name := "sig_" + sig.ID.String()
	d.pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: "png"}, bytes.NewReader(raster))
	if d.pdf.Err() {
		// A raster fpdf cannot parse must not poison the whole document.
		d.pdf.ClearError()
		return ink
	}
	d.sigBytes += len(raster)
	ink = sigInk{name: name, aspect: aspect, ok: true}
	return ink
}

// normalizeSignatureImage decodes a stored signature and re-encodes it as an
// 8-bit non-interlaced PNG within the raster bounds, returning the encoded
// bytes and the source aspect ratio.
func normalizeSignatureImage(raw []byte) ([]byte, float64, bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, 0, false
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxSignatureSourcePixels {
		return nil, 0, false
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, 0, false
	}
	b := src.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil, 0, false
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, downscaleNRGBA(src, maxSignatureWidthPx, maxSignatureHeightPx)); err != nil {
		return nil, 0, false
	}
	return buf.Bytes(), float64(b.Dx()) / float64(b.Dy()), true
}

// downscaleNRGBA box-averages src down until it fits maxW × maxH, converting
// to NRGBA. An image already within bounds is converted only.
func downscaleNRGBA(src image.Image, maxW, maxH int) *image.NRGBA {
	b := src.Bounds()
	n := 1
	for (b.Dx()+n-1)/n > maxW || (b.Dy()+n-1)/n > maxH {
		n++
	}
	w, h := (b.Dx()+n-1)/n, (b.Dy()+n-1)/n
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sr, sg, sb, sa, count uint64
			for oy := y * n; oy < (y+1)*n && oy < b.Dy(); oy++ {
				for ox := x * n; ox < (x+1)*n && ox < b.Dx(); ox++ {
					cr, cg, cb, ca := src.At(b.Min.X+ox, b.Min.Y+oy).RGBA()
					sr += uint64(cr)
					sg += uint64(cg)
					sb += uint64(cb)
					sa += uint64(ca)
					count++
				}
			}
			if count == 0 {
				continue
			}
			a := sa / count
			i := dst.PixOffset(x, y)
			dst.Pix[i+3] = to8(a)
			if a == 0 {
				continue
			}
			// RGBA() is alpha-premultiplied; un-premultiply the average.
			unpre := func(v uint64) uint8 { return to8(v / count * 0xffff / a) }
			dst.Pix[i] = unpre(sr)
			dst.Pix[i+1] = unpre(sg)
			dst.Pix[i+2] = unpre(sb)
		}
	}
	return dst
}

// to8 narrows a 16-bit colour channel to 8 bits, clamping input above the
// channel maximum.
func to8(v uint64) uint8 {
	if v > 0xffff {
		v = 0xffff
	}
	return uint8(v >> 8)
}

// drawEndorsement fills a signed flight's remarks cell, which the caller has
// already drawn empty: `remarks` on the left, the attested block on the right.
// Leaves the font and text colour reset for the next data row.
func (d *pdfDoc) drawEndorsement(x, y, w, h float64, remarks string, sig *models.FlightSignature) {
	g, pdf := d.g, d.pdf
	zone := sigZoneWidth(w)
	if zone <= 0 {
		return
	}
	zx := x + w - zone

	if remarks != "" && w-zone > 3 {
		// CellFormat applies its own left padding, so the text lines up with
		// the unsigned rows above and below.
		avail := w - zone - 0.8
		setText(pdf, [3]int{0, 0, 0})
		pdf.SetXY(x, y)
		// Truncated rather than shrunk, so a signed row's remark reads at the
		// same size as the rows above and below it.
		text := d.fitText("", g.fontBody, g.fontBody, remarks, avail-2*pdf.GetCellMargin())
		pdf.CellFormat(avail, h, text, "", 0, "L", false, 0, "")
	}

	// Wash and gold edge marker set the block apart from free-text remarks.
	setFill(pdf, colSigFill)
	pdf.Rect(zx, y+0.2, zone-0.2, h-0.4, "F")
	setFill(pdf, colAccent)
	pdf.Rect(zx, y+0.2, 0.4, h-0.4, "F")

	inkX := zx + 1.2
	inkW := zone*sigInkFrac - 1.2
	textX := zx + zone*sigInkFrac + sigGap
	textW := zone - zone*sigInkFrac - sigGap - 0.8

	// Where the block cannot carry both, the name takes the space from the
	// ink: an unattributed mark tells an inspector nothing.
	ink := d.signatureInk(sig)
	if ink.ok && inkW > 2 && ink.aspect > 0 && textW >= sigOneLineMinW {
		ih := h - 1.6
		iw := ih * ink.aspect
		if iw > inkW {
			iw, ih = inkW, inkW/ink.aspect
		}
		pdf.ImageOptions(ink.name, inkX, y+(h-1.0-ih)/2, iw, ih, false,
			fpdf.ImageOptions{ImageType: "png"}, 0, "")
		// Baseline rule, so the ink reads as signed onto a line. It tracks
		// the ink rather than the slot: a dense row's ink is height-bound
		// and would otherwise sit on a rule several times its width.
		ruleW := iw + 1.2
		if ruleW > inkW {
			ruleW = inkW
		}
		setDraw(pdf, colBorderStrong)
		pdf.SetLineWidth(0.15)
		pdf.Line(inkX, y+h-0.9, inkX+ruleW, y+h-0.9)
	} else {
		textX, textW = zx+1.2, zone-2.0
	}

	name := strings.TrimSpace(safeStr(sig.InstructorName))
	if name == "" {
		name = "Signed"
	}
	// The credential number identifies the signer, so it survives a squeeze
	// that the signing date does not.
	meta, metaShort := "", ""
	if cred := strings.TrimSpace(safeStr(sig.InstructorCredentialNo)); cred != "" {
		metaShort = "No. " + cred
		meta = metaShort
	}
	if sig.SignedAt != nil {
		date := sig.SignedAt.UTC().Format("02 Jan 06")
		if meta != "" {
			meta += " · " + date
		} else {
			meta, metaShort = date, date
		}
	}

	// The block's own labels are positioned to the millimetre, so they carry
	// no cell padding of their own.
	cellMargin := pdf.GetCellMargin()
	pdf.SetCellMargin(0)
	defer pdf.SetCellMargin(cellMargin)

	switch {
	case textW >= sigTwoLineMinW && h >= sigTwoLineMinRow && meta != "":
		lineH := (h - 0.8) / 2
		setText(pdf, colBand)
		pdf.SetXY(textX, y+0.4)
		pdf.CellFormat(textW, lineH, d.fitName("B", g.fontBody-0.6, 3.0, name, textW), "", 0, "L", false, 0, "")
		setText(pdf, colMuted)
		pdf.SetXY(textX, y+0.4+lineH)
		pdf.CellFormat(textW, lineH, d.fitAny("", g.fontBody-1.2, 2.8, textW, meta, metaShort), "", 0, "L", false, 0, "")
	case textW >= sigOneLineMinW:
		setText(pdf, colBand)
		pdf.SetXY(textX, y)
		pdf.CellFormat(textW, h, d.fitName("B", g.fontBody-0.6, 2.8, name, textW), "", 0, "L", false, 0, "")
	}

	pdf.SetFont("Helvetica", "", g.fontBody)
	setText(pdf, [3]int{0, 0, 0})
}

// fitAny renders the first candidate that fits maxW whole, falling back to
// fitText on the last one.
func (d *pdfDoc) fitAny(style string, size, minSize, maxW float64, candidates ...string) string {
	if len(candidates) == 0 {
		return ""
	}
	for _, c := range candidates[:len(candidates)-1] {
		d.pdf.SetFont("Helvetica", style, minSize)
		if d.pdf.GetStringWidth(d.tr(c)) <= maxW {
			return d.fitText(style, size, minSize, c, maxW)
		}
	}
	return d.fitText(style, size, minSize, candidates[len(candidates)-1], maxW)
}

// fitName renders an instructor's name into maxW, falling back to initials
// plus surname before fitText resorts to cutting the name off.
func (d *pdfDoc) fitName(style string, size, minSize float64, name string, maxW float64) string {
	if short := abbreviateName(name); short != name {
		d.pdf.SetFont("Helvetica", style, minSize)
		if d.pdf.GetStringWidth(d.tr(name)) > maxW {
			name = short
		}
	}
	return d.fitText(style, size, minSize, name, maxW)
}

// abbreviateName reduces every given name to an initial, keeping the surname
// intact: "Katrin Vogelsang" becomes "K. Vogelsang".
func abbreviateName(s string) string {
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return s
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts[:len(parts)-1] {
		out = append(out, string([]rune(p)[:1])+".")
	}
	return strings.Join(append(out, parts[len(parts)-1]), " ")
}

// fitText shrinks the font from size toward minSize and, when the text still
// does not fit maxW, truncates it. Returns the CP1252-mapped string to draw
// and leaves the chosen font selected.
func (d *pdfDoc) fitText(style string, size, minSize float64, s string, maxW float64) string {
	pdf := d.pdf
	pdf.SetFont("Helvetica", style, size)
	for size > minSize && pdf.GetStringWidth(d.tr(s)) > maxW {
		size -= 0.25
		pdf.SetFont("Helvetica", style, size)
	}
	if pdf.GetStringWidth(d.tr(s)) <= maxW {
		return d.tr(s)
	}
	r := []rune(s)
	for len(r) > 1 {
		r = r[:len(r)-1]
		cand := strings.TrimRight(string(r), " ") + "..."
		if pdf.GetStringWidth(d.tr(cand)) <= maxW {
			return d.tr(cand)
		}
	}
	return ""
}
