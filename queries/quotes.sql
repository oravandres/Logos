-- name: ListQuotes :many
SELECT id, title, text, author_id, image_id, category_id, created_at, updated_at
FROM quotes
WHERE (author_id = sqlc.narg('filter_author_id') OR sqlc.narg('filter_author_id') IS NULL)
  AND (category_id = sqlc.narg('filter_category_id') OR sqlc.narg('filter_category_id') IS NULL)
  AND (title ILIKE '%' || sqlc.narg('search_title') || '%' OR sqlc.narg('search_title') IS NULL)
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: CountQuotes :one
SELECT count(*) FROM quotes
WHERE (author_id = sqlc.narg('filter_author_id') OR sqlc.narg('filter_author_id') IS NULL)
  AND (category_id = sqlc.narg('filter_category_id') OR sqlc.narg('filter_category_id') IS NULL)
  AND (title ILIKE '%' || sqlc.narg('search_title') || '%' OR sqlc.narg('search_title') IS NULL);

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

-- name: DeleteQuote :exec
DELETE FROM quotes WHERE id = $1;
