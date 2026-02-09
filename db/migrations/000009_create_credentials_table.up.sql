-- Create credential_type enum and credentials table

CREATE TYPE credential_type AS ENUM (
    'EASA_CLASS1_MEDICAL',
    'EASA_CLASS2_MEDICAL',
    'EASA_LAPL_MEDICAL',
    'FAA_CLASS1_MEDICAL',
    'FAA_CLASS2_MEDICAL',
    'FAA_CLASS3_MEDICAL',
    'LANG_ICAO_LEVEL4',
    'LANG_ICAO_LEVEL5',
    'LANG_ICAO_LEVEL6',
    'SEC_CLEARANCE_ZUP',
    'SEC_CLEARANCE_ZUBB',
    'OTHER'
);

CREATE TABLE credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_type credential_type NOT NULL,
    credential_number VARCHAR(100),
    issue_date DATE NOT NULL,
    expiry_date DATE,
    issuing_authority VARCHAR(255) NOT NULL,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_credentials_user ON credentials(user_id);
CREATE INDEX idx_credentials_user_type ON credentials(user_id, credential_type);
CREATE INDEX idx_credentials_expiry ON credentials(expiry_date)
    WHERE expiry_date IS NOT NULL;

-- Trigger for updated_at
CREATE TRIGGER update_credentials_updated_at
    BEFORE UPDATE ON credentials
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
