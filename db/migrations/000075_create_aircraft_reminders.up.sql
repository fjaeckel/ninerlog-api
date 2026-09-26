-- Aircraft reminders: dated maintenance and paperwork items per aircraft
-- (annual inspection, insurance, rescue-system repack, rescue-rocket expiry,
-- ARC, ELT battery, custom). interval_months rolls due_date forward when an
-- item is completed. Also enables the aircraft_reminder notification
-- category for every user.

CREATE TABLE aircraft_reminders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    aircraft_id UUID NOT NULL REFERENCES aircraft(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN (
        'ANNUAL_INSPECTION', 'INSURANCE', 'RESCUE_SYSTEM_REPACK',
        'RESCUE_ROCKET_EXPIRY', 'ARC', 'ELT_BATTERY', 'CUSTOM')),
    label TEXT NULL CHECK (label IS NULL OR char_length(label) <= 100),
    due_date DATE NOT NULL,
    interval_months INTEGER NULL CHECK (interval_months BETWEEN 1 AND 240),
    last_done_on DATE NULL,
    notes TEXT NULL CHECK (notes IS NULL OR char_length(notes) <= 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT aircraft_reminders_custom_label
        CHECK (kind <> 'CUSTOM' OR (label IS NOT NULL AND btrim(label) <> ''))
);

CREATE INDEX idx_aircraft_reminders_user_due ON aircraft_reminders(user_id, due_date);
CREATE INDEX idx_aircraft_reminders_aircraft ON aircraft_reminders(aircraft_id);

CREATE TRIGGER update_aircraft_reminders_updated_at
    BEFORE UPDATE ON aircraft_reminders
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE notification_preferences
    ALTER COLUMN enabled_categories SET DEFAULT '{credential_medical,credential_language,credential_security,credential_other,rating_expiry,currency_passenger,currency_night,currency_instrument,currency_flight_review,currency_revalidation,aircraft_reminder}';

UPDATE notification_preferences
SET enabled_categories = array_append(enabled_categories, 'aircraft_reminder')
WHERE NOT ('aircraft_reminder' = ANY(enabled_categories));
