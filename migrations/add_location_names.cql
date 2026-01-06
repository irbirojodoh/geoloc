-- Enhanced location names cache table with full address details
-- Drop and recreate with more fields

USE geoloc;

DROP TABLE IF EXISTS location_names;

CREATE TABLE IF NOT EXISTS location_names (
    geohash_prefix TEXT PRIMARY KEY,
    display_name TEXT,
    name TEXT,
    -- Address details
    village TEXT,
    city_district TEXT,
    city TEXT,
    state TEXT,
    region TEXT,
    postcode TEXT,
    country TEXT,
    country_code TEXT,
    -- Coordinates
    latitude DOUBLE,
    longitude DOUBLE,
    created_at TIMESTAMP
);
