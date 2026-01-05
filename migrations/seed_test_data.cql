-- Seed data for UI testing
-- Location: Jakarta, Indonesia (-6.3653, 106.8269)
-- Geohash prefix: qqguw (5 chars ~5km precision)

USE geoloc;

-- ============== USERS (20 users) ==============
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'john_doe', 'john@example.com', 'John Doe', 'Tech enthusiast from Jakarta', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'jane_smith', 'jane@example.com', 'Jane Smith', 'Food blogger', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'alex_wong', 'alex@example.com', 'Alex Wong', 'Photographer', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'maria_garcia', 'maria@example.com', 'Maria Garcia', 'Travel lover', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'david_chen', 'david@example.com', 'David Chen', 'Coffee addict', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'sarah_lee', 'sarah@example.com', 'Sarah Lee', 'Fitness enthusiast', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'mike_brown', 'mike@example.com', 'Mike Brown', 'Music producer', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'emma_wilson', 'emma@example.com', 'Emma Wilson', 'Book worm', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'ryan_taylor', 'ryan@example.com', 'Ryan Taylor', 'Startup founder', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'lisa_anderson', 'lisa@example.com', 'Lisa Anderson', 'Art lover', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'tom_martinez', 'tom@example.com', 'Tom Martinez', 'Gaming streamer', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'nina_patel', 'nina@example.com', 'Nina Patel', 'Yoga instructor', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'chris_jackson', 'chris@example.com', 'Chris Jackson', 'Sports fan', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'amy_moore', 'amy@example.com', 'Amy Moore', 'Nature explorer', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'kevin_white', 'kevin@example.com', 'Kevin White', 'Movie critic', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'olivia_harris', 'olivia@example.com', 'Olivia Harris', 'Fashion designer', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'james_clark', 'james@example.com', 'James Clark', 'Chef', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'sophia_lewis', 'sophia@example.com', 'Sophia Lewis', 'Pet lover', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'daniel_walker', 'daniel@example.com', 'Daniel Walker', 'Tech blogger', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at) VALUES (uuid(), 'mia_robinson', 'mia@example.com', 'Mia Robinson', 'Student life', 'hashed_password', toTimestamp(now()), toTimestamp(now()));
