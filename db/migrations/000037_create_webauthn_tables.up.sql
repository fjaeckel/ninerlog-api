-- Stores WebAuthn / Passkey credentials registered by users.
CREATE TABLE webauthn_credentials (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id   BYTEA NOT NULL UNIQUE,
    public_key      BYTEA NOT NULL,
    attestation_type TEXT NOT NULL DEFAULT '',
    aaguid          BYTEA,
    sign_count      BIGINT NOT NULL DEFAULT 0,
    transports      TEXT[] NOT NULL DEFAULT '{}',
    label           TEXT,
    user_present    BOOLEAN NOT NULL DEFAULT TRUE,
    user_verified   BOOLEAN NOT NULL DEFAULT FALSE,
    backup_eligible BOOLEAN NOT NULL DEFAULT FALSE,
    backup_state    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at    TIMESTAMPTZ
);

CREATE INDEX idx_webauthn_credentials_user_id ON webauthn_credentials(user_id);

-- Short-lived storage for in-flight WebAuthn ceremony challenges.
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
