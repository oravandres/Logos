# Logos Service — Implementation Plan

## Overview

Logos is a famous quotes database service. It provides a RESTful CRUD API backed
by PostgreSQL, deployed on the existing K3s cluster via Argo CD. The service is
written in Go, producing a single static binary — ideal for minimal container
images and the resource-constrained Raspberry Pi cluster.

---

## 1. Repository Landscape

| Repo | Role for Logos |
|------|---------------|
| **Logos** (this repo) | Application source code, Dockerfile, migrations, tests |
| **MiMi** | Kubernetes manifests (Deployment, Service, Ingress, Argo CD Application) |
| **LogosUI** | Future frontend (empty repo, out of scope for now) |

---

## 2. Tech Stack

| Layer | Choice | Rationale |
|-------|--------|-----------|
| Language | Go 1.26 | Static binary, low memory footprint, great for K8s |
| HTTP router | chi (`go-chi/chi/v5`) | Lightweight, idiomatic, stdlib-compatible |
| Database | PostgreSQL 16 | Relational data with strong indexing |
| DB driver | pgx v5 (`jackc/pgx/v5`) | Pure Go, high-performance Postgres driver |
| Query layer | sqlc | Type-safe SQL → Go code generation, no ORM overhead |
| Migrations | golang-migrate | Versioned SQL migrations, embeddable, CLI + library |
| Validation | go-playground/validator | Struct tag-based validation |
| Metrics | prometheus/client_golang | Consistent with existing ServiceMonitor |
| Config | Environment variables (envconfig or stdlib) | Simple, 12-factor |
| Logging | log/slog (stdlib) | Structured logging, zero dependencies |
| Image storage | URL reference | Quote images stored as URLs referencing external storage |
| Containerisation | Multi-stage Docker (golang → distroless/static) | Tiny final image (~10 MB) |

---

## 3. Database Schema

### 3.1 Table: `categories`

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | `UUID` | PK, default `gen_random_uuid()` | |
| `name` | `VARCHAR(100)` | NOT NULL | e.g. "portrait", "philosophy", "politician" |
| `type` | `VARCHAR(50)` | NOT NULL | Discriminator: `image`, `quote`, `author` |
| `created_at` | `TIMESTAMPTZ` | NOT NULL, default `NOW()` | |

**Indexes:**

- `uq_categories_name_type` — Unique on `(name, type)` (same name allowed across types)
- `ix_categories_type` — B-tree on `type` (filter by entity type)

### 3.2 Table: `images`

Standalone image registry. Images are uploaded independently and referenced
by authors and quotes via FK.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | `UUID` | PK, default `gen_random_uuid()` | |
| `url` | `VARCHAR(2048)` | NOT NULL | Path or URL to the image file |
| `alt_text` | `VARCHAR(500)` | NULLABLE | Accessibility / description |
| `category_id` | `UUID` | FK → `categories.id`, NULLABLE | Image category (e.g. "portrait", "background") |
| `created_at` | `TIMESTAMPTZ` | NOT NULL, default `NOW()` | |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL, default `NOW()` | |

**Indexes:**

- `ix_images_category_id` — B-tree on `category_id` (filter by category)

### 3.3 Table: `authors`

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | `UUID` | PK, default `gen_random_uuid()` | |
| `name` | `VARCHAR(255)` | NOT NULL | |
| `bio` | `TEXT` | NULLABLE | Short biography |
| `born_date` | `DATE` | NULLABLE | |
| `died_date` | `DATE` | NULLABLE | |
| `image_id` | `UUID` | FK → `images.id`, NULLABLE | Author portrait / photo |
| `category_id` | `UUID` | FK → `categories.id`, NULLABLE | e.g. "philosopher", "politician" |
| `created_at` | `TIMESTAMPTZ` | NOT NULL, default `NOW()` | |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL, default `NOW()` | |

**Indexes:**

- `ix_authors_name` — B-tree on `name` (search / autocomplete)
- `ix_authors_category_id` — B-tree on `category_id` (filter by category)

### 3.4 Table: `quotes`

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | `UUID` | PK, default `gen_random_uuid()` | |
| `title` | `VARCHAR(500)` | NOT NULL | Short label or summary |
| `text` | `TEXT` | NOT NULL | The actual quote |
| `author_id` | `UUID` | FK → `authors.id`, NOT NULL | |
| `image_id` | `UUID` | FK → `images.id`, NULLABLE | Image representing the quote |
| `category_id` | `UUID` | FK → `categories.id`, NULLABLE | e.g. "philosophy", "motivation" |
| `created_at` | `TIMESTAMPTZ` | NOT NULL, default `NOW()` | |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL, default `NOW()` | |

**Indexes:**

- `ix_quotes_author_id` — B-tree on `author_id` (FK lookups, filter by author)
- `ix_quotes_image_id` — B-tree on `image_id` (FK lookup)
- `ix_quotes_category_id` — B-tree on `category_id` (filter by category)
- `ix_quotes_title` — B-tree on `title` (search)
- `ix_quotes_created_at` — B-tree on `created_at` (sorting / pagination)

### 3.5 Table: `tags`

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | `UUID` | PK, default `gen_random_uuid()` | |
| `name` | `VARCHAR(100)` | NOT NULL, UNIQUE | e.g. "stoicism", "love", "war" |
| `created_at` | `TIMESTAMPTZ` | NOT NULL, default `NOW()` | |

**Indexes:**

- `uq_tags_name` — Unique on `name` (implicit from UNIQUE constraint)

### 3.6 Table: `quote_tags` (join)

| Column | Type | Constraints |
|--------|------|-------------|
| `quote_id` | `UUID` | PK (composite), FK → `quotes.id` ON DELETE CASCADE |
| `tag_id` | `UUID` | PK (composite), FK → `tags.id` ON DELETE CASCADE |

**Indexes:**

- Composite PK covers `(quote_id, tag_id)`
- `ix_quote_tags_tag_id` — B-tree on `tag_id` (reverse lookup)

### ER Diagram (text)

```
                categories
               ╱    │    ╲
              ╱     │     ╲
images ──────╱  authors    quotes ∞────── quote_tags ──────∞ tags
   ▲              │  ▲       │
   │              │  │       │
   └──────────────┘  └───────┘
      image_id         image_id

 categories.type = 'image'  → used by images
 categories.type = 'author' → used by authors
 categories.type = 'quote'  → used by quotes
```

---

## 4. API Design

Base path: `/api/v1`

### 4.1 Categories

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/categories` | List categories (filterable by `?type=image\|quote\|author`) |
| `GET` | `/categories/{id}` | Get single category |
| `POST` | `/categories` | Create category (name + type) |
| `PUT` | `/categories/{id}` | Update category |
| `DELETE` | `/categories/{id}` | Delete category (nullifies FKs on referencing rows) |

### 4.2 Images

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/images` | List images (paginated, filterable by `?category_id=`) |
| `GET` | `/images/{id}` | Get single image |
| `POST` | `/images` | Upload / register image (url, alt_text, category_id) |
| `PUT` | `/images/{id}` | Update image metadata |
| `DELETE` | `/images/{id}` | Delete image (nullifies FKs on authors/quotes) |

### 4.3 Authors

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/authors` | List authors (paginated, searchable by name, filterable by category) |
| `GET` | `/authors/{id}` | Get single author with image, category, quote count |
| `POST` | `/authors` | Create author (accepts image_id, category_id) |
| `PUT` | `/authors/{id}` | Update author |
| `DELETE` | `/authors/{id}` | Delete author (fails if quotes exist) |

### 4.4 Quotes

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/quotes` | List quotes (paginated, filterable by author/tag/category) |
| `GET` | `/quotes/{id}` | Get single quote with author, image, category, tags |
| `POST` | `/quotes` | Create quote (accepts image_id, category_id, tag IDs) |
| `PUT` | `/quotes/{id}` | Update quote |
| `DELETE` | `/quotes/{id}` | Delete quote (cascades quote_tags) |

### 4.5 Tags

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/tags` | List all tags |
| `POST` | `/tags` | Create tag |
| `DELETE` | `/tags/{id}` | Delete tag (removes associations) |

### 4.6 Operational

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check (DB connectivity) |
| `GET` | `/metrics` | Prometheus metrics |

---

## 5. Project Structure (Logos repo)

```
Logos/
├── cmd/
│   └── logos/
│       └── main.go              # Entrypoint: config, DB pool, router, server
├── internal/
│   ├── config/
│   │   └── config.go            # Env-based configuration struct
│   ├── database/
│   │   ├── database.go          # pgxpool connection setup
│   │   └── queries/             # sqlc generated code
│   │       ├── db.go
│   │       ├── models.go
│   │       ├── categories.sql.go
│   │       ├── images.sql.go
│   │       ├── authors.sql.go
│   │       ├── quotes.sql.go
│   │       └── tags.sql.go
│   ├── handler/
│   │   ├── categories.go        # HTTP handlers for /categories
│   │   ├── images.go            # HTTP handlers for /images
│   │   ├── authors.go           # HTTP handlers for /authors
│   │   ├── quotes.go            # HTTP handlers for /quotes
│   │   ├── tags.go              # HTTP handlers for /tags
│   │   ├── health.go            # GET /health
│   │   └── respond.go           # JSON response/error helpers
│   ├── middleware/
│   │   ├── logging.go           # Request logging (slog)
│   │   └── metrics.go           # Prometheus HTTP middleware
│   ├── model/
│   │   ├── category.go          # API request/response types
│   │   ├── image.go
│   │   ├── author.go
│   │   ├── quote.go
│   │   └── tag.go
│   └── router/
│       └── router.go            # chi router setup, route registration
├── migrations/
│   ├── 000001_create_categories.up.sql
│   ├── 000001_create_categories.down.sql
│   ├── 000002_create_images.up.sql
│   ├── 000002_create_images.down.sql
│   ├── 000003_create_authors.up.sql
│   ├── 000003_create_authors.down.sql
│   ├── 000004_create_quotes.up.sql
│   ├── 000004_create_quotes.down.sql
│   ├── 000005_create_tags.up.sql
│   ├── 000005_create_tags.down.sql
│   ├── 000006_create_quote_tags.up.sql
│   └── 000006_create_quote_tags.down.sql
├── queries/                     # sqlc SQL source files
│   ├── categories.sql
│   ├── images.sql
│   ├── authors.sql
│   ├── quotes.sql
│   └── tags.sql
├── sqlc.yaml                    # sqlc configuration
├── go.mod
├── go.sum
├── Dockerfile
├── build-and-import.sh
├── .gitignore
├── PLAN.md
└── README.md
```

**Key conventions:**

- `cmd/` for the binary entrypoint, `internal/` for all private packages.
- `handler/` owns HTTP concerns (decode request, call DB, encode response).
- `model/` holds API-facing types (separate from sqlc-generated DB models).
- `queries/` contains the raw SQL that sqlc compiles; output lands in
  `internal/database/queries/`.
- Migrations are plain `.sql` files consumed by golang-migrate.

---

## 6. Kubernetes Deployment (MiMi repo)

### 6.1 PostgreSQL (StatefulSet)

A dedicated PostgreSQL 16 instance in namespace `logos`.

- **StatefulSet** with 1 replica, `volumeClaimTemplate` for persistent data.
- **Secret** (SealedSecret) for `POSTGRES_PASSWORD`.
- **Service** (ClusterIP, port 5432) named `logos-postgres`.
- **Resource requests/limits** sized for Raspberry Pi (256Mi–512Mi RAM).

### 6.2 Logos API (Deployment)

- **Deployment** with 1 replica, image `logos-api:latest`, `imagePullPolicy: Never`.
- **Environment variables** for `DATABASE_URL` (referencing the postgres Secret).
- **Init container**: runs the logos binary with a `migrate` subcommand or a
  standalone golang-migrate container to apply pending migrations.
- **Health probes**: startup/readiness/liveness on `/api/v1/health`.
- **Service** (ClusterIP, port 8000) named `logos-api`.
- **Labels**: `app.kubernetes.io/name: logos-api`, `app.kubernetes.io/part-of: logos`.

### 6.3 Ingress

- Host: `logos.mimi.local`
- TLS via cert-manager (`mimi-internal-ca`)
- Traefik `ingressClassName`
- Path `/api/v1` → `logos-api:8000`
- Path `/metrics` → `logos-api:8000`

### 6.4 Argo CD Application

New file: `manifests/argocd-apps/logos-app.yaml`

- Source path: `manifests/logos`
- Destination namespace: `logos`
- Sync wave: `"4"` (after AI platform)
- Automated prune + selfHeal, `CreateNamespace=true`

### 6.5 ServiceMonitor

Standard `ServiceMonitor` selecting `app.kubernetes.io/part-of: logos`, scraping
`/metrics` on port `http`, interval `30s`.

---

## 7. Build & Deploy Flow

```
1.  Developer pushes to Logos repo
2.  On the build host (DarkBase or CI):
      docker build -t logos-api:latest .
      docker save logos-api:latest | sudo k3s ctr images import -
3.  Add/update manifests in MiMi repo (manifests/logos/*)
4.  Push MiMi repo → Argo CD auto-syncs
5.  Init container runs migrations (golang-migrate)
6.  Logos API pod starts, connects to logos-postgres
```

A `build-and-import.sh` script will be added to this repo for step 2.

### Dockerfile strategy (multi-stage)

```dockerfile
# Stage 1: Build
FROM golang:1.26.2-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /logos ./cmd/logos

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian12
COPY --from=builder /logos /logos
COPY migrations/ /migrations/
EXPOSE 8000
ENTRYPOINT ["/logos"]
```

Final image is ~10-15 MB with the static Go binary + migration SQL files.

---

## 8. Implementation Order

| Phase | Scope | Deliverable |
|-------|-------|-------------|
| **Phase 1** | Project skeleton | `go.mod`, `Dockerfile`, `cmd/logos/main.go` (health + metrics), `.gitignore`, config |
| **Phase 2** | Database layer | Migrations (SQL files), sqlc config + queries, connection pool setup |
| **Phase 3** | Categories CRUD | Handler, model types, SQL queries, tests |
| **Phase 4** | Images CRUD | Handler, model types, SQL queries, tests |
| **Phase 5** | Authors CRUD | Handler, model types (with image/category joins), SQL queries, tests |
| **Phase 6** | Quotes CRUD | Handler, model types (with author/image/category joins), SQL queries, tests |
| **Phase 7** | Tags CRUD + quote-tag association | Handler, model types, join queries, tests |
| **Phase 8** | Kubernetes manifests (MiMi repo) | Namespace, PostgreSQL StatefulSet, Logos Deployment, Ingress, Argo CD app, ServiceMonitor, SealedSecret |
| **Phase 9** | Build script + README | `build-and-import.sh`, updated `README.md` |

---

## 9. Configuration (Environment Variables)

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://logos:logos@localhost:5432/logos?sslmode=disable` | PostgreSQL connection string (pgx format) |
| `LOG_LEVEL` | `info` | slog level (debug, info, warn, error) |
| `API_HOST` | `0.0.0.0` | Server bind address |
| `API_PORT` | `8000` | Server bind port |
| `MIGRATIONS_PATH` | `file:///migrations` | Path to migration files (for golang-migrate) |

---

## 10. Open Questions / Future Scope

- **Image upload**: For now images are stored as URLs. A future iteration could
  accept file uploads and push to MinIO (already deployed on DarkBase).
- **Full-text search**: PostgreSQL `tsvector` index on `quotes.text` for
  efficient text search. Can be added as a migration later.
- **Rate limiting**: Not needed initially (internal network only).
- **LogosUI**: Separate repo, separate Argo CD app; will consume this API.
- **CI pipeline**: GitHub Actions workflow for `go vet`, `go test`, `golangci-lint`,
  and `kubeconform` (matching MiMi's CI patterns).
