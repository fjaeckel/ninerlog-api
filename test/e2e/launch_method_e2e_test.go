//go:build e2e

package e2e_test

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func gliderFlight(date, reg, method string) map[string]interface{} {
	f := map[string]interface{}{
		"date": date, "aircraftReg": reg, "aircraftType": "ASK21",
		"departureIcao": "EDHM", "arrivalIcao": "EDHM",
		"offBlockTime": "10:00", "onBlockTime": "10:09", "landings": 1,
	}
	if method != "" {
		f["launchMethod"] = method
	}
	return f
}

func TestLaunchMethod_Validation(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("launch-val"), "SecurePass123!", "LaunchVal")

	for _, m := range []string{"winch", "aerotow", "self-launch", "car", "bungee"} {
		t.Run("accepts "+m, func(t *testing.T) {
			requireStatus(t, c.POST("/flights", gliderFlight(today(), "D-1234", m)), http.StatusCreated)
		})
	}
	for _, m := range []string{"catapult", "W", "self-launch-winch"} {
		t.Run("rejects "+m, func(t *testing.T) {
			assertStatus(t, c.POST("/flights", gliderFlight(today(), "D-1234", m)), http.StatusBadRequest)
		})
	}
	t.Run("update rejects an invalid method", func(t *testing.T) {
		r := c.POST("/flights", gliderFlight(today(), "D-1234", "winch"))
		requireStatus(t, r, http.StatusCreated)
		var f map[string]interface{}
		r.JSON(&f)
		id, _ := f["id"].(string)
		assertStatus(t, c.PUT("/flights/"+id, map[string]interface{}{"launchMethod": "catapult"}), http.StatusBadRequest)
	})
}

// L2: a Vereinsflieger export with S.-Art W/F/E imports with the launch
// method set, and the imported gliders are classed GLIDER.
func TestImportVereinsflieger_L2LaunchMethodsAndGliderClass(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("launch-vf"), "SecurePass123!", "Lena Berger")

	d := todayGerman()
	row := func(reg, start, end, mins, code string) string {
		return fmt.Sprintf("\"%s\";\"%s\";\"Berger, Lena\";\"\";\"%s\";\"%s\";\"%s\";\"Hartenholm EDHM\";\"Hartenholm EDHM\";\"1\";\"%s\";\"N\";\"K\";\"LSV Musterstadt e.V.\";\"\"\n",
			d, reg, start, end, mins, code)
	}
	file := "\"Datum\";\"Lfz.\";\"Pilot\";\"Begleiter/FI\";\"Start\";\"Landung\";\"Flugzeit\";\"Startort\";\"Landeort\";\"Landungen\";\"S.-Art\";\"Flugart\";\"Abr.\";\"Verein\";\"Bemerkung\"\n" +
		row("D-1234", "10:02", "10:10", "8", "W") +
		row("D-5678", "13:05", "15:20", "135", "F") +
		row("D-KLSG", "16:00", "17:00", "60", "E") +
		row("D-MXYZ", "17:30", "18:00", "30", "E") +
		row("D-1234", "18:30", "18:38", "8", "X")

	upload := uploadCSV(t, c, "vereinsflieger.csv", file)
	requireStatus(t, upload, http.StatusOK)
	var up map[string]interface{}
	upload.JSON(&up)
	if up["format"] != "VEREINSFLIEGER_CSV" {
		t.Fatalf("format = %v, want VEREINSFLIEGER_CSV", up["format"])
	}
	launchMapped := false
	for _, m := range up["suggestedMappings"].([]interface{}) {
		mm := m.(map[string]interface{})
		if mm["sourceColumn"] == "S.-Art" && mm["targetField"] == "launchMethod" {
			launchMapped = true
		}
	}
	if !launchMapped {
		t.Errorf("S.-Art is not suggested as launchMethod: %v", up["suggestedMappings"])
	}

	conf := c.POST("/imports/confirm", map[string]interface{}{"uploadToken": up["uploadToken"]})
	requireStatus(t, conf, http.StatusCreated)
	var result map[string]interface{}
	conf.JSON(&result)
	if n, _ := result["importedCount"].(float64); n != 5 {
		t.Fatalf("importedCount = %v, want 5 (an unknown S.-Art code must not fail the row): %s", result["importedCount"], conf.Body)
	}

	list := c.GET("/flights")
	requireStatus(t, list, http.StatusOK)
	var page map[string]interface{}
	list.JSON(&page)
	got := map[string]int{}
	for _, item := range page["data"].([]interface{}) {
		f := item.(map[string]interface{})
		m, _ := f["launchMethod"].(string)
		got[fmt.Sprintf("%v %s", f["aircraftReg"], m)]++
	}
	for k, n := range map[string]int{
		"D-1234 winch": 1, "D-5678 aerotow": 1, "D-KLSG self-launch": 1,
		"D-MXYZ self-launch": 1, "D-1234 ": 1,
	} {
		if got[k] != n {
			t.Errorf("flights %q = %d, want %d (all: %v)", k, got[k], n, got)
		}
	}

	fleet := c.GET("/aircraft")
	requireStatus(t, fleet, http.StatusOK)
	var fp map[string]interface{}
	fleet.JSON(&fp)
	classes := map[string]interface{}{}
	for _, item := range fp["data"].([]interface{}) {
		a := item.(map[string]interface{})
		classes[a["registration"].(string)] = a["aircraftClass"]
		if a["ulKind"] != nil {
			t.Errorf("%v: ulKind = %v, want unset", a["registration"], a["ulKind"])
		}
	}
	for reg, want := range map[string]interface{}{
		"D-1234": "GLIDER", "D-5678": "GLIDER", "D-KLSG": nil, "D-MXYZ": "ULTRALIGHT",
	} {
		if classes[reg] != want {
			t.Errorf("aircraft %s class = %v, want %v", reg, classes[reg], want)
		}
	}
}

// An existing aircraft keeps its class when an import names it.
func TestImport_NeverOverwritesExistingAircraftClass(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("launch-keep"), "SecurePass123!", "Keep")

	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": "D-KLSG", "type": "ASK21MI", "make": "Schleicher", "model": "ASK 21 Mi",
		"aircraftClass": "TMG",
	}), http.StatusCreated)

	file := "Date,Registration,Type,From,To,Total Time,Launch Method\n" +
		fmt.Sprintf("%s,D-KLSG,ASK21MI,EDHM,EDHM,0:30,aerotow\n", today())
	upload := uploadCSV(t, c, "sheet.csv", file)
	requireStatus(t, upload, http.StatusOK)
	var up map[string]interface{}
	upload.JSON(&up)
	requireStatus(t, c.POST("/imports/confirm", map[string]interface{}{"uploadToken": up["uploadToken"]}), http.StatusCreated)

	fleet := c.GET("/aircraft")
	var fp map[string]interface{}
	fleet.JSON(&fp)
	data := fp["data"].([]interface{})
	if len(data) != 1 || data[0].(map[string]interface{})["aircraftClass"] != "TMG" {
		t.Errorf("fleet = %s, want D-KLSG still TMG", fleet.Body)
	}
}

// L2/L5: the launch method survives every CSV layout's export and re-import,
// in a column (standard) or in the remarks cell (easa, faa, weblogbook).
func TestExportImportRoundTrip_LaunchMethod(t *testing.T) {
	for _, layout := range []string{"standard", "easa", "faa", "weblogbook"} {
		t.Run(layout, func(t *testing.T) {
			src := NewE2EClient(t)
			registerAndLogin(t, src, uniqueEmail("launch-rt"), "SecurePass123!", "Lena Berger")
			f := gliderFlight(pastDate(3), "D-1234", "winch")
			f["remarks"] = "Thermik"
			requireStatus(t, src.POST("/flights", f), http.StatusCreated)

			exp := src.GET("/exports/csv?format=" + layout)
			requireStatus(t, exp, http.StatusOK)
			if layout == "standard" {
				records, err := csv.NewReader(strings.NewReader(string(exp.Body))).ReadAll()
				if err != nil {
					t.Fatal(err)
				}
				header := records[0]
				if header[len(header)-1] != "LaunchMethod" || records[1][len(header)-1] != "winch" {
					t.Errorf("standard CSV last column = %q/%q, want LaunchMethod/winch", header[len(header)-1], records[1][len(header)-1])
				}
			} else if !strings.Contains(string(exp.Body), "Thermik [Launch: winch]") {
				t.Errorf("%s CSV remarks do not carry the launch method:\n%s", layout, exp.Body)
			}

			dst := NewE2EClient(t)
			registerAndLogin(t, dst, uniqueEmail("launch-rt-dst"), "SecurePass123!", "Lena Berger")
			upload := uploadCSV(t, dst, "ninerlog_"+layout+".csv", string(exp.Body))
			requireStatus(t, upload, http.StatusOK)
			var up map[string]interface{}
			upload.JSON(&up)
			requireStatus(t, dst.POST("/imports/confirm", map[string]interface{}{"uploadToken": up["uploadToken"]}), http.StatusCreated)

			list := dst.GET("/flights")
			requireStatus(t, list, http.StatusOK)
			var page map[string]interface{}
			list.JSON(&page)
			data, _ := page["data"].([]interface{})
			if len(data) != 1 {
				t.Fatalf("imported %d flights, want 1", len(data))
			}
			got := data[0].(map[string]interface{})
			if got["launchMethod"] != "winch" {
				t.Errorf("launchMethod = %v, want winch", got["launchMethod"])
			}
			if rm, _ := got["remarks"].(string); rm != "Thermik" {
				t.Errorf("remarks = %q, want Thermik", rm)
			}

			fleet := dst.GET("/aircraft")
			var fp map[string]interface{}
			fleet.JSON(&fp)
			ac, _ := fp["data"].([]interface{})
			if len(ac) != 1 || ac[0].(map[string]interface{})["aircraftClass"] != "GLIDER" {
				t.Errorf("fleet = %s, want D-1234 as GLIDER", fleet.Body)
			}
		})
	}
}

// L5 (until the sailplane layout): the printed logbook names the launch method
// in the remarks column.
func TestExportPDF_LaunchMethodInRemarks(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("launch-pdf"), "SecurePass123!", "Lena Berger")
	requireStatus(t, c.POST("/flights", gliderFlight(pastDate(2), "D-1234", "winch")), http.StatusCreated)

	for _, format := range []string{"easa", "faa"} {
		t.Run(format, func(t *testing.T) {
			resp := c.GET("/exports/pdf?format=" + format)
			requireStatus(t, resp, http.StatusOK)
			if !strings.Contains(pdfStreamText(resp.Body), "[Launch: winch]") {
				t.Errorf("%s PDF does not carry [Launch: winch]", format)
			}
		})
	}
}

func TestJSONImport_LaunchMethod(t *testing.T) {
	src := NewE2EClient(t)
	registerAndLogin(t, src, uniqueEmail("launch-json"), "SecurePass123!", "Lena Berger")
	requireStatus(t, src.POST("/flights", gliderFlight(pastDate(4), "D-1234", "aerotow")), http.StatusCreated)

	exp := src.GET("/exports/json")
	requireStatus(t, exp, http.StatusOK)
	var backup map[string]interface{}
	if err := json.Unmarshal(exp.Body, &backup); err != nil {
		t.Fatal(err)
	}

	t.Run("valid method is restored", func(t *testing.T) {
		dst := NewE2EClient(t)
		registerAndLogin(t, dst, uniqueEmail("launch-json-ok"), "SecurePass123!", "Dst")
		requireStatus(t, dst.POST("/imports/json", backup), http.StatusOK)
		list := dst.GET("/flights")
		var page map[string]interface{}
		list.JSON(&page)
		data, _ := page["data"].([]interface{})
		if len(data) != 1 || data[0].(map[string]interface{})["launchMethod"] != "aerotow" {
			t.Errorf("restored flights = %s, want one aerotow flight", list.Body)
		}
	})

	t.Run("invalid method fails the restore", func(t *testing.T) {
		flights := backup["flights"].([]interface{})
		flights[0].(map[string]interface{})["launchMethod"] = "catapult"
		dst := NewE2EClient(t)
		registerAndLogin(t, dst, uniqueEmail("launch-json-bad"), "SecurePass123!", "Dst")
		r := dst.POST("/imports/json", backup)
		assertStatus(t, r, http.StatusBadRequest)
		if !strings.Contains(string(r.Body), "invalid launch method") {
			t.Errorf("error = %s, want it to name the invalid launch method", r.Body)
		}
		list := dst.GET("/flights")
		var page map[string]interface{}
		list.JSON(&page)
		if data, _ := page["data"].([]interface{}); len(data) != 0 {
			t.Errorf("an invalid flight was stored: %s", list.Body)
		}
	})
}

// A2: Vereinsflieger "E" on a Cessna (SEP_LAND) imports with no launch method,
// so no launch marker reaches its printed or exported remarks.
func TestImportVereinsflieger_A2NoLaunchMethodOnPoweredAircraft(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("launch-a2"), "SecurePass123!", "Mark")
	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": "D-EABC", "type": "C172", "make": "Cessna", "model": "172S",
		"aircraftClass": "SEP_LAND",
	}), http.StatusCreated)

	d := todayGerman()
	file := "\"Datum\";\"Lfz.\";\"Pilot\";\"Begleiter/FI\";\"Start\";\"Landung\";\"Flugzeit\";\"Startort\";\"Landeort\";\"Landungen\";\"S.-Art\";\"Flugart\";\"Abr.\";\"Verein\";\"Bemerkung\"\n" +
		fmt.Sprintf("\"%s\";\"D-EABC\";\"Rivera, Alex\";\"\";\"09:12\";\"10:47\";\"95\";\"Uetersen EDHE\";\"Stade EDHS\";\"1\";\"E\";\"N\";\"K\";\"Aero-Club\";\"Rundflug\"\n", d) +
		fmt.Sprintf("\"%s\";\"D-1234\";\"Rivera, Alex\";\"\";\"12:00\";\"12:08\";\"8\";\"Uetersen EDHE\";\"Uetersen EDHE\";\"1\";\"W\";\"N\";\"K\";\"Aero-Club\";\"\"\n", d)
	upload := uploadCSV(t, c, "vereinsflieger.csv", file)
	requireStatus(t, upload, http.StatusOK)
	var up map[string]interface{}
	upload.JSON(&up)
	requireStatus(t, c.POST("/imports/confirm", map[string]interface{}{"uploadToken": up["uploadToken"]}), http.StatusCreated)

	list := c.GET("/flights")
	requireStatus(t, list, http.StatusOK)
	var page map[string]interface{}
	list.JSON(&page)
	for _, item := range page["data"].([]interface{}) {
		f := item.(map[string]interface{})
		switch f["aircraftReg"] {
		case "D-EABC":
			if f["launchMethod"] != nil {
				t.Errorf("D-EABC launchMethod = %v, want none", f["launchMethod"])
			}
			if f["remarks"] != "Rundflug" {
				t.Errorf("D-EABC remarks = %v, want Rundflug", f["remarks"])
			}
		case "D-1234":
			if f["launchMethod"] != "winch" {
				t.Errorf("D-1234 launchMethod = %v, want winch", f["launchMethod"])
			}
		}
	}

	exp := c.GET("/exports/csv?format=easa")
	requireStatus(t, exp, http.StatusOK)
	for _, line := range strings.Split(string(exp.Body), "\n") {
		if strings.Contains(line, "D-EABC") && strings.Contains(line, "[Launch:") {
			t.Errorf("EASA CSV row for D-EABC carries a launch marker: %s", line)
		}
	}
}
