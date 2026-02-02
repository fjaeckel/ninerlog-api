-- Create license_type enum
CREATE TYPE license_type AS ENUM (
    'EASA_PPL',
    'FAA_PPL',
    'EASA_SPL',
    'FAA_SPORT',
    'EASA_CPL',
    'FAA_CPL',
    'EASA_ATPL',
    'FAA_ATPL',
    'EASA_IR',
    'FAA_IR'
);

-- Create licenses table
CREATE TABLE licenses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    license_type license_type NOT NULL,
    license_number VARCHAR(100) NOT NULL,
    issue_date DATE NOT NULL,
    expiry_date DATE,
    issuing_authority VARCHAR(100) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create indexes for licenses
CREATE INDEX idx_licenses_user_id ON licenses(user_id);
CREATE INDEX idx_licenses_user_active ON licenses(user_id, is_active);
CREATE INDEX idx_licenses_type ON licenses(license_type);

-- Create trigger to update updated_at
CREATE TRIGGER update_licenses_updated_at
    BEFORE UPDATE ON licenses
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
