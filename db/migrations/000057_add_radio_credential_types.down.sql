-- Postgres cannot drop a value from an enum, so reversing migration 57 means
-- rebuilding the type without the radio certificates and re-pointing the
-- column at it.
--
-- The USING cast deliberately FAILS if any credential still holds one of the
-- removed values. Rolling this back must not silently discard a pilot's
-- records -- re-type or delete those credentials first, then run the down
-- migration again.

ALTER TYPE credential_type RENAME TO credential_type_old;

CREATE TYPE credential_type AS ENUM (
    'EASA_CLASS1_MEDICAL',
    'EASA_CLASS2_MEDICAL',
    'EASA_LAPL_MEDICAL',
    'FAA_CLASS1_MEDICAL',
    'FAA_CLASS2_MEDICAL',
    'FAA_CLASS3_MEDICAL',
    'LANG_ICAO_LEVEL4',
    'LANG_ICAO_LEVEL5',
    'LANG_ICAO_LEVEL6',
    'SEC_CLEARANCE_ZUP',
    'SEC_CLEARANCE_ZUBB',
    'OTHER'
);

ALTER TABLE credentials
    ALTER COLUMN credential_type TYPE credential_type
    USING credential_type::text::credential_type;

DROP TYPE credential_type_old;
