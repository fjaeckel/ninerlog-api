-- Retain calculation sources alongside recorded integer-minute flight totals.
ALTER TABLE users ADD COLUMN default_flight_time_method TEXT NOT NULL DEFAULT 'block'
    CHECK (default_flight_time_method IN ('block', 'hobbs', 'manual'));
ALTER TABLE flights
    ADD COLUMN flight_time_method TEXT NOT NULL DEFAULT 'block'
        CHECK (flight_time_method IN ('block', 'hobbs', 'manual')),
    ADD COLUMN hobbs_start_tenths INTEGER,
    ADD COLUMN hobbs_end_tenths INTEGER,
    ADD COLUMN manual_total_time INTEGER,
    ADD CONSTRAINT flights_hobbs_readings CHECK (
        (flight_time_method <> 'hobbs' AND hobbs_start_tenths IS NULL AND hobbs_end_tenths IS NULL)
        OR (flight_time_method = 'hobbs' AND NOT is_simulator
            AND hobbs_start_tenths IS NOT NULL AND hobbs_end_tenths IS NOT NULL
            AND hobbs_start_tenths BETWEEN 0 AND 99999999
            AND hobbs_end_tenths BETWEEN 0 AND 99999999
            AND hobbs_end_tenths > hobbs_start_tenths)
    ),
    ADD CONSTRAINT flights_manual_time CHECK (
        (flight_time_method <> 'manual' AND manual_total_time IS NULL)
        OR (flight_time_method = 'manual' AND NOT is_simulator AND manual_total_time IS NOT NULL AND manual_total_time > 0)
    );

ALTER TABLE flights
    ADD COLUMN pic_time_override BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN dual_time_override BOOLEAN NOT NULL DEFAULT false;
