-- Restore the non-unique lookup index. The merged-away duplicate rows are not
-- recoverable -- they were deleted, and their references were consolidated onto
-- the surviving contact. Reverting only lifts the constraint so duplicates can
-- accumulate again.

DROP INDEX IF EXISTS idx_contacts_user_lower_name;
CREATE INDEX IF NOT EXISTS idx_contacts_name ON contacts (user_id, LOWER(name));
