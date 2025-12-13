-- Enable PostGIS extension
CREATE EXTENSION IF NOT EXISTS postgis;

-- Create posts table with geospatial support
CREATE TABLE IF NOT EXISTS posts (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    content TEXT NOT NULL,
    location GEOGRAPHY(POINT, 4326) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create spatial index on location column for fast nearest neighbor queries
CREATE INDEX idx_posts_location ON posts USING GIST (location);

-- Create index on created_at for temporal queries
CREATE INDEX idx_posts_created_at ON posts (created_at DESC);

-- Create index on user_id for user-specific queries
CREATE INDEX idx_posts_user_id ON posts (user_id);
