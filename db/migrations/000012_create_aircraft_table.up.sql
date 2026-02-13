-- Create aircraft table for user's fleet management
CREATE TABLE aircraft (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    registration VARCHAR(20) NOT NULL,
    type VARCHAR(50) NOT NULL,
    make VARCHAR(100) NOT NULL,
    model VARCHAR(100) NOT NULL,
    category VARCHAR(20),
    engine_type VARCHAR(20),
    is_complex BOOLEAN NOT NULL DEFAULT false,
    is_high_performance BOOLEAN NOT NULL DEFAULT false,
    is_tailwheel BOOLEAN NOT NULL DEFAULT false,
    notes TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Each user can only have one aircraft with the same registration
CREATE UNIQUE INDEX idx_aircraft_user_registration ON aircraft(user_id, registration);

-- Index for listing aircraft by user
CREATE INDEX idx_aircraft_user ON aircraft(user_id);

-- Trigger for updated_at
CREATE TRIGGER update_aircraft_updated_at
    BEFORE UPDATE ON aircraft
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Column comments
COMMENT ON COLUMN aircraft.registration IS 'Aircraft registration/tail number (e.g. D-EFGH, N12345)';
COMMENT ON COLUMN aircraft.type IS 'Aircraft type designation (e.g. C172, PA28, ASK21)';
COMMENT ON COLUMN aircraft.make IS 'Aircraft manufacturer (e.g. Cessna, Piper)';
COMMENT ON COLUMN aircraft.model IS 'Aircraft model name (e.g. 172 Skyhawk, Cherokee)';
COMMENT ON COLUMN aircraft.category IS 'Aircraft category (e.g. SEP, MEP, TMG)';
COMMENT ON COLUMN aircraft.engine_type IS 'Engine type: piston, turboprop, jet, electric';
COMMENT ON COLUMN aircraft.is_complex IS 'Has retractable gear, flaps, and constant speed propeller';
COMMENT ON COLUMN aircraft.is_high_performance IS 'Has more than 200 HP';
COMMENT ON COLUMN aircraft.is_tailwheel IS 'Has tailwheel (conventional gear)';
COMMENT ON COLUMN aircraft.is_active IS 'Whether aircraft is still active in the user fleet';
