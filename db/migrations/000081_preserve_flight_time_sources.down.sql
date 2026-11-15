-- Refuse lossy downgrade when the previous application cannot read the data.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM flights WHERE
        (flight_time_method <> 'hobbs' AND (hobbs_start_tenths IS NOT NULL OR hobbs_end_tenths IS NOT NULL))
        OR (flight_time_method = 'hobbs' AND (hobbs_start_tenths IS NULL OR hobbs_end_tenths IS NULL))
        OR (flight_time_method <> 'manual' AND manual_total_time IS NOT NULL)) THEN
        RAISE EXCEPTION 'Preserved source observations or total overrides require this schema; export and reconcile explicitly before downgrade';
    END IF;
END $$;
ALTER TABLE flights DROP CONSTRAINT flights_hobbs_readings, DROP CONSTRAINT flights_manual_time;
ALTER TABLE flights ADD CONSTRAINT flights_hobbs_readings CHECK (
    (flight_time_method <> 'hobbs' AND hobbs_start_tenths IS NULL AND hobbs_end_tenths IS NULL)
    OR (flight_time_method = 'hobbs' AND NOT is_simulator
        AND hobbs_start_tenths IS NOT NULL AND hobbs_end_tenths IS NOT NULL
        AND hobbs_start_tenths BETWEEN 0 AND 99999999
        AND hobbs_end_tenths BETWEEN 0 AND 99999999 AND hobbs_end_tenths > hobbs_start_tenths)
), ADD CONSTRAINT flights_manual_time CHECK (
    (flight_time_method <> 'manual' AND manual_total_time IS NULL)
    OR (flight_time_method = 'manual' AND NOT is_simulator AND manual_total_time IS NOT NULL AND manual_total_time > 0)
);
