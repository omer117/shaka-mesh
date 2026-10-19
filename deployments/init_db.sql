-- 1. הפעלת הרחבת PostGIS
CREATE EXTENSION IF NOT EXISTS postgis;

-- 2. יצירת טבלת חופים
CREATE TABLE IF NOT EXISTS beaches (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    -- עמודה גיאוגרפית מיוחדת עבור חיפושי מרחק מהירים
    location GEOGRAPHY(Point, 4326),
    wave_height DOUBLE PRECISION DEFAULT 0.0,
    wind_speed DOUBLE PRECISION DEFAULT 0.0,
    wave_period_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
    swell_direction_degrees DOUBLE PRECISION NOT NULL DEFAULT 0,
    water_temperature_celsius DOUBLE PRECISION NOT NULL DEFAULT 0,
    tide_height_meters DOUBLE PRECISION NOT NULL DEFAULT 0,
    tide_phase VARCHAR(20) NOT NULL DEFAULT 'N/A',
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 3. יצירת Spatial Index כדי שחיפושי מרחק ירוצו בביצועים מעולים
CREATE INDEX IF NOT EXISTS idx_beaches_location ON beaches USING GIST (location);

-- 4. Seed Data: הכנסת חופים ראשוניים לבדיקה
INSERT INTO beaches (
    id, name, latitude, longitude, location, wind_speed, wave_period_seconds,
    swell_direction_degrees, water_temperature_celsius, tide_height_meters,
    tide_phase, description
)
VALUES 
    ('b1', 'Hilton Beach Tel Aviv', 32.0886, 34.7674, ST_SetSRID(ST_MakePoint(34.7674, 32.0886), 4326)::geography, 7.5, 7.8, 285, 24.1, 0.42, 'Rising', 'Exposed reef break with a fast takeoff; best on a clean west swell.'),
    ('b2', 'Metzitzim Beach', 32.0935, 34.7712, ST_SetSRID(ST_MakePoint(34.7712, 32.0935), 4326)::geography, 6.2, 6.9, 290, 24.0, 0.38, 'Rising', 'Sheltered sandy bay with a mellow longboard peak and easy beach access.'),
    ('b3', 'Maridian Beach Haifa', 32.8122, 34.9580, ST_SetSRID(ST_MakePoint(34.9580, 32.8122), 4326)::geography, 9.1, 8.4, 305, 23.2, 0.51, 'Falling', 'Open northern coast beach that handles longer-period winter swell.')
ON CONFLICT (id) DO NOTHING;

-- 5. User accounts for auth-service
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) PRIMARY KEY,
    email VARCHAR(320) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 6. Refresh tokens for auth-service (hashed; rotation + reuse detection)
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    family_id VARCHAR(36) NOT NULL,
    token_hash CHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    revoked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family ON refresh_tokens (family_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires ON refresh_tokens (expires_at);

-- 7. User profile columns (display name, bio, home beach, avatar)
-- home_beach_id is validated against beach-service by auth-service, so there
-- is deliberately no foreign key to keep the services decoupled.
ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name VARCHAR(200) NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS bio TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS home_beach_id VARCHAR(36);
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url TEXT NOT NULL DEFAULT '';
