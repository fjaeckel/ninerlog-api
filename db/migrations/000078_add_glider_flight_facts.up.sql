-- Glider flight facts on flights: the launch count (Part-SFCL launch recency),
-- its manual-override flag, the outlanding and tow-flight flags, and the
-- release height in metres. A NULL launches value reads as the take-off count,
-- at least one per flight.

ALTER TABLE flights
    ADD COLUMN launches INTEGER
        CONSTRAINT flights_launches_check CHECK (launches >= 0),
    ADD COLUMN launches_override BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN is_outlanding BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN is_tow_flight BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN release_height_m INTEGER
        CONSTRAINT flights_release_height_m_check CHECK (release_height_m BETWEEN 0 AND 20000);
