-- Instructor sign-off requests/records for a single flight log entry.
--
-- This table is append-only history: a 'live' row is created already
-- 'completed'; a 'deferred' row starts 'pending' (token-based, delivered by
-- email and/or a shareable link+QR) and transitions to 'completed',
-- 'revoked' (cancelled before anyone signed) or 'expired' (token lapsed).
-- A 'completed' row can later become 'voided' (invalidated after the fact,
-- reason required) which unlocks the flight for editing again. Voiding never
-- deletes the row -- a new signature attempt creates a fresh row instead --
-- so the full history of a flight's sign-offs stays queryable.
--
-- The current "is this flight locked" state is NOT derived from this table
-- at read time; see migration 000043 for the denormalized flights.signature_id
-- pointer that makes that check O(1).

CREATE TABLE flight_signatures (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    flight_id                UUID NOT NULL REFERENCES flights(id) ON DELETE CASCADE,
    user_id                  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    method                   TEXT NOT NULL CHECK (method IN ('live', 'deferred')),
    status                   TEXT NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending', 'completed', 'revoked', 'voided', 'expired')),

    -- Deferred (email/link) token fields -- null for 'live', which completes
    -- synchronously and never has a token.
    token_hash               TEXT,
    token_expires_at         TIMESTAMPTZ,

    -- Owner-supplied delivery target for deferred mode. Mutable until the
    -- request completes (resend can update it and rotate the token).
    instructor_email         TEXT,
    email_sent_at            TIMESTAMPTZ,
    email_send_count         INT NOT NULL DEFAULT 0,

    -- Optional link back to a saved Contact, purely a display prefill --
    -- there is no instructor account/login anywhere in this system.
    contact_id               UUID REFERENCES contacts(id) ON DELETE SET NULL,

    -- Captured at completion time: by the owner (for 'live'), or by the
    -- instructor through the public /sign/{token} flow (for 'deferred').
    instructor_name          TEXT,
    instructor_credential_no TEXT,
    signature_image          BYTEA,
    signed_at                TIMESTAMPTZ,
    signer_ip                TEXT,
    signer_user_agent        TEXT,

    -- Void trail (only set when status = 'voided')
    voided_at                TIMESTAMPTZ,
    voided_reason            TEXT,

    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Only one active (pending) request per flight at a time -- the owner must
-- void/revoke/let-expire before creating a new one. Also prevents two
-- concurrent requests racing to sign the same flight.
CREATE UNIQUE INDEX flight_signatures_one_pending_per_flight
    ON flight_signatures (flight_id) WHERE status = 'pending';

-- Token lookup for the public /sign/{token} endpoints.
CREATE UNIQUE INDEX flight_signatures_token_hash_idx
    ON flight_signatures (token_hash) WHERE token_hash IS NOT NULL;

CREATE INDEX flight_signatures_flight_id_idx ON flight_signatures (flight_id);
CREATE INDEX flight_signatures_user_id_idx ON flight_signatures (user_id);

COMMENT ON TABLE flight_signatures IS 'Instructor sign-off requests/records for a single flight log entry (append-only history; current lock pointer lives on flights.signature_id)';
COMMENT ON COLUMN flight_signatures.method IS 'live = captured in person on the owner''s device with no token; deferred = token-based flow, delivered by email and/or a shareable link+QR';
COMMENT ON COLUMN flight_signatures.signature_image IS 'PNG bytes from the signer''s drawn signature, max 500KB enforced at the handler';
