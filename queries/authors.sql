-- name: ListAuthors :many
SELECT id, name, bio, born_date, died_date, image_id, category_id, created_at, updated_at
FROM authors
WHERE (category_id = sqlc.narg('filter_category_id') OR sqlc.narg('filter_category_id') IS NULL)
  AND (name ILIKE '%' || sqlc.narg('search_name') || '%' OR sqlc.narg('search_name') IS NULL)
ORDER BY name ASC
LIMIT $1 OFFSET $2;

-- name: CountAuthors :one
SELECT count(*) FROM authors
WHERE (category_id = sqlc.narg('filter_category_id') OR sqlc.narg('filter_category_id') IS NULL)
  AND (name ILIKE '%' || sqlc.narg('search_name') || '%' OR sqlc.narg('search_name') IS NULL);

-- name: GetAuthor :one
SELECT id, name, bio, born_date, died_date, image_id, category_id, created_at, updated_at
FROM authors WHERE id = $1;

-- name: CreateAuthor :one
INSERT INTO authors (name, bio, born_date, died_date, image_id, category_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, name, bio, born_date, died_date, image_id, category_id, created_at, updated_at;

-- name: UpdateAuthor :one
UPDATE authors
SET name = $1, bio = $2, born_date = $3, died_date = $4,
    image_id = $5, category_id = $6, updated_at = NOW()
WHERE id = $7
RETURNING id, name, bio, born_date, died_date, image_id, category_id, created_at, updated_at;

-- name: DeleteAuthor :exec
DELETE FROM authors WHERE id = $1;
