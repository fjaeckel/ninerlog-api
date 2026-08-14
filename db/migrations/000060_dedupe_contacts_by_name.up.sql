-- Make a contact's name unique per user, case- and whitespace-insensitively.
--
-- Contacts were only ever created by two paths that did not know about each
-- other: import confirm (which deduplicated on LOWER(name)) and POST /contacts
-- (which did not deduplicate at all). Flight create/update never created one,
-- so crew members typed in the UI were stored as bare text with a NULL
-- contact_id. The result is a table where the same person can appear several
-- times, and where a user who never imported has an empty address book while
-- their logbook is full of crew names.
--
-- Contact identity is the name alone. A person is one contact regardless of how
-- many roles they fly in -- the role lives on flight_crew_members, so the same
-- contact is free to be PIC on one flight and Instructor on the next.
--
-- Merge order below matters: repoint the referencing tables before deleting the
-- losing rows, or ON DELETE SET NULL silently drops the links this migration
-- exists to consolidate.
--
-- Deleting the losers fires record_contact_deletion (migration 000054), so a
-- delta-sync client that mirrors the merged-away ids is told they are gone
-- rather than keeping them forever. That is intentional -- do not suppress the
-- trigger here.

-- 1. Normalise stored names first, so " John Smith " and "John Smith" collapse
--    into the same group in step 2 rather than surviving as separate rows that
--    the unique index would then reject.
UPDATE contacts SET name = BTRIM(name) WHERE name <> BTRIM(name);

-- Names that are only whitespace cannot be represented once trimmed and were
-- never usable. There is no meaningful merge target for them.
DELETE FROM contacts WHERE name = '';

-- 2. Pick a survivor per (user_id, LOWER(name)): the oldest row, with the id as
--    a deterministic tie-break so a re-run picks the same winner.
CREATE TEMP TABLE contact_merge AS
SELECT c.id AS loser_id,
       (
           SELECT s.id
           FROM contacts s
           WHERE s.user_id = c.user_id AND LOWER(s.name) = LOWER(c.name)
           ORDER BY s.created_at, s.id
           LIMIT 1
       ) AS winner_id
FROM contacts c;

DELETE FROM contact_merge WHERE loser_id = winner_id;

-- 3. Fill gaps on the survivor from the rows about to disappear, so a merge
--    never loses an email or phone number the user typed. First non-NULL by the
--    same ordering wins; a survivor that already has a value keeps it.
UPDATE contacts w
SET email = COALESCE(w.email, m.email),
    phone = COALESCE(w.phone, m.phone),
    notes = COALESCE(w.notes, m.notes),
    updated_at = NOW()
FROM (
    SELECT cm.winner_id,
           (ARRAY_REMOVE(ARRAY_AGG(l.email ORDER BY l.created_at, l.id), NULL))[1] AS email,
           (ARRAY_REMOVE(ARRAY_AGG(l.phone ORDER BY l.created_at, l.id), NULL))[1] AS phone,
           (ARRAY_REMOVE(ARRAY_AGG(l.notes ORDER BY l.created_at, l.id), NULL))[1] AS notes
    FROM contact_merge cm
    JOIN contacts l ON l.id = cm.loser_id
    GROUP BY cm.winner_id
) m
WHERE w.id = m.winner_id
  AND (w.email IS NULL OR w.phone IS NULL OR w.notes IS NULL);

-- 4. Repoint everything that references a losing contact.
UPDATE flight_crew_members fcm
SET contact_id = m.winner_id
FROM contact_merge m
WHERE fcm.contact_id = m.loser_id;

UPDATE flight_signatures fs
SET contact_id = m.winner_id
FROM contact_merge m
WHERE fs.contact_id = m.loser_id;

-- 5. Drop the merged-away rows.
DELETE FROM contacts c USING contact_merge m WHERE c.id = m.loser_id;

DROP TABLE contact_merge;

-- 6. Enforce it from here on. This index also serves the case-insensitive name
--    lookup that idx_contacts_name provided, so that one is redundant.
DROP INDEX IF EXISTS idx_contacts_name;
CREATE UNIQUE INDEX idx_contacts_user_lower_name ON contacts (user_id, LOWER(name));
