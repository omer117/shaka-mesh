-- Adds profile fields to existing databases (init_db.sql only runs on a fresh volume).
-- Safe to run more than once. Apply with:
--   docker compose exec -T beach-db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" < deployments/migrations/003_user_profile.sql
ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name VARCHAR(200) NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS bio TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS home_beach_id VARCHAR(36);
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url TEXT NOT NULL DEFAULT '';
