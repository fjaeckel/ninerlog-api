-- Denormalized lock pointer: a flight is locked against edits/deletes
-- exactly when this is non-null. Kept separate from the append-only
-- flight_signatures history table so the lock check stays O(1) with no
-- join. Cleared back to NULL when the active signature is voided.
ALTER TABLE flights ADD COLUMN signature_id UUID REFERENCES flight_signatures(id) ON DELETE SET NULL;

CREATE INDEX flights_signature_id_idx ON flights (signature_id) WHERE signature_id IS NOT NULL;

COMMENT ON COLUMN flights.signature_id IS 'Non-null iff the flight is locked by a completed, non-voided instructor signature';
