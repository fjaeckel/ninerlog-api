-- Per-user pilot profile: the stored half of the adaptive disciplines
-- ("toolkits"). Only intent is stored; evidence and status are derived on read.
-- One row per user (UPSERT-style); a missing row means mode 'adaptive' and
-- intent 'auto' for every discipline.
CREATE TABLE pilot_profiles (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    mode TEXT NOT NULL DEFAULT 'adaptive' CHECK (mode IN ('adaptive', 'everything')),
    disciplines JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER update_pilot_profiles_updated_at
    BEFORE UPDATE ON pilot_profiles
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE pilot_profiles IS 'Per-user discipline intents and display mode for the adaptive pilot profile.';
COMMENT ON COLUMN pilot_profiles.disciplines IS 'Map of discipline to {"intent": "on"|"off"|"goal"|"auto", "acknowledgedAt": timestamp}.';
