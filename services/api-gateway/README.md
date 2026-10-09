# API Gateway

The gateway listens on port `8000` by default and routes auth requests to
`AUTH_SERVICE_URL` and beach requests to `BEACH_SERVICE_URL`.

`JWT_SECRET` is required and must match auth-service. `JWT_ISSUER` must also
match; its default is `shaka-mesh-auth`. Upstream URLs default to local
development addresses and can be overridden for deployment.

`GET /api/v1/beaches` and other beach `GET` requests are public. Auth routes
under `/api/v1/auth/` are public. Other beach methods require an unexpired
Bearer token signed with HS256 by auth-service. Verified `user_id` and `email`
claims are added to the request context and forwarded to beach-service as
`X-Authenticated-User-ID` and `X-Authenticated-User-Email`.

For local full-stack startup, set a random secret of at least 32 bytes before
running Compose. For Bash/Git Bash:

```bash
export JWT_SECRET="$(openssl rand -hex 32)"
docker compose -f deployments/docker-compose.yml up --build
```

For PowerShell:

```powershell
$bytes = [byte[]]::new(32)
[Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
$env:JWT_SECRET = [Convert]::ToHexString($bytes)
docker compose -f deployments\docker-compose.yml up --build
```

The gateway is exposed on `http://localhost:8000`. The shared Compose network
keeps backend service ports internal.
