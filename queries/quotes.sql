-- name: ListQuotes :many
-- The tag filter is expressed as `id IN (SELECT ...)` rather than a correlated
-- EXISTS so PostgreSQL can hash the inner result once and run it as a hashed
-- semi-join driven from ix_quote_tags_tag_id. With a correlated EXISTS the
-- planner re-probes quote_tags_pkey per outer row (~100K loops, ~300K buffers
-- on a 100K-quote dataset for a sparse tag); the IN form drops to ~1.2K
-- buffers regardless of tag selectivity. See PR #14 EXPLAIN ANALYZE evidence.
--
-- The search_q filter runs a websearch_to_tsquery('english', ...) against the
-- quotes.search_vector GIN index (added in migration 000007). websearch_to_tsquery
-- is used deliberately over plainto_tsquery / to_tsquery: it accepts the
-- user-facing search-box syntax ("exact phrase", -excluded, or) and never
-- raises SQLSTATE 42601 on stray punctuation, so any string typed by a user is
-- a valid input. When search_q is NULL, the clause short-circuits and the
-- tsquery expression is never evaluated — the planner reduces it to a
-- constant-false OR constant-true and prunes the @@ branch entirely.
--
-- When search_q is set the ORDER BY switches to ts_rank_cd DESC first, falling
-- back to (created_at DESC, id DESC) for tie-breaking and for the
-- deterministic-pagination rule in 12-pr-review-lessons.mdc §Indexing. When
-- search_q is NULL the CASE returns NULL uniformly and the secondary keys take
-- over, so the historic ordering is preserved bit-for-bit.
--
-- search_vector is included in the returned columns so sqlc keeps emitting
-- the canonical `Quote` row type (rather than per-query ListQuotesRow etc.).
-- The value is never consumed in Go; see the sqlc.yaml override mapping
-- tsvector -> string.
SELECT id, title, text, author_id, image_id, category_id, created_at, updated_at, search_vector
FROM quotes
WHERE (author_id = sqlc.narg('filter_author_id') OR sqlc.narg('filter_author_id') IS NULL)
  AND (category_id = sqlc.narg('filter_category_id') OR sqlc.narg('filter_category_id') IS NULL)
  AND (title ILIKE '%' || sqlc.narg('search_title') || '%' OR sqlc.narg('search_title') IS NULL)
  AND (
    sqlc.narg('filter_tag_id')::uuid IS NULL
    OR id IN (
      SELECT qt.quote_id FROM quote_tags qt
      WHERE qt.tag_id = sqlc.narg('filter_tag_id')
    )
  )
  AND (
    sqlc.narg('search_q')::text IS NULL
    OR search_vector @@ websearch_to_tsquery('english', sqlc.narg('search_q'))
  )
ORDER BY
  CASE
    WHEN sqlc.narg('search_q')::text IS NULL THEN NULL
    ELSE ts_rank_cd(search_vector, websearch_to_tsquery('english', sqlc.narg('search_q')))
  END DESC NULLS LAST,
  created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: CountQuotes :one
-- Mirrors the WHERE shape used in ListQuotes so the same plans fire. Keeping
-- the two clauses byte-identical (modulo SELECT vs count and the ORDER BY +
-- LIMIT that count doesn't need) prevents pagination totals from drifting
-- from the returned items if the filter shape ever changes again.
SELECT count(*) FROM quotes
WHERE (author_id = sqlc.narg('filter_author_id') OR sqlc.narg('filter_author_id') IS NULL)
  AND (category_id = sqlc.narg('filter_category_id') OR sqlc.narg('filter_category_id') IS NULL)
  AND (title ILIKE '%' || sqlc.narg('search_title') || '%' OR sqlc.narg('search_title') IS NULL)
  AND (
    sqlc.narg('filter_tag_id')::uuid IS NULL
    OR id IN (
      SELECT qt.quote_id FROM quote_tags qt
      WHERE qt.tag_id = sqlc.narg('filter_tag_id')
    )
  )
  AND (
    sqlc.narg('search_q')::text IS NULL
    OR search_vector @@ websearch_to_tsquery('english', sqlc.narg('search_q'))
  );

-- name: GetQuote :one
SELECT id, title, text, author_id, image_id, category_id, created_at, updated_at, search_vector
FROM quotes WHERE id = $1;

-- name: GetQuoteForKeyShare :one
SELECT id FROM quotes WHERE id = $1 FOR KEY SHARE;

-- name: CreateQuote :one
INSERT INTO quotes (title, text, author_id, image_id, category_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, title, text, author_id, image_id, category_id, created_at, updated_at, search_vector;

-- name: UpdateQuote :one
UPDATE quotes
SET title = $1, text = $2, author_id = $3, image_id = $4,
    category_id = $5, updated_at = NOW()
WHERE id = $6
RETURNING id, title, text, author_id, image_id, category_id, created_at, updated_at, search_vector;

-- name: DeleteQuote :one
DELETE FROM quotes WHERE id = $1 RETURNING id;
