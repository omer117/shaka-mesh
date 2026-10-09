# Auth Service

Provides account registration and JWT access-token issuance.

## Configuration

The service requires `DATABASE_URL` and `JWT_SECRET`. The signing secret must
be at least 32 bytes. `JWT_ISSUER` defaults to `shaka-mesh-auth`, `JWT_TTL`
defaults to `15m`, and `HTTP_ADDR` defaults to `:8081`.

Register and login requests are `POST /auth/register` and `POST /auth/login`.
Passwords must be 8–72 bytes; password hashes use bcrypt and are never included
in API responses. Duplicate emails return `409`; login failures return the
same generic `401` response.

When run directly, the database schema in `deployments/init_db.sql` must
already have been applied. Existing database volumes do not rerun initialization
scripts; apply `deployments/migrations/001_create_users.sql` to those databases
before starting the service.
