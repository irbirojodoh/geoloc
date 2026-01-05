USE geoloc;

-- 1. Insert Users
INSERT INTO users (id, username, email, full_name, created_at)
VALUES (a1b2c3d4-e5f6-7890-1234-567890abcdef, 'droggeman2', 'droggeman2@deliciousdays.com', 'Daryle Roggeman.', toTimestamp(now()));

INSERT INTO users (id, username, email, full_name, created_at)
VALUES (b2c3d4e5-f678-9012-3456-7890abcdef12, 'pbrownhill4', 'pbrownhill4@nymag.com', 'Patricia Brownhill', toTimestamp(now()));

INSERT INTO users (id, username, email, full_name, created_at)
VALUES (c3d4e5f6-7890-1234-5678-90abcdef1234, 'bdarlaston8', 'bdarlaston8@guardian.co.uk', 'Bud Darlaston', toTimestamp(now()));

-- 2. Insert Posts (linked to Seno's UUID above)
-- Note: 'id' for posts is usually a TIMEUUID. We use now() for simplicity,
-- or you can generate specific TIMEUUIDs if you need exact ordering.
INSERT INTO posts (id, user_id, content, location_lat, location_long, created_at)
VALUES (now(), a1b2c3d4-e5f6-7890-1234-567890abcdef, 'Just deployed the new Cassandra cluster! 🚀', -6.2088, 106.8456, toTimestamp(now()));

INSERT INTO posts (id, user_id, content, location_lat, location_long, created_at)
VALUES (now(), a1b2c3d4-e5f6-7890-1234-567890abcdef, 'Debugging race conditions in Go is fun.', -6.2088, 106.8456, toTimestamp(now()));

-- 3. Insert Follows (droggeman2 follows pbrownhill4)
INSERT INTO follows (follower_id, followee_id, created_at)
VALUES (a1b2c3d4-e5f6-7890-1234-567890abcdef, b2c3d4e5-f678-9012-3456-7890abcdef12, toTimestamp(now()));

-- 4. Insert Locations (for your geoloc feature)
-- Assuming you have a table for tracking user location history
INSERT INTO user_locations (user_id, geohash, timestamp)
VALUES (a1b2c3d4-e5f6-7890-1234-567890abcdef, 'qqgu', toTimestamp(now()));
