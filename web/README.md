# Shaka Web

The Next.js client has three primary areas:

- `/` is the surf-gear storefront with a database-backed catalog, signed-in
  shopping bag and simulated checkout.
- `/forum` lists persisted forum categories and discussions. Signed-in users can
  start a topic and reply.
- `/conditions` is the live beach conditions map and nearby-beach finder.

## Run locally

Start the API, database, and notification service using the root Compose
instructions. From this directory, copy `.env.example` to `.env.local` if the
local service URLs need to be changed, then run:

```powershell
npm.cmd install
npm.cmd run dev
```

The default API gateway URL is `http://localhost:8000`, and the live swell
event stream defaults to `http://localhost:8082/events`. `NEXT_PUBLIC_API_BASE_URL`
and `NEXT_PUBLIC_NOTIFICATION_BASE_URL` can override these defaults.

Create an account or sign in before adding items to the bag or posting in the
forum. The demo checkout creates a saved order and updates inventory, but does
not collect payment.

## Validate

```powershell
npm.cmd run lint
npx.cmd tsc --noEmit
npm.cmd run build
```
