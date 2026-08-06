-- Tombstones: make deletions visible to delta sync.
--
-- A record removed from flights/aircraft/contacts/credentials/licenses simply
-- stops appearing in the list endpoints, so a client that mirrors the logbook
-- keeps it forever. This table records "this id is gone", and
-- GET /sync/deletions replays it against the same watermark a client already
-- uses for updatedSince (migration 000053).
--
-- Rows are written by an AFTER DELETE trigger rather than by the repositories,
-- because deletions reach the database by several routes that never touch a
-- repository -- the raw SQL in DeleteAllUserData, the admin user delete, and
-- ON DELETE CASCADE. A trigger cannot be forgotten by a future caller, and it
-- runs inside the deleting transaction, so a tombstone cannot go missing after
-- a delete that the client was told succeeded.
--
-- The trigger deliberately skips rows whose owning user no longer exists.
-- DELETE FROM users removes the parent row before its ON DELETE CASCADE fires,
-- so every cascaded child delete sees no user -- which is exactly when a
-- tombstone is pointless: the account is gone and no client will ever sync it
-- again. The check also keeps this table's own cascade from racing the others,
-- whichever order the referential-integrity triggers happen to fire in.
--
-- Retention is bounded (TOMBSTONE_RETENTION, default 90 days) and swept
-- hourly. A client whose watermark predates the horizon is told so explicitly
-- rather than silently missing a delete.

CREATE TABLE deletion_tombstones (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL CHECK (
        entity_type IN ('flight', 'aircraft', 'contact', 'credential', 'license')
    ),
    entity_id UUID NOT NULL,
    deleted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The feed is "this user's deletions after a watermark, oldest first". The
-- trailing id makes paging deterministic when a bulk delete stamps thousands
-- of rows with one transaction timestamp.
CREATE INDEX idx_deletion_tombstones_user_deleted_at
    ON deletion_tombstones (user_id, deleted_at, id);

-- The sweeper deletes by age across all users.
CREATE INDEX idx_deletion_tombstones_deleted_at ON deletion_tombstones (deleted_at);

-- One tombstone per record. Re-deleting an id that was somehow recreated
-- refreshes the stamp instead of duplicating the entry.
CREATE UNIQUE INDEX idx_deletion_tombstones_entity
    ON deletion_tombstones (user_id, entity_type, entity_id);

CREATE OR REPLACE FUNCTION record_deletion_tombstone() RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM users WHERE id = OLD.user_id) THEN
        RETURN OLD;
    END IF;

    INSERT INTO deletion_tombstones (user_id, entity_type, entity_id, deleted_at)
    VALUES (OLD.user_id, TG_ARGV[0], OLD.id, NOW())
    ON CONFLICT (user_id, entity_type, entity_id)
    DO UPDATE SET deleted_at = EXCLUDED.deleted_at;

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER record_flight_deletion
    AFTER DELETE ON flights
    FOR EACH ROW EXECUTE FUNCTION record_deletion_tombstone('flight');

CREATE TRIGGER record_aircraft_deletion
    AFTER DELETE ON aircraft
    FOR EACH ROW EXECUTE FUNCTION record_deletion_tombstone('aircraft');

CREATE TRIGGER record_contact_deletion
    AFTER DELETE ON contacts
    FOR EACH ROW EXECUTE FUNCTION record_deletion_tombstone('contact');

CREATE TRIGGER record_credential_deletion
    AFTER DELETE ON credentials
    FOR EACH ROW EXECUTE FUNCTION record_deletion_tombstone('credential');

CREATE TRIGGER record_license_deletion
    AFTER DELETE ON licenses
    FOR EACH ROW EXECUTE FUNCTION record_deletion_tombstone('license');
