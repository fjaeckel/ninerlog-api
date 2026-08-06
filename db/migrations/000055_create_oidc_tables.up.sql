-- Tables backing optional OIDC single sign-on (enabled with OIDC_ISSUER).
--
-- These tables exist on every deployment but stay empty unless OIDC is
-- configured. Nothing here replaces the users table: an OIDC login still
-- resolves to a row in users, and every downstream foreign key (flights,
-- licenses, …) keeps pointing at users(id).

-- Links an external identity (issuer + subject) to a local user.
--
-- The pair (issuer, subject) is the only stable identifier an OIDC provider
-- guarantees. Email is stored for diagnostics and admin display only —
-- matching on it would let anyone who can set their address at the IdP take
-- over an existing account, so lookups always go through (issuer, subject).
CREATE TABLE oidc_identities (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    issuer       TEXT NOT NULL,
    subject      TEXT NOT NULL,
    email        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMPTZ,
    UNIQUE (issuer, subject)
);

CREATE INDEX idx_oidc_identities_user_id ON oidc_identities(user_id);

-- In-flight authorization requests: the CSRF state, the replay-binding nonce,
-- and the PKCE code verifier for one browser round trip to the IdP.
--
-- state_hash stores the SHA-256 of the state parameter rather than the value
-- itself, so a dump of this table cannot be replayed against a pending login
-- (same reasoning as the hashed WebAuthn ceremony handles in 000051).
-- Rows are consumed exactly once via DELETE … RETURNING and swept by a
-- background reaper; the expires_at predicate on consumption is what makes a
-- stalled reaper harmless.
-- browser_hash binds the pending login to the browser that started it: the
-- authorize step sets a random value in a SameSite=Lax cookie and stores only
-- its SHA-256 here. Without that binding an attacker could complete their own
-- authorization in a victim's browser and silently sign the victim into the
-- attacker's account (login CSRF).
CREATE TABLE oidc_login_states (
    state_hash    BYTEA PRIMARY KEY,
    browser_hash  BYTEA NOT NULL,
    nonce         TEXT NOT NULL,
    code_verifier TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_oidc_login_states_expires_at ON oidc_login_states(expires_at);

-- Single-use handoff codes bridging the IdP redirect back to the SPA.
--
-- The callback is a browser navigation, so it cannot return a JSON token pair.
-- Putting the access/refresh tokens in the redirect URL instead would leak
-- them into browser history, the Referer header and any reverse-proxy access
-- log. The callback therefore stores a short-lived code (hashed, like the
-- state above), and the SPA exchanges it for the real tokens over POST.
CREATE TABLE oidc_handoff_codes (
    code_hash  BYTEA PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_oidc_handoff_codes_expires_at ON oidc_handoff_codes(expires_at);
