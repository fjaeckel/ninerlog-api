ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_flight_list_column_mode_check;

ALTER TABLE users
    DROP COLUMN flight_list_column_mode,
    DROP COLUMN flight_list_columns;
