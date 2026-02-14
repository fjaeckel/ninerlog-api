-- Restructure licenses: replace enum with flexible authority + type fields
-- Licenses don't expire — expiry belongs on class ratings

-- Add new columns
ALTER TABLE licenses ADD COLUMN regulatory_authority VARCHAR(100);
ALTER TABLE licenses ADD COLUMN license_type_text VARCHAR(100);
ALTER TABLE licenses ADD COLUMN requires_separate_logbook BOOLEAN NOT NULL DEFAULT false;

-- Migrate existing enum values to authority + type pairs
UPDATE licenses SET
    regulatory_authority = CASE
        WHEN license_type::text LIKE 'EASA_%' THEN 'EASA'
        WHEN license_type::text LIKE 'FAA_%' THEN 'FAA'
        ELSE 'OTHER'
    END,
    license_type_text = CASE
        WHEN license_type::text = 'EASA_PPL' THEN 'PPL'
        WHEN license_type::text = 'FAA_PPL' THEN 'PPL'
        WHEN license_type::text = 'EASA_SPL' THEN 'SPL'
        WHEN license_type::text = 'FAA_SPORT' THEN 'Sport'
        WHEN license_type::text = 'EASA_CPL' THEN 'CPL'
        WHEN license_type::text = 'FAA_CPL' THEN 'CPL'
        WHEN license_type::text = 'EASA_ATPL' THEN 'ATPL'
        WHEN license_type::text = 'FAA_ATPL' THEN 'ATPL'
        WHEN license_type::text = 'EASA_IR' THEN 'IR'
        WHEN license_type::text = 'FAA_IR' THEN 'IR'
        ELSE license_type::text
    END;

-- Set requires_separate_logbook for SPL/Sport
UPDATE licenses SET requires_separate_logbook = true
WHERE license_type::text IN ('EASA_SPL', 'FAA_SPORT');

-- Make new columns NOT NULL after migration
ALTER TABLE licenses ALTER COLUMN regulatory_authority SET NOT NULL;
ALTER TABLE licenses ALTER COLUMN license_type_text SET NOT NULL;

-- Drop old columns
ALTER TABLE licenses DROP COLUMN license_type;
ALTER TABLE licenses DROP COLUMN expiry_date;

-- Rename new column to final name
ALTER TABLE licenses RENAME COLUMN license_type_text TO license_type;

-- Drop the old enum type
DROP TYPE IF EXISTS license_type;

-- Add index on regulatory authority
CREATE INDEX idx_licenses_authority ON licenses(regulatory_authority);
