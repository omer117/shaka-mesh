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

The browser CORS origin is configured with `CORS_ALLOWED_ORIGIN` (defaults to
`http://localhost:3000`). The notification service's SSE endpoint is available
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

The gateway is exposed on `http://localhost:8000`. The shared Compose network
keeps backend service ports internal.
