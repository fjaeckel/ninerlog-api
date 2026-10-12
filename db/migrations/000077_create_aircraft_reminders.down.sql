UPDATE notification_preferences
SET enabled_categories = array_remove(enabled_categories, 'aircraft_reminder');

ALTER TABLE notification_preferences
    ALTER COLUMN enabled_categories SET DEFAULT '{credential_medical,credential_language,credential_security,credential_other,rating_expiry,currency_passenger,currency_night,currency_instrument,currency_flight_review,currency_revalidation}';

DROP TRIGGER IF EXISTS update_aircraft_reminders_updated_at ON aircraft_reminders;
DROP TABLE IF EXISTS aircraft_reminders;
