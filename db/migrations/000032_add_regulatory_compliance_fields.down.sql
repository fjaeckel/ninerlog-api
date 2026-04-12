-- Revert Phase 6c regulatory compliance fields
ALTER TABLE flights DROP COLUMN IF EXISTS pic_name;
ALTER TABLE flights DROP COLUMN IF EXISTS multi_pilot_time;
ALTER TABLE flights DROP COLUMN IF EXISTS fstd_type;
ALTER TABLE flights DROP COLUMN IF EXISTS approaches;
ALTER TABLE flights DROP COLUMN IF EXISTS endorsements;
