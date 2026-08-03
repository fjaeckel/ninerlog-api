-- Reshape webauthn_sessions so ceremony state is keyed by the SHA-256 of an
-- opaque random handle rather than by a plaintext UUID primary key.
--
-- Rows in this table are single-use and live for WEBAUTHN_SESSION_TTL (5m by
-- default), so there is no data worth preserving: the table is recreated
-- rather than altered in place. Any ceremony in flight across the deploy
-- fails and the user retries, which is the same outcome as an API restart.
DROP TABLE IF EXISTS webauthn_sessions;

CREATE TABLE webauthn_sessions (
    -- SHA-256 of the raw handle. Mirrors refresh-token storage: reading this
    -- table does not yield a usable credential.
    id_hash     BYTEA PRIMARY KEY,
    -- Nullable: registration is scoped to an authenticated user, but
    -- discoverable (usernameless) login has no user at begin time.
    user_id     UUID NULL REFERENCES users(id) ON DELETE CASCADE,
    ceremony    TEXT NOT NULL CHECK (ceremony IN ('registration', 'login')),
    data        JSONB NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_webauthn_sessions_expires_at
    ON webauthn_sessions (expires_at);

-- Supports the per-user open-ceremony cap. Partial: discoverable-login rows
-- have a NULL user_id and are never evicted by this path.
CREATE INDEX idx_webauthn_sessions_user_created
    ON webauthn_sessions (user_id, created_at DESC)
    WHERE user_id IS NOT NULL;
