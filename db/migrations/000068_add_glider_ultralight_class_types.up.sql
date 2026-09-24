-- Add GLIDER and ULTRALIGHT to the class_type enum and normalise matching
-- free-text aircraft classes to them.

ALTER TYPE class_type ADD VALUE IF NOT EXISTS 'GLIDER';
ALTER TYPE class_type ADD VALUE IF NOT EXISTS 'ULTRALIGHT';

UPDATE aircraft
SET aircraft_class = 'GLIDER'
WHERE upper(trim(aircraft_class)) IN ('GLIDER', 'SAILPLANE', 'SEGELFLUGZEUG');

UPDATE aircraft
SET aircraft_class = 'ULTRALIGHT'
WHERE upper(trim(aircraft_class)) IN ('ULTRALIGHT', 'ULTRALEICHT', 'UL', 'MICROLIGHT');
