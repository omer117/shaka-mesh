# Beach Service

The service exposes a JSON REST API and periodically refreshes each beach's
stored swell height and wind speed from Open-Meteo. Swell is read from the
Marine API and wind from the Forecast API in knots; both are saved to the
existing `wave_height` and `wind_speed` columns.

At startup, the service idempotently seeds a curated list of major Israeli
Mediterranean surf beaches in addition to the original sample entries. These
records provide approximate break coordinates and descriptions for discovery;
they are not an exhaustive catalog of every local peak, and beach access can
change. The conditions page can request browser location permission and then
sorts the full beach list by straight-line distance, showing each distance.

## Run locally

Start the Postgres/PostGIS service from `deployments`, then run the API from
this directory. In PowerShell:

```powershell
$env:DATABASE_URL = 'postgres://<user>:<password>@localhost:5433/<database>?sslmode=disable'
$env:AMQP_URL = 'amqp://surfer_dev:surfer_dev_password@localhost:5672/'
$env:HTTP_ADDR = ':8080'
go run ./cmd
```

Replace the database URL placeholders with values from your local Postgres
configuration.

In Bash/Git Bash, use `export` instead of PowerShell's `$env:` syntax:

```bash
export DATABASE_URL='postgres://shaka_user:shaka_password@localhost:5433/shaka_beach_db?sslmode=disable'
export AMQP_URL='amqp://surfer_dev:surfer_dev_password@localhost:5672/'
export HTTP_ADDR=':8080'
go run ./cmd
```

`DATABASE_URL` and `AMQP_URL` are required. For services running in the
Compose network, use `amqp://surfer_dev:surfer_dev_password@rabbitmq:5672/`
instead of `localhost`. `HTTP_ADDR` defaults to `:8080`.
`SWELL_REFRESH_INTERVAL` accepts Go duration values such as `15m` or `1h` and
defaults to `30m`. An initial refresh runs at startup. Provider errors are
logged and do not erase previously stored values.

Beach records include wind speed, swell height and period, swell direction,
water temperature, tide height and tide phase. The bundled beach entries are
illustrative sample conditions for period, direction, temperature, and tide.
Swell height and wind speed are refreshed from Open-Meteo at startup and on
the configured interval. `GET /api/v1/beaches/{id}/live-conditions` triggers
an immediate refresh and returns the updated record.

When a successfully stored swell reading is above `1.5` meters, the service
publishes a persistent JSON `SwellAlertEvent` to the durable topic exchange
`surf_events` with routing key `surf.swell.alert`. Publishing failures are
logged by the refresh loop.

### Database integration tests

Repository integration tests run against a real PostGIS database started with
`testcontainers-go` (image `postgis/postgis:16-3.4`). They cover the
nearby-beach query (`ST_DWithin` radius filter and ordering by distance),
create/get/update and the idempotent Israel beach seed. They sit behind the
`integration` build tag, so plain `go test ./...` stays fast and needs no
Docker. To run them, start Docker and run:

```bash
go test -tags=integration ./...
```

The container is initialised with `deployments/init_db.sql`, the same file
Docker Compose mounts into Postgres.

The RabbitMQ publisher integration test can be run against the local broker:

```bash
RABBITMQ_TEST_URL='amqp://surfer_dev:surfer_dev_password@localhost:5672/' go test -count=1 ./internal/messaging -run TestRabbitMQPublisherIntegration
```

For local E2E testing only, set `E2E_ALERT_TEST_TOKEN` to a random value at
least 32 characters long before starting the beach service. This conditionally
enables `POST /api/v1/beaches/_e2e/swell-alert`, requiring both a valid gateway
JWT and a matching `X-E2E-Test-Token` header. The cloud Compose overlay always
disables this endpoint.

From Git Bash at the repository root, run the E2E audit using the same token
for the service and script:

```bash
export JWT_SECRET="$(openssl rand -hex 32)"
export E2E_ALERT_TEST_TOKEN="$(openssl rand -hex 32)"
docker compose -f deployments/docker-compose.yml up -d --build --no-deps --force-recreate beach-service
bash scripts/test-e2e.sh
unset E2E_ALERT_TEST_TOKEN
docker compose -f deployments/docker-compose.yml up -d --no-deps --force-recreate beach-service
```

The JWT secret is only needed by Compose for variable interpolation here; the
command recreates only beach-service and does not change the running auth or
gateway secret. The final command recreates beach-service without the
test-only route enabled. The script requires Docker, `curl`, Node.js, and npm;
it creates and removes a temporary test account.

## Endpoints

| Method | Path                                                      | Description              |
| ------ | --------------------------------------------------------- | ------------------------ |
| `GET`  | `/health`                                                 | Liveness response        |
| `GET`  | `/api/v1/beaches`                                         | List beaches             |
| `GET`  | `/api/v1/beaches/{id}`                                    | Get one beach            |
| `GET`  | `/api/v1/beaches/{id}/live-conditions`                    | Refresh live swell/wind  |
| `GET`  | `/api/v1/beaches/nearby?lat=32.08&lon=34.77&radius_km=10` | Find beaches by distance |
| `POST` | `/api/v1/beaches`                                         | Create a beach           |

Create requests accept `name`, `latitude`, `longitude`, and optional
`description`. IDs and timestamps are generated by the service/database.
Validation and API errors use JSON responses; invalid input returns `400`,
unknown beach IDs return `404`, and successful creation returns `201`.
