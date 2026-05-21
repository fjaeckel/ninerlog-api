package cloudbackup

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func makeFlight(t *testing.T, date string, offBlock string) *models.Flight {
	t.Helper()
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}
	id := uuid.New()
	f := &models.Flight{ID: id, Date: d}
	if offBlock != "" {
		f.OffBlockTime = &offBlock
	}
	return f
}

func TestBuildPayloadGzipRoundTrip(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	flights := []*models.Flight{makeFlight(t, "2024-01-01", "08:00:00")}
	aircraft := []*models.Aircraft{{ID: uuid.New(), Registration: "D-ABCD"}}
	credentials := []*models.Credential{}
	licenses := []licenseWithRatings{}

	r, meta, err := buildPayload(now, "1.0", "NinerLog JSON Backup", flights, aircraft, licenses, credentials, 0)
	if err != nil {
		t.Fatalf("buildPayload: %v", err)
	}
	defer r.Close()
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if meta.SizeBytes != int64(len(body)) {
		t.Errorf("size mismatch: meta=%d body=%d", meta.SizeBytes, len(body))
	}
	if meta.ContentType != "application/gzip" {
		t.Errorf("content type: %s", meta.ContentType)
	}
	if meta.Filename != "ninerlog-backup-2024-01-02T03-04-05Z.json.gz" {
		t.Errorf("filename: %s", meta.Filename)
	}
	if meta.FlightCount != 1 || meta.AircraftCount != 1 {
		t.Errorf("counts wrong: %+v", meta)
	}

	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gz.Close()
	raw, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, raw)
	}
	if got["version"] != "1.0" || got["format"] != "NinerLog JSON Backup" {
		t.Errorf("version/format missing: %+v", got)
	}
	if got["exportedAt"] != "2024-01-02T03:04:05Z" {
		t.Errorf("exportedAt: %v", got["exportedAt"])
	}
	if _, ok := got["flights"]; !ok {
		t.Errorf("flights key missing")
	}
}

func TestBuildPayloadSHA256IgnoresExportedAt(t *testing.T) {
	flights := []*models.Flight{makeFlight(t, "2024-01-01", "08:00:00")}
	aircraft := []*models.Aircraft{{ID: uuid.New(), Registration: "D-ABCD"}}

	_, meta1, err := buildPayload(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		"1.0", "NinerLog JSON Backup", flights, aircraft, nil, nil, 0)
	if err != nil {
		t.Fatalf("buildPayload 1: %v", err)
	}
	_, meta2, err := buildPayload(time.Date(2024, 6, 6, 12, 0, 0, 0, time.UTC),
		"1.0", "NinerLog JSON Backup", flights, aircraft, nil, nil, 0)
	if err != nil {
		t.Fatalf("buildPayload 2: %v", err)
	}
	if meta1.SHA256 != meta2.SHA256 {
		t.Errorf("hash should ignore exportedAt: %s vs %s", meta1.SHA256, meta2.SHA256)
	}
}

func TestBuildPayloadSHA256ChangesWhenDataChanges(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	flights1 := []*models.Flight{makeFlight(t, "2024-01-01", "08:00:00")}
	flights2 := []*models.Flight{makeFlight(t, "2024-01-01", "09:00:00")}

	_, m1, _ := buildPayload(now, "1.0", "x", flights1, nil, nil, nil, 0)
	_, m2, _ := buildPayload(now, "1.0", "x", flights2, nil, nil, nil, 0)
	if m1.SHA256 == m2.SHA256 {
		t.Errorf("hash should differ when payload differs")
	}
}

func TestSortFlightsChronological(t *testing.T) {
	f1 := makeFlight(t, "2024-01-02", "10:00:00")
	f2 := makeFlight(t, "2024-01-01", "12:00:00")
	f3 := makeFlight(t, "2024-01-01", "08:00:00")
	flights := []*models.Flight{f1, f2, f3}
	sortFlightsChronological(flights)
	if flights[0] != f3 || flights[1] != f2 || flights[2] != f1 {
		t.Errorf("unexpected order: %+v", flights)
	}
}
