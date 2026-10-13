#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

API_BASE_URL="${API_BASE_URL:-http://localhost:8000}"
NOTIFICATION_BASE_URL="${NOTIFICATION_BASE_URL:-http://localhost:8082}"
RABBITMQ_MANAGEMENT_URL="${RABBITMQ_MANAGEMENT_URL:-http://localhost:15672}"
RABBITMQ_USER="${RABBITMQ_USER:-surfer_dev}"
RABBITMQ_PASSWORD="${RABBITMQ_PASSWORD:-surfer_dev_password}"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-shaka-beach-db}"
POSTGRES_USER="${POSTGRES_USER:-shaka_user}"
POSTGRES_DB="${POSTGRES_DB:-shaka_beach_db}"
NOTIFICATION_CONTAINER="${NOTIFICATION_CONTAINER:-deployments-notification-service-1}"
E2E_ALERT_TEST_TOKEN="${E2E_ALERT_TEST_TOKEN:-}"

FAILURES=0
TEST_EMAIL="shaka-e2e-$(date -u +%Y%m%dt%H%M%S)-$$@example.test"
TEST_PASSWORD="E2E-$(date +%s)-$$-Pass!"
TEST_USER_CREATED=0
SSE_PID=""
TMP_DIR="$(mktemp -d)"

cleanup() {
  if [[ -n "$SSE_PID" ]]; then
    kill "$SSE_PID" 2>/dev/null || true
    wait "$SSE_PID" 2>/dev/null || true
  fi
  if [[ "$TEST_USER_CREATED" -eq 1 ]]; then
    docker exec "$POSTGRES_CONTAINER" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 \
      -c "DELETE FROM users WHERE email = '$TEST_EMAIL'" >/dev/null 2>&1 || \
      echo "WARN: could not remove E2E user $TEST_EMAIL"
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

pass() {
  printf 'PASS: %s\n' "$1"
}

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  FAILURES=$((FAILURES + 1))
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'Missing required command: %s\n' "$1" >&2
    exit 2
  fi
}

http_request() {
  local output_file="$1"
  shift
  HTTP_STATUS="$(curl --silent --show-error --max-time 20 \
    --output "$output_file" --write-out '%{http_code}' "$@")"
}

db_query() {
  docker exec "$POSTGRES_CONTAINER" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 \
    -X -qAt -c "$1"
}

json_assert() {
  node -e '
const fs = require("node:fs");
const [file, rule, ...args] = process.argv.slice(1);
const value = JSON.parse(fs.readFileSync(file, "utf8"));
const isNumber = (item) => typeof item === "number" && Number.isFinite(item);
let valid = false;

switch (rule) {
  case "status-ok":
    valid = value.status === "ok";
    break;
  case "published":
    valid = value.status === "published";
    break;
  case "registration":
    valid = typeof value.id === "string" && value.email === args[0];
    break;
  case "access-token":
    valid = typeof value.access_token === "string" && value.access_token.length > 0;
    if (valid) process.stdout.write(value.access_token);
    break;
  case "beaches":
    valid = Array.isArray(value) && value.length > 0 && value.every((beach) =>
      typeof beach.id === "string" &&
      typeof beach.name === "string" &&
      isNumber(beach.latitude) &&
      isNumber(beach.longitude) &&
      isNumber(beach.wave_height) &&
      isNumber(beach.wind_speed)
    );
    break;
  case "non-empty-array":
    valid = Array.isArray(value) && value.length > 0;
    break;
  case "array":
    valid = Array.isArray(value);
    break;
  case "ids":
    valid = Array.isArray(value);
    if (valid) process.stdout.write(value.map((item) => item.id).join("\n"));
    break;
  case "conditions":
    valid = isNumber(value.wave_height) && isNumber(value.wind_speed);
    break;
  case "beach-detail":
    valid = isNumber(value.wave_height) && value.wave_height >= 0 &&
      isNumber(value.wind_speed) && value.wind_speed >= 0 &&
      typeof value.updated_at === "string";
    break;
  case "rabbit-overview":
    valid = typeof value.rabbitmq_version === "string" &&
      typeof value.object_totals === "object" && value.object_totals !== null;
    break;
  default:
    throw new Error(`Unknown JSON assertion: ${rule}`);
}

if (!valid) process.exit(1);
' "$@"
}

json_credentials() {
  node -e 'process.stdout.write(JSON.stringify({email: process.argv[1], password: process.argv[2]}))' \
    "$TEST_EMAIL" "$TEST_PASSWORD"
}

for command in curl docker node npm; do
  require_command "$command"
done
if [[ "${#E2E_ALERT_TEST_TOKEN}" -lt 32 ]]; then
  printf 'Set E2E_ALERT_TEST_TOKEN to the same random value configured on beach-service (at least 32 characters).\n' >&2
  exit 2
fi

printf '\n== 1. Infrastructure health ==\n'
if DB_RESULT="$(db_query "SELECT PostGIS_Version(), to_regclass('public.beaches'), to_regclass('public.idx_beaches_location');" 2>"$TMP_DIR/postgres.err")" &&
  [[ "$DB_RESULT" == *"|beaches|idx_beaches_location" ]]; then
  pass "PostgreSQL responds; PostGIS, beaches table, and GiST index exist"
else
  fail "PostgreSQL/PostGIS schema check failed: $(cat "$TMP_DIR/postgres.err" 2>/dev/null || printf '%s' "$DB_RESULT")"
fi

if EXPLAIN_RESULT="$(db_query "
  SET enable_seqscan = off;
  EXPLAIN SELECT id FROM beaches
  WHERE ST_DWithin(location,
    ST_SetSRID(ST_MakePoint(34.7674, 32.0886), 4326)::geography,
    15000);
" 2>"$TMP_DIR/spatial.err")" &&
  grep -q 'idx_beaches_location' <<<"$EXPLAIN_RESULT"; then
  pass "PostGIS radius query plan uses idx_beaches_location"
else
  fail "PostGIS spatial query did not use the GiST index: $(cat "$TMP_DIR/spatial.err" 2>/dev/null || printf '%s' "$EXPLAIN_RESULT")"
fi

if http_request "$TMP_DIR/rabbit-overview.json" \
  --user "$RABBITMQ_USER:$RABBITMQ_PASSWORD" \
  "$RABBITMQ_MANAGEMENT_URL/api/overview" &&
  [[ "$HTTP_STATUS" == "200" ]] &&
  json_assert "$TMP_DIR/rabbit-overview.json" rabbit-overview; then
  pass "RabbitMQ Management API is healthy"
else
  fail "RabbitMQ Management API /api/overview did not return a valid overview (HTTP ${HTTP_STATUS:-no response})"
fi

printf '\n== 2. Authentication and gateway ==\n'
if http_request "$TMP_DIR/gateway-health.json" "$API_BASE_URL/health" &&
  [[ "$HTTP_STATUS" == "200" ]] &&
  json_assert "$TMP_DIR/gateway-health.json" status-ok; then
  pass "API gateway health endpoint"
else
  fail "API gateway health check failed (HTTP ${HTTP_STATUS:-no response})"
fi

if http_request "$TMP_DIR/register.json" \
  --header 'Content-Type: application/json' \
  --data "$(json_credentials)" \
  "$API_BASE_URL/api/v1/auth/register" &&
  [[ "$HTTP_STATUS" == "201" ]]; then
  TEST_USER_CREATED=1
  if json_assert "$TMP_DIR/register.json" registration "$TEST_EMAIL"; then
    pass "Unique E2E user registration returns 201"
  else
    fail "Registration returned 201 but its response did not match the user contract: $(cat "$TMP_DIR/register.json")"
  fi
else
  fail "E2E user registration failed (HTTP ${HTTP_STATUS:-no response}): $(cat "$TMP_DIR/register.json" 2>/dev/null || true)"
fi

ACCESS_TOKEN=""
if [[ "$TEST_USER_CREATED" -eq 1 ]] &&
  http_request "$TMP_DIR/login.json" \
    --header 'Content-Type: application/json' \
    --data "$(json_credentials)" \
    "$API_BASE_URL/api/v1/auth/login" &&
  [[ "$HTTP_STATUS" == "200" ]] &&
  ACCESS_TOKEN="$(json_assert "$TMP_DIR/login.json" access-token)" &&
  [[ "$ACCESS_TOKEN" =~ ^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$ ]]; then
  pass "Login returns a three-part JWT access token"
else
  fail "Login did not return a valid JWT-shaped access token (HTTP ${HTTP_STATUS:-no response})"
fi

if http_request "$TMP_DIR/unauthorized.json" \
  --header 'Content-Type: application/json' \
  --data '{}' \
  "$API_BASE_URL/api/v1/beaches" &&
  [[ "$HTTP_STATUS" == "401" ]]; then
  pass "Protected beach creation is rejected without a token (401)"
else
  fail "Protected beach creation without a token returned HTTP ${HTTP_STATUS:-no response}, expected 401"
fi

if [[ -n "$ACCESS_TOKEN" ]]; then
  if http_request "$TMP_DIR/authorized-validation.json" \
    --header 'Content-Type: application/json' \
    --header "Authorization: Bearer $ACCESS_TOKEN" \
    --data '{}' \
    "$API_BASE_URL/api/v1/beaches" &&
    [[ "$HTTP_STATUS" == "400" ]]; then
    pass "JWT is accepted by the gateway and request reaches beach validation (400)"
  else
    fail "Valid JWT was not accepted through the gateway (HTTP ${HTTP_STATUS:-no response}, expected downstream 400)"
  fi
fi

printf '\n== 3. Beach, PostGIS, and swell data ==\n'
if http_request "$TMP_DIR/beaches.json" "$API_BASE_URL/api/v1/beaches" &&
  [[ "$HTTP_STATUS" == "200" ]] &&
  json_assert "$TMP_DIR/beaches.json" beaches; then
  pass "Beach list returns a non-empty array with required coordinate and condition fields"
else
  fail "Beach list/schema check failed (HTTP ${HTTP_STATUS:-no response})"
fi

if http_request "$TMP_DIR/nearby.json" \
  "$API_BASE_URL/api/v1/beaches/nearby?lat=32.0886&lon=34.7674&radius=15" &&
  [[ "$HTTP_STATUS" == "200" ]] &&
  json_assert "$TMP_DIR/nearby.json" non-empty-array; then
  API_NEARBY_IDS="$(json_assert "$TMP_DIR/nearby.json" ids)"
  DB_NEARBY_IDS="$(db_query "
    SELECT id FROM beaches
    WHERE ST_DWithin(location,
      ST_SetSRID(ST_MakePoint(34.7674, 32.0886), 4326)::geography,
      15000)
    ORDER BY ST_Distance(
      location,
      ST_SetSRID(ST_MakePoint(34.7674, 32.0886), 4326)::geography
    );
  ")"
  if [[ "$API_NEARBY_IDS" == "$DB_NEARBY_IDS" ]]; then
    pass "Nearby API uses 15 km PostGIS filtering and returns DB distance order"
  else
    fail "Nearby API result order differs from PostGIS distance order"
  fi
else
  fail "Nearby beach query failed (HTTP ${HTTP_STATUS:-no response}); API currently expects radius_km"
fi

LIVE_BEACH_ID="$(node -e 'const beaches=JSON.parse(require("node:fs").readFileSync(process.argv[1],"utf8"));process.stdout.write(beaches[0]?.id ?? "")' "$TMP_DIR/beaches.json" 2>/dev/null || true)"
if [[ -n "$LIVE_BEACH_ID" ]]; then
  if http_request "$TMP_DIR/live-conditions.json" \
    "$API_BASE_URL/api/v1/beaches/$LIVE_BEACH_ID/live-conditions" &&
    [[ "$HTTP_STATUS" == "200" ]]; then
    if json_assert "$TMP_DIR/live-conditions.json" conditions; then
      LIVE_WAVE_HEIGHT="$(node -e 'process.stdout.write(String(JSON.parse(require("node:fs").readFileSync(process.argv[1],"utf8")).wave_height))' "$TMP_DIR/live-conditions.json")"
      LIVE_WIND_SPEED="$(node -e 'process.stdout.write(String(JSON.parse(require("node:fs").readFileSync(process.argv[1],"utf8")).wind_speed))' "$TMP_DIR/live-conditions.json")"
      DB_LIVE_CONDITIONS="$(db_query "SELECT wave_height, wind_speed FROM beaches WHERE id = '$LIVE_BEACH_ID';")"
      if node -e '
const [expectedWave, expectedWind, actual] = process.argv.slice(1);
const [actualWave, actualWind] = actual.split("|").map(Number);
if (Math.abs(Number(expectedWave) - actualWave) > 0.001 ||
    Math.abs(Number(expectedWind) - actualWind) > 0.001) process.exit(1);
' "$LIVE_WAVE_HEIGHT" "$LIVE_WIND_SPEED" "$DB_LIVE_CONDITIONS"; then
        pass "Live-conditions endpoint refreshed and persisted swell and wind to PostgreSQL"
      else
        fail "Live-conditions response does not match persisted PostgreSQL values"
      fi
    else
      fail "Live-conditions endpoint response is missing numeric wave_height/wind_speed fields"
    fi
  else
    fail "Live-conditions refresh failed (HTTP ${HTTP_STATUS:-no response}); check Open-Meteo access"
  fi

  if http_request "$TMP_DIR/beach-detail.json" \
    "$API_BASE_URL/api/v1/beaches/$LIVE_BEACH_ID" &&
    [[ "$HTTP_STATUS" == "200" ]] &&
    json_assert "$TMP_DIR/beach-detail.json" beach-detail; then
    pass "Existing beach detail exposes persisted swell/wind fields"
  else
    fail "Beach detail failed its persisted condition-field checks (HTTP ${HTTP_STATUS:-no response})"
  fi
fi

printf '\n== 4. Community, RabbitMQ event, and notification ==\n'
if http_request "$TMP_DIR/forum-categories.json" \
  "$API_BASE_URL/api/v1/forum/categories" &&
  [[ "$HTTP_STATUS" == "200" ]] &&
  json_assert "$TMP_DIR/forum-categories.json" array; then
  pass "Community service forum categories through the gateway"
else
  fail "Community service check failed (HTTP ${HTTP_STATUS:-no response})"
fi

if http_request "$TMP_DIR/notification-health.txt" "$NOTIFICATION_BASE_URL/health" &&
  [[ "$HTTP_STATUS" == "200" ]]; then
  pass "Notification service health endpoint"
else
  fail "Notification service health check failed (HTTP ${HTTP_STATUS:-no response})"
fi

EVENT_ID="shaka-e2e-$(date -u +%Y%m%dT%H%M%S)-$$"
SSE_OUTPUT="$TMP_DIR/notifications.sse"
curl --silent --show-error --no-buffer --max-time 25 \
  "$NOTIFICATION_BASE_URL/events" >"$SSE_OUTPUT" 2>&1 &
SSE_PID=$!
sleep 1

PUBLISH_REQUEST="$(node -e 'process.stdout.write(JSON.stringify({beach_id:process.argv[1],beach_name:"Shaka E2E Test",wave_height:2.1}))' "$EVENT_ID")"

if http_request "$TMP_DIR/publish.json" \
  --header 'Content-Type: application/json' \
  --header "Authorization: Bearer $ACCESS_TOKEN" \
  --header "X-E2E-Test-Token: $E2E_ALERT_TEST_TOKEN" \
  --data "$PUBLISH_REQUEST" \
  "$API_BASE_URL/api/v1/beaches/_e2e/swell-alert" &&
  [[ "$HTTP_STATUS" == "202" ]] &&
  json_assert "$TMP_DIR/publish.json" published; then
  pass "Beach service published a >1.5 m synthetic test alert to RabbitMQ"
else
  fail "Beach service E2E alert trigger failed (HTTP ${HTTP_STATUS:-no response}); ensure E2E_ALERT_TEST_TOKEN is configured"
fi

EVENT_RECEIVED=0
for _ in $(seq 1 40); do
  if grep -Fq "$EVENT_ID" "$SSE_OUTPUT"; then
    EVENT_RECEIVED=1
    break
  fi
  if ! kill -0 "$SSE_PID" 2>/dev/null; then
    break
  fi
  sleep 0.5
done
if [[ "$EVENT_RECEIVED" -eq 1 ]]; then
  pass "Notification consumer consumed the RabbitMQ event and broadcast it over SSE"
else
  fail "Swell alert did not arrive on the notification SSE stream"
fi
kill "$SSE_PID" 2>/dev/null || true
wait "$SSE_PID" 2>/dev/null || true
SSE_PID=""

if docker logs --since 5m "$NOTIFICATION_CONTAINER" 2>&1 | grep -Fq "$EVENT_ID"; then
  pass "Notification service logged consumption of the test event"
else
  fail "Notification service log does not contain the test event ID; check NOTIFICATION_CONTAINER"
fi


printf '\n== 5. Frontend production build ==\n'
if (cd "$ROOT_DIR/web" && npm run build); then
  pass "Next.js production build"
else
  fail "Next.js production build failed"
fi

printf '\n'
if [[ "$FAILURES" -gt 0 ]]; then
  printf 'E2E audit finished with %d failure(s). Resolve the reported gaps before release.\n' "$FAILURES" >&2
  exit 1
fi
printf 'E2E audit passed: all system checks and the production build are green.\n'
