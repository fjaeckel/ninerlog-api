-- Rebuild class_type without GLIDER and ULTRALIGHT; class ratings using them
-- become OTHER. Aircraft classes are free text and stay as they are.

ALTER TABLE class_ratings
    ALTER COLUMN class_type TYPE TEXT USING class_type::text;

UPDATE class_ratings
SET class_type = 'OTHER'
WHERE class_type IN ('GLIDER', 'ULTRALIGHT');

ALTER TYPE class_type RENAME TO class_type_old;

CREATE TYPE class_type AS ENUM (
    'SEP_LAND', 'SEP_SEA',
    'MEP_LAND', 'MEP_SEA',
    'SET_LAND', 'SET_SEA',
    'TMG', 'IR', 'OTHER'
);

ALTER TABLE class_ratings
    ALTER COLUMN class_type TYPE class_type
    USING class_type::class_type;

DROP TYPE class_type_old;
