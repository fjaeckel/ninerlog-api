-- Reaping of unverified accounts, and a record of what SMTP told us about
-- each send.
--
-- Two related concerns:
--
--   1. When email verification is enforced (SMTP configured), an account that
--      never verifies is a dead account: it cannot log in and it never will.
--      It gets one reminder a day after signup; that reminder starts a 30-day
--      clock, after which the account is deleted.
--
--   2. An SMTP client only learns about deliverability during the conversation
--      itself — a 5xx to RCPT TO says the mailbox does not exist. That signal
--      is currently discarded into a counter. Record it per recipient so a
--      dead address is visible rather than inferred.

ALTER TABLE users
    ADD COLUMN verification_reminder_sent_at TIMESTAMPTZ;

-- The reaper sweeps on a timer and only ever looks at unverified accounts, so
-- the index is partial: its size is proportional to the number of accounts
-- stuck unverified, not to the users table.
CREATE INDEX idx_users_unverified_cleanup
    ON users (created_at, verification_reminder_sent_at)
    WHERE email_verified = FALSE;

-- Append-only log of send attempts. One row per attempt, including the ones
-- that never reached the wire (invalid address, suppressed recipient).
CREATE TABLE IF NOT EXISTS email_delivery_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    -- Nulled rather than cascaded: a bounce history describes an address, and
    -- reaping the account that happened to own it should not erase the
    -- evidence of why it was reaped.
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    recipient VARCHAR(255) NOT NULL,
    email_type VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    smtp_code INTEGER,
    detail TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_email_delivery_events_recipient ON email_delivery_events (recipient, created_at DESC);
CREATE INDEX idx_email_delivery_events_created_at ON email_delivery_events (created_at DESC);

-- Addresses we refuse to mail again. Only a recipient-level permanent refusal
-- lands here — never an authentication failure or a connection problem, which
-- would otherwise suppress every address at once.
CREATE TABLE IF NOT EXISTS email_suppressions (
    email VARCHAR(255) PRIMARY KEY,
    reason VARCHAR(32) NOT NULL,
    smtp_code INTEGER,
    detail TEXT,
    first_bounced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_bounced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    bounce_count INTEGER NOT NULL DEFAULT 1
);
