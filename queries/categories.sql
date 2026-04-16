-- name: ListCategories :many
SELECT id, name, type, created_at FROM categories
WHERE (type = sqlc.narg('filter_type') OR sqlc.narg('filter_type') IS NULL)
ORDER BY name
LIMIT $1 OFFSET $2;

-- name: CountCategories :one
SELECT count(*) FROM categories
WHERE (type = sqlc.narg('filter_type') OR sqlc.narg('filter_type') IS NULL);

-- name: GetCategory :one
SELECT id, name, type, created_at FROM categories WHERE id = $1;

-- name: CreateCategory :one
INSERT INTO categories (name, type)
VALUES ($1, $2)
RETURNING id, name, type, created_at;

-- name: UpdateCategory :one
UPDATE categories
SET name = $1, type = $2
WHERE id = $3
RETURNING id, name, type, created_at;

-- name: DeleteCategory :one
DELETE FROM categories WHERE id = $1 RETURNING id;
