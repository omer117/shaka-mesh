# Community Service

Provides the persisted surf shop and forum APIs using the shared PostgreSQL
database. The service applies its idempotent schema and seeds a sample surf-gear catalog,
forum categories, discussions, and replies at startup. Seed discussions use
clearly marked demo display names and do not create fake login accounts.

## Configuration

- `DATABASE_URL` is required.
- `HTTP_ADDR` defaults to `:8083`.

The API is intended to be reached through the API gateway.

### Shop

- `GET /api/v1/shop/products` lists the seeded catalog.
- `GET /api/v1/shop/cart` gets the authenticated user's cart.
- `POST /api/v1/shop/cart/items` adds `{ "product_id": "...", "quantity": 1 }`.
- `DELETE /api/v1/shop/cart/items/{productID}` removes an item.
- `POST /api/v1/shop/checkout` atomically reserves inventory, creates a saved
  order, and empties the cart. This is a simulated checkout: no payment is
  collected and the resulting order is marked `paid`.

### Forum

- `GET /api/v1/forum/categories` lists seeded categories and topic counts.
- `GET /api/v1/forum/threads?category=local-breaks` lists recent topics.
- `GET /api/v1/forum/threads/{threadID}` reads a topic and its replies.
- `POST /api/v1/forum/threads` creates an authenticated topic.
- `POST /api/v1/forum/threads/{threadID}/posts` adds an authenticated reply.

The gateway validates JWTs and forwards verified user ID and email headers for
authenticated mutations. Product and forum reads are public.

## Tests

Run `go test ./...` and `go vet ./...` from this directory.

### Integration tests

Repository integration tests run against a real PostGIS database started with
`testcontainers-go` (image `postgis/postgis:16-3.4`). They cover the shop
checkout transaction (stock decreases, order created, cart emptied, full
rollback when a line is short), a concurrent checkout for the last unit that
must never oversell, and the forum queries. They sit behind the `integration`
build tag, so plain `go test ./...` stays fast and needs no Docker. To run them,
start Docker and run:

```bash
go test -tags=integration ./...
```

The container is initialised with `deployments/init_db.sql` (which provides the
shared `users` table), then the service's own `Migrate` is applied, as at
startup.
