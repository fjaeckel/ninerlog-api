-- Per-user "initial hours snapshot": a carried-forward total of pre-existing
-- flying experience that is added on top of logged flights when computing
-- user-level statistics. One row per user (UPSERT-style).
CREATE TABLE flight_baselines (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    baseline_date DATE NOT NULL,
    total_flights INTEGER NOT NULL DEFAULT 0,
    total_minutes INTEGER NOT NULL DEFAULT 0,
    pic_minutes INTEGER NOT NULL DEFAULT 0,
    sic_minutes INTEGER NOT NULL DEFAULT 0,
    dual_minutes INTEGER NOT NULL DEFAULT 0,
    dual_given_minutes INTEGER NOT NULL DEFAULT 0,
    multi_pilot_minutes INTEGER NOT NULL DEFAULT 0,
    night_minutes INTEGER NOT NULL DEFAULT 0,
    ifr_minutes INTEGER NOT NULL DEFAULT 0,
    solo_minutes INTEGER NOT NULL DEFAULT 0,
    cross_country_minutes INTEGER NOT NULL DEFAULT 0,
    landings_day INTEGER NOT NULL DEFAULT 0,
    landings_night INTEGER NOT NULL DEFAULT 0,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT flight_baselines_non_negative CHECK (
        total_flights >= 0
        AND total_minutes >= 0
        AND pic_minutes >= 0
        AND sic_minutes >= 0
        AND dual_minutes >= 0
        AND dual_given_minutes >= 0
        AND multi_pilot_minutes >= 0
        AND night_minutes >= 0
        AND ifr_minutes >= 0
        AND solo_minutes >= 0
        AND cross_country_minutes >= 0
        AND landings_day >= 0
        AND landings_night >= 0
    )
);

CREATE TRIGGER update_flight_baselines_updated_at
    BEFORE UPDATE ON flight_baselines
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE flight_baselines IS 'Optional per-user initial-hours snapshot added to aggregated user-level statistics.';
COMMENT ON COLUMN flight_baselines.baseline_date IS 'Cutoff date the snapshot covers (inclusive). Used to decide whether the baseline applies to a date-filtered statistics query.';
COMMENT ON COLUMN flight_baselines.notes IS 'Optional free-form context (e.g. paper logbook source).';
