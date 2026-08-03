-- Restore the pre-hash shape from 000037. Ceremony state is transient and
-- single-use, so nothing is migrated back.
DROP TABLE IF EXISTS webauthn_sessions;

CREATE TABLE webauthn_sessions (
    id            UUID PRIMARY KEY,
    user_id       UUID REFERENCES users(id) ON DELETE CASCADE,
    challenge     TEXT NOT NULL,
    session_data  JSONB NOT NULL,
    purpose       TEXT NOT NULL,
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_webauthn_sessions_expires_at ON webauthn_sessions(expires_at);
