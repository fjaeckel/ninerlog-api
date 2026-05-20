-- Cloud backup destinations: per-user S3-compatible (and future provider) targets.
-- The pilot owns the destination; we store only the minimum credential needed
-- to upload a single file under a single prefix, encrypted at rest with a
-- server-held AES-256-GCM key (BACKUP_CREDENTIALS_KEY env var).
CREATE TABLE backup_destinations (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id               UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider              TEXT NOT NULL,
    display_name          TEXT NOT NULL,
    config                JSONB NOT NULL DEFAULT '{}'::jsonb,
    credential_hint       TEXT NOT NULL DEFAULT '',
    credentials_enc       BYTEA NOT NULL,
    credentials_nonce     BYTEA NOT NULL,
    schedule              TEXT NOT NULL DEFAULT 'manual',
    schedule_hour_utc     SMALLINT NOT NULL DEFAULT 3,
    schedule_day_of_week  SMALLINT,
    schedule_day_of_month SMALLINT,
    retention_count       SMALLINT NOT NULL DEFAULT 30,
    status                TEXT NOT NULL DEFAULT 'active',
    last_error            TEXT NOT NULL DEFAULT '',
    enabled               BOOLEAN NOT NULL DEFAULT TRUE,
    consecutive_failures  INTEGER NOT NULL DEFAULT 0,
    last_run_at           TIMESTAMPTZ,
    last_success_at       TIMESTAMPTZ,
    last_success_sha256   TEXT NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT backup_destinations_schedule_valid CHECK (schedule IN ('manual', 'daily', 'weekly', 'monthly')),
    CONSTRAINT backup_destinations_status_valid CHECK (status IN ('active', 'paused', 'error')),
    CONSTRAINT backup_destinations_hour_range CHECK (schedule_hour_utc >= 0 AND schedule_hour_utc <= 23),
    CONSTRAINT backup_destinations_dow_range CHECK (schedule_day_of_week IS NULL OR (schedule_day_of_week >= 0 AND schedule_day_of_week <= 6)),
    CONSTRAINT backup_destinations_dom_range CHECK (schedule_day_of_month IS NULL OR (schedule_day_of_month >= 1 AND schedule_day_of_month <= 28)),
    CONSTRAINT backup_destinations_retention_nonneg CHECK (retention_count >= 0)
);

CREATE INDEX idx_backup_destinations_user_id ON backup_destinations(user_id);
CREATE INDEX idx_backup_destinations_scheduler ON backup_destinations(enabled, status, schedule)
    WHERE enabled = TRUE AND status = 'active' AND schedule <> 'manual';

-- Backup runs: audit log of every backup attempt. Renders the destination's
-- run history and lets the scheduler answer "is a run due now?" without
-- listing the remote bucket.
CREATE TABLE backup_runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    destination_id  UUID NOT NULL REFERENCES backup_destinations(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status          TEXT NOT NULL,
    trigger         TEXT NOT NULL DEFAULT 'scheduled',
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    duration_ms     INTEGER,
    size_bytes      BIGINT,
    sha256          TEXT NOT NULL DEFAULT '',
    flight_count    INTEGER,
    aircraft_count  INTEGER,
    license_count   INTEGER,
    credential_count INTEGER,
    remote_path     TEXT NOT NULL DEFAULT '',
    error_message   TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT backup_runs_status_valid CHECK (status IN ('success', 'failed', 'skipped')),
    CONSTRAINT backup_runs_trigger_valid CHECK (trigger IN ('scheduled', 'manual'))
);

CREATE INDEX idx_backup_runs_destination_started ON backup_runs(destination_id, started_at DESC);
CREATE INDEX idx_backup_runs_user_started ON backup_runs(user_id, started_at DESC);
