package handlers

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func resetSessions() {
	sessionMu.Lock()
	uploadSessions = make(map[string]*uploadSession)
	sessionMu.Unlock()
}

func TestStoreSession_EnforcesPerUserCap(t *testing.T) {
	resetSessions()
	defer resetSessions()

	user := uuid.New()
	for i := 0; i < maxSessionsPerUser; i++ {
		if err := storeSession(fmt.Sprintf("tok-%d", i), &uploadSession{userID: user, createdAt: time.Now()}); err != nil {
			t.Fatalf("session %d rejected early: %v", i, err)
		}
	}
	if err := storeSession("one-too-many", &uploadSession{userID: user, createdAt: time.Now()}); err == nil {
		t.Errorf("expected rejection past %d concurrent sessions for one user", maxSessionsPerUser)
	}

	// A different user is unaffected by another user's usage.
	if err := storeSession("other-user", &uploadSession{userID: uuid.New(), createdAt: time.Now()}); err != nil {
		t.Errorf("unrelated user was blocked: %v", err)
	}
}

func TestStoreSession_EnforcesGlobalCap(t *testing.T) {
	resetSessions()
	defer resetSessions()

	for i := 0; i < maxTotalSessions; i++ {
		_ = storeSession(fmt.Sprintf("g-%d", i), &uploadSession{userID: uuid.New(), createdAt: time.Now()})
	}
	if err := storeSession("over", &uploadSession{userID: uuid.New(), createdAt: time.Now()}); err == nil {
		t.Errorf("expected rejection past the %d global session cap", maxTotalSessions)
	}
}

func TestCleanupOldSessions_EvictsExpired(t *testing.T) {
	resetSessions()
	defer resetSessions()

	_ = storeSession("fresh", &uploadSession{userID: uuid.New(), createdAt: time.Now()})
	sessionMu.Lock()
	uploadSessions["stale"] = &uploadSession{userID: uuid.New(), createdAt: time.Now().Add(-2 * sessionTTL)}
	sessionMu.Unlock()

	cleanupOldSessions()

	sessionMu.Lock()
	defer sessionMu.Unlock()
	if _, ok := uploadSessions["stale"]; ok {
		t.Error("expired session was not evicted")
	}
	if _, ok := uploadSessions["fresh"]; !ok {
		t.Error("live session was wrongly evicted")
	}
}

// An oversized file must be refused during parsing rather than materialized.
func TestParseCSV_RejectsTooManyRows(t *testing.T) {
	var b strings.Builder
	b.WriteString("Date,AircraftID,From,To,TotalTime\n")
	for i := 0; i < maxImportRows+10; i++ {
		b.WriteString("2026-01-01,D-ABCD,EDDF,EDDM,1.0\n")
	}

	_, _, _, err := parseCSV([]byte(b.String()))
	if err == nil {
		t.Fatal("expected a row-cap error for an oversized file")
	}
	if !strings.Contains(err.Error(), "too many rows") {
		t.Errorf("error = %v, want a row-cap error", err)
	}
}

func TestParseCSV_AcceptsNormalFile(t *testing.T) {
	csv := "Date,AircraftID,From,To,TotalTime\n2026-01-01,D-ABCD,EDDF,EDDM,1.0\n"
	_, rows, _, err := parseCSV([]byte(csv))
	if err != nil {
		t.Fatalf("normal file rejected: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("rows = %d, want 1", len(rows))
	}
}
