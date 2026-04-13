-- Revert Phase 7: Remove preferred_locale column
ALTER TABLE users DROP COLUMN IF EXISTS preferred_locale;
