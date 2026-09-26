-- Ultralight kind ("Luftsportgeräteart") on aircraft classed ULTRALIGHT and on
-- ULTRALIGHT class ratings. NULL means unspecified.
--
-- THREE_AXIS_MOTORGLIDER is an aircraft-only kind: a three-axis ultralight that
-- meets the TMG definition. It is flown on a THREE_AXIS rating.

ALTER TABLE aircraft
    ADD COLUMN ul_kind VARCHAR(30)
        CONSTRAINT aircraft_ul_kind_check CHECK (ul_kind IN (
            'THREE_AXIS', 'THREE_AXIS_MOTORGLIDER', 'WEIGHT_SHIFT',
            'GYROPLANE', 'HELICOPTER', 'POWERED_PARAGLIDER', 'SAILPLANE'
        ));

ALTER TABLE class_ratings
    ADD COLUMN ul_kind VARCHAR(30)
        CONSTRAINT class_ratings_ul_kind_check CHECK (ul_kind IN (
            'THREE_AXIS', 'WEIGHT_SHIFT', 'GYROPLANE',
            'HELICOPTER', 'POWERED_PARAGLIDER', 'SAILPLANE'
        ));
