-- Session epoch for access-token invalidation.
--
-- Access tokens are stateless 15-minute JWTs and AuthMiddleware only checked
-- the signature and expiry, so nothing could revoke one early. Disabling an
-- account or changing a password deleted the user's REFRESH tokens, which only
-- stops the session being extended -- the outstanding access token kept working
-- for up to 15 more minutes, including writes.
--
-- Bumping tokens_valid_after invalidates every access token issued before that
-- instant. NULL means "no invalidation event yet" and all tokens are accepted.
ALTER TABLE users ADD COLUMN tokens_valid_after TIMESTAMPTZ;

COMMENT ON COLUMN users.tokens_valid_after IS
    'Access tokens issued before this instant are rejected. Bumped on password change, account disable and admin 2FA reset.';
