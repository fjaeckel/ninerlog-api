package handlers

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/gin-gonic/gin"
)

func packTestDB() map[string]airports.AirportInfo {
	return map[string]airports.AirportInfo{
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt am Main", Latitude: 50.0333, Longitude: 8.5706, Elevation: 364, Country: "DE"},
		"LOWI": {ICAO: "LOWI", Name: "Innsbruck", Latitude: 47.2602, Longitude: 11.3439, Elevation: 1907, Country: "AT"},
	}
}

func TestGetAirportPack(t *testing.T) {
	gin.SetMode(gin.TestMode)
	airports.SetTestDB(packTestDB())
	defer airports.SetTestDB(nil)

	h := &APIHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.GetAirportPack(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/gzip" {
		t.Errorf("Content-Type = %q, want application/gzip", ct)
	}

	zr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("body is not gzip: %v", err)
	}
	defer zr.Close()
	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("reading pack: %v", err)
	}

	var env struct {
		Etag        string          `json:"etag"`
		Count       int             `json:"count"`
		Airports    json.RawMessage `json:"airports"`
		GeneratedAt string          `json:"generatedAt"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshalling envelope: %v", err)
	}
	if env.Count != 2 || env.Etag == "" || env.GeneratedAt == "" {
		t.Errorf("unexpected envelope: %+v", env)
	}

	var records []map[string]any
	if err := json.Unmarshal(env.Airports, &records); err != nil {
		t.Fatalf("unmarshalling airports: %v", err)
	}
	if len(records) != 2 || records[0]["icao"] != "EDDF" {
		t.Errorf("unexpected records: %v", records)
	}
}

func TestGetAirportPack_Unavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	airports.SetTestDB(nil)

	h := &APIHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.GetAirportPack(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestGetAirportPackStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	airports.SetTestDB(packTestDB())
	defer airports.SetTestDB(nil)

	h := &APIHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.GetAirportPackStatus(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var status struct {
		Etag        string `json:"etag"`
		GeneratedAt string `json:"generatedAt"`
		Count       int    `json:"count"`
		SizeBytes   int    `json:"sizeBytes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("unmarshalling status: %v", err)
	}
	if status.Etag == "" || status.GeneratedAt == "" || status.Count != 2 || status.SizeBytes <= 0 {
		t.Errorf("unexpected status: %+v", status)
	}
}

func TestGetAirportPackStatus_Unavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	airports.SetTestDB(nil)

	h := &APIHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.GetAirportPackStatus(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}
