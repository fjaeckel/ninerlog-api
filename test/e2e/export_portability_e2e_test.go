//go:build e2e

package e2e_test

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// End-to-end coverage for the leave-whenever-you-want exports.
//
// The unit tests in internal/service/portability pin the layouts against a
// synthetic bundle. These tests check the thing a unit test cannot: that a real
// account, created through the API, comes back out complete — through the
// router, the auth middleware, the services and the database.

// seedPortabilityAccount creates a small but representative logbook: a fleet
// aircraft, a dual training flight, a flight on an aircraft that is not in the
// fleet, plus a licence and a credential.
func seedPortabilityAccount(t *testing.T, c *E2EClient) {
	t.Helper()

	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": "D-EPRT", "type": "C172",
		"make": "Cessna", "model": "172S", "aircraftClass": "SEP_LAND",
	}), http.StatusCreated)

	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": pastDate(20), "aircraftReg": "D-EPRT", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:30",
		"landings": 1, "isDual": true,
		"instructorName": "Karl Fluglehrer",
		"remarks":        "Navigation training",
	}), http.StatusCreated)

	// An aircraft the pilot never added to their fleet. Every export must still
	// carry this flight — that is the whole point.
	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": pastDate(10), "aircraftReg": "G-UNFLT", "aircraftType": "PA28",
		"departureIcao": "EGTB", "arrivalIcao": "EGLM",
		"offBlockTime": "11:00", "onBlockTime": "12:00",
		"landings": 1, "isPic": true,
	}), http.StatusCreated)

	requireStatus(t, c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL(A)",
		"licenseNumber": "DE.FCL.E2E.001", "issueDate": pastDate(900),
		"issuingAuthority": "LBA",
	}), http.StatusCreated)

	requireStatus(t, c.POST("/credentials", map[string]interface{}{
		"credentialType": "EASA_CLASS2_MEDICAL", "issueDate": pastDate(200),
		"issuingAuthority": "AME Müller",
	}), http.StatusCreated)
}

// TestExportTargetsIsDiscoverable checks that clients can enumerate the
// destinations rather than hard-coding them.
func TestExportTargetsIsDiscoverable(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("port-targets"), "SecurePass123!", "PortTargets")

	resp := c.GET("/exports/targets")
	requireStatus(t, resp, http.StatusOK)

	var body struct {
		Targets []struct {
			Id          string `json:"id"`
			Product     string `json:"product"`
			ContentType string `json:"contentType"`
			Extension   string `json:"extension"`
			Notes       string `json:"notes"`
			Verified    bool   `json:"verified"`
		} `json:"targets"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	seen := map[string]bool{}
	for _, target := range body.Targets {
		seen[target.Id] = true
		if target.Product == "" || target.Notes == "" || target.Extension == "" {
			t.Errorf("target %q is missing fields the UI renders: %+v", target.Id, target)
		}
	}
	for _, want := range []string{"foreflight", "logten", "myflightbook", "crewlounge"} {
		if !seen[want] {
			t.Errorf("target %q is not advertised", want)
		}
	}
}

// TestExportLogbookForEveryTarget is the migration path itself: for each
// destination the pilot gets a non-empty file, named for download, carrying
// every flight — including the one on an unfleeted aircraft.
func TestExportLogbookForEveryTarget(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("port-logbook"), "SecurePass123!", "PortLogbook")
	seedPortabilityAccount(t, c)

	for _, target := range []string{"foreflight", "logten", "myflightbook", "crewlounge"} {
		t.Run(target, func(t *testing.T) {
			resp := c.GET("/exports/logbook?target=" + target)
			requireStatus(t, resp, http.StatusOK)

			body := string(resp.Body)
			if len(body) == 0 {
				t.Fatal("export is empty")
			}
			for _, reg := range []string{"D-EPRT", "G-UNFLT"} {
				if !strings.Contains(body, reg) {
					t.Errorf("export is missing the flight on %s", reg)
				}
			}
			if cd := resp.Headers.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
				t.Errorf("Content-Disposition = %q, want an attachment so the browser downloads it", cd)
			}
		})
	}
}

// TestExportLogbookRejectsUnknownTarget pins the 400 rather than letting a
// typo produce an empty file the pilot might trust.
func TestExportLogbookRejectsUnknownTarget(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("port-badtarget"), "SecurePass123!", "PortBad")

	requireStatus(t, c.GET("/exports/logbook?target=logbookpro"), http.StatusBadRequest)
}

// TestExportLogbookRequiresAuth confirms the export is scoped to the caller.
func TestExportLogbookRequiresAuth(t *testing.T) {
	c := NewE2EClient(t)
	c.ClearToken()

	requireStatus(t, c.GET("/exports/logbook?target=foreflight"), http.StatusUnauthorized)
	requireStatus(t, c.GET("/exports/archive"), http.StatusUnauthorized)
	requireStatus(t, c.GET("/exports/targets"), http.StatusUnauthorized)
}

// TestExportLogbookIsScopedToTheCallingPilot is the ownership check that
// matters most here: an export endpoint that leaked another pilot's logbook
// would be a privacy breach of an entire career record.
func TestExportLogbookIsScopedToTheCallingPilot(t *testing.T) {
	owner := NewE2EClient(t)
	registerAndLogin(t, owner, uniqueEmail("port-owner"), "SecurePass123!", "PortOwner")
	seedPortabilityAccount(t, owner)

	stranger := NewE2EClient(t)
	registerAndLogin(t, stranger, uniqueEmail("port-stranger"), "SecurePass123!", "PortStranger")

	resp := stranger.GET("/exports/logbook?target=foreflight")
	requireStatus(t, resp, http.StatusOK)
	for _, reg := range []string{"D-EPRT", "G-UNFLT"} {
		if strings.Contains(string(resp.Body), reg) {
			t.Fatalf("another pilot's aircraft %s appeared in this account's export", reg)
		}
	}
}

// TestExportPortabilityArchive checks the lossless path end to end.
func TestExportPortabilityArchive(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("port-archive"), "SecurePass123!", "PortArchive")
	seedPortabilityAccount(t, c)

	resp := c.GET("/exports/archive")
	requireStatus(t, resp, http.StatusOK)

	if ct := resp.Headers.Get("Content-Type"); !strings.Contains(ct, "zip") {
		t.Errorf("Content-Type = %q, want a zip type", ct)
	}

	r, err := zip.NewReader(bytes.NewReader(resp.Body), int64(len(resp.Body)))
	if err != nil {
		t.Fatalf("archive is not a readable ZIP: %v", err)
	}

	members := map[string]string{}
	for _, f := range r.File {
		rc, oerr := f.Open()
		if oerr != nil {
			t.Fatalf("open %s: %v", f.Name, oerr)
		}
		data, rerr := io.ReadAll(rc)
		rc.Close()
		if rerr != nil {
			t.Fatalf("read %s: %v", f.Name, rerr)
		}
		members[f.Name] = string(data)
	}

	for _, want := range []string{
		"manifest.json", "README.md", "flights.csv", "aircraft.csv",
		"licenses.csv", "class-ratings.csv", "credentials.csv",
		"contacts.csv", "crew.csv", "signatures.csv",
	} {
		if _, ok := members[want]; !ok {
			t.Errorf("archive is missing %s", want)
		}
	}

	// The records no vendor format carries must be here.
	if !strings.Contains(members["licenses.csv"], "DE.FCL.E2E.001") {
		t.Error("licenses.csv does not carry the pilot's licence")
	}
	if !strings.Contains(members["credentials.csv"], "EASA_CLASS2_MEDICAL") {
		t.Error("credentials.csv does not carry the pilot's medical")
	}

	// Both flights, including the one on the unfleeted aircraft.
	records, err := csv.NewReader(strings.NewReader(members["flights.csv"])).ReadAll()
	if err != nil {
		t.Fatalf("flights.csv is not valid CSV: %v", err)
	}
	if got := len(records) - 1; got != 2 {
		t.Errorf("flights.csv has %d data rows, want 2", got)
	}

	// The manifest must agree with what is actually in the archive.
	var manifest struct {
		Format        string         `json:"format"`
		FormatVersion string         `json:"formatVersion"`
		Counts        map[string]int `json:"counts"`
		Files         []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(members["manifest.json"]), &manifest); err != nil {
		t.Fatalf("manifest.json is not valid JSON: %v", err)
	}
	if manifest.Format != "ninerlog-portability-archive" {
		t.Errorf("manifest format = %q", manifest.Format)
	}
	if manifest.Counts["flights"] != 2 {
		t.Errorf("manifest claims %d flights, want 2", manifest.Counts["flights"])
	}
	for _, f := range manifest.Files {
		if _, ok := members[f.Path]; !ok {
			t.Errorf("manifest lists %q but the archive does not contain it", f.Path)
		}
	}
}

// TestExportArchiveForEmptyAccount checks that a pilot who has logged nothing
// still gets a valid archive rather than an error.
func TestExportArchiveForEmptyAccount(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("port-empty"), "SecurePass123!", "PortEmpty")

	resp := c.GET("/exports/archive")
	requireStatus(t, resp, http.StatusOK)

	if _, err := zip.NewReader(bytes.NewReader(resp.Body), int64(len(resp.Body))); err != nil {
		t.Fatalf("archive for an empty account is not a readable ZIP: %v", err)
	}
}
