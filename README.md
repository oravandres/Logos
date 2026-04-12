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

## Development

```bash
# Prerequisites: Go 1.26+, PostgreSQL, sqlc

# Run migrations and start the server
DATABASE_URL="postgres://logos:logos@localhost:5432/logos?sslmode=disable" \
  go run ./cmd/logos

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

## Build & Deploy

```bash
# Build container image and import into k3s
./build-and-import.sh
```

## API Endpoints

Base path: `/api/v1`

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/categories` | List categories (?type=image\|quote\|author) |
| `POST` | `/categories` | Create category |
| `GET` | `/categories/{id}` | Get category |
| `PUT` | `/categories/{id}` | Update category |
| `DELETE` | `/categories/{id}` | Delete category |
| `GET` | `/images` | List images (?category_id=) |
| `POST` | `/images` | Register image |
| `GET` | `/images/{id}` | Get image |
| `PUT` | `/images/{id}` | Update image |
| `DELETE` | `/images/{id}` | Delete image |
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
