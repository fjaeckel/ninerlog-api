-- Rebuild class_type without GYROPLANE; class ratings using it become OTHER.
-- Aircraft classes are free text and stay as they are.

ALTER TABLE aircraft DROP COLUMN IF EXISTS mtom_kg;

ALTER TABLE class_ratings
    ALTER COLUMN class_type TYPE TEXT USING class_type::text;

UPDATE class_ratings
SET class_type = 'OTHER'
WHERE class_type = 'GYROPLANE';

ALTER TYPE class_type RENAME TO class_type_old;

CREATE TYPE class_type AS ENUM (
    'SEP_LAND', 'SEP_SEA',
    'MEP_LAND', 'MEP_SEA',
    'SET_LAND', 'SET_SEA',
    'TMG', 'IR', 'OTHER',
    'GLIDER', 'ULTRALIGHT'
);

ALTER TABLE class_ratings
    ALTER COLUMN class_type TYPE class_type
    USING class_type::class_type;

DROP TYPE class_type_old;
