# Free remote deployment

This layout runs the Go services, PostgreSQL/PostGIS, and RabbitMQ together on
an Oracle Cloud Always Free VM. Vercel serves the Next.js frontend. Caddy is the
only public container: it obtains and renews HTTPS certificates and routes
`/events` to the notification service and all other requests to the API gateway.
The database and broker ports are not published in the cloud Compose overlay.

Oracle's Always Free compute quota is limited, capacity is not guaranteed in
every region, and idle instances can be reclaimed. Oracle account creation
requires identity and payment-card verification. Vercel's free Hobby tier is
intended for personal, non-commercial use. Review both providers' current terms
and limits before relying on the deployment.

## 1. Create the VM

Create an Ubuntu or Oracle Linux ARM VM in the Oracle account's home region,
within the Always Free Ampere A1 quota. Assign a reserved public IPv4 address.
In the VM's network security rules, allow TCP 22 only from your own address,
and TCP 80 and 443 from the internet. UDP 443 is optional (HTTP/3). Do not open
the database, RabbitMQ, or application ports.

Install Docker Engine and the Docker Compose plugin using the official
instructions for the selected Linux distribution. Ensure the operating
system firewall also allows TCP 80 and 443.

## 2. Prepare the project and configuration

After the feature branches are available on GitHub, clone the root repository
and check out the branch containing this deployment overlay. Create the
deployment environment file:

```sh
cd shaka-mesh
cp deployments/.env.example deployments/.env
```

Replace every placeholder in `deployments/.env`. Generate long random
hexadecimal values for `POSTGRES_PASSWORD`, `RABBITMQ_DEFAULT_PASS`, and
`JWT_SECRET` (at least 64 hex characters for the JWT secret). Keep this file
private and never commit it.

Set `PUBLIC_HOSTNAME` to `<reserved-public-ip>.nip.io`; this hostname resolves
to the VM and allows Caddy to request a publicly trusted HTTPS certificate.
After deploying the frontend, set `CORS_ALLOWED_ORIGIN` to its exact HTTPS
Vercel origin, with no trailing slash.

> The credentials in `.env` must remain unchanged when reusing the database
> volume. PostgreSQL only applies its initialization credentials to an empty
> data directory.

## 3. Start the backend

From the repository root on the VM:

```sh
docker compose \
  --env-file deployments/.env \
  -f deployments/docker-compose.yml \
  -f deployments/docker-compose.cloud.yml \
  up -d --build
```

Check service health with `docker compose ... ps`. Once Caddy has obtained its
certificate, `https://<reserved-public-ip>.nip.io/health` should return the
gateway health response.

## 4. Deploy the frontend to Vercel

From `web/`, link the Vercel CLI to your Vercel account and create a project.
Set these project environment variables for the Production environment before
deploying:

```text
NEXT_PUBLIC_API_BASE_URL=https://<reserved-public-ip>.nip.io
NEXT_PUBLIC_NOTIFICATION_BASE_URL=https://<reserved-public-ip>.nip.io
```

Deploy with the Vercel CLI from the `web/` directory. Use the resulting
production origin (for example, `https://your-project.vercel.app`) as
`CORS_ALLOWED_ORIGIN` on the VM, then restart the backend stack so both the
gateway and notification service apply it.

## Local Compose safety

The cloud overlay changes only the cloud deployment. The regular
`docker-compose.yml` remains the local development setup.
