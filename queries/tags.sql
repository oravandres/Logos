-- name: ListTags :many
SELECT id, name, created_at FROM tags
ORDER BY name
LIMIT $1 OFFSET $2;

-- name: CountTags :one
SELECT count(*) FROM tags;

-- name: GetTag :one
SELECT id, name, created_at FROM tags WHERE id = $1;

-- name: CreateTag :one
INSERT INTO tags (name)
VALUES ($1)
RETURNING id, name, created_at;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = $1;
