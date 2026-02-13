-- Add route, takeoff, and extended time/auto-calculated columns to flights

-- Route waypoints: comma-separated ICAO codes for VFR/IFR flight plans (e.g. "EDDF,EDDS,EDDM")
ALTER TABLE flights ADD COLUMN route TEXT;

-- Takeoffs (day/night)
ALTER TABLE flights ADD COLUMN takeoffs_day INTEGER NOT NULL DEFAULT 0 CHECK (takeoffs_day >= 0);
ALTER TABLE flights ADD COLUMN takeoffs_night INTEGER NOT NULL DEFAULT 0 CHECK (takeoffs_night >= 0);

-- Auto-calculated fields (with manual override support)
ALTER TABLE flights ADD COLUMN solo_time DECIMAL(5,2) NOT NULL DEFAULT 0 CHECK (solo_time >= 0);
ALTER TABLE flights ADD COLUMN cross_country_time DECIMAL(5,2) NOT NULL DEFAULT 0 CHECK (cross_country_time >= 0);
ALTER TABLE flights ADD COLUMN distance DECIMAL(8,2) NOT NULL DEFAULT 0 CHECK (distance >= 0);
ALTER TABLE flights ADD COLUMN all_landings INTEGER NOT NULL DEFAULT 0 CHECK (all_landings >= 0);

-- Manual override flags: when true, the server does NOT auto-calculate the field
ALTER TABLE flights ADD COLUMN takeoffs_day_override BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE flights ADD COLUMN takeoffs_night_override BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE flights ADD COLUMN landings_day_override BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE flights ADD COLUMN landings_night_override BOOLEAN NOT NULL DEFAULT false;

-- Backfill: set all_landings from existing day + night landings
UPDATE flights SET all_landings = landings_day + landings_night;
