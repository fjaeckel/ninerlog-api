//go:build e2e

package e2e_test

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func igcFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "pkg", "igc", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// postIGC sends a multipart IGC upload with optional extra form fields.
func postIGC(t *testing.T, c *E2EClient, path, filename string, data []byte, fields map[string]string) *Response {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write part: %v", err)
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("write field: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req, _ := http.NewRequest("POST", baseURL+"/api/v1"+path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return &Response{StatusCode: resp.StatusCode, Body: body, Headers: resp.Header}
}

type igcImportResponse struct {
	Flight  map[string]interface{} `json:"flight"`
	FileID  string                 `json:"fileId"`
	Summary map[string]interface{} `json:"summary"`
}

func TestIGCImport(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("igc"), "SecurePass123!", "Petra Example")

	selfLaunch := igcFixture(t, "selflaunch.igc")
	winch := igcFixture(t, "winch.igc")

	t.Run("feature probe reports flight file limits", func(t *testing.T) {
		resp := c.GET("/features")
		requireStatus(t, resp, http.StatusOK)
		var features map[string]interface{}
		resp.JSON(&features)
		ff, ok := features["flightFiles"].(map[string]interface{})
		if !ok || ff["maxBytes"].(float64) != 5*1024*1024 || ff["maxPerFlight"].(float64) != 5 {
			t.Fatalf("flightFiles = %v", features["flightFiles"])
		}
	})

	t.Run("preview does not store anything", func(t *testing.T) {
		resp := postIGC(t, c, "/flights/igc/preview", "p2.igc", selfLaunch, nil)
		requireStatus(t, resp, http.StatusOK)
		var p map[string]interface{}
		resp.JSON(&p)
		if p["launchMethod"] != "self-launch" || p["outlanding"] != true || p["gliderRegistration"] != "D-KXYZ" {
			t.Errorf("preview = %v", p)
		}
		list := c.GET("/flights")
		requireStatus(t, list, http.StatusOK)
		if strings.Contains(string(list.Body), "D-KXYZ") {
			t.Error("preview created a flight")
		}
	})

	var p2 igcImportResponse
	t.Run("P2 Petra self-launched out-and-return with an outlanding imports as one flight with launch method self-launch, outlanding flag and distance", func(t *testing.T) {
		resp := postIGC(t, c, "/flights/igc", "2026-08-10-PX.igc", selfLaunch, nil)
		requireStatus(t, resp, http.StatusCreated)
		resp.JSON(&p2)
		f := p2.Flight
		if f["aircraftReg"] != "D-KXYZ" || f["aircraftType"] != "ASG 29E" || f["date"] != "2026-08-10" {
			t.Errorf("flight = %v %v %v", f["aircraftReg"], f["aircraftType"], f["date"])
		}
		if f["launchMethod"] != "self-launch" {
			t.Errorf("launchMethod = %v", f["launchMethod"])
		}
		if f["isOutlanding"] != true {
			t.Errorf("isOutlanding = %v", f["isOutlanding"])
		}
		if f["departureIcao"] != "EDER" {
			t.Errorf("departureIcao = %v", f["departureIcao"])
		}
		if f["departureTime"] != "09:31:46" || f["arrivalTime"] != "10:22:46" || f["totalTime"].(float64) != 51 {
			t.Errorf("times = %v-%v total %v", f["departureTime"], f["arrivalTime"], f["totalTime"])
		}
		if rh, _ := f["releaseHeightM"].(float64); rh < 700 || rh > 800 {
			t.Errorf("releaseHeightM = %v", f["releaseHeightM"])
		}
		if d, _ := p2.Summary["freeDistanceKm"].(float64); d < 39 || d > 41 {
			t.Errorf("freeDistanceKm = %v", p2.Summary["freeDistanceKm"])
		}
		if d, _ := p2.Summary["outAndReturnDistanceKm"].(float64); d < 78 || d > 82 {
			t.Errorf("outAndReturnDistanceKm = %v", p2.Summary["outAndReturnDistanceKm"])
		}
		if p2.FileID == "" {
			t.Error("no fileId")
		}

		ac := c.GET("/aircraft")
		requireStatus(t, ac, http.StatusOK)
		if !strings.Contains(string(ac.Body), `"D-KXYZ"`) || !strings.Contains(string(ac.Body), `"GLIDER"`) {
			t.Errorf("aircraft not auto-created as GLIDER: %s", ac.Body)
		}
	})

	t.Run("re-importing the same file is a conflict", func(t *testing.T) {
		assertStatus(t, postIGC(t, c, "/flights/igc", "again.igc", selfLaunch, nil), http.StatusConflict)
	})

	t.Run("L winch fixture detected as winch with release height", func(t *testing.T) {
		resp := postIGC(t, c, "/flights/igc", "winch.igc", winch, nil)
		requireStatus(t, resp, http.StatusCreated)
		var res igcImportResponse
		resp.JSON(&res)
		if res.Flight["launchMethod"] != "winch" || res.Summary["launchMethod"] != "winch" {
			t.Errorf("launch = %v / %v", res.Flight["launchMethod"], res.Summary["launchMethod"])
		}
		if rh, _ := res.Flight["releaseHeightM"].(float64); rh < 380 || rh > 460 {
			t.Errorf("releaseHeightM = %v", res.Flight["releaseHeightM"])
		}
		if res.Flight["isOutlanding"] != false {
			t.Errorf("isOutlanding = %v", res.Flight["isOutlanding"])
		}
		if res.Flight["launches"].(float64) != 1 {
			t.Errorf("launches = %v", res.Flight["launches"])
		}
	})

	t.Run("attach to an existing flight", func(t *testing.T) {
		created := c.POST("/flights", map[string]interface{}{
			"date": "2026-07-02", "aircraftReg": "D-5678", "aircraftType": "LS4",
			"departureIcao": "EDER", "arrivalIcao": "EDER",
			"departureTime": "12:01", "arrivalTime": "12:23", "landings": 1, "launchMethod": "aerotow",
		})
		requireStatus(t, created, http.StatusCreated)
		var fl map[string]interface{}
		created.JSON(&fl)
		flightID := fl["id"].(string)

		preview := postIGC(t, c, "/flights/igc/preview", "tow.igc", igcFixture(t, "aerotow.igc"), nil)
		requireStatus(t, preview, http.StatusOK)
		var p map[string]interface{}
		preview.JSON(&p)
		if p["matchingFlightId"] != flightID {
			t.Errorf("matchingFlightId = %v, want %s", p["matchingFlightId"], flightID)
		}

		resp := postIGC(t, c, "/flights/igc", "tow.igc", igcFixture(t, "aerotow.igc"), map[string]string{"flightId": flightID})
		requireStatus(t, resp, http.StatusCreated)
		var res igcImportResponse
		resp.JSON(&res)
		if res.Flight["id"] != flightID || res.Flight["totalTime"].(float64) != 22 {
			t.Errorf("flight = %v, total %v", res.Flight["id"], res.Flight["totalTime"])
		}
		list := c.GET("/flights/" + flightID + "/files")
		requireStatus(t, list, http.StatusOK)
		var files []map[string]interface{}
		list.JSON(&files)
		if len(files) != 1 || files[0]["kind"] != "IGC" || files[0]["filename"] != "tow.igc" || files[0]["id"] != res.FileID {
			t.Errorf("files = %v", files)
		}
	})

	t.Run("download round trip", func(t *testing.T) {
		flightID := p2.Flight["id"].(string)
		resp := c.GET(fmt.Sprintf("/flights/%s/files/%s", flightID, p2.FileID))
		requireStatus(t, resp, http.StatusOK)
		if !bytes.Equal(resp.Body, selfLaunch) {
			t.Errorf("downloaded %d bytes, want the %d uploaded", len(resp.Body), len(selfLaunch))
		}
		if ct := resp.Headers.Get("Content-Type"); ct != "application/octet-stream" {
			t.Errorf("Content-Type = %q", ct)
		}
		if cd := resp.Headers.Get("Content-Disposition"); cd != `attachment; filename="2026-08-10-PX.igc"` {
			t.Errorf("Content-Disposition = %q", cd)
		}
	})

	t.Run("rejects files that are not IGC, too large or without a take-off", func(t *testing.T) {
		bad := postIGC(t, c, "/flights/igc/preview", "x.csv", []byte("date,reg\n2026-01-01,D-1234\n"), nil)
		requireStatus(t, bad, http.StatusBadRequest)
		if !strings.Contains(string(bad.Body), "IGC A record") {
			t.Errorf("body = %s", bad.Body)
		}
		ground := []byte("AXXX\nHFDTE010726\nB1000005000000N01000000EA0010000100\nB1000105000000N01000000EA0010000100\n")
		assertStatus(t, postIGC(t, c, "/flights/igc", "g.igc", ground, nil), http.StatusBadRequest)
		big := append([]byte("AXXX\nHFDTE010726\n"), bytes.Repeat([]byte("LXXX padding padding padding\n"), 200000)...)
		assertStatus(t, postIGC(t, c, "/flights/igc/preview", "big.igc", big, nil), http.StatusRequestEntityTooLarge)
	})

	t.Run("cross-user access is 404", func(t *testing.T) {
		other := NewE2EClient(t)
		registerAndLogin(t, other, uniqueEmail("igc-other"), "SecurePass123!", "Other Pilot")
		flightID := p2.Flight["id"].(string)
		assertStatus(t, other.GET("/flights/"+flightID+"/files"), http.StatusNotFound)
		assertStatus(t, other.GET(fmt.Sprintf("/flights/%s/files/%s", flightID, p2.FileID)), http.StatusNotFound)
		assertStatus(t, other.DELETE(fmt.Sprintf("/flights/%s/files/%s", flightID, p2.FileID)), http.StatusNotFound)
		assertStatus(t, postIGC(t, other, "/flights/igc", "w.igc", igcFixture(t, "outlanding.igc"), map[string]string{"flightId": flightID}), http.StatusNotFound)
		requireStatus(t, c.GET(fmt.Sprintf("/flights/%s/files/%s", flightID, p2.FileID)), http.StatusOK)
	})

	t.Run("export and import round trip carries the file", func(t *testing.T) {
		exp := c.GET("/exports/json")
		requireStatus(t, exp, http.StatusOK)
		var backup map[string]interface{}
		exp.JSON(&backup)
		files, _ := backup["flightFiles"].([]interface{})
		if len(files) != 3 {
			t.Fatalf("exported %d flight files, want 3", len(files))
		}
		first := files[0].(map[string]interface{})
		if first["contentEncoding"] != "gzip+base64" || first["content"] == "" || first["kind"] != "IGC" {
			t.Errorf("exported file = %v", first["contentEncoding"])
		}

		dest := NewE2EClient(t)
		registerAndLogin(t, dest, uniqueEmail("igc-restore"), "SecurePass123!", "Petra Example")
		imp := dest.POST("/imports/json", backup)
		requireStatus(t, imp, http.StatusOK)
		var summary map[string]interface{}
		imp.JSON(&summary)
		if summary["flightFilesImported"].(float64) != 3 || summary["flightFilesSkipped"].(float64) != 0 {
			t.Fatalf("summary = %v", summary)
		}

		list := dest.GET("/flights?pageSize=100")
		requireStatus(t, list, http.StatusOK)
		var page struct {
			Data []map[string]interface{} `json:"data"`
		}
		list.JSON(&page)
		var restoredID string
		for _, f := range page.Data {
			if f["aircraftReg"] == "D-KXYZ" {
				restoredID = f["id"].(string)
			}
		}
		if restoredID == "" {
			t.Fatal("restored P2 flight not found")
		}
		rf := dest.GET("/flights/" + restoredID + "/files")
		requireStatus(t, rf, http.StatusOK)
		var restored []map[string]interface{}
		rf.JSON(&restored)
		if len(restored) != 1 {
			t.Fatalf("restored files = %v", restored)
		}
		dl := dest.GET(fmt.Sprintf("/flights/%s/files/%s", restoredID, restored[0]["id"]))
		requireStatus(t, dl, http.StatusOK)
		if !bytes.Equal(dl.Body, selfLaunch) {
			t.Error("restored content differs from the original")
		}

		again := dest.POST("/imports/json", backup)
		requireStatus(t, again, http.StatusOK)
		again.JSON(&summary)
		if summary["flightFilesImported"].(float64) != 0 || summary["flightFilesSkipped"].(float64) != 3 {
			t.Errorf("second restore summary = %v", summary)
		}
	})

	t.Run("delete removes the file, not the flight", func(t *testing.T) {
		flightID := p2.Flight["id"].(string)
		assertStatus(t, c.DELETE(fmt.Sprintf("/flights/%s/files/%s", flightID, p2.FileID)), http.StatusNoContent)
		assertStatus(t, c.GET(fmt.Sprintf("/flights/%s/files/%s", flightID, p2.FileID)), http.StatusNotFound)
		requireStatus(t, c.GET("/flights/"+flightID), http.StatusOK)
	})
}
