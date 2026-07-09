-- Flight sessions: live tap-to-log capture of block/flight times from a
-- mobile device. A session accumulates event timestamps (off block, takeoff,
-- landing, on block) while the flight is happening and is converted into a
-- regular flights row when the pilot goes on blocks. Keeping in-progress data
-- out of the flights table means currency, statistics, and exports never see
-- incomplete entries.
--
-- Timestamps are stored as full TIMESTAMPTZ (unlike flights.off_block_time
-- which is a bare TIME) so flights crossing midnight UTC stay unambiguous;
-- the date + HH:MM:SS split happens at conversion.

CREATE TABLE flight_sessions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status         TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'completed', 'discarded')),

    aircraft_reg   TEXT,
    departure_icao CHAR(4),
    arrival_icao   CHAR(4),

    off_block_at   TIMESTAMPTZ,
    takeoff_at     TIMESTAMPTZ,
    landing_at     TIMESTAMPTZ,
    on_block_at    TIMESTAMPTZ,

    -- Set when the session is completed and a flight row has been created
    flight_id      UUID REFERENCES flights(id) ON DELETE SET NULL,

    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A user can only have one in-progress flight at a time. Double-taps and
-- offline retries hit the existing open session instead of creating a second.
CREATE UNIQUE INDEX flight_sessions_one_open_per_user
    ON flight_sessions (user_id) WHERE status = 'open';

CREATE INDEX flight_sessions_user_id_idx ON flight_sessions (user_id);

COMMENT ON TABLE flight_sessions IS 'In-progress flights captured live via tap-to-log (quick log) from mobile devices';
COMMENT ON COLUMN flight_sessions.off_block_at IS 'Off-block (chocks off / engine start) instant in UTC';
COMMENT ON COLUMN flight_sessions.on_block_at IS 'On-block (chocks on / engine shutdown) instant in UTC';
