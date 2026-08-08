-- Reverse the rename and narrow the accepted formats back to images.
--
-- The CHECK is restored first and deliberately FAILS if any stored file is a
-- PDF: rolling this back must not silently orphan rows the application would
-- then refuse to serve. Delete those files first, then run this again.

ALTER TABLE document_files DROP CONSTRAINT document_files_content_type_check;
ALTER TABLE document_files ADD CONSTRAINT document_images_content_type_check
    CHECK (content_type IN ('image/jpeg', 'image/png'));

ALTER TRIGGER update_document_files_updated_at ON document_files
    RENAME TO update_document_images_updated_at;

ALTER TABLE document_files RENAME CONSTRAINT document_files_one_subject TO document_images_one_subject;
ALTER TABLE document_files RENAME CONSTRAINT document_files_byte_size_check TO document_images_byte_size_check;
ALTER TABLE document_files RENAME CONSTRAINT document_files_pkey TO document_images_pkey;
ALTER TABLE document_files RENAME CONSTRAINT document_files_user_id_fkey TO document_images_user_id_fkey;
ALTER TABLE document_files RENAME CONSTRAINT document_files_license_id_fkey TO document_images_license_id_fkey;
ALTER TABLE document_files RENAME CONSTRAINT document_files_credential_id_fkey TO document_images_credential_id_fkey;

ALTER INDEX idx_document_files_license RENAME TO idx_document_images_license;
ALTER INDEX idx_document_files_credential RENAME TO idx_document_images_credential;
ALTER INDEX idx_document_files_user RENAME TO idx_document_images_user;

ALTER TABLE document_files RENAME TO document_images;
