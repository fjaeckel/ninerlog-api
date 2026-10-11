-- Flight audit trail and strict logbook mode (issue #260).

ALTER TABLE users
    ADD COLUMN strict_logbook BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN strict_logbook_since TIMESTAMPTZ;

ALTER TABLE licenses
    ADD COLUMN issuing_country CHAR(2) CHECK (issuing_country ~ '^[A-Z]{2}$');

ALTER TABLE flight_signatures
    ADD COLUMN signed_fields_hash TEXT;

-- Strict mode switches, oldest first per user.
CREATE TABLE logbook_mode_events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    strict     BOOLEAN NOT NULL,
    trigger    TEXT NOT NULL CHECK (trigger IN ('licence', 'manual', 'import')),
    license_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_logbook_mode_events_user ON logbook_mode_events (user_id, created_at);

CREATE TABLE logbook_certifications (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    certified_through DATE NOT NULL,
    certified_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_logbook_certifications_user ON logbook_certifications (user_id, certified_through DESC);

-- One row per change: the flight and its crew as they were before it.
CREATE TABLE flight_revisions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    flight_id     UUID NOT NULL,
    revision_no   INT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('update', 'delete')),
    snapshot      JSONB NOT NULL,
    crew_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb,
    source        TEXT NOT NULL CHECK (source IN ('user_edit', 'recalc', 'aircraft_rename', 'import', 'unknown')),
    reason        TEXT,
    is_correction BOOLEAN NOT NULL DEFAULT false,
    changed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (flight_id, revision_no)
);
CREATE INDEX idx_flight_revisions_user ON flight_revisions (user_id, changed_at DESC);
CREATE INDEX idx_flight_revisions_voided ON flight_revisions (user_id, changed_at DESC) WHERE kind = 'delete';

-- Pre-images of flights touched by the current transaction; emptied at commit.
CREATE TABLE flight_revision_pending (
    tx_id         BIGINT NOT NULL,
    flight_id     UUID NOT NULL,
    user_id       UUID NOT NULL,
    snapshot      JSONB NOT NULL,
    crew_snapshot JSONB NOT NULL,
    source        TEXT,
    reason        TEXT,
    is_correction BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (tx_id, flight_id)
);

CREATE OR REPLACE FUNCTION flight_revision_crew(fid UUID) RETURNS JSONB AS $$
    SELECT COALESCE(
        jsonb_agg(jsonb_build_object('name', name, 'role', role::text, 'contactId', contact_id) ORDER BY role::text, name),
        '[]'::jsonb)
    FROM flight_crew_members WHERE flight_id = fid
$$ LANGUAGE sql STABLE;

-- Crew compared by name and role only.
CREATE OR REPLACE FUNCTION flight_revision_crew_key(crew JSONB) RETURNS JSONB AS $$
    SELECT COALESCE(jsonb_agg(e - 'contactId' ORDER BY e->>'role', e->>'name'), '[]'::jsonb)
    FROM jsonb_array_elements(COALESCE(crew, '[]'::jsonb)) e
$$ LANGUAGE sql IMMUTABLE;

CREATE OR REPLACE FUNCTION flight_revision_capture(fid UUID) RETURNS VOID AS $$
DECLARE
    snap      JSONB;
    owner     UUID;
    born_here BOOLEAN;
    src       TEXT := NULLIF(current_setting('ninerlog.change_source', true), '');
BEGIN
    IF src = 'create' THEN
        RETURN;
    END IF;
    IF EXISTS (SELECT 1 FROM flight_revision_pending WHERE tx_id = txid_current() AND flight_id = fid) THEN
        RETURN;
    END IF;

    SELECT to_jsonb(f), f.user_id, f.xmin = pg_current_xact_id()::xid
      INTO snap, owner, born_here
      FROM flights f WHERE f.id = fid
       FOR UPDATE;
    IF snap IS NULL OR born_here THEN
        RETURN;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM users WHERE id = owner AND strict_logbook) THEN
        RETURN;
    END IF;

    INSERT INTO flight_revision_pending
        (tx_id, flight_id, user_id, snapshot, crew_snapshot, source, reason, is_correction)
    VALUES (
        txid_current(), fid, owner, snap, flight_revision_crew(fid), src,
        NULLIF(current_setting('ninerlog.change_reason', true), ''),
        COALESCE(NULLIF(current_setting('ninerlog.change_correction', true), '')::boolean, false))
    ON CONFLICT (tx_id, flight_id) DO NOTHING;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION flight_revision_capture_flight() RETURNS TRIGGER AS $$
BEGIN
    PERFORM flight_revision_capture(OLD.id);
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION flight_revision_capture_crew() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        PERFORM flight_revision_capture(NEW.flight_id);
        RETURN NEW;
    END IF;
    PERFORM flight_revision_capture(OLD.flight_id);
    IF TG_OP = 'UPDATE' AND NEW.flight_id <> OLD.flight_id THEN
        PERFORM flight_revision_capture(NEW.flight_id);
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER flight_revision_capture
    BEFORE UPDATE OR DELETE ON flights
    FOR EACH ROW EXECUTE FUNCTION flight_revision_capture_flight();

CREATE TRIGGER flight_revision_capture_crew
    BEFORE INSERT OR UPDATE OR DELETE ON flight_crew_members
    FOR EACH ROW EXECUTE FUNCTION flight_revision_capture_crew();

-- At commit: compare the pre-image with the final state and keep a revision when they differ.
CREATE OR REPLACE FUNCTION flight_revision_finalize() RETURNS TRIGGER AS $$
DECLARE
    ignored TEXT[] := ARRAY['updated_at', 'signature_id'];
    cur     JSONB;
    rev_kind TEXT;
    next_no INT;
BEGIN
    DELETE FROM flight_revision_pending WHERE tx_id = NEW.tx_id AND flight_id = NEW.flight_id;

    IF NOT EXISTS (SELECT 1 FROM users WHERE id = NEW.user_id) THEN
        RETURN NULL;
    END IF;

    SELECT to_jsonb(f) INTO cur FROM flights f WHERE f.id = NEW.flight_id;
    IF cur IS NULL THEN
        rev_kind := 'delete';
    ELSIF (cur - ignored) = (NEW.snapshot - ignored)
          AND flight_revision_crew_key(flight_revision_crew(NEW.flight_id)) = flight_revision_crew_key(NEW.crew_snapshot) THEN
        RETURN NULL;
    ELSE
        rev_kind := 'update';
    END IF;

    SELECT COALESCE(MAX(revision_no), 0) + 1 INTO next_no
      FROM flight_revisions WHERE flight_id = NEW.flight_id;

    INSERT INTO flight_revisions
        (user_id, flight_id, revision_no, kind, snapshot, crew_snapshot, source, reason, is_correction)
    VALUES (
        NEW.user_id, NEW.flight_id, next_no, rev_kind, NEW.snapshot, NEW.crew_snapshot,
        CASE WHEN NEW.source IN ('user_edit', 'recalc', 'aircraft_rename', 'import') THEN NEW.source ELSE 'unknown' END,
        NEW.reason, NEW.is_correction);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER flight_revision_finalize
    AFTER INSERT ON flight_revision_pending
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION flight_revision_finalize();

-- Append-only, except the cascade from deleting the user.
CREATE OR REPLACE FUNCTION flight_revisions_append_only() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' AND NOT EXISTS (SELECT 1 FROM users WHERE id = OLD.user_id) THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'flight_revisions is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER flight_revisions_append_only
    BEFORE UPDATE OR DELETE ON flight_revisions
    FOR EACH ROW EXECUTE FUNCTION flight_revisions_append_only();
