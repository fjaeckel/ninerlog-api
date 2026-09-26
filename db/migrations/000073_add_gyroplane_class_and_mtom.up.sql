-- GYROPLANE class type for the EASA gyroplane pilot licence (GPL, FCL.240.G),
-- normalising matching free-text aircraft classes, and the aircraft's maximum
-- certificated take-off mass in kilograms (FCL.035(a)(5) credits Annex I
-- gyroplanes of at least 450 kg).

ALTER TYPE class_type ADD VALUE IF NOT EXISTS 'GYROPLANE';

UPDATE aircraft
SET aircraft_class = 'GYROPLANE'
WHERE upper(trim(aircraft_class)) IN ('GYROPLANE', 'GYROCOPTER', 'TRAGSCHRAUBER');

ALTER TABLE aircraft
    ADD COLUMN mtom_kg INTEGER
        CONSTRAINT aircraft_mtom_kg_check CHECK (mtom_kg > 0 AND mtom_kg <= 1000000);
