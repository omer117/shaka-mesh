# API Gateway

The gateway listens on port `8000` by default and routes auth requests to
`AUTH_SERVICE_URL`, beach requests to `BEACH_SERVICE_URL`, and shop/forum
requests to `COMMUNITY_SERVICE_URL` (default `http://localhost:8083`).

`JWT_SECRET` is required and must match auth-service. `JWT_ISSUER` must also
match; its default is `shaka-mesh-auth`. Upstream URLs default to local
development addresses and can be overridden for deployment.

Public reads include beach `GET` requests, `GET /api/v1/shop/products`, and
forum category/thread `GET` requests. Auth routes under `/api/v1/auth/` are
public. Mutating beach, shop, cart, and forum operations require an unexpired
JWT signed with HS256 by auth-service. Verified `user_id` and `email` claims
are added to the request context and forwarded to beach-service and
community-service as `X-Authenticated-User-ID` and
`X-Authenticated-User-Email`.

## Brute-force protection

`POST /api/v1/auth/login` and `/api/v1/auth/register` are rate-limited per
client IP and per submitted email (case-insensitive) with an in-memory token
bucket (`internal/ratelimit`, behind a `Limiter` interface so Redis can replace
it for multi-replica deployments). Exceeding a limit returns `429` with a
`Retry-After` header (seconds) and the usual CORS headers. Login and register
have separate buckets.

| Variable | Default | Meaning |
| --- | --- | --- |
| `AUTH_RATE_LIMIT_IP_BURST` / `AUTH_RATE_LIMIT_IP_PER_MINUTE` | 20 / 20 | Attempts allowed at once, then refill per minute, per IP |
| `AUTH_RATE_LIMIT_EMAIL_BURST` / `AUTH_RATE_LIMIT_EMAIL_PER_MINUTE` | 5 / 5 | Same, per email |
| `TRUSTED_PROXY_HOPS` | `0` | Reverse proxies in front of the gateway. `0` ignores `X-Forwarded-For` (anyone could forge it to dodge the limit); `1` behind Caddy takes the last entry Caddy appended. The cloud overlay sets `1`. |

Limits reset when the gateway restarts and are per replica. If you run the
gateway directly on the internet, leave `TRUSTED_PROXY_HOPS=0`.

The browser CORS origin is configured with `CORS_ALLOWED_ORIGIN` (defaults to
`http://localhost:3000`). It must be one explicit origin; `*` is rejected at
startup because the gateway answers with `Access-Control-Allow-Credentials:
true` (needed for the HttpOnly refresh cookie). Allowed methods are `GET, POST,
PATCH, DELETE, OPTIONS`. `Set-Cookie` and `Cookie` headers pass through the
`/api/v1/auth/*` proxy untouched, so `POST /auth/refresh` and `/auth/logout`
work with the cookie set by auth-service. The notification service's SSE endpoint is available
directly at `http://localhost:8082/events`.

For local full-stack startup, set a random secret of at least 32 bytes before
running Compose. For Bash/Git Bash:

```bash
export JWT_SECRET="$(openssl rand -hex 32)"
docker compose -f deployments/docker-compose.yml up --build
```

For PowerShell:

```powershell
$bytes = [byte[]]::new(32)
$rng = [Security.Cryptography.RandomNumberGenerator]::Create()
$rng.GetBytes($bytes)
$rng.Dispose()
$env:JWT_SECRET = ([BitConverter]::ToString($bytes) -replace '-', '').ToLowerInvariant()
docker compose -f deployments\docker-compose.yml up --build
```

The gateway is exposed on `http://localhost:8000`. The auth, beach and
community services are reachable only on the shared Compose network. The local
Compose file also publishes the notification service (`8082`), PostgreSQL
(`5433`) and RabbitMQ (`5672`, `15672`) to the host; the cloud overlay publishes
none of these.
