package cloudbackup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/testutil/restorefake"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func winchIGC(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "pkg", "igc", "testdata", "winch.igc"))
	require.NoError(t, err)
	return b
}

func TestRestore_FlightFiles(t *testing.T) {
	data := winchIGC(t)
	file := func(flightID uuid.UUID) FlightFile {
		return FlightFile{
			FlightID: flightID, Kind: string(models.FlightFileKindIGC), Filename: "winch.igc",
			SizeBytes: len(data), SHA256: hexSum(data), ContentEncoding: FlightFileEncodingGzipBase64, Content: gz(t, data),
		}
	}

	t.Run("attached to the restored flight; orphans and repeats skipped", func(t *testing.T) {
		store := restorefake.New()
		p := samplePayload()
		flightID := p.Flights[0].ID
		p.FlightFiles = []FlightFile{file(flightID), file(flightID), file(uuid.New())}

		res, err := NewRestorer(store).Restore(context.Background(), uuid.New(), p, "")
		require.NoError(t, err)
		assert.Equal(t, 1, res.FlightFilesImported)
		assert.Equal(t, 2, res.FlightFilesSkipped)
		require.Len(t, store.State.FlightFiles, 1)
		assert.Equal(t, store.State.Flights[0].ID, store.State.FlightFiles[0].FlightID)
		assert.Equal(t, data, store.State.FlightFiles[0].Content)
	})

	t.Run("corrupt content fails the restore", func(t *testing.T) {
		store := restorefake.New()
		p := samplePayload()
		bad := file(p.Flights[0].ID)
		bad.SHA256 = hexSum([]byte("other"))
		p.FlightFiles = []FlightFile{bad}

		_, err := NewRestorer(store).Restore(context.Background(), uuid.New(), p, "")
		require.Error(t, err)
		assert.Zero(t, store.Commits)
	})
}
