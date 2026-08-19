package airports

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// packAirport is one record in the downloadable pack, shaped like the
// Airport schema in the OpenAPI spec.
type packAirport struct {
	ICAO      string  `json:"icao"`
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Elevation int     `json:"elevation,omitempty"`
	Country   string  `json:"country,omitempty"`
}

// packEnvelope is the document inside the gzip stream.
type packEnvelope struct {
	Etag        string          `json:"etag"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Count       int             `json:"count"`
	Airports    json.RawMessage `json:"airports"`
}

// PackStatus describes the downloadable pack built from the live snapshot.
type PackStatus struct {
	Etag        string
	GeneratedAt time.Time
	Count       int
	SizeBytes   int
}

// Pack returns the gzip-compressed pack of the live snapshot and its status.
// ok is false when no database is loaded.
func Pack() ([]byte, PackStatus, bool) {
	s := current.Load()
	if s == nil {
		packUnavailable.Inc()
		return nil, PackStatus{}, false
	}
	s.buildPack()
	if len(s.packGz) == 0 {
		packUnavailable.Inc()
		return nil, PackStatus{}, false
	}
	packSuccess.Inc()
	return s.packGz, s.packStatus(), true
}

// PackInfo returns the pack status without the pack bytes. ok is false when
// no database is loaded.
func PackInfo() (PackStatus, bool) {
	s := current.Load()
	if s == nil {
		packStatusUnavailable.Inc()
		return PackStatus{}, false
	}
	s.buildPack()
	if len(s.packGz) == 0 {
		packStatusUnavailable.Inc()
		return PackStatus{}, false
	}
	packStatusSuccess.Inc()
	return s.packStatus(), true
}

func (s *snapshot) packStatus() PackStatus {
	return PackStatus{
		Etag:        s.packEtag,
		GeneratedAt: s.loadedAt,
		Count:       s.count(),
		SizeBytes:   len(s.packGz),
	}
}

// buildPack assembles the pack once per snapshot: the airport list (already
// in ICAO order) as JSON, an etag hashed over those bytes only, and a
// gzip-compressed envelope around both.
func (s *snapshot) buildPack() {
	s.packOnce.Do(func() {
		start := time.Now()

		records := make([]packAirport, len(s.list))
		for i, a := range s.list {
			records[i] = packAirport{
				ICAO:      a.ICAO,
				Name:      a.Name,
				Latitude:  a.Latitude,
				Longitude: a.Longitude,
				Elevation: a.Elevation,
				Country:   a.Country,
			}
		}
		body, err := json.Marshal(records)
		if err != nil {
			return
		}

		sum := sha256.Sum256(body)
		etag := hex.EncodeToString(sum[:16])

		envelope, err := json.Marshal(packEnvelope{
			Etag:        etag,
			GeneratedAt: s.loadedAt.UTC(),
			Count:       len(records),
			Airports:    body,
		})
		if err != nil {
			return
		}

		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			return
		}
		if _, err := zw.Write(envelope); err != nil {
			return
		}
		if err := zw.Close(); err != nil {
			return
		}

		s.packGz = buf.Bytes()
		s.packEtag = etag

		PackBuildDurationSeconds.Observe(time.Since(start).Seconds())
		PackBytes.Set(float64(len(s.packGz)))
	})
}
