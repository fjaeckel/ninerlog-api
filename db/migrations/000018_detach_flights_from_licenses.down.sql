-- Re-add license_id to flights
ALTER TABLE flights ADD COLUMN license_id UUID REFERENCES licenses(id);

-- Re-add default_license_id to users
ALTER TABLE users ADD COLUMN default_license_id UUID REFERENCES licenses(id) ON DELETE SET NULL;

-- Re-add is_active to licenses
ALTER TABLE licenses ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT true;
