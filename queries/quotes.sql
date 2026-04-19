-- name: ListQuotes :many
-- The tag filter is expressed as `id IN (SELECT ...)` rather than a correlated
-- EXISTS so PostgreSQL can hash the inner result once and run it as a hashed
-- semi-join driven from ix_quote_tags_tag_id. With a correlated EXISTS the
-- planner re-probes quote_tags_pkey per outer row (~100K loops, ~300K buffers
-- on a 100K-quote dataset for a sparse tag); the IN form drops to ~1.2K
-- buffers regardless of tag selectivity. See PR #14 EXPLAIN ANALYZE evidence.
SELECT id, title, text, author_id, image_id, category_id, created_at, updated_at
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
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: CountQuotes :one
-- Mirrors the IN-subquery shape used in ListQuotes so the same hashed semi-join
-- plan fires. Keeping the two clauses byte-identical (modulo SELECT vs count)
-- prevents pagination totals from drifting from the returned items if the
-- filter shape ever changes again.
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
  );

-- name: GetQuote :one
SELECT id, title, text, author_id, image_id, category_id, created_at, updated_at
FROM quotes WHERE id = $1;

-- name: GetQuoteForKeyShare :one
SELECT id FROM quotes WHERE id = $1 FOR KEY SHARE;

-- name: CreateQuote :one
INSERT INTO quotes (title, text, author_id, image_id, category_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, title, text, author_id, image_id, category_id, created_at, updated_at;

-- name: UpdateQuote :one
UPDATE quotes
SET title = $1, text = $2, author_id = $3, image_id = $4,
    category_id = $5, updated_at = NOW()
WHERE id = $6
RETURNING id, title, text, author_id, image_id, category_id, created_at, updated_at;

-- name: DeleteQuote :one
DELETE FROM quotes WHERE id = $1 RETURNING id;
