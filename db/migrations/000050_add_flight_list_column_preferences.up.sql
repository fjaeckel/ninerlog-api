-- Per-user choice of which optional columns the flights list shows.
--
-- 'auto' (the default, and the behaviour every existing user keeps) lets the
-- client pick the optional columns from the data on the page, so a VFR-only
-- pilot never sees an empty IFR column. 'custom' means the user has decided,
-- and flight_list_columns is that decision — an empty array in custom mode is
-- a valid answer ("show none of the optional columns").
ALTER TABLE users
    ADD COLUMN flight_list_column_mode VARCHAR(10) NOT NULL DEFAULT 'auto',
    ADD COLUMN flight_list_columns TEXT[] NOT NULL DEFAULT '{}';

ALTER TABLE users
    ADD CONSTRAINT users_flight_list_column_mode_check
    CHECK (flight_list_column_mode IN ('auto', 'custom'));
