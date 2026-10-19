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
logged and rejected without requeue, which sends them to the dead-letter queue
(see below); if RabbitMQ disconnects, the service reconnects with exponential
backoff (1s doubling up to 30s).

## Message flow and failure behavior

```
beach-service --publish (persistent, confirmed)--> surf_events (topic)
    --surf.swell.#--> notifications_swell_queue --> notification-service --> SSE clients
                          | Reject(false)
                          v
            swell-alerts.dlx (direct) --> swell-alerts.dlq
```

- **Publisher (beach-service):** messages are persistent (`delivery_mode=2`) and
  the channel is in confirm mode. `PublishSwellAlert` returns success only after
  the broker acks the message, or an error after 5 seconds without a confirm or
  on a nack. If the connection or channel drops (`NotifyClose`), publishes fail
  fast with `ErrNotConnected` while a background loop reconnects with exponential
  backoff (1s doubling up to 30s). Callers are not retried automatically; a
  failed publish surfaces as an error to the caller (the ingestor logs it and the next refresh cycle tries again).
- **Consumer:** manual acks with prefetch 1. Valid alerts are acked after being
  handed to the SSE hub. Invalid JSON or invalid fields are rejected without
  requeue, so the broker dead-letters them to `swell-alerts.dlq` for inspection
  (use the management UI at `http://localhost:15672`, queue `swell-alerts.dlq`).
  On connection loss, `Run` returns and the consumer reconnects, re-declaring
  the topology. Unacked deliveries are redelivered by the broker.
- **Delivery guarantee:** at-least-once between beach-service and the consumer.
  A crash between handling and ack can cause a duplicate alert. SSE delivery to
  browsers is best-effort and not replayed.

### Migration caveat: existing queue

`notifications_swell_queue` now has `x-dead-letter-exchange` arguments. RabbitMQ
refuses to redeclare an existing durable queue with different arguments
(`PRECONDITION_FAILED`), so the consumer will fail to start against a broker that
still has the old queue. Delete it once (management UI, or
`rabbitmqctl delete_queue notifications_swell_queue`, or
`docker compose -f deployments/docker-compose.yml down -v` for local dev) and
restart the service; it is recreated with the dead-letter settings. Alerts
waiting in the old queue are lost when it is deleted.

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
