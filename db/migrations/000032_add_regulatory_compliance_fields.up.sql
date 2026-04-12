-- Phase 6c: EASA/FAA Regulatory Compliance fields
-- Adds pic_name, multi_pilot_time, fstd_type, approaches (JSONB), endorsements

-- PIC Name (EASA AMC1 FCL.050 Col 12)
ALTER TABLE flights ADD COLUMN pic_name VARCHAR(255);

-- Multi-Pilot Time (EASA AMC1 FCL.050 Col 10) — time in minutes
ALTER TABLE flights ADD COLUMN multi_pilot_time INTEGER NOT NULL DEFAULT 0;

-- FSTD type designation (EASA AMC1 FCL.050 Col 22, FAA §61.51(b)(1)(iv))
ALTER TABLE flights ADD COLUMN fstd_type VARCHAR(50);

-- Structured instrument approaches (FAA §61.51(g)(3))
-- Array of objects: [{"type": "ILS", "runway": "09L", "airport": "EDDF"}]
ALTER TABLE flights ADD COLUMN approaches JSONB NOT NULL DEFAULT '[]';

-- Endorsements (EASA AMC1 FCL.050 Col 24, FAA §61.51(h))
-- Separate from pilot remarks — instructor endorsements, skill test references
ALTER TABLE flights ADD COLUMN endorsements TEXT;

-- Migrate existing approaches_count data to approaches JSONB
-- Convert integer count to array of {"type": "Unknown"} entries
UPDATE flights
SET approaches = (
    SELECT COALESCE(
        jsonb_agg(jsonb_build_object('type', 'Unknown')),
        '[]'::jsonb
    )
    FROM generate_series(1, GREATEST(approaches_count, 0))
)
WHERE approaches_count > 0;
