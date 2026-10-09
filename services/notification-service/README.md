# Notification Service

Consumes swell alerts from RabbitMQ, logs them, and streams them to connected
browser clients over Server-Sent Events (SSE).

## Run locally

Start RabbitMQ from the repository root:

```powershell
docker compose -f deployments\docker-compose.yml up -d rabbitmq
```

From `services/notification-service`, configure the broker URL and start the
consumer. For Bash/Git Bash, use `export` instead of PowerShell's `$env:`
assignments.

PowerShell:

```powershell
$env:AMQP_URL = 'amqp://surfer_dev:surfer_dev_password@localhost:5672/'
go run ./cmd
```

Bash/Git Bash:

```bash
export AMQP_URL='amqp://surfer_dev:surfer_dev_password@localhost:5672/'
go run ./cmd
```

The service declares the durable `surf_events` topic exchange and durable
`notifications_swell_queue`, then binds `surf.swell.#`. Valid alerts are logged,
published to connected SSE clients, and acknowledged. Malformed messages are
logged and rejected without requeue; if RabbitMQ disconnects, the service
retries with backoff up to 30 seconds.

The browser event stream is available at `GET http://localhost:8082/events`.
It sends named `swell-alert` events containing the `SwellAlertEvent` JSON
payload. `HTTP_ADDR` defaults to `:8082`, and `CORS_ALLOWED_ORIGIN` defaults to
`http://localhost:3000`. The stream is public and does not replay events that
were emitted before a client connected. Slow subscribers may skip older alerts
so the consumer never blocks on a browser connection.

Press Ctrl+C to stop the consumer and HTTP server cleanly.

## Verify a browser alert

Open the dashboard while the notification service is running, then use the
RabbitMQ management UI at `http://localhost:15672` to publish a message to the
`surf_events` exchange with routing key `surf.swell.alert` and this JSON body:

```json
{
  "beach_id": "test-beach",
  "beach_name": "Hilton Beach",
  "wave_height": 1.8,
  "timestamp": "2026-10-09T07:00:00Z"
}
```

The notification service should log the alert and the dashboard should display
a toast. For local frontend configuration, copy `web/.env.example` to
`web/.env.local` and run the app from `web/`.

Services running inside the Compose network should use
`amqp://surfer_dev:surfer_dev_password@rabbitmq:5672/` rather than `localhost`.

## Tests

Unit tests run with `go test ./...`. To run the broker integration test as
well, set `RABBITMQ_TEST_URL`:

```bash
RABBITMQ_TEST_URL='amqp://surfer_dev:surfer_dev_password@localhost:5672/' go test -count=1 ./...
```
