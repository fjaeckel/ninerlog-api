-- Flight recorder files (IGC) attached to a flight. The raw bytes are kept
-- next to the logbook entry they document and travel with the JSON export.
-- Storage is BYTEA, as for document_files; the service caps a file at 5 MB and
-- a flight at 5 files. One file is stored once per flight (flight_id, sha256).

CREATE TABLE flight_files (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    flight_id  UUID NOT NULL REFERENCES flights(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('IGC')),
    filename   TEXT NOT NULL CHECK (char_length(filename) BETWEEN 1 AND 255),
    content    BYTEA NOT NULL,
    size_bytes INTEGER NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 5242880),
    sha256     TEXT NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT flight_files_flight_sha256_key UNIQUE (flight_id, sha256)
);

CREATE INDEX idx_flight_files_user ON flight_files(user_id, sha256);

COMMENT ON TABLE flight_files IS 'Flight recorder files (IGC) attached to a flight; max 5 MB and 5 files per flight, enforced in the service layer';
COMMENT ON COLUMN flight_files.content IS 'Raw file bytes, served only over an authenticated request';
