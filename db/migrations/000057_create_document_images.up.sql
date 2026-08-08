-- Reference photos attached to a licence or a credential.
--
-- Pilots are required to carry the physical document; storing a photo of it
-- next to the logbook entry means the licence number, class ratings and
-- expiry dates can be checked against the actual paper without digging the
-- wallet out. The image is reference material only -- nothing in the domain
-- logic reads it.
--
-- Storage is BYTEA in Postgres rather than an object store. NinerLog is
-- self-hosted from a single Docker Compose file with one database and no
-- guaranteed blob backend, and these are scans of identity documents: keeping
-- them inside the same database means they inherit the existing backup,
-- restore and ON DELETE CASCADE story instead of needing a second one. Volume
-- is bounded by design (5 MB per image, 5 images per document), and Postgres
-- TOASTs the payload out of line, so metadata queries never read it.
--
-- A row belongs to exactly one subject -- a licence or a credential, never
-- both and never neither (document_images_one_subject). Two nullable FKs
-- rather than a polymorphic (subject_type, subject_id) pair, because real
-- foreign keys give real cascade semantics: deleting a licence takes its
-- photos with it.

CREATE TABLE document_images (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    license_id    UUID REFERENCES licenses(id) ON DELETE CASCADE,
    credential_id UUID REFERENCES credentials(id) ON DELETE CASCADE,

    -- Only formats whose header the API can parse and re-verify server-side.
    -- Anything else (SVG above all, which is a script-execution vector when
    -- served inline) is rejected at the handler before it reaches this table.
    content_type  TEXT NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png')),

    -- Denormalized from data so listings never have to read the payload.
    byte_size     INTEGER NOT NULL CHECK (byte_size > 0 AND byte_size <= 5242880),
    width         INTEGER,
    height        INTEGER,

    -- Client-supplied, sanitized (basename only, length-capped). Display
    -- only -- never used to build a path.
    filename      TEXT,
    caption       TEXT,

    data          BYTEA NOT NULL,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT document_images_one_subject CHECK (
        (license_id IS NOT NULL AND credential_id IS NULL) OR
        (license_id IS NULL AND credential_id IS NOT NULL)
    )
);

-- Listing and the per-document count guard both go through these.
CREATE INDEX idx_document_images_license
    ON document_images(license_id, created_at)
    WHERE license_id IS NOT NULL;
CREATE INDEX idx_document_images_credential
    ON document_images(credential_id, created_at)
    WHERE credential_id IS NOT NULL;

-- Account deletion and the per-user quota check.
CREATE INDEX idx_document_images_user ON document_images(user_id);

CREATE TRIGGER update_document_images_updated_at
    BEFORE UPDATE ON document_images
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE document_images IS 'Reference photos/scans attached to a licence or credential; max 5 MB and 5 images per document, enforced in the service layer';
COMMENT ON COLUMN document_images.data IS 'Raw image bytes (JPEG or PNG), served only over an authenticated request';
