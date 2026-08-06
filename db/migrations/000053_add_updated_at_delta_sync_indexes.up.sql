-- Delta sync (updatedSince) support.
--
-- The list endpoints for flights, aircraft, contacts, credentials and licenses
-- accept an `updatedSince` query parameter that compiles to
-- `AND updated_at > $n` on top of the existing `user_id = $1` predicate. Without
-- a composite index every incremental pull degenerates into a scan of the user's
-- whole logbook — precisely the cost the parameter exists to avoid. The leading
-- user_id column also keeps these usable for the plain user-scoped listing.
--
-- DESC matches how a sync client reads the result (newest change first) and lets
-- the planner satisfy "give me the highest updated_at" without a sort.

CREATE INDEX IF NOT EXISTS idx_flights_user_updated_at ON flights (user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_aircraft_user_updated_at ON aircraft (user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_contacts_user_updated_at ON contacts (user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_credentials_user_updated_at ON credentials (user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_licenses_user_updated_at ON licenses (user_id, updated_at DESC);
