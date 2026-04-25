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
| `search_vector` | `TSVECTOR` | GENERATED ALWAYS AS … STORED | Full-text search index material (migration 000007); `setweight(title, 'A') || setweight(text, 'B')` under the `'english'` config |

**Indexes:**

- `ix_quotes_author_id` — B-tree on `author_id` (FK lookups, filter by author)
- `ix_quotes_image_id` — B-tree on `image_id` (FK lookup)
- `ix_quotes_category_id` — B-tree on `category_id` (filter by category)
- `ix_quotes_title_trgm` — GIN (trigram) on `title` (legacy `?title=` substring filter)
- `ix_quotes_created_at` — B-tree on `created_at` (sorting / pagination)
- `ix_quotes_search_vector` — GIN on `search_vector` (full-text `?q=` search)

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
| `GET` | `/quotes` | List quotes (paginated, filterable by author/tag/category, searchable via `?q`) |
| `GET` | `/quotes/{id}` | Get single quote with author, image, category, tags |
| `POST` | `/quotes` | Create quote (accepts image_id, category_id, tag IDs) |
| `PUT` | `/quotes/{id}` | Update quote |
| `DELETE` | `/quotes/{id}` | Delete quote (cascades quote_tags) |

**Query parameters on `GET /quotes`:**

| Param | Type | Description |
|-------|------|-------------|
| `limit`, `offset` | int | Pagination window |
| `author_id`, `category_id`, `tag_id` | UUID | Facet filters (AND-composed) |
| `title` | string | Legacy substring filter on `title` (trigram-accelerated `ILIKE`) |
| `q` | string | Full-text search across title (weight A) + body (weight B) via a stored `tsvector`; uses `websearch_to_tsquery('english', …)` so the user-facing syntax `"exact phrase"`, `-excluded`, `or` is accepted with no parse errors. Results are `ts_rank_cd`-ranked when set; ordering falls back to `created_at DESC, id DESC` when absent. |

### 4.5 Tags

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/tags` | List all tags |
| `POST` | `/tags` | Create tag |
| `DELETE` | `/tags/{id}` | Delete tag (removes associations) |

### 4.6 Operational

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/livez` | Liveness — process-level only, never touches the DB. `200 {"status":"ok"}`. |
| `GET` | `/readyz` | Readiness — pings the DB. `200 {"status":"ready"}` or `503 {"status":"unready"}`. |
| `GET` | `/api/v1/health` | Legacy readiness (`{"status":"healthy"\|"unhealthy"}`); kept for LogosUI / external monitors that pinned the original body shape. New consumers should use `/readyz`. |
| `GET` | `/metrics` | Prometheus metrics. Scraped via `ServiceMonitor` on the ClusterIP service; not exposed through the public Ingress. |

---

## 5. Project Structure (Logos repo)

```
Logos/
├── cmd/
│   └── logos/
│       ├── main.go              # Entrypoint: parseMode → migrate / serve / both
│       └── main_test.go         # CLI dispatch table (locks the K8s manifest contract)
├── internal/
│   ├── config/
│   │   ├── config.go            # Env-based configuration struct, fail-fast parsing
│   │   └── config_test.go
│   ├── database/
│   │   ├── database.go          # pgxpool connection setup, RunMigrations
│   │   ├── database_test.go     # DSN scheme normalization
│   │   └── dbq/                 # sqlc-generated query layer (DO NOT EDIT)
│   │       ├── db.go
│   │       ├── models.go
│   │       ├── categories.sql.go
│   │       ├── images.sql.go
│   │       ├── authors.sql.go
│   │       ├── quotes.sql.go
│   │       ├── tags.sql.go
│   │       └── quote_tags.sql.go
│   ├── handler/
│   │   ├── categories.go        # HTTP handlers for /categories
│   │   ├── images.go            # HTTP handlers for /images
│   │   ├── authors.go           # HTTP handlers for /authors
│   │   ├── quotes.go            # HTTP handlers for /quotes
│   │   ├── tags.go              # HTTP handlers for /tags
│   │   ├── quote_tags.go        # /quotes/{id}/tags (transactional, FOR KEY SHARE)
│   │   ├── category_check.go    # validateCategoryType + sentinel errors
│   │   ├── dberror.go           # PgError code → HTTP status classification
│   │   ├── health.go            # /livez, /readyz, legacy /api/v1/health
│   │   ├── respond.go           # JSON / error helpers, decode(), parseUUID, parsePagination
│   │   └── *_test.go            # Table-driven tests per handler
│   ├── middleware/
│   │   ├── logging.go           # Request logging (slog) — currently lacks request_id correlation
│   │   └── metrics.go           # Prometheus HTTP middleware (route-pattern label)
│   ├── model/
│   │   ├── category.go          # API request/response types
│   │   ├── image.go
│   │   ├── author.go
│   │   ├── quote.go             # Single-source quoteResponseFromFields adapter
│   │   ├── tag.go
│   │   ├── pagination.go        # PaginatedResponse[T], DefaultLimit, MaxLimit
│   │   └── convert.go           # pgtype <-> google/uuid, pgtype.Text/Date helpers
│   └── router/
│       └── router.go            # chi router setup, middleware wiring, route registration
├── migrations/
│   ├── 000001_create_categories.{up,down}.sql
│   ├── 000002_create_images.{up,down}.sql
│   ├── 000003_create_authors.{up,down}.sql           # pg_trgm extension, category-type triggers
│   ├── 000004_create_quotes.{up,down}.sql            # category-type triggers + bidirectional guard
│   ├── 000005_create_tags.{up,down}.sql
│   ├── 000006_create_quote_tags.{up,down}.sql        # join table with composite PK
│   ├── 000007_add_quotes_search_vector.{up,down}.sql # tsvector + GIN
│   └── embed.go                                      # //go:embed *.sql → migrations.FS
├── queries/                     # sqlc SQL source files (compiled into internal/database/dbq/)
│   ├── categories.sql
│   ├── images.sql
│   ├── authors.sql
│   ├── quotes.sql
│   ├── tags.sql
│   └── quote_tags.sql
├── scripts/
│   └── pre-push                 # tidy + vet + lint + tests + build
├── .github/workflows/
│   ├── ci.yml                   # tests, race, vet, golangci-lint, staticcheck, govulncheck, attest
│   └── docker.yml               # main-only multi-arch GHCR push (linux/amd64, linux/arm64)
├── .cursor/rules/               # Coding/architecture rules + PR-review lessons (alwaysApply)
├── .golangci.yml                # v2 config; default:none + explicit linter allowlist
├── AGENTS.md                    # Repo expectations for agentic / human contributors
├── Dockerfile                   # multi-stage, BUILDPLATFORM cross-compile, distroless:nonroot
├── Makefile                     # verify / lint / test / build / install-hooks
├── build-and-import.sh          # local k3s ctr import for the homelab cluster
├── sqlc.yaml                    # sqlc configuration
├── go.mod
├── go.sum
├── PLAN.md
└── README.md
```

**Key conventions:**

- `cmd/` for the binary entrypoint, `internal/` for all private packages.
- `handler/` owns HTTP concerns (decode request, call DB, encode response).
- `model/` holds API-facing types (separate from sqlc-generated DB models).
- `queries/` contains the raw SQL that sqlc compiles; output lands in
  `internal/database/dbq/`. The output package was renamed from the
  originally-planned `queries` to `dbq` to avoid the queries-source vs
  queries-generated path collision.
- Migrations are plain `.sql` files consumed by golang-migrate via
  `//go:embed *.sql` (see `migrations/embed.go`).

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
| ~~Phase 1~~ | Project skeleton | `go.mod`, `Dockerfile`, `cmd/logos/main.go` (health + metrics), `.gitignore`, config |
| ~~Phase 2~~ | Database layer | Migrations (SQL files), sqlc config + queries, connection pool setup |
| ~~Phase 3~~ | Categories CRUD | Handler, model types, SQL queries, tests |
| ~~Phase 4~~ | Images CRUD | Handler, model types, SQL queries, tests |
| ~~Phase 5~~ | Authors CRUD | Handler, model types (with image/category joins), SQL queries, tests |
| ~~Phase 6~~ | Quotes CRUD | Handler, model types (with author/image/category joins), SQL queries, tests |
| ~~Phase 7~~ | Tags CRUD + quote-tag association | Handler, model types, join queries, tests |
| ~~Phase 8~~ | Kubernetes manifests (MiMi repo) | Namespace, PostgreSQL StatefulSet, Logos Deployment, Ingress, Argo CD app, ServiceMonitor, SealedSecret |
| ~~Phase 9~~ | Build script + README | `build-and-import.sh`, updated `README.md` |
| ~~Phase 10~~ | Quotes full-text search | Migration `000007_add_quotes_search_vector.{up,down}.sql`, `?q=` param on `GET /quotes` with `websearch_to_tsquery` + `ts_rank_cd`, handler + regression tests pinning the sqlc arg layout |

---

## 9. Configuration (Environment Variables)

Config is loaded once at startup via `internal/config.Load`. Invalid values
fail fast — operators see a configuration error in the pod log instead of a
silent fallback to a default.

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://logos:logos@localhost:5432/logos?sslmode=disable` | PostgreSQL DSN; `postgres://`, `postgresql://`, and `pgx5://` schemes are accepted. |
| `API_HOST` | `0.0.0.0` | Server bind address. Ignored by `logos migrate`. |
| `API_PORT` | `8000` | Server bind port (validated 0–65535). Ignored by `logos migrate`. |
| `LOG_LEVEL` | `info` | slog level (`debug`, `info`, `warn`, `error`). Anything else is a startup error. |
| `CORS_ALLOWED_ORIGINS` | *(unset)* | Comma-separated origin allowlist. **Fail-closed**: when unset, the CORS middleware is not installed at all (no preflight handling, no `Access-Control-*` headers). |
| `MIGRATIONS_PATH` | *(unused)* | Reserved. The binary currently only reads migrations from the embedded `migrations.FS`. Do not rely on this variable; either drop it from the documented surface or wire it through to `RunMigrations`. **Action: track in §11.4.** |

---

## 10. Open Questions / Future Scope

- **Image upload**: For now images are stored as URLs. A future iteration could
  accept file uploads and push to MinIO (already deployed on DarkBase).
- ~~**Full-text search**: PostgreSQL `tsvector` index on `quotes.text`.~~
  _Shipped._ Migration `000007_add_quotes_search_vector.up.sql` adds a stored
  generated `tsvector` column (`setweight(title, 'A') || setweight(text, 'B')`)
  with a GIN index. `GET /quotes?q=…` uses `websearch_to_tsquery('english', …)`
  and `ts_rank_cd` ordering; see §4.4. The `'english'` config is hard-coded for
  v1; a per-row `search_config` column can be added if a multilingual corpus
  materializes.
- **Rate limiting**: Not needed initially (internal network only). Revisit if
  the service is exposed beyond the cluster ingress; see §11.5.
- **LogosUI**: Separate repo, separate Argo CD app; will consume this API.
- ~~**CI pipeline**: GitHub Actions workflow for `go vet`, `go test`,
  `golangci-lint`.~~ _Shipped._ `.github/workflows/ci.yml` runs vet, race
  tests, golangci-lint v2, staticcheck, govulncheck, build provenance
  attestation; `.github/workflows/docker.yml` publishes a multi-arch image to
  GHCR on `main`. `kubeconform` lives in MiMi (manifests live there).

---

## 11. Production-Hardening Roadmap (Post-MVP)

The MVP (Phases 1–10) is feature-complete and production-deployed. This section
captures concrete improvements that move the service from "production-ready" to
"production-hardened". Items are grouped by priority and each carries a
**Why** (the failure mode it prevents or capability it unlocks) and an
**Acceptance** sketch (how we know it shipped).

Use this section as the standing backlog for new branches: each subsection is
a self-contained PR, sized to land in isolation without coupling to the others.

### 11.1 Tier 1 — Operational hardening

These directly affect on-call surface area and request-path correctness.
Default to landing these before extending features.

#### 11.1.1 Request-scoped DB timeouts

**Problem.** Every handler passes `r.Context()` straight to the pool, so a
slow / tarpit client holds a Postgres backend for as long as the TCP read
window allows. Under load this can exhaust pool capacity (`pgxpool` default
`MaxConns = max(4, GOMAXPROCS)`), and `WriteTimeout: 30s` on the HTTP server
is a *response* deadline that fires after the client has already monopolized
a connection.

**Why.** Bound the cost of a single slow caller. Without this, a single
misbehaving client can deny service to the rest of the workload.

**Acceptance.** A small `httputil.WithDBTimeout(d)` middleware (or a helper
applied per-handler) wraps `r.Context()` with a configurable deadline (default
**5s**, override via `DB_QUERY_TIMEOUT`). Tests assert that queries time out
with `context.DeadlineExceeded` when the server is slow.

#### 11.1.2 pgxpool tuning + observability

**Problem.** `pgxpool.NewWithConfig` is called with whatever `ParseConfig`
returns from the URL — no `MaxConns`, `MinConns`, `MaxConnLifetime`, or
`MaxConnIdleTime` are set. Defaults are reasonable for a 4-core dev box; a
2-core RPi 4 + 100 concurrent clients is a different story. We also have no
way to alert on saturation: pool internals are not exposed.

**Why.** Connection saturation is a top-3 outage cause for Postgres-backed
Go services. We need both sane defaults and visibility into them.

**Acceptance.**
1. `internal/config` adds `DB_MAX_CONNS`, `DB_MIN_CONNS`, `DB_MAX_CONN_LIFETIME`,
   `DB_MAX_CONN_IDLE_TIME` with defaults tuned for the cluster (e.g. 10/2/30m/5m).
2. A new collector wraps `pgxpool.Pool.Stat()` and exports
   `logos_db_pool_{acquired,idle,max,total}_connections` and
   `logos_db_pool_acquire_duration_seconds`. Dashboard hooks live in MiMi.
3. Verified by a unit test that the values from the env are wired into
   `pgxpool.Config` (no smoke test of real saturation needed in this PR).

#### 11.1.3 Request ID correlation in logs

**Problem.** `chimw.RequestID` runs in the middleware chain and stamps a
`X-Request-Id` header, but `internal/middleware.Logging` ignores it — the
slog line for `request` carries `method`, `path`, `status`, `duration_ms`,
`remote`, but **no request_id**. Operators tracing a 500 across pods have no
way to stitch a client report (`X-Request-Id: abc`) to a server log line.

**Why.** Correlation is the cheapest single observability win available; cost
to add is ~5 lines. Without it, every "what happened to my request" question
takes a full log scan.

**Acceptance.** `Logging` reads `chimw.GetReqID(r.Context())` and includes it
as `request_id` on every emitted log. A unit test for the middleware asserts
the field is present (use `slogtest` or capture via a custom `slog.Handler`).
Probe paths (`/livez`, `/readyz`, `/metrics`) get filtered out of access logs
in the same change to keep volume bounded — see §11.1.4.

#### 11.1.4 Skip access logs for probe + metrics paths

**Problem.** `/livez` and `/readyz` are hit every few seconds by the kubelet,
and `/metrics` is hit every 30s by Prometheus. Each call emits an `INFO`
slog line. On a stable pod that is ~3000 lines/hour of pure noise; on log
ingestion-priced platforms (Loki/Datadog) it's a real cost line.

**Why.** Signal-to-noise. Probes are already covered by Prometheus
counters (`logos_http_requests_total{route="/readyz"}`); a per-request log
line adds nothing.

**Acceptance.** `Logging` checks `chi.RouteContext(r.Context()).RoutePattern()`
against a small skip-list; matching routes are not logged at INFO (still
counted in metrics, still error-logged on 5xx). Test covers both the skip
case and the "5xx is still logged" case.

#### 11.1.5 HTTP server timeouts hardening

**Problem.** `cmd/logos/main.go` sets `ReadTimeout: 10s`, `WriteTimeout:
30s`, `IdleTimeout: 60s` but no `ReadHeaderTimeout` and no `MaxHeaderBytes`.
A Slowloris-style attacker can dribble headers indefinitely under
`ReadTimeout: 10s` (the timeout covers the whole read, but combined with
keep-alive socket reuse the practical envelope is wider than intended).

**Why.** Defense in depth. The existing settings cover happy-path latency
SLAs but not adversarial inputs.

**Acceptance.** Add `ReadHeaderTimeout: 5s` and `MaxHeaderBytes: 1 << 16`
(64 KiB). Document the rationale in a code comment. No new tests needed —
behavior is locked by `net/http` semantics.

#### 11.1.6 Graceful shutdown via signal.NotifyContext + pool ordering

**Problem.** Shutdown today uses a manual `signal.Notify` channel + a select
on a `errCh`. The pool is closed via `defer pool.Close()` in `runServe`,
which fires *after* `srv.Shutdown` returns — fine on the happy path, but the
ordering is implicit and easy to break in a future refactor. There's also
no SIGHUP handling for log-level reload, which is convenient for ops.

**Why.** Idempotent, ordered shutdown is a recurring source of flakes (pool
closed mid-flight; in-flight DB calls panic on a closed pool).

**Acceptance.** Switch to `signal.NotifyContext(ctx, SIGINT, SIGTERM)`.
Document the shutdown order with a short comment and an integration-style
test (using `httptest.NewServer` + a fake pool) that asserts in-flight
requests complete before the pool is closed.

### 11.2 Tier 2 — Test depth and CI

#### 11.2.1 Postgres-backed integration tests via testcontainers-go

**Problem.** Handler tests stub `dbq.DBTX`. They prove the handler shape is
correct, but nothing exercises the actual SQL — sqlc validates *syntax* but
not *behavior*. The category-type trigger, the FOR-KEY-SHARE locking in
`quote_tags`, the FTS ranking ORDER BY, the FK ON DELETE SET NULL semantics
on `images` and `categories` — none of these have a single test that asserts
the database does what the comments claim it does.

**Why.** Three of the last six PRs (#11, #12, #14) fixed bugs in the
SQL/query interaction surface (transaction atomicity, dead SQL, query
plan choice). A real-DB test would have caught all three before review.

**Acceptance.**
1. New `internal/database/integration_test.go` (build tag `integration`)
   spins up `testcontainers-go` with `postgres:16-alpine`.
2. `make test-integration` runs it locally; CI runs it as a separate job.
3. Coverage targets:
   - Trigger: inserting a quote with `category.type='author'` returns 23514.
   - Locking: concurrent `RemoveTag` + `AddTag` against the same quote is
     serializable (run in two goroutines, no anomalies).
   - FTS: a quote with `title="Stoic"` ranks above one with `text="stoic"`.
   - Cascade: deleting an image NULLs `quotes.image_id` and `authors.image_id`.

#### 11.2.2 Migration-up/down round-trip test

**Problem.** Down migrations are written but never exercised. Migration
`000003` declares `CREATE EXTENSION IF NOT EXISTS pg_trgm`; the down
migration must not drop it (per `.cursor/rules/12 → Migrations`). The only
way to know we got that right is to actually run `Up()` then `Down()` and
inspect the schema.

**Why.** Down migrations are the rollback story. If they're broken, we have
no rollback story.

**Acceptance.** An integration test (same harness as 11.2.1) loops
`Up()` → `Down()` → `Up()` and asserts the final schema matches the head.

#### 11.2.3 Fuzz tests for the decode boundary

**Problem.** `internal/handler/respond.go::decode` is the only place that
parses untrusted bytes. It correctly rejects multiple JSON documents,
oversized bodies, and unknown fields, but no fuzzer has ever explored it.
Cursor rules §05 calls out fuzzing for parsers as standard.

**Why.** Cheap insurance. Even if no exploit emerges, the corpus becomes a
regression seed for the next change to body parsing.

**Acceptance.** `respond_fuzz_test.go` with `FuzzDecode` exercising random
content-types, bodies, and trailing data. `go test -fuzz=FuzzDecode -fuzztime=30s`
is added to CI's nightly job (not the main PR job — fuzz time would dominate).

#### 11.2.4 Coverage gate + sqlc drift gate

**Problem.** CI does not enforce a coverage threshold. It also does not run
`sqlc generate` and check for drift, so the generated `dbq` package can
silently fall behind `queries/*.sql`.

**Why.** Both are cheap, both prevent quiet regressions.

**Acceptance.** Add to `.github/workflows/ci.yml`:
- A coverage step (`go test ./... -coverprofile=cover.out`) plus a
  `go tool cover -func=cover.out | awk '... >= 70.0'` gate. Start at 70%,
  ratchet upward.
- A drift step that installs sqlc, runs `sqlc generate`, and fails on
  `git diff --exit-code internal/database/dbq`.

#### 11.2.5 Router-level smoke test

**Problem.** `internal/router/router.go::New` is uncovered. A typo that
fails to register `DELETE /quotes/{id}/tags/{tagID}` would not be caught.

**Why.** Single unit test that walks `chi.Walk` and asserts every documented
route in §4 is registered with the expected method.

**Acceptance.** New `internal/router/router_test.go` that calls `router.New`
with a fake pool/cfg, then asserts the route table matches a golden list.
Failures point at the missing route by name, not at a 404 from a nondescript
HTTP test.

### 11.3 Tier 3 — Observability

#### 11.3.1 OpenTelemetry tracing

**Problem.** No tracing. `AGENTS.md` and `.cursor/rules/04` both call for
trace correlation; nothing emits spans today.

**Why.** Once latency anomalies appear (and they will, at the first
multi-region or noisy-neighbor event), bisecting handler vs DB vs network
without spans is painful.

**Acceptance.**
1. Add `go.opentelemetry.io/otel`, `otel/sdk`, `otel/trace`, and
   `otel-contrib/instrumentation/github.com/jackc/pgx/v5/otelpgx` (or
   equivalent).
2. New `internal/observability/tracing.go` initializes the OTLP exporter from
   `OTEL_EXPORTER_OTLP_ENDPOINT` (or no-op when unset → fail-closed).
3. Wrap `chi` with the `otelhttp` middleware; wrap `pgxpool` with `otelpgx`.
4. Spans cover request → handler → DB call. The slog logger picks up
   `trace_id` and `span_id` and emits them as fields.
5. Document in `README.md` how to point at a Tempo / Jaeger collector.

#### 11.3.2 Runtime + build-info metrics

**Problem.** `/metrics` exposes only `logos_http_*` counters and the
default `process_*` and `go_*` collectors that `promauto` sets up
implicitly via the default registry. There is no `logos_build_info`
gauge with `version`, `commit`, `built_at` labels — operators cannot tell
which image is serving without reading the deployment manifest.

**Why.** Build-info is the cheapest form of "what's in production right
now" answer; ops dashboards canonically pin this gauge.

**Acceptance.**
1. Add `-ldflags '-X main.version=… -X main.commit=… -X main.builtAt=…'` to
   the Dockerfile build step.
2. New `internal/observability/buildinfo.go` registers a `prometheus.GaugeFunc`
   labeled with those values, set to `1`.
3. Optional `GET /api/v1/version` returns the same triple as JSON for human
   curl.

### 11.4 Tier 4 — API and DX polish

#### 11.4.1 Validation library + length checks

**Problem.** Validation is hand-rolled per handler. `tags.go` checks
`len(req.Name) > 100` against `VARCHAR(100)`; equivalent guards for
`categories.name` (`VARCHAR(100)`), `images.url` (`VARCHAR(2048)`),
`images.alt_text` (`VARCHAR(500)`), `authors.name` (`VARCHAR(255)`),
`quotes.title` (`VARCHAR(500)`) are missing — an oversized payload reaches
Postgres and surfaces as a generic 500 instead of a clear 400.

**Why.** Failing closer to the boundary is both a UX win and a defense win.
Cursor rules §12 → "Input validation" calls this out explicitly.

**Acceptance.** Either:
- (a) Adopt `github.com/go-playground/validator/v10` (already in the planned
  tech stack §2 but unused) and add struct tags to the request types in
  `internal/model/`. A single `validate.Struct(&req)` call replaces the
  hand-rolled if-trees and produces a uniform 400 body.
- (b) Add explicit length checks to each handler matching the column width.
  Less churn but more code to keep in sync with migrations.

Either way: a regression test per resource asserting that an oversized
field returns 400 with a clear field name.

#### 11.4.2 Cursor pagination on list endpoints

**Problem.** All list endpoints use `?limit=&offset=`. Offset pagination
is O(N) for large `offset`; deep paging in a 1M-row corpus becomes a
denial-of-service against ourselves. It also returns inconsistent windows
under concurrent writes (a row inserted between requests can be skipped or
duplicated).

**Why.** Industry-standard at this scale; trivial change because the
ordering already includes a deterministic tiebreaker (`created_at, id`).

**Acceptance.** Add `?cursor=<base64(created_at,id)>` as an alternative to
`offset=`. Keep `offset` for backward compatibility. The response gains
`next_cursor` (nullable) and the existing `total` becomes optional / capped
(`total` over a multi-million-row corpus is itself slow).

#### 11.4.3 Idempotency keys on POST endpoints

**Problem.** Submitting `POST /quotes` twice creates two rows. `tags` and
`categories` have unique constraints that prevent dupes; `quotes` and
`authors` do not. A flaky network on the LogosUI side will create
duplicates.

**Why.** Standard pattern; small surface area; immediate UX win.

**Acceptance.** Accept `Idempotency-Key: <uuid>` header on POST. New
`idempotency_keys` table (key, sha256(body), response_body, response_code,
created_at) with a 24h TTL purge job (`pg_cron` or scheduled K8s `CronJob`).
Repeated submissions with the same key replay the cached response.

#### 11.4.4 Resource embedding on GET endpoints

**Problem.** `GET /quotes/{id}` returns `author_id`, `image_id`,
`category_id` only — clients fetching "show me this quote with its author"
need three round trips. UI side then waterfall-renders.

**Why.** Standard `?include=author,image,tags` pattern; lifts most LogosUI
detail-screen latency in one PR.

**Acceptance.** Add `?include=author,image,category,tags` as a
comma-separated query param. The handler joins or batch-fetches and embeds
the included resources under their own keys (`author: {…}`). Documented
in §4.4. Tests cover empty `include`, single resource, and the full set.

#### 11.4.5 Drop or wire MIGRATIONS_PATH

**Problem.** `MIGRATIONS_PATH` is documented in `README.md` and §9 but
the binary always reads from `migrations.FS`. Either silently misleading
or unfinished.

**Why.** Sharp config edges erode trust in the rest of the documented surface.

**Acceptance.** Either remove the env var from docs and config (preferred —
embedded migrations are simpler and correct for our deploy model), or wire
it through `RunMigrations` so an operator can override the source for
disaster-recovery scenarios. Decide and execute in a single small PR.

### 11.5 Tier 5 — Speculative / longer horizon

These are not committed; record only so they don't get re-discovered later.

- **Rate limiting at the handler** (`golang.org/x/time/rate` or a chi
  middleware) — only worth it once the service is exposed beyond the
  cluster ingress or once a noisy-neighbor incident occurs.
- **OpenAPI 3 spec** generated from the handler/model types (e.g.
  `swag` or `huma`) so LogosUI can auto-generate a typed client. Trade-off:
  another DSL to maintain; revisit when LogosUI development starts.
- **`docker-compose.yml` for local dev**: today the README says "PostgreSQL
  required" without offering a one-shot. Compose would be a nice ramp,
  though `testcontainers` (§11.2.1) covers the test path.
- **CHANGELOG.md / release tagging**: 65 commits in, no change history
  document. Adopt Conventional Commits + `git-cliff` to derive a CHANGELOG
  on tagged releases.
- **Dependabot + CODEOWNERS + PR template**: no `.github/dependabot.yml`,
  no `CODEOWNERS`, no PR template. All low-effort wins but only useful
  once there are multiple human reviewers.
- **GoReleaser** for tagged binary + container releases. Only relevant if
  someone runs Logos outside our K8s cluster.
- **`debug/pprof` endpoint behind an internal-only listener** — handy for
  the next "where is the goroutine leak" hunt, but only safe on a separate
  bind address (never on the public port).

---

## 12. Suggested Next Increment

If you are starting work after this PLAN update, the highest-leverage three
PRs to land first — in this order — are:

1. **§11.1.3 Request-ID logging + §11.1.4 probe-path filter** — single small
   PR, no schema changes, immediate observability win.
2. **§11.1.1 Per-request DB timeouts + §11.1.2 pool tuning + metrics** —
   the "stop a single bad client from taking us down" PR. Configurable, no
   wire-protocol change.
3. **§11.2.1 testcontainers integration tests** — unblocks safe future SQL
   work, catches the class of bug that has dominated recent fixes.

After that, anything in §11.2 / §11.3 / §11.4 is roughly equal-priority and
can be triaged based on whichever observable problem fires first.
