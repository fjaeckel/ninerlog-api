ALTER TABLE flights
    ADD COLUMN solo_time DECIMAL(5, 2) NOT NULL DEFAULT 0 CHECK (solo_time >= 0);
