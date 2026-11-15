-- Retain meter observations independently of the authoritative duration.
-- No recorded totals, methods or credits are rewritten.
ALTER TABLE flights DROP CONSTRAINT flights_hobbs_readings,
    DROP CONSTRAINT flights_manual_time;
ALTER TABLE flights ADD CONSTRAINT flights_hobbs_readings CHECK (
    (hobbs_start_tenths IS NULL OR hobbs_start_tenths BETWEEN 0 AND 99999999)
    AND (hobbs_end_tenths IS NULL OR hobbs_end_tenths BETWEEN 0 AND 99999999)
    AND (hobbs_start_tenths IS NULL OR hobbs_end_tenths IS NULL OR hobbs_end_tenths > hobbs_start_tenths)
    AND (NOT is_simulator OR (hobbs_start_tenths IS NULL AND hobbs_end_tenths IS NULL))
    AND (flight_time_method <> 'hobbs' OR manual_total_time IS NOT NULL
        OR (hobbs_start_tenths IS NOT NULL AND hobbs_end_tenths IS NOT NULL))
), ADD CONSTRAINT flights_manual_time CHECK (
    (manual_total_time IS NULL OR (NOT is_simulator AND manual_total_time > 0))
    AND (flight_time_method <> 'manual' OR manual_total_time IS NOT NULL)
);
