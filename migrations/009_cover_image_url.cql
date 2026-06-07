-- Cover image URL column for user profiles
-- Apply with: cqlsh -f migrations/009_cover_image_url.cql

USE geoloc;

ALTER TABLE users ADD cover_image_url TEXT;
