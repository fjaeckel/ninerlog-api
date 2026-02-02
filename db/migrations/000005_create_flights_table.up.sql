-- Create flights table
CREATE TABLE flights (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    license_id UUID NOT NULL REFERENCES licenses(id) ON DELETE CASCADE,
    date DATE NOT NULL,
    
    -- Aircraft information
    aircraft_reg VARCHAR(20) NOT NULL,
    aircraft_type VARCHAR(50) NOT NULL,
    
    -- Route information
    departure_icao VARCHAR(4),
    arrival_icao VARCHAR(4),
    departure_time TIME,
    arrival_time TIME,
    
    -- Flight times (stored as decimal hours)
    total_time DECIMAL(5,2) NOT NULL CHECK (total_time >= 0),
    pic_time DECIMAL(5,2) NOT NULL DEFAULT 0 CHECK (pic_time >= 0),
    dual_time DECIMAL(5,2) NOT NULL DEFAULT 0 CHECK (dual_time >= 0),
    solo_time DECIMAL(5,2) NOT NULL DEFAULT 0 CHECK (solo_time >= 0),
    night_time DECIMAL(5,2) NOT NULL DEFAULT 0 CHECK (night_time >= 0),
    ifr_time DECIMAL(5,2) NOT NULL DEFAULT 0 CHECK (ifr_time >= 0),
    
    -- Landings
    landings_day INTEGER NOT NULL DEFAULT 0 CHECK (landings_day >= 0),
    landings_night INTEGER NOT NULL DEFAULT 0 CHECK (landings_night >= 0),
    
    -- Additional information
    remarks TEXT,
    
    -- Metadata
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Validation: sum of pic_time, dual_time, solo_time should not exceed total_time
    CONSTRAINT valid_time_distribution CHECK (
        (pic_time + dual_time + solo_time) <= total_time
    ),
    
    -- Validation: night_time should not exceed total_time
    CONSTRAINT valid_night_time CHECK (night_time <= total_time),
    
    -- Validation: ifr_time should not exceed total_time
    CONSTRAINT valid_ifr_time CHECK (ifr_time <= total_time)
);

-- Create indexes for flights
CREATE INDEX idx_flights_user_id ON flights(user_id);
CREATE INDEX idx_flights_license_id ON flights(license_id);
CREATE INDEX idx_flights_date ON flights(date DESC);
CREATE INDEX idx_flights_user_date ON flights(user_id, date DESC);
CREATE INDEX idx_flights_license_date ON flights(license_id, date DESC);
CREATE INDEX idx_flights_aircraft_reg ON flights(aircraft_reg);

-- Create trigger to update updated_at
CREATE TRIGGER update_flights_updated_at
    BEFORE UPDATE ON flights
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
