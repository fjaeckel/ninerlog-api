-- Adds the is_simulator discriminator separating FSTD sessions from flights.
--
-- An FSTD session records date, device type and session duration only
-- (EASA AMC1 FCL.050 Cols 20-22). Its duration lives in
-- simulated_flight_time and is never summed into total_time.

ALTER TABLE flights ADD COLUMN is_simulator BOOLEAN NOT NULL DEFAULT FALSE;

-- Rows carrying an FSTD type designation are device sessions. Their session
-- duration was previously entered as block time; recover it into
-- simulated_flight_time where that column is still empty.
UPDATE flights
SET simulated_flight_time = total_time
WHERE fstd_type IS NOT NULL
  AND btrim(fstd_type) <> ''
  AND simulated_flight_time = 0
  AND total_time > 0;

-- Clear every flight-time, pilot-function and landing column on device
-- sessions so they stop contributing to flight totals.
UPDATE flights
SET is_simulator       = TRUE,
    total_time         = 0,
    pic_time           = 0,
    dual_time          = 0,
    sic_time           = 0,
    dual_given_time    = 0,
    multi_pilot_time   = 0,
    solo_time          = 0,
    cross_country_time = 0,
    night_time         = 0,
    ifr_time           = 0,
    landings_day       = 0,
    landings_night     = 0,
    all_landings       = 0,
    takeoffs_day       = 0,
    takeoffs_night     = 0,
    distance           = 0,
    is_pic             = FALSE,
    is_dual            = FALSE
WHERE fstd_type IS NOT NULL
  AND btrim(fstd_type) <> '';

-- Serves the "flights only" predicate carried by every aggregate query.
CREATE INDEX idx_flights_user_date_not_simulator
    ON flights (user_id, date DESC)
    WHERE NOT is_simulator;

CREATE INDEX idx_flights_user_simulator
    ON flights (user_id)
    WHERE is_simulator;
