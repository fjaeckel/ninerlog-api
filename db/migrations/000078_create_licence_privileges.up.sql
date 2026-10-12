-- Licence privileges: ratings, endorsements and authorisations recorded on a
-- licence beside its class ratings (sailplane and banner towing, cloud flying,
-- aerobatics, TMG night, FI(S)/BI(S)/FE(S), trained launch methods, German UL
-- passenger authorisation, UL towing and type briefing (Einweisung)). detail
-- carries the launch method, UL kind or aircraft type a kind needs.

CREATE TABLE licence_privileges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    license_id UUID NOT NULL REFERENCES licenses(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN (
        'SAILPLANE_TOWING', 'BANNER_TOWING', 'CLOUD_FLYING', 'AEROBATIC_BASIC',
        'AEROBATIC_ADVANCED', 'TMG_NIGHT', 'FI_S', 'BI_S', 'FE_S',
        'UL_PASSENGER_AUTH', 'UL_TOWING', 'UL_TYPE_BRIEFING', 'LAUNCH_METHOD_TRAINED')),
    detail TEXT NULL CHECK (detail IS NULL OR char_length(detail) <= 100),
    issued_on DATE NULL,
    expires_on DATE NULL,
    notes TEXT NULL CHECK (notes IS NULL OR char_length(notes) <= 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT licence_privileges_dates CHECK (issued_on IS NULL OR expires_on IS NULL OR expires_on >= issued_on)
);

CREATE INDEX idx_licence_privileges_user ON licence_privileges(user_id);
CREATE INDEX idx_licence_privileges_license ON licence_privileges(license_id);

CREATE TRIGGER update_licence_privileges_updated_at
    BEFORE UPDATE ON licence_privileges
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
