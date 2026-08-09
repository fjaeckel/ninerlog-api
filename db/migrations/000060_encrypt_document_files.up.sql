-- Encrypt stored licence/credential files at rest.
--
-- These are scans of identity documents, and until now `data` held the file
-- verbatim: anyone holding a database dump, a volume snapshot or a stray
-- pg_dump in an object store held the pilot's licence. The API process already
-- carries an operator-supplied key (ENCRYPTION_KEY), so the bytes are now
-- sealed with AES-256-GCM under a subkey derived for this purpose alone, and
-- the column stores ciphertext.
--
-- What this defends: the database considered separately from the application —
-- dumps, backups, snapshots, replicas, and read access won by any route that
-- does not also yield the API's environment. What it does NOT defend: a
-- compromised API process, which necessarily holds the key.
--
-- DESTRUCTIVE: any file stored before this runs is DELETED.
--
-- Nothing can encrypt those rows from inside a migration — the key lives in the
-- application's environment, not the database's — and the alternative is a
-- nullable nonce meaning "this one is still in the clear", i.e. a permanent
-- second storage format and a decrypt path that has to guess which it is
-- holding. This feature is days old and no released version ships it, so the
-- rows being dropped are test uploads. Making the column NOT NULL now buys a
-- storage layer with exactly one shape, which is worth more than those rows.
-- Re-upload after deploying.
--
-- The nonce lives in its own column rather than being prefixed onto `data`.
-- Both work; a separate column means the encryption state of a row is visible
-- in a query rather than hidden in the first twelve bytes of a BYTEA, and it
-- leaves `data` holding exactly one thing.
--
-- The authentication tag also covers the row's id, owner and content type,
-- which are NOT stored in the ciphertext. Copying one row's blob onto another
-- therefore produces something that no longer decrypts: an attacker with write
-- access to this table can destroy a file, but cannot move one pilot's scan
-- onto another pilot's licence.

DELETE FROM document_files;

ALTER TABLE document_files ADD COLUMN data_nonce BYTEA NOT NULL;

-- AES-GCM's 96-bit nonce. A wrong-length one is a bug somewhere above, and it
-- is cheaper to refuse the write than to discover it when the file will not
-- open.
ALTER TABLE document_files ADD CONSTRAINT document_files_data_nonce_size
    CHECK (octet_length(data_nonce) = 12);

COMMENT ON COLUMN document_files.data_nonce IS 'AES-256-GCM nonce for data; every row is encrypted, so this is never null';
COMMENT ON COLUMN document_files.data IS 'File bytes as AES-256-GCM ciphertext (key derived from ENCRYPTION_KEY); served only over an authenticated request, PDFs always as an attachment';
COMMENT ON COLUMN document_files.byte_size IS 'Size of the PLAINTEXT file in bytes, not of the stored ciphertext';
