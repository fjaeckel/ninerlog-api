-- Re-create enum type
CREATE TYPE license_type AS ENUM (
    'EASA_PPL', 'FAA_PPL', 'EASA_SPL', 'FAA_SPORT',
    'EASA_CPL', 'FAA_CPL', 'EASA_ATPL', 'FAA_ATPL',
    'EASA_IR', 'FAA_IR'
);

-- Re-add old columns
ALTER TABLE licenses ADD COLUMN expiry_date DATE;

-- Rename license_type back
ALTER TABLE licenses RENAME COLUMN license_type TO license_type_text;
ALTER TABLE licenses ADD COLUMN license_type license_type;

-- Attempt to reverse migrate (best effort)
UPDATE licenses SET license_type = (regulatory_authority || '_' || license_type_text)::license_type;

-- Drop new columns
ALTER TABLE licenses DROP COLUMN regulatory_authority;
ALTER TABLE licenses DROP COLUMN license_type_text;
ALTER TABLE licenses DROP COLUMN requires_separate_logbook;

-- Drop new index
DROP INDEX IF EXISTS idx_licenses_authority;
