-- Allow a flight to be logged as both PIC and dual received.
--
-- 14 CFR §61.51(e)(1)(i) lets a pilot who is rated for the aircraft log PIC
-- time while receiving instruction as sole manipulator of the controls
-- (flight review, IPC, complex or high-performance endorsement training).
-- Those minutes are one slice of total time logged in two columns, so
-- pic_time and dual_time overlap rather than adding up; the application
-- counts the overlap once (models.Flight.ValidateTimeDistribution).

ALTER TABLE flights
    DROP CONSTRAINT IF EXISTS pic_dual_exclusive;

COMMENT ON COLUMN flights.is_pic IS 'Whether this flight was logged as pilot-in-command; may be set together with is_dual (PIC while receiving instruction, 14 CFR §61.51(e)(1)(i))';
COMMENT ON COLUMN flights.is_dual IS 'Whether this flight was logged as dual instruction received; may be set together with is_pic';
