-- name: ListImages :many
SELECT id, url, alt_text, category_id, created_at, updated_at FROM images
WHERE (category_id = sqlc.narg('filter_category_id') OR sqlc.narg('filter_category_id') IS NULL)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountImages :one
SELECT count(*) FROM images
WHERE (category_id = sqlc.narg('filter_category_id') OR sqlc.narg('filter_category_id') IS NULL);

-- name: GetImage :one
SELECT id, url, alt_text, category_id, created_at, updated_at FROM images WHERE id = $1;

-- name: CreateImage :one
INSERT INTO images (url, alt_text, category_id)
VALUES ($1, $2, $3)
RETURNING id, url, alt_text, category_id, created_at, updated_at;

-- name: UpdateImage :one
UPDATE images
SET url = $1, alt_text = $2, category_id = $3, updated_at = NOW()
WHERE id = $4
RETURNING id, url, alt_text, category_id, created_at, updated_at;

-- name: DeleteImage :one
DELETE FROM images WHERE id = $1 RETURNING id;
