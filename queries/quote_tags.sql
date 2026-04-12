-- name: ListTagsByQuote :many
SELECT t.id, t.name, t.created_at
FROM tags t
JOIN quote_tags qt ON qt.tag_id = t.id
WHERE qt.quote_id = $1
ORDER BY t.name;

-- name: ListQuotesByTag :many
SELECT q.id, q.title, q.text, q.author_id, q.image_id, q.category_id, q.created_at, q.updated_at
FROM quotes q
JOIN quote_tags qt ON qt.quote_id = q.id
WHERE qt.tag_id = $1
ORDER BY q.created_at DESC, q.id DESC
LIMIT $2 OFFSET $3;

-- name: CountQuotesByTag :one
SELECT count(*)
FROM quote_tags
WHERE tag_id = $1;

-- name: AddTagToQuote :exec
INSERT INTO quote_tags (quote_id, tag_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveTagFromQuote :exec
DELETE FROM quote_tags
WHERE quote_id = $1 AND tag_id = $2;

-- name: ReplaceQuoteTags :exec
DELETE FROM quote_tags WHERE quote_id = $1;
