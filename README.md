# Logos

Famous quotes database service. RESTful CRUD API backed by PostgreSQL,
built in Go, deployed on K3s via Argo CD.

## Tech Stack

- **Go 1.26** — chi router, pgx v5, sqlc, golang-migrate
- **PostgreSQL 16** — relational storage
- **Prometheus** — metrics at `/metrics`
- **Docker** — multi-stage build (distroless final image)

## Project Structure

```
cmd/logos/          Entrypoint
internal/
  config/           Environment-based configuration
  database/         pgxpool setup, migrations
  database/dbq/     sqlc-generated query layer
  handler/          HTTP handlers (CRUD per resource)
  middleware/       Logging, Prometheus metrics
  model/            API request/response types + DB conversions
  router/           chi router wiring
migrations/         SQL migration files (golang-migrate)
queries/            SQL source files (sqlc input)
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://logos:logos@localhost:5432/logos?sslmode=disable` | PostgreSQL connection string |
| `API_HOST` | `0.0.0.0` | Bind address |
| `API_PORT` | `8000` | Bind port |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `CORS_ALLOWED_ORIGINS` | *(none — CORS disabled)* | Comma-separated list of allowed CORS origins |
| `LOGOS_IMAGE_UPLOAD_DIR` | *(unset — uploads disabled)* | Directory where uploaded / generated image blobs are persisted. Mount a Kubernetes PVC here. When unset, `POST /api/v1/images/uploads`, `GET /api/v1/images/{id}/blob`, and `POST /api/v1/images:generate` return `503 Service Unavailable`. |
| `LOGOS_IMAGE_MAX_UPLOAD_BYTES` | `10485760` (10 MiB) | Hard cap on the request body for `POST /api/v1/images/uploads`. Mirror this value on any reverse-proxy `client_max_body_size` so the limit is enforced consistently. |
| `LOGOS_IMAGEGEN_PROVIDER` | *(unset — generation disabled)* | Image generation backend. Today: `darkbase`. When unset, `POST /api/v1/images:generate` returns `503`. |
| `LOGOS_IMAGEGEN_BASE_URL` | *(unset)* | Root URL of the configured generator (e.g. `http://image-adapter.darkbase.svc:8081`). Required to enable the endpoint. |
| `LOGOS_IMAGEGEN_AUTH_TOKEN` | *(unset)* | Optional Bearer token sent on every outbound generator request. |
| `LOGOS_IMAGEGEN_TIMEOUT_SECONDS` | `120` | Wall-clock cap for a single generate request, applied as a context deadline. Fires `504 Gateway Timeout` on the API. |

Invalid `API_PORT`, `LOG_LEVEL`, `LOGOS_IMAGE_MAX_UPLOAD_BYTES`, `LOGOS_IMAGEGEN_PROVIDER`, and `LOGOS_IMAGEGEN_TIMEOUT_SECONDS` values now fail fast during startup instead of silently falling back.

## CLI

`logos` dispatches on its first positional argument:

| Invocation       | Behaviour |
|------------------|-----------|
| `logos`          | Run migrations, then start the HTTP server. Backward-compatible default for local dev and single-container deploys. |
| `logos migrate`  | Run pending migrations and exit. Designed for a Kubernetes `initContainer` so the schema is advanced before the serving container starts. |
| `logos serve`    | Start the HTTP server without running migrations. Pair with `logos migrate` in an `initContainer` so the main container never opens a listener against a stale schema. |
| `logos help`     | Print the full usage (also `-h` / `--help`). |

Unknown subcommands and extra positional arguments exit non-zero with a diagnostic so a typo in a Pod spec crash-loops instead of silently reverting to the default path.

## Development

```bash
# Prerequisites: Go 1.26+, PostgreSQL, sqlc

# Run migrations and start the server (default, no subcommand)
DATABASE_URL="postgres://logos:logos@localhost:5432/logos?sslmode=disable" \
  go run ./cmd/logos

# Run migrations only (matches the Kubernetes initContainer)
DATABASE_URL="postgres://logos:logos@localhost:5432/logos?sslmode=disable" \
  go run ./cmd/logos migrate

# Start the server against an already-migrated database (matches the main container)
DATABASE_URL="postgres://logos:logos@localhost:5432/logos?sslmode=disable" \
  go run ./cmd/logos serve

# Regenerate sqlc code after changing queries/
sqlc generate

# Build
go build -o logos ./cmd/logos
```

## Quality Gate

Run before pushing:

```bash
make verify
```

Install the pre-push git hook to run lint + tests automatically before every push:

```bash
make install-hooks
```

## Build & Deploy

```bash
# Build container image and import into k3s
./build-and-import.sh

# Kubernetes manifests live in the MiMi repo (manifests/logos/).
# Argo CD auto-syncs once manifests are pushed to MiMi main.
# After importing the image, restart the deployment:
kubectl rollout restart deployment logos-api -n logos
```

## API Endpoints

Base path: `/api/v1`

Probe endpoints outside the API base path:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/livez` | Liveness — process-level only, never touches the DB. Body: `{"status":"ok"}`. |
| `GET` | `/readyz` | Readiness — pings the database. Body: `{"status":"ready"}` (200) or `{"status":"unready"}` (503). |

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Legacy compatibility readiness check. Same dependency check as `/readyz`, but emits the original body shape: `{"status":"healthy"}` (200) or `{"status":"unhealthy"}` (503). New consumers should use `/readyz`. |
| `GET` | `/categories` | List categories (?type=image\|quote\|author) |
| `POST` | `/categories` | Create category |
| `GET` | `/categories/{id}` | Get category |
| `PUT` | `/categories/{id}` | Update category |
| `DELETE` | `/categories/{id}` | Delete category |
| `GET` | `/images` | List images (?category_id=) |
| `POST` | `/images` | Register image by external URL (JSON body: `{url, alt_text?, category_id?}`) |
| `POST` | `/images/uploads` | Upload image from disk (multipart/form-data: `file`, `alt_text?`, `category_id?`). Persists to the local blobstore and returns the new image row with `source: "uploaded"`. Returns `503` when `LOGOS_IMAGE_UPLOAD_DIR` is not configured. |
| `POST` | `/images:generate` | Generate a new image via the configured backend (DarkBase today). JSON body: `{prompt, model?, width?, height?, seed?, steps?, cfg_scale?, alt_text?, category_id?}`. Synchronous: returns the persisted image row with `source: "generated"` once the bytes are downloaded. Returns `400` for client errors, `502` if the worker errored, `503` when not configured / backend unavailable, `504` on `LOGOS_IMAGEGEN_TIMEOUT_SECONDS`. |
| `GET` | `/images/{id}` | Get image |
| `GET` | `/images/{id}/blob` | Stream the raw bytes of an `uploaded` / `generated` image. `404` for `external_url` rows; `503` when uploads are not configured. |
| `PUT` | `/images/{id}` | Update image |
| `DELETE` | `/images/{id}` | Delete image (and its on-disk blob for `uploaded` / `generated` rows) |
| `GET` | `/authors` | List authors (?category_id=&name=) |
| `POST` | `/authors` | Create author |
| `GET` | `/authors/{id}` | Get author |
| `PUT` | `/authors/{id}` | Update author |
| `DELETE` | `/authors/{id}` | Delete author |
| `GET` | `/quotes` | List quotes (?author_id=&category_id=&title=) |
| `POST` | `/quotes` | Create quote |
| `GET` | `/quotes/{id}` | Get quote |
| `PUT` | `/quotes/{id}` | Update quote |
| `DELETE` | `/quotes/{id}` | Delete quote |
| `GET` | `/quotes/{id}/tags` | List tags for a quote |
| `POST` | `/quotes/{id}/tags` | Add tag to quote (body: `{tag_id}`) |
| `DELETE` | `/quotes/{id}/tags/{tagID}` | Remove tag from quote |
| `GET` | `/tags` | List tags |
| `POST` | `/tags` | Create tag |
| `GET` | `/tags/{id}` | Get tag |
| `DELETE` | `/tags/{id}` | Delete tag (cascades associations) |
| `GET` | `/metrics` | Prometheus metrics |

Write endpoints expect `application/json`, reject multiple JSON documents in one request body, and limit payload size to 1 MiB.
