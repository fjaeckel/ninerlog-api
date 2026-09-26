package cloudbackup

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func gz(t *testing.T, data []byte) string {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(data)
	_ = w.Close()
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func hexSum(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

func TestFlightFile_RoundTrip(t *testing.T) {
	content := []byte("AXXX\r\nHFDTE150708\r\nB1000005000000N01000000EA0010000100\r\n")
	stored := &models.FlightFile{
		FlightID: uuid.New(), Kind: models.FlightFileKindIGC, Filename: "a.igc",
		SizeBytes: len(content), SHA256: hexSum(content), Content: content,
	}
	ff, err := NewFlightFile(stored)
	if err != nil {
		t.Fatal(err)
	}
	if ff.ContentEncoding != FlightFileEncodingGzipBase64 || ff.FlightID != stored.FlightID || ff.Kind != "IGC" {
		t.Errorf("portable = %+v", ff)
	}
	again, _ := NewFlightFile(stored)
	if again.Content != ff.Content {
		t.Error("encoding is not deterministic")
	}
	got, err := ff.Decode()
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("decode = %q, %v", got, err)
	}
}

func TestFlightFile_DecodeRejects(t *testing.T) {
	content := []byte("AXXX\n")
	big := bytes.Repeat([]byte("B"), models.MaxFlightFileBytes+1)
	tests := []struct {
		name string
		ff   FlightFile
	}{
		{"not base64", FlightFile{ContentEncoding: FlightFileEncodingGzipBase64, Content: "!!!"}},
		{"not gzip", FlightFile{ContentEncoding: FlightFileEncodingGzipBase64, Content: base64.StdEncoding.EncodeToString(content)}},
		{"unknown encoding", FlightFile{ContentEncoding: "zstd", Content: gz(t, content)}},
		{"SHA-256 mismatch", FlightFile{ContentEncoding: FlightFileEncodingGzipBase64, Content: gz(t, content), SHA256: hexSum([]byte("other"))}},
		{"decompresses beyond the size cap", FlightFile{ContentEncoding: FlightFileEncodingGzipBase64, Content: gz(t, big)}},
		{"empty", FlightFile{ContentEncoding: "base64", Content: ""}},
		{"encoded beyond the size cap", FlightFile{ContentEncoding: "base64", Content: base64.StdEncoding.EncodeToString(big)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.ff.Decode(); !errors.Is(err, ErrInvalidFlightFileContent) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	t.Run("plain base64 is accepted", func(t *testing.T) {
		ff := FlightFile{ContentEncoding: "base64", Content: base64.StdEncoding.EncodeToString(content), SHA256: hexSum(content)}
		if got, err := ff.Decode(); err != nil || !bytes.Equal(got, content) {
			t.Fatalf("decode = %q, %v", got, err)
		}
	})
}
