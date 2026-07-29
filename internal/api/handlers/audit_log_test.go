package handlers

import (
	"encoding/json"
	"testing"
)

// admin_audit_log.details is JSONB, so a payload that is not valid JSON is
// rejected by Postgres and the audit row is lost. Both historical failures came
// from assembling that payload by hand:
//
//   - `{"email":"%s"}` with only quotes escaped. Go's mail.ParseAddress accepts
//     a quoted local-part and re-emits it unquoted ("back\\slash"@x ->
//     back\slash@x); a raw backslash is not a valid JSON escape, so the insert
//     failed. A user choosing their own address could make admin actions
//     against them leave no trace.
//   - Announcement create/delete passed a bare message string and a bare UUID.
//     Neither is valid JSON, so those actions were never logged at all.
//
// json.Marshal handles every case. These tests pin that.
func TestAuditDetails_MarshalHandlesHostileValues(t *testing.T) {
	cases := []struct {
		name    string
		details map[string]any
	}{
		{"backslash in email", map[string]any{"email": `back\slash@evil.test`}},
		{"quote in email", map[string]any{"email": `x"@evil.test`}},
		{"quote and backslash in name", map[string]any{"email": "a@b.c", "name": `Bob "The \ Pilot"`}},
		{"control characters", map[string]any{"message": "line1\nline2\ttab"}},
		{"unicode", map[string]any{"message": "Grüße – 日本語"}},
		{"bare uuid value", map[string]any{"announcementId": "2b1f0f4e-0000-0000-0000-000000000000"}},
		{"numeric counters", map[string]any{"refreshTokensDeleted": 3, "resetTokensDeleted": 0}},
		{"empty", map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(tc.details)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}
			if !json.Valid(payload) {
				t.Errorf("payload is not valid JSON, the JSONB insert would fail: %s", payload)
			}
			var round map[string]any
			if err := json.Unmarshal(payload, &round); err != nil {
				t.Errorf("payload does not round-trip: %v", err)
			}
		})
	}
}

// The old hand-built form is retained here only to demonstrate that the
// backslash case really did produce invalid JSON, so the fix is not cosmetic.
func TestAuditDetails_LegacyHandBuiltFormWasBroken(t *testing.T) {
	email := `back\slash@evil.test`
	legacy := `{"email":"` + email + `"}` // quotes escaped, backslashes not
	if json.Valid([]byte(legacy)) {
		t.Fatal("expected the legacy hand-built payload to be invalid JSON")
	}
	fixed, err := json.Marshal(map[string]any{"email": email})
	if err != nil || !json.Valid(fixed) {
		t.Fatalf("marshalled payload should be valid: %v %s", err, fixed)
	}
}
