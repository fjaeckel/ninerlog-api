-- Detach flights from licenses: flights are no longer tied to a specific license
-- Flight time counts toward all applicable class ratings across all licenses

-- Remove license_id foreign key and column from flights
ALTER TABLE flights DROP CONSTRAINT IF EXISTS flights_license_id_fkey;
ALTER TABLE flights DROP COLUMN IF EXISTS license_id;

-- Remove default_license_id from users
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_default_license_id_fkey;
ALTER TABLE users DROP COLUMN IF EXISTS default_license_id;

-- Remove is_active from licenses (replaced by class rating expiry tracking later)
ALTER TABLE licenses DROP COLUMN IF EXISTS is_active;
