-- Simplify flight fields: remove pilot_comments (merged into remarks),
-- night_time/landings_day/landings_night/is_pic/is_dual are now auto-calculated

ALTER TABLE flights DROP COLUMN IF EXISTS pilot_comments;
