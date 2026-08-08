-- Add the German radio operator certificates to the credential_type enum.
--
-- credential_type is a Postgres ENUM (migration 9), not a free-text column, so
-- adding a value to the OpenAPI spec and the Go model is not enough -- the
-- INSERT is rejected by the type itself until the value exists here too.
--
-- Three separate certificates, not three levels of one: BZF II covers VFR
-- radio in German only, BZF I adds English, and AZF is the full certificate
-- required for IFR. None of them expires, which the nullable expiry_date
-- already allows.
--
-- ADD VALUE inside a transaction is fine on Postgres 12+ as long as the new
-- value is not *used* in the same transaction, which nothing here does.

ALTER TYPE credential_type ADD VALUE IF NOT EXISTS 'RADIO_BZF2';
ALTER TYPE credential_type ADD VALUE IF NOT EXISTS 'RADIO_BZF1';
ALTER TYPE credential_type ADD VALUE IF NOT EXISTS 'RADIO_AZF';
