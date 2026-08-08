-- Widen licence/credential attachments from images to documents in general,
-- and rename the table to match.
--
-- A pilot's licence scan often arrives as a PDF rather than a photo -- that is
-- what an authority emails, and what a scanner produces. Accepting only JPEG
-- and PNG made people photograph their own screen to get a document in.
--
-- Renaming now rather than living with a misnomer: migration 57 landed days
-- ago and no client outside this repo pair consumes it yet, so this is the
-- cheapest the rename will ever be. ALTER ... RENAME preserves the data, the
-- indexes' contents and the foreign keys; only the names change.
--
-- PDFs cannot be validated the way images are. There is no header to decode
-- and no dimensions to bound, so the service checks the %PDF- signature and
-- the %%EOF trailer and leans on the size cap. They are also an *active*
-- format -- embedded JavaScript, embedded attachments -- so unlike images they
-- are served with Content-Disposition: attachment and are never rendered
-- inside the app's own origin.

ALTER TABLE document_images RENAME TO document_files;

ALTER INDEX idx_document_images_license RENAME TO idx_document_files_license;
ALTER INDEX idx_document_images_credential RENAME TO idx_document_files_credential;
ALTER INDEX idx_document_images_user RENAME TO idx_document_files_user;

-- Every constraint keeps its old name through a table rename, so they are
-- renamed explicitly. Leaving "document_images_pkey" on a table called
-- document_files is exactly the kind of half-rename that confuses the next
-- reader.
ALTER TABLE document_files RENAME CONSTRAINT document_images_one_subject TO document_files_one_subject;
ALTER TABLE document_files RENAME CONSTRAINT document_images_byte_size_check TO document_files_byte_size_check;
ALTER TABLE document_files RENAME CONSTRAINT document_images_pkey TO document_files_pkey;
ALTER TABLE document_files RENAME CONSTRAINT document_images_user_id_fkey TO document_files_user_id_fkey;
ALTER TABLE document_files RENAME CONSTRAINT document_images_license_id_fkey TO document_files_license_id_fkey;
ALTER TABLE document_files RENAME CONSTRAINT document_images_credential_id_fkey TO document_files_credential_id_fkey;

ALTER TRIGGER update_document_images_updated_at ON document_files
    RENAME TO update_document_files_updated_at;

-- The CHECK is recreated rather than renamed: its expression changes.
ALTER TABLE document_files DROP CONSTRAINT document_images_content_type_check;
ALTER TABLE document_files ADD CONSTRAINT document_files_content_type_check
    CHECK (content_type IN ('image/jpeg', 'image/png', 'application/pdf'));

COMMENT ON TABLE document_files IS 'Reference photos/scans attached to a licence or credential (JPEG, PNG or PDF); max 5 MB and 5 files per document, enforced in the service layer';
COMMENT ON COLUMN document_files.data IS 'Raw file bytes, served only over an authenticated request; PDFs are always served as an attachment';
COMMENT ON COLUMN document_files.width IS 'Pixel width for images; NULL for formats without intrinsic dimensions, such as PDF';
COMMENT ON COLUMN document_files.height IS 'Pixel height for images; NULL for formats without intrinsic dimensions, such as PDF';
