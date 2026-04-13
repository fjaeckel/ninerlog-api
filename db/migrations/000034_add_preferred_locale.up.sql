-- Phase 7: Add preferred_locale column for i18n support
ALTER TABLE users ADD COLUMN preferred_locale VARCHAR(10) NOT NULL DEFAULT 'en';
