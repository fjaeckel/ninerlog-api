-- Irreversible by nature.
--
-- The up migration cleared 2FA enrolments, recovery codes, sessions and backup
-- destination credentials because none of them could be decrypted any more.
-- There is nothing to restore: the values are gone from the database and the
-- keys that could have read them are gone from the environment.
--
-- Rolling the schema back is therefore a no-op rather than an error, so a
-- `migrate down` across this version is not blocked by a step that cannot
-- succeed. Users re-enrol in 2FA and re-create their backup destinations either
-- way; restore a pre-upgrade dump if that is not acceptable.

SELECT 1;
