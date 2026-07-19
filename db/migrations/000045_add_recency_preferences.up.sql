-- User preferences for the informational 90-day landing recency indicators
-- (styled after EASA FCL.060(b)). Per-model is on by default; per-registration
-- is off by default because FCL.060(b) recency is defined per type/class and
-- per-registration tracking is purely a familiarity aid.
ALTER TABLE users
    ADD COLUMN recency_per_model BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN recency_per_registration BOOLEAN NOT NULL DEFAULT FALSE;
