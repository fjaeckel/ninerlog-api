-- Add default license preference to users
ALTER TABLE users ADD COLUMN default_license_id UUID REFERENCES licenses(id) ON DELETE SET NULL;
