-- Import status and format enums
CREATE TYPE import_format AS ENUM ('CSV', 'FOREFLIGHT_CSV', 'XLS', 'XLSX');
CREATE TYPE import_status AS ENUM ('completed', 'partial', 'failed');

-- Flight import history table
CREATE TABLE flight_imports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    license_id UUID NOT NULL REFERENCES licenses(id) ON DELETE CASCADE,
    file_name VARCHAR(255) NOT NULL,
    import_format import_format NOT NULL,
    import_status import_status NOT NULL DEFAULT 'completed',
    total_rows INTEGER NOT NULL DEFAULT 0,
    imported_count INTEGER NOT NULL DEFAULT 0,
    skipped_count INTEGER NOT NULL DEFAULT 0,
    error_count INTEGER NOT NULL DEFAULT 0,
    duplicate_count INTEGER NOT NULL DEFAULT 0,
    imported_flight_ids UUID[] DEFAULT '{}',
    errors JSONB DEFAULT '[]',
    column_mappings JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_flight_imports_user ON flight_imports(user_id);
CREATE INDEX idx_flight_imports_license ON flight_imports(license_id);
