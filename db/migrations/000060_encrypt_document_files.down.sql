-- Remove at-rest encryption for licence/credential files.
--
-- DESTRUCTIVE, and unavoidably so. Every remaining row holds AES-GCM
-- ciphertext, and nothing in the database can turn it back into a file — the
-- key is in the application's environment. Dropping the nonce column on its own
-- would leave those blobs in `data` for the application to serve to a browser
-- as if they were JPEGs: not a rollback, a silent corruption of every stored
-- scan. So the rows go with the column.
--
-- To roll back and keep the files, download them through the API first, or
-- restore a dump taken before the upgrade.

DELETE FROM document_files;

ALTER TABLE document_files DROP CONSTRAINT document_files_data_nonce_size;
ALTER TABLE document_files DROP COLUMN data_nonce;

COMMENT ON COLUMN document_files.data IS 'Raw file bytes, served only over an authenticated request; PDFs are always served as an attachment';
COMMENT ON COLUMN document_files.byte_size IS NULL;
