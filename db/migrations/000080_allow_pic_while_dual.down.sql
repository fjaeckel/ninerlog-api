-- NOT VALID leaves rows already logged as both PIC and dual untouched.
ALTER TABLE flights
    ADD CONSTRAINT pic_dual_exclusive CHECK (NOT (is_pic AND is_dual)) NOT VALID;

COMMENT ON COLUMN flights.is_pic IS 'Whether this flight was logged as pilot-in-command';
COMMENT ON COLUMN flights.is_dual IS 'Whether this flight was logged as dual instruction received';
