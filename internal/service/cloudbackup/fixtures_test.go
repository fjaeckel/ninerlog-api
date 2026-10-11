package cloudbackup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/testutil/restorefake"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every backup under testdata was written by a released export and must keep
// restoring into an empty account, without warnings, for as long as the
// project exists. A payload change that breaks one of them is a breaking
// change to every backup already in a user's hands.

func TestRestore_CommittedBackupsStillRestore(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "backup-v*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			raw, err := os.ReadFile(file)
			require.NoError(t, err)
			var p Payload
			require.NoError(t, json.Unmarshal(raw, &p))

			wantVersion := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(file), "backup-v"), ".json")
			assert.Equal(t, wantVersion, p.Version)

			store := restorefake.New()
			res, err := NewRestorer(store).Restore(context.Background(), uuid.New(), &p, RestoreModeRestore)

			require.NoError(t, err)
			assert.Empty(t, res.Warnings)
			assert.Equal(t, len(p.Flights), res.FlightsImported)
			assert.Equal(t, len(p.Aircraft), res.AircraftImported)
			assert.Equal(t, len(p.Licenses), res.LicensesImported)
			assert.Equal(t, len(p.Credentials), res.CredentialsImported)
			assert.Equal(t, len(p.Contacts), res.ContactsImported)
			assert.Equal(t, len(p.CustomCurrencyRules), res.CustomCurrencyRulesImported)
			assert.Equal(t, len(p.CustomReports), res.CustomReportsImported)
			assert.True(t, res.NotificationPreferencesImported)
			assert.True(t, res.FlightBaselineImported)
			assert.Zero(t, res.ContactsCreated)
			assert.Equal(t, len(p.FlightRevisions), res.FlightRevisionsImported)
			assert.Equal(t, len(p.LogbookCertifications), res.LogbookCertificationsImported)
		})
	}
}

func TestRestore_CurrentVersionHasACommittedBackup(t *testing.T) {
	_, err := os.Stat(filepath.Join("testdata", "backup-v"+PayloadVersion+".json"))
	require.NoError(t, err, "commit a fictional backup written by the current export as testdata/backup-v%s.json", PayloadVersion)
}

func TestRestore_CommittedBackupHasEverySection(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "backup-v"+PayloadVersion+".json"))
	require.NoError(t, err)
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &doc))
	for _, section := range []string{
		"flights", "aircraft", "licenses", "credentials", "contacts",
		"customCurrencyRules", "customReports", "notificationPreferences", "flightBaseline",
		"logbookMode", "flightRevisions", "logbookCertifications",
	} {
		assert.Contains(t, doc, section)
	}
}
