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
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 3. יצירת Spatial Index כדי שחיפושי מרחק ירוצו בביצועים מעולים
CREATE INDEX IF NOT EXISTS idx_beaches_location ON beaches USING GIST (location);

-- 4. Seed Data: הכנסת חופים ראשוניים לבדיקה
INSERT INTO beaches (id, name, latitude, longitude, location, description)
VALUES 
    ('b1', 'Hilton Beach Tel Aviv', 32.0886, 34.7674, ST_SetSRID(ST_MakePoint(34.7674, 32.0886), 4326)::geography, 'Top surf spot with solid reef break'),
    ('b2', 'Metzitzim Beach', 32.0935, 34.7712, ST_SetSRID(ST_MakePoint(34.7712, 32.0935), 4326)::geography, 'Protected bay, good for longboarding'),
    ('b3', 'Maridian Beach Haifa', 32.8122, 34.9580, ST_SetSRID(ST_MakePoint(34.9580, 32.8122), 4326)::geography, 'Popular surf beach in northern Israel')
ON CONFLICT (id) DO NOTHING;

-- 5. User accounts for auth-service
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) PRIMARY KEY,
    email VARCHAR(320) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);