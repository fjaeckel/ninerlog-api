//go:build e2e

package e2e_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"strings"
	"net/http"
	"testing"
)

// e2ePNG / e2eJPEG produce genuinely decodable images — the API rejects
// anything that only *claims* to be one, so a fixture of random bytes with the
// right magic prefix would not get past upload.
func e2ePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func e2eJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// uploadDocumentFile posts one multipart image to a document's image
// collection. The declared part Content-Type is settable so a test can prove
// the server does not trust it.
func uploadDocumentFile(t *testing.T, c *E2EClient, path, filename, partContentType, caption string, data []byte) *Response {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	header := make(map[string][]string)
	header["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename=%q`, filename)}
	if partContentType != "" {
		header["Content-Type"] = []string{partContentType}
	}
	part, err := w.CreatePart(header)
	if err != nil {
		t.Fatalf("create multipart part: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart part: %v", err)
	}
	if caption != "" {
		if err := w.WriteField("caption", caption); err != nil {
			t.Fatalf("write caption field: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, _ := http.NewRequest("POST", baseURL+"/api/v1"+path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return &Response{StatusCode: resp.StatusCode, Body: body, Headers: resp.Header}
}

func createFileTestLicense(t *testing.T, c *E2EClient, number string) string {
	t.Helper()
	resp := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": number,
		"issueDate": "2023-01-15", "issuingAuthority": "LBA",
	})
	requireStatus(t, resp, http.StatusCreated)
	var lic map[string]interface{}
	resp.JSON(&lic)
	return lic["id"].(string)
}

func TestLicenseFiles(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("licimg"), "SecurePass123!", "Licence Image User")
	licID := createFileTestLicense(t, c, "DE-IMG-001")
	base := fmt.Sprintf("/licenses/%s/files", licID)

	var imageID string
	pngData := e2ePNG(t, 240, 160)

	t.Run("feature probe reports the capability", func(t *testing.T) {
		resp := c.GET("/features")
		requireStatus(t, resp, http.StatusOK)
		var features map[string]interface{}
		resp.JSON(&features)
		docs, ok := features["documentFiles"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected documentFiles in %v", features)
		}
		if docs["maxBytes"].(float64) != 5*1024*1024 {
			t.Errorf("maxBytes = %v, want 5242880", docs["maxBytes"])
		}
		if docs["maxPerDocument"].(float64) != 5 {
			t.Errorf("maxPerDocument = %v, want 5", docs["maxPerDocument"])
		}
	})

	t.Run("empty before any upload", func(t *testing.T) {
		resp := c.GET(base)
		requireStatus(t, resp, http.StatusOK)
		var images []interface{}
		resp.JSON(&images)
		if len(images) != 0 {
			t.Errorf("expected no images, got %d", len(images))
		}
	})

	t.Run("upload png", func(t *testing.T) {
		resp := uploadDocumentFile(t, c, base, "licence-front.png", "image/png", "Front page", pngData)
		requireStatus(t, resp, http.StatusCreated)

		var img map[string]interface{}
		resp.JSON(&img)
		imageID = img["id"].(string)
		if img["contentType"] != "image/png" {
			t.Errorf("contentType = %v, want image/png", img["contentType"])
		}
		if img["width"].(float64) != 240 || img["height"].(float64) != 160 {
			t.Errorf("dimensions = %v×%v, want 240×160", img["width"], img["height"])
		}
		if img["byteSize"].(float64) != float64(len(pngData)) {
			t.Errorf("byteSize = %v, want %d", img["byteSize"], len(pngData))
		}
		if img["caption"] != "Front page" {
			t.Errorf("caption = %v, want %q", img["caption"], "Front page")
		}
		if img["licenseId"] != licID {
			t.Errorf("licenseId = %v, want %v", img["licenseId"], licID)
		}
	})

	t.Run("upload jpeg", func(t *testing.T) {
		resp := uploadDocumentFile(t, c, base, "licence-back.jpg", "image/jpeg", "", e2eJPEG(t, 64, 64))
		requireStatus(t, resp, http.StatusCreated)
		var img map[string]interface{}
		resp.JSON(&img)
		if img["contentType"] != "image/jpeg" {
			t.Errorf("contentType = %v, want image/jpeg", img["contentType"])
		}
	})

	t.Run("list returns metadata only", func(t *testing.T) {
		resp := c.GET(base)
		requireStatus(t, resp, http.StatusOK)
		var images []map[string]interface{}
		resp.JSON(&images)
		if len(images) != 2 {
			t.Fatalf("len(images) = %d, want 2", len(images))
		}
		for _, img := range images {
			if _, leaked := img["data"]; leaked {
				t.Error("listing exposed the image payload")
			}
		}
	})

	t.Run("download returns the original bytes", func(t *testing.T) {
		resp := c.GET(base + "/" + imageID)
		requireStatus(t, resp, http.StatusOK)
		if ct := resp.Headers.Get("Content-Type"); ct != "image/png" {
			t.Errorf("Content-Type = %q, want image/png", ct)
		}
		if !bytes.Equal(resp.Body, pngData) {
			t.Error("downloaded bytes differ from the uploaded image")
		}
	})

	t.Run("download requires authentication", func(t *testing.T) {
		token := c.token
		c.ClearToken()
		resp := c.GET(base + "/" + imageID)
		c.SetToken(token)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 — image bytes must never be reachable anonymously", resp.StatusCode)
		}
	})

	t.Run("declared content type is not trusted", func(t *testing.T) {
		// A real PNG announced as JPEG is stored as what it actually is …
		resp := uploadDocumentFile(t, c, base, "mislabelled.jpg", "image/jpeg", "", e2ePNG(t, 32, 32))
		requireStatus(t, resp, http.StatusCreated)
		var img map[string]interface{}
		resp.JSON(&img)
		if img["contentType"] != "image/png" {
			t.Errorf("contentType = %v, want image/png (sniffed, not declared)", img["contentType"])
		}
		requireStatus(t, c.DELETE(base+"/"+img["id"].(string)), http.StatusNoContent)

		// … and a script announced as PNG is refused outright.
		svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
		resp = uploadDocumentFile(t, c, base, "evil.png", "image/png", "", svg)
		requireStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("rejects a non-image file", func(t *testing.T) {
		resp := uploadDocumentFile(t, c, base, "notes.txt", "text/plain", "", []byte("just some text"))
		requireStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("enforces the per-document limit", func(t *testing.T) {
		// Two are already attached; fill the remaining slots, then overflow.
		for i := 0; i < 3; i++ {
			resp := uploadDocumentFile(t, c, base, fmt.Sprintf("extra-%d.png", i), "image/png", "", e2ePNG(t, 16, 16))
			requireStatus(t, resp, http.StatusCreated)
		}
		resp := uploadDocumentFile(t, c, base, "one-too-many.png", "image/png", "", e2ePNG(t, 16, 16))
		requireStatus(t, resp, http.StatusConflict)
	})

	t.Run("another user cannot read or delete them", func(t *testing.T) {
		other := NewE2EClient(t)
		registerAndLogin(t, other, uniqueEmail("licimg-other"), "SecurePass123!", "Other User")

		requireStatus(t, other.GET(base), http.StatusNotFound)
		requireStatus(t, other.GET(base+"/"+imageID), http.StatusNotFound)
		requireStatus(t, other.DELETE(base+"/"+imageID), http.StatusNotFound)
	})

	t.Run("delete", func(t *testing.T) {
		requireStatus(t, c.DELETE(base+"/"+imageID), http.StatusNoContent)
		requireStatus(t, c.GET(base+"/"+imageID), http.StatusNotFound)
		requireStatus(t, c.DELETE(base+"/"+imageID), http.StatusNotFound)
	})

	t.Run("deleting the licence removes its files", func(t *testing.T) {
		doomed := createFileTestLicense(t, c, "DE-IMG-DOOM")
		doomedBase := fmt.Sprintf("/licenses/%s/files", doomed)
		resp := uploadDocumentFile(t, c, doomedBase, "front.png", "image/png", "", e2ePNG(t, 16, 16))
		requireStatus(t, resp, http.StatusCreated)

		requireStatus(t, c.DELETE("/licenses/"+doomed), http.StatusNoContent)
		requireStatus(t, c.GET(doomedBase), http.StatusNotFound)
	})
}

func TestCredentialFiles(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("credimg"), "SecurePass123!", "Credential Image User")

	// The German radio certificate doubles as coverage for the new enum values.
	resp := c.POST("/credentials", map[string]interface{}{
		"credentialType": "RADIO_AZF", "credentialNumber": "AZF-4711",
		"issueDate": "2022-05-04", "issuingAuthority": "Bundesnetzagentur",
	})
	requireStatus(t, resp, http.StatusCreated)
	var cred map[string]interface{}
	resp.JSON(&cred)
	credID := cred["id"].(string)
	base := fmt.Sprintf("/credentials/%s/files", credID)

	var imageID string
	data := e2eJPEG(t, 200, 120)

	t.Run("upload", func(t *testing.T) {
		resp := uploadDocumentFile(t, c, base, "azf.jpg", "image/jpeg", "Scan", data)
		requireStatus(t, resp, http.StatusCreated)
		var img map[string]interface{}
		resp.JSON(&img)
		imageID = img["id"].(string)
		if img["credentialId"] != credID {
			t.Errorf("credentialId = %v, want %v", img["credentialId"], credID)
		}
		if img["licenseId"] != nil {
			t.Errorf("licenseId should be null on a credential image, got %v", img["licenseId"])
		}
	})

	t.Run("download", func(t *testing.T) {
		resp := c.GET(base + "/" + imageID)
		requireStatus(t, resp, http.StatusOK)
		if !bytes.Equal(resp.Body, data) {
			t.Error("downloaded bytes differ from the uploaded image")
		}
	})

	// An image id is only meaningful under the document it hangs off; the same
	// id addressed through a licence must not resolve.
	t.Run("not reachable through a licence URL", func(t *testing.T) {
		licID := createFileTestLicense(t, c, "DE-IMG-CROSS")
		requireStatus(t, c.GET(fmt.Sprintf("/licenses/%s/files/%s", licID, imageID)), http.StatusNotFound)
	})

	t.Run("unknown credential", func(t *testing.T) {
		requireStatus(t, c.GET("/credentials/00000000-0000-0000-0000-000000000000/files"), http.StatusNotFound)
	})

	t.Run("deleting the credential removes its files", func(t *testing.T) {
		requireStatus(t, c.DELETE("/credentials/"+credID), http.StatusNoContent)
		requireStatus(t, c.GET(base), http.StatusNotFound)
	})
}

// The three German radio certificates are new enum members; a round-trip
// through create/list is what proves the spec, model and column agree.
func TestGermanRadioCredentialTypes(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("radio"), "SecurePass123!", "Radio User")

	for _, credType := range []string{"RADIO_BZF2", "RADIO_BZF1", "RADIO_AZF"} {
		t.Run(credType, func(t *testing.T) {
			resp := c.POST("/credentials", map[string]interface{}{
				"credentialType": credType, "issueDate": "2021-09-01",
				"issuingAuthority": "Bundesnetzagentur",
			})
			requireStatus(t, resp, http.StatusCreated)
			var cred map[string]interface{}
			resp.JSON(&cred)
			if cred["credentialType"] != credType {
				t.Errorf("credentialType = %v, want %v", cred["credentialType"], credType)
			}
			// These certificates do not expire — the API must accept that.
			if cred["expiryDate"] != nil {
				t.Errorf("expiryDate = %v, want null", cred["expiryDate"])
			}
		})
	}
}

// e2ePDF is a minimal but structurally real PDF — signature and %%EOF trailer,
// which is exactly what the API checks for.
func e2ePDF() []byte {
	return []byte("%PDF-1.4\n" +
		"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n" +
		"2 0 obj<</Type/Pages/Kids[]/Count 0>>endobj\n" +
		"trailer<</Root 1 0 R>>\n" +
		"%%EOF\n")
}

// A PDF is the format an authority actually issues, so it must round-trip —
// and it must come back as an attachment, never inline, because nothing on the
// server parsed it and it can carry active content.
func TestDocumentFilePDF(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("pdf"), "SecurePass123!", "PDF User")
	licID := createFileTestLicense(t, c, "DE-PDF-001")
	base := fmt.Sprintf("/licenses/%s/files", licID)
	data := e2ePDF()

	var fileID string

	t.Run("upload", func(t *testing.T) {
		resp := uploadDocumentFile(t, c, base, "licence.pdf", "application/pdf", "Official scan", data)
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		fileID = f["id"].(string)
		if f["contentType"] != "application/pdf" {
			t.Errorf("contentType = %v, want application/pdf", f["contentType"])
		}
		// No intrinsic pixel size, so the API must report null rather than 0.
		if f["width"] != nil || f["height"] != nil {
			t.Errorf("dimensions = %v×%v, want null for a PDF", f["width"], f["height"])
		}
	})

	t.Run("served as an attachment, never inline", func(t *testing.T) {
		resp := c.GET(base + "/" + fileID)
		requireStatus(t, resp, http.StatusOK)
		if ct := resp.Headers.Get("Content-Type"); ct != "application/pdf" {
			t.Errorf("Content-Type = %q, want application/pdf", ct)
		}
		cd := resp.Headers.Get("Content-Disposition")
		if !strings.HasPrefix(cd, "attachment") {
			t.Errorf("Content-Disposition = %q, want it to start with attachment", cd)
		}
		if !bytes.Equal(resp.Body, data) {
			t.Error("downloaded bytes differ from the uploaded PDF")
		}
	})

	t.Run("an image is still served inline", func(t *testing.T) {
		resp := uploadDocumentFile(t, c, base, "front.png", "image/png", "", e2ePNG(t, 32, 32))
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)

		got := c.GET(base + "/" + f["id"].(string))
		requireStatus(t, got, http.StatusOK)
		if cd := got.Headers.Get("Content-Disposition"); !strings.HasPrefix(cd, "inline") {
			t.Errorf("Content-Disposition = %q, want it to start with inline", cd)
		}
	})

	t.Run("a truncated PDF is refused", func(t *testing.T) {
		resp := uploadDocumentFile(t, c, base, "broken.pdf", "application/pdf", "", data[:len(data)-8])
		requireStatus(t, resp, http.StatusBadRequest)
	})
}
