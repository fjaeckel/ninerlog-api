-- Give refresh tokens a session identity, so a user can hold several
-- concurrent sessions instead of exactly one.
--
-- Before this, login deleted every refresh token a user held, which meant
-- signing in on a phone silently ended the browser session and vice versa.
-- session_id survives rotation: each refresh mints a new token row carrying the
-- same session_id, so one session is a chain of rows, and the profile screen
-- can list "devices" rather than tokens.
--
-- rotated_at is what makes rotation tolerant of a concurrent refresh. Two tabs
-- racing to renew the same token would otherwise leave the loser holding a
-- revoked token and log it out; within the reuse grace window the loser is
-- served instead. Past the window, presenting a rotated token is a replay and
-- the whole session is revoked.
--
-- It is deliberately separate from revoked_at: a token revoked outright — by a
-- logout, a password change, or the owner ending the session — gets no grace,
-- so signing out takes effect at once.
--
-- Existing rows each become their own session via the column default.

ALTER TABLE refresh_tokens
    ADD COLUMN IF NOT EXISTS session_id UUID NOT NULL DEFAULT gen_random_uuid(),
    ADD COLUMN IF NOT EXISTS device_label VARCHAR(120) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS user_agent VARCHAR(512) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS ip_address VARCHAR(45) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS rotated_at TIMESTAMPTZ;

-- Backfill revoked_at for rows revoked before this migration so the grace
-- window treats them as long expired rather than as revoked "just now".
UPDATE refresh_tokens
SET revoked_at = updated_at
WHERE revoked = true AND revoked_at IS NULL;

-- Session listing and revocation are always user-scoped.
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_session
    ON refresh_tokens(user_id, session_id);

-- Eviction picks the least recently used live session.
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_last_used
    ON refresh_tokens(user_id, last_used_at);
