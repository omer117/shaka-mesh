# Prompt for the coding agent: Shaka-Mesh

You are working in `github.com/omer117/shaka-mesh` (a monorepo with 5 Go microservices in `services/`, a Next.js 16 + React 19 + Tailwind 4 client in `web/`, and Docker Compose + Caddy in `deployments/`). Read `ARCHITECTURE.md`, every `services/*/README.md`, `deployments/REMOTE_DEPLOYMENT.md` and `web/AGENTS.md` before you change anything. `web/AGENTS.md` says that this Next.js version differs from what you know: read the guide in `node_modules/next/dist/docs/` before you write web code.

## Ground rules

- **One branch and one PR per task**, created from an up-to-date `main`. Use the branch names given below. Never commit directly to `main`. Keep PRs small: if a task grows past roughly 600 changed lines (excluding lockfiles and images), split it into stacked PRs.
- **Commits:** Conventional Commits (`feat(auth): …`, `fix(beach): …`, `docs: …`, `test: …`), each one building and passing tests.
- **Do not touch CI/CD.** Do not add or change `.github/workflows`, a `Jenkinsfile`, or any pipeline config. The repo owner is building the CI/CD himself. If a task would benefit from CI, write the commands it should run in the PR description instead.
- **Keep the existing architecture:** Clean/Hexagonal layout per service (`domain`, `repository`, `service`, `handler`), standard library `net/http` routing, `pgx`, `amqp091-go`. Do not add a framework (Gin, Echo, etc.) or an ORM.
- **Tests are part of every task.** Unit tests for new logic, and table-driven tests where it fits. `go vet ./...` and `go test ./...` must pass in every service you touch. `npm run lint` and `npm run build` must pass in `web/`.
- **Secrets:** never commit real secrets. New config goes through environment variables, with placeholders in `deployments/.env.example` and wiring in both `docker-compose.yml` and `docker-compose.cloud.yml`.
- **Docs stay true.** If a change makes any `.md` file wrong, fix that file in the same PR.
- **Every PR description:** what changed, why, how to test it locally (exact commands), and screenshots for UI changes.
- **Teach as you go.** The owner is learning this codebase. In each PR description, add a short "How it works" section (5-10 lines) that explains the key design decision and one trade-off, in plain English.
- Order: do the tasks in the order below. Each one is independent, but later UI tasks assume the new navbar.

---

## Task 1: Fix the docs to match the code
Branch: `docs/architecture-sync`

`ARCHITECTURE.md` describes things that do not exist: gRPC between services, refresh tokens, Argon2, a separate Store & Orders service, and an `OrderPlaced` event. In the code, services talk only over HTTP, auth uses bcrypt with short-lived JWTs only, the shop lives inside `community-service`, and checkout publishes no event. Rewrite `ARCHITECTURE.md` so that it describes the current system accurately: correct the diagram, the service list and the ports. Move planned work to a clearly labeled "Roadmap" section. Keep the Hebrew and English mix the file already uses.

## Task 2: Message reliability (RabbitMQ)
Branch: `feat/messaging-reliability`

- **Publisher (`beach-service/internal/messaging`):** enable publisher confirms and wait for the confirm, with a timeout, before reporting success. Detect a lost connection or channel (`NotifyClose`) and reconnect with exponential backoff. Publish messages as persistent.
- **Consumer (`notification-service/internal/consumer`):** declare a dead-letter exchange and queue (for example `swell-alerts.dlx` / `swell-alerts.dlq`) and bind the main queue to it, so that `Reject(false)` sends poison messages to the DLQ instead of dropping them. Reconnect on connection loss.
- Add unit tests for the reconnect and confirm logic, using interfaces and fakes like the existing consumer tests do. Document the message flow and the failure behavior in `services/notification-service/README.md`.

## Task 3: Integration tests against a real database
Branch: `test/integration-postgres`

Add integration tests for the repository layers (`beach-service`, `auth-service`, `community-service`) using `testcontainers-go` with the `postgis/postgis:16-3.4` image. Cover at least: the nearby-beach query (`ST_DWithin` and ordering by distance), user creation with a duplicate-email conflict, and the checkout transaction: stock decreases, the order is created and the cart is emptied. Also cover the concurrency case where two checkouts compete for the last unit, and make sure stock never goes negative; if it can, fix the query (`SELECT … FOR UPDATE` or a conditional `UPDATE`). Put the tests behind a build tag (`//go:build integration`) so that plain `go test ./...` stays fast, and document `go test -tags=integration ./...` in each service README.

## Task 4: Stronger authentication
Branch: `feat/auth-hardening` (split into stacked PRs if large: `feat/auth-refresh-tokens`, `feat/auth-rate-limit`, `feat/auth-rs256`)

1. **Refresh tokens with rotation:** a short-lived access token (keep 15 minutes) plus an opaque refresh token. Store the refresh token hashed in Postgres with an expiry, user id, family id and a revoked flag. On refresh, rotate the token and revoke the old one. If an already-used refresh token is presented again, revoke the whole family (reuse detection). Add `POST /auth/refresh` and `POST /auth/logout`.
2. **Token transport in the browser:** stop keeping tokens in `sessionStorage`. Send the refresh token as an `HttpOnly; Secure; SameSite=Lax` cookie scoped to the auth path, and keep the access token in memory only. Update `web/lib/use-auth-session.ts` to refresh silently. Remove the duplicated session logic in `web/app/conditions/page.tsx` so that every page uses the hook. Check that CORS (`credentials`) still works with the Vercel origin.
3. **Asymmetric signing:** move from HS256 with a shared secret to RS256 (or EdDSA). `auth-service` signs and serves a JWKS at `/.well-known/jwks.json`, and `api-gateway` verifies with the public key only. Support key rotation through a `kid` header.
4. **Brute-force protection:** rate-limit `POST /auth/login` and `POST /auth/register` in `api-gateway`, per IP and per email (token bucket, in memory is fine, behind an interface so that Redis can replace it later). Return `429` with a `Retry-After` header.
5. **Password policy:** a minimum length of 10 and a check against a small embedded list of common passwords. Return clear validation errors.
6. Tests for every point above. Update `services/auth-service/README.md` with a sequence diagram (Mermaid) of login → refresh → logout.

Out of scope unless trivial: email verification and OAuth providers. List them as follow-ups in the PR description.

## Task 5: Minimal, polished navbar
Branch: `feat/web-navbar`

Replace `web/components/site-header.tsx` with a minimal, modern navbar:
- logo or wordmark on the left, links (Shop, Surf conditions, Community) in the center or on the right, and a cart icon with a count badge;
- an avatar button with a dropdown (Profile, Sign out) when signed in, and a "Sign in" button when signed out;
- transparent over the hero, turning solid with a subtle blur once the page scrolls; sticky;
- a hamburger menu with a slide-in panel on mobile;
- a visible active link, a keyboard-accessible dropdown, `aria-*` attributes, and focus rings.

Use the existing `lucide-react` icons. No new UI library.

## Task 6: Redesign with surf imagery
Branch: `feat/web-redesign`

- Build a small design system in Tailwind 4 (`@theme` tokens): an ocean and sand palette, a display font and a body font through `next/font`, and spacing and radius scales. Support light and dark mode.
- Give each page (Shop, Conditions, Community) a full-width hero with a surf photo, plus surf imagery in section backgrounds where it helps. Use **only images whose license allows free use without attribution problems**, such as Unsplash or Pexels. Download them into `web/public/images/`, optimize them (WebP, at most about 300 KB each), serve them through `next/image` with `alt` text, and record each image's source URL and license in `web/public/images/CREDITS.md`. Do not hotlink.
- Make the cards, forms, dialogs and the map container consistent with the new tokens. Keep the existing functionality working.
- Check the layout at 375 px, 768 px and 1440 px, and attach before and after screenshots to the PR.

## Task 7: User profile with photo upload
Branch: `feat/user-profile`

- **Backend:** add profile fields to `auth-service` (or a new `profile` module inside it, following the same layering): display name, bio, home beach (id from beach-service) and `avatar_url`. Endpoints: `GET /me`, `PATCH /me` and `POST /me/avatar` (multipart), all authenticated through the gateway.
- **Upload handling:** accept JPEG, PNG and WebP up to 2 MB, and check the real content type from the file bytes, not from the header. Re-encode the image and resize it to 256×256 (this strips EXIF and location data). Store it behind a `BlobStore` interface with a local-disk implementation (a Docker volume mounted into the service, served through the gateway at `/media/avatars/{id}.webp` with cache headers). Leave a TODO for an OCI Object Storage implementation. Use random file names, never names the user supplies.
- **Web:** a `/profile` page reachable from the navbar avatar menu, with an editable form, an avatar preview before upload, upload progress, and error states. Show the avatar in the navbar and next to forum posts.
- Tests for validation (wrong type, too large, a spoofed content type), for authorization (a user cannot edit another user's profile), and for the image pipeline.

## Task 8: Detailed README
Branch: `docs/readme`

Write a root `README.md`. A recruiter should understand the project in 30 seconds, and a developer should be able to run it in 5 minutes. Structure it in layers:
1. **Top:** name, a one-line pitch, a screenshot or GIF, and a short "What this project demonstrates" list (microservices, event-driven messaging, geospatial queries, JWT auth with refresh rotation, real-time SSE, Docker deployment).
2. **Architecture:** a Mermaid diagram of the services, the broker, the DB and the client; a table of the services (port, responsibility, main endpoints); and the swell-alert flow as a Mermaid sequence diagram.
3. **Quick start:** prerequisites, `cp deployments/.env.example deployments/.env`, `docker compose up`, the URLs to open, and how to run `scripts/test-e2e.sh`.
4. **Development:** running one service locally, running unit and integration tests, and the repo layout.
5. **Design decisions and trade-offs:** why HTTP between services and not gRPC, why RabbitMQ, why SSE and not WebSockets, why the shared DB is a known compromise, and the auth design.
6. **Deployment:** a summary that links to `deployments/REMOTE_DEPLOYMENT.md`.
7. **Roadmap and known limitations.**
8. A note that the project was built with AI-assisted development.

Write it in English. Do it last, so that it describes the final state after tasks 1-7.

---

When all PRs are open, post a summary listing each branch, PR link, status, and anything you could not finish and why.
