ALTER TABLE flights DROP COLUMN IF EXISTS ground_training_time;
ALTER TABLE flights DROP COLUMN IF EXISTS simulated_flight_time;
ALTER TABLE flights DROP COLUMN IF EXISTS dual_given_time;
ALTER TABLE flights DROP COLUMN IF EXISTS sic_time;
ALTER TABLE flights DROP COLUMN IF EXISTS pilot_comments;
ALTER TABLE flights DROP COLUMN IF EXISTS instructor_comments;
ALTER TABLE flights DROP COLUMN IF EXISTS instructor_name;

DROP TABLE IF EXISTS flight_crew_members;
DROP TABLE IF EXISTS contacts;
DROP TYPE IF EXISTS crew_role;
