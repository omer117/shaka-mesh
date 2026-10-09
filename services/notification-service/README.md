# Notification Service

Consumes swell alerts from RabbitMQ and logs them as simulated real-time
notifications.

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

The consumer declares the durable `surf_events` topic exchange and durable
`notifications_swell_queue`, then binds `surf.swell.#`. Valid alerts are logged
and acknowledged. Malformed messages are logged and rejected without requeue;
if RabbitMQ disconnects, the service retries with backoff up to 30 seconds.
Press Ctrl+C to stop cleanly.

Services running inside the Compose network should use
`amqp://surfer_dev:surfer_dev_password@rabbitmq:5672/` rather than `localhost`.

## Tests

Unit tests run with `go test ./...`. To run the broker integration test as
well, set `RABBITMQ_TEST_URL`:

```bash
RABBITMQ_TEST_URL='amqp://surfer_dev:surfer_dev_password@localhost:5672/' go test -count=1 ./...
```
