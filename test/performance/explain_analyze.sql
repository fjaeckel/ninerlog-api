-- EXPLAIN ANALYZE tests for critical NinerLog queries
-- Run against perf database after seeding:
--   psql -h localhost -p 5435 -U perfuser -d ninerlog_perf -f test/performance/explain_analyze.sql
--
-- Prerequisites: Run seed.js first to populate test data

\timing on
\pset format wrapped

-- Get a sample user_id for testing
\echo '=========================================='
\echo 'Setup: Getting sample user ID'
\echo '=========================================='
SELECT id AS sample_user_id FROM users WHERE email = 'perfuser-0000@ninerlog-perf.com' \gset

-- ==========================================
-- 1. Flight List — unfiltered (paginated)
-- ==========================================
\echo ''
\echo '=========================================='
\echo '1. Flight List — unfiltered, page 1'
\echo '=========================================='

EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT id, user_id, date, aircraft_reg, aircraft_type,
       departure_icao, arrival_icao, total_time, is_pic, is_dual,
       pic_time, dual_time, night_time, ifr_time,
       landings_day, landings_night
FROM flights
WHERE user_id = :'sample_user_id'
ORDER BY date DESC
LIMIT 25 OFFSET 0;

-- ==========================================
-- 2. Flight List — date range filter
-- ==========================================
\echo ''
\echo '=========================================='
\echo '2. Flight List — date range filter'
\echo '=========================================='

EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT id, user_id, date, aircraft_reg, aircraft_type,
       departure_icao, arrival_icao, total_time
FROM flights
WHERE user_id = :'sample_user_id'
  AND date >= '2025-01-01'
  AND date <= '2025-12-31'
ORDER BY date DESC
LIMIT 25 OFFSET 0;

-- ==========================================
-- 3. Flight List — text search (LIKE)
-- ==========================================
\echo ''
\echo '=========================================='
\echo '3. Flight List — text search (worst case)'
\echo '=========================================='

EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT id, user_id, date, aircraft_reg, aircraft_type,
       departure_icao, arrival_icao, total_time
FROM flights
WHERE user_id = :'sample_user_id'
  AND (
    UPPER(aircraft_reg) LIKE UPPER('%C172%')
    OR UPPER(aircraft_type) LIKE UPPER('%C172%')
    OR UPPER(departure_icao) LIKE UPPER('%C172%')
    OR UPPER(arrival_icao) LIKE UPPER('%C172%')
    OR UPPER(COALESCE(remarks, '')) LIKE UPPER('%C172%')
  )
ORDER BY date DESC
LIMIT 25 OFFSET 0;

-- ==========================================
-- 4. Flight Count — for pagination
-- ==========================================
\echo ''
\echo '=========================================='
\echo '4. Flight Count — pagination total'
\echo '=========================================='

EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT COUNT(*)
FROM flights
WHERE user_id = :'sample_user_id';

-- ==========================================
-- 5. Statistics aggregation (dashboard)
-- ==========================================
\echo ''
\echo '=========================================='
\echo '5. Statistics — dashboard aggregation'
\echo '=========================================='

EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT
    COUNT(*) as total_flights,
    COALESCE(SUM(total_time), 0) as total_minutes,
    COALESCE(SUM(pic_time), 0) as pic_minutes,
    COALESCE(SUM(dual_time), 0) as dual_minutes,
    COALESCE(SUM(night_time), 0) as night_minutes,
    COALESCE(SUM(ifr_time), 0) as ifr_minutes,
    COALESCE(SUM(solo_time), 0) as solo_minutes,
    COALESCE(SUM(cross_country_time), 0) as cross_country_minutes,
    COALESCE(SUM(landings_day), 0) as landings_day,
    COALESCE(SUM(landings_night), 0) as landings_night
FROM flights
WHERE user_id = :'sample_user_id';

-- ==========================================
-- 6. Monthly trends report
-- ==========================================
\echo ''
\echo '=========================================='
\echo '6. Monthly Trends — 12 months'
\echo '=========================================='

EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT
    TO_CHAR(date_trunc('month', date), 'YYYY-MM') AS month,
    COUNT(*) as flights,
    SUM(total_time) as total_minutes,
    SUM(pic_time) as pic_minutes,
    SUM(dual_time) as dual_minutes,
    SUM(night_time) as night_minutes,
    SUM(ifr_time) as ifr_minutes,
    SUM(landings_day) as day_landings,
    SUM(landings_night) as night_landings
FROM flights
WHERE user_id = :'sample_user_id'
  AND date >= date_trunc('month', CURRENT_DATE - interval '12 months')
GROUP BY date_trunc('month', date)
ORDER BY date_trunc('month', date) ASC;

-- ==========================================
-- 7. Route statistics (map page)
-- ==========================================
\echo ''
\echo '=========================================='
\echo '7. Route Statistics — top routes'
\echo '=========================================='

EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT departure_icao, arrival_icao, COUNT(*) AS flight_count
FROM flights
WHERE user_id = :'sample_user_id'
  AND departure_icao IS NOT NULL
  AND arrival_icao IS NOT NULL
GROUP BY departure_icao, arrival_icao
ORDER BY flight_count DESC;

-- ==========================================
-- 8. Currency — progress by aircraft class
-- ==========================================
\echo ''
\echo '=========================================='
\echo '8. Currency — progress by aircraft class (JOIN)'
\echo '=========================================='

EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT
    COUNT(*) as flight_count,
    COALESCE(SUM(f.total_time), 0) as total_minutes,
    COALESCE(SUM(f.pic_time), 0) as pic_minutes,
    COALESCE(SUM(f.landings_day + f.landings_night), 0) as total_landings,
    COALESCE(SUM(f.landings_day), 0) as day_landings,
    COALESCE(SUM(f.landings_night), 0) as night_landings
FROM flights f
INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
WHERE f.user_id = :'sample_user_id'
  AND a.aircraft_class = 'SEP'
  AND f.date >= (CURRENT_DATE - interval '90 days');

-- ==========================================
-- 9. Stats by aircraft class (reports)
-- ==========================================
\echo ''
\echo '=========================================='
\echo '9. Stats by Aircraft Class — reports'
\echo '=========================================='

EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT
    COALESCE(a.aircraft_class, 'Unclassified') as class,
    COUNT(*) as flights,
    SUM(f.total_time) as total_minutes,
    SUM(f.pic_time) as pic_minutes,
    SUM(f.dual_time) as dual_minutes,
    SUM(f.landings_day + f.landings_night) as total_landings
FROM flights f
LEFT JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
WHERE f.user_id = :'sample_user_id'
GROUP BY COALESCE(a.aircraft_class, 'Unclassified');

-- ==========================================
-- 10. Last flight review (currency)
-- ==========================================
\echo ''
\echo '=========================================='
\echo '10. Last Flight Review — currency check'
\echo '=========================================='

EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT date
FROM flights
WHERE user_id = :'sample_user_id'
  AND is_flight_review = true
ORDER BY date DESC
LIMIT 1;

\echo ''
\echo '=========================================='
\echo 'Done — review plans for sequential scans'
\echo '=========================================='
