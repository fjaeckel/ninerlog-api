ALTER TABLE flights
    DROP CONSTRAINT IF EXISTS pic_dual_exclusive,
    DROP COLUMN IF EXISTS is_pic,
    DROP COLUMN IF EXISTS is_dual;
