DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM flights WHERE flight_time_method <> 'block' OR pic_time_override OR dual_time_override)
       OR EXISTS (SELECT 1 FROM users WHERE default_flight_time_method <> 'block') THEN
        RAISE EXCEPTION 'Export Hobbs/manual records and explicitly convert them to block time before rolling back flight-time source support';
    END IF;
END $$;
ALTER TABLE flights DROP CONSTRAINT flights_hobbs_readings, DROP CONSTRAINT flights_manual_time,
    DROP COLUMN manual_total_time,
    DROP COLUMN hobbs_end_tenths, DROP COLUMN hobbs_start_tenths, DROP COLUMN flight_time_method;
ALTER TABLE users DROP COLUMN default_flight_time_method;

ALTER TABLE flights DROP COLUMN pic_time_override, DROP COLUMN dual_time_override;
