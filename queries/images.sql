-- name: ListImages :many
SELECT id, url, alt_text, category_id, source, content_type, size_bytes,
       width, height, prompt, model, seed, generated_at, created_at, updated_at
FROM images
WHERE (category_id = sqlc.narg('filter_category_id') OR sqlc.narg('filter_category_id') IS NULL)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountImages :one
SELECT count(*) FROM images
WHERE (category_id = sqlc.narg('filter_category_id') OR sqlc.narg('filter_category_id') IS NULL);

-- name: GetImage :one
SELECT id, url, alt_text, category_id, source, content_type, size_bytes,
       width, height, prompt, model, seed, generated_at, created_at, updated_at
FROM images WHERE id = $1;

-- name: CreateImage :one
-- External-URL flow (today's behavior). `source` defaults to 'external_url'
-- in the schema; left implicit so the existing JSON CRUD handler is
-- unchanged on the wire.
INSERT INTO images (url, alt_text, category_id)
VALUES ($1, $2, $3)
RETURNING id, url, alt_text, category_id, source, content_type, size_bytes,
          width, height, prompt, model, seed, generated_at, created_at, updated_at;

-- name: CreateUploadedImage :one
-- Multipart upload flow: server has already written the blob to disk and
-- decoded its dimensions. Caller supplies the canonical id (so the URL
-- on the row matches the on-disk filename) and the audit columns.
INSERT INTO images (id, url, alt_text, category_id, source,
                    content_type, size_bytes, width, height)
VALUES ($1, $2, $3, $4, 'uploaded', $5, $6, $7, $8)
RETURNING id, url, alt_text, category_id, source, content_type, size_bytes,
          width, height, prompt, model, seed, generated_at, created_at, updated_at;

-- name: CreateGeneratedImage :one
-- Generation flow: Logos has just downloaded the bytes from a generator
-- (DarkBase image-adapter today, Sparky later). Same blob layout as the
-- uploaded path; additional audit columns are non-NULL.
INSERT INTO images (id, url, alt_text, category_id, source,
                    content_type, size_bytes, width, height,
                    prompt, model, seed, generated_at)
VALUES ($1, $2, $3, $4, 'generated', $5, $6, $7, $8, $9, $10, $11, NOW())
RETURNING id, url, alt_text, category_id, source, content_type, size_bytes,
          width, height, prompt, model, seed, generated_at, created_at, updated_at;

-- name: UpdateImage :one
-- Updates only the user-editable surface (url / alt_text / category).
-- Source-specific audit columns (content_type, prompt, …) are immutable
-- post-ingest by design — re-running generation should produce a new row.
UPDATE images
SET url = $1, alt_text = $2, category_id = $3, updated_at = NOW()
WHERE id = $4
RETURNING id, url, alt_text, category_id, source, content_type, size_bytes,
          width, height, prompt, model, seed, generated_at, created_at, updated_at;

-- name: DeleteImage :one
-- Returns id + source so the handler can decide whether to also remove
-- the blob from the local blobstore (for 'uploaded' and 'generated'
-- rows; 'external_url' rows have no on-disk artifact to clean up).
DELETE FROM images WHERE id = $1 RETURNING id, source;
