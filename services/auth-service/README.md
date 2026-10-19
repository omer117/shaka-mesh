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

## Tests

Run `go test ./...` and `go vet ./...` from this directory.

### Integration tests

Repository integration tests run against a real PostGIS database started with
`testcontainers-go` (image `postgis/postgis:16-3.4`). They cover user creation,
the duplicate-email conflict (`ErrEmailInUse`, including a concurrent
registration race) and lookup by email. They sit behind the `integration` build
tag, so plain `go test ./...` stays fast and needs no Docker. To run them, start
Docker and run:

```bash
go test -tags=integration ./...
```

The container is initialised with `deployments/init_db.sql`, the same file
Docker Compose mounts into Postgres.
