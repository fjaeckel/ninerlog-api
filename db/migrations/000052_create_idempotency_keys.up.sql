-- Server-side idempotency records for mutating requests that carry an
-- `Idempotency-Key` header.
--
-- Why this exists: an offline-capable client queues writes and replays them
-- when connectivity returns. If a POST /flights commits and the response is
-- lost, the client cannot tell "not applied" from "applied but unacknowledged"
-- and retries -- producing a duplicate logbook entry. In a logbook that is a
-- correctness failure, so the server has to make the retry a no-op that
-- returns the *original* response.
--
-- One row per (user, key). The row is claimed before the handler runs
-- (state = 'in_progress') and finalized with the captured response afterwards
-- (state = 'completed'). A replay of a completed row short-circuits the
-- handler entirely; a replay of an in-progress row is rejected with 409.
--
-- Rows are keyed per user so one user's key can never collide with (or
-- disclose) another's. They are disposable: everything older than
-- IDEMPOTENCY_TTL (24h by default) is swept by a background reaper, and a
-- claim whose request died without finalizing becomes claimable again after
-- IDEMPOTENCY_LEASE (60s by default) so a crashed request cannot wedge a key
-- for the full retention window.
CREATE TABLE idempotency_keys (
    user_id               UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Client-chosen key, opaque to the server. Bounded and character-checked
    -- at the middleware; the CHECK here is a backstop against a stray writer.
    idempotency_key       TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 255),

    -- SHA-256 over method, path+query and (for non-multipart requests) the
    -- request body. Reusing a key with a different payload is a client bug --
    -- silently returning the first response would hide it -- so the request is
    -- refused with 422 instead.
    request_hash          BYTEA NOT NULL,

    state                 TEXT NOT NULL CHECK (state IN ('in_progress', 'completed')),

    -- Captured response. NULL on a completed row means the response could not
    -- be stored (over IDEMPOTENCY_MAX_RESPONSE_BYTES); a replay of such a row
    -- is refused rather than re-executed, because re-executing is exactly the
    -- duplicate this table exists to prevent.
    response_status       INT,
    response_body         BYTEA,
    response_content_type TEXT,

    created_at            TIMESTAMPTZ NOT NULL,
    completed_at          TIMESTAMPTZ,
    expires_at            TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (user_id, idempotency_key)
);

-- Drives the retention sweep.
CREATE INDEX idempotency_keys_expires_at_idx ON idempotency_keys (expires_at);

COMMENT ON TABLE idempotency_keys IS 'Replay records for mutating requests carrying an Idempotency-Key header; per-user, swept after IDEMPOTENCY_TTL';
COMMENT ON COLUMN idempotency_keys.request_hash IS 'SHA-256 of method, path+query and request body -- guards against reusing one key for two different requests';
COMMENT ON COLUMN idempotency_keys.created_at IS 'When the claim was taken. Doubles as a fencing token: only the request that claimed the row may finalize or release it';
