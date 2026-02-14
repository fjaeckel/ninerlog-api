-- Class ratings: what expires and requires revalidation on pilot licenses
-- Each license can have multiple class ratings (SEP, MEP, IR, etc.)

-- Class type enum for airplane class ratings
CREATE TYPE class_type AS ENUM (
    'SEP_LAND', 'SEP_SEA',
    'MEP_LAND', 'MEP_SEA',
    'SET_LAND', 'SET_SEA',
    'TMG', 'IR', 'OTHER'
);

-- Class ratings table
CREATE TABLE class_ratings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    license_id UUID NOT NULL REFERENCES licenses(id) ON DELETE CASCADE,
    class_type class_type NOT NULL,
    issue_date DATE NOT NULL,
    expiry_date DATE,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_class_ratings_license_id ON class_ratings(license_id);
CREATE INDEX idx_class_ratings_class_type ON class_ratings(class_type);
CREATE INDEX idx_class_ratings_expiry ON class_ratings(expiry_date) WHERE expiry_date IS NOT NULL;

-- Add class_rating to aircraft (maps aircraft to a class type)
ALTER TABLE aircraft ADD COLUMN class_rating class_type;
