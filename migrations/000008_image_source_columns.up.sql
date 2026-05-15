-- Image source provenance + per-blob metadata.
--
-- Why these columns now?
--   * `source` lets the API distinguish "user pasted a URL we don't own"
--     from "we own the bytes" (uploaded or generated). The DELETE handler
--     uses it to decide whether to also unlink the blob from the local
--     blobstore; the GET .../blob handler uses it to short-circuit
--     external-url rows with a 404 instead of trying to read disk.
--   * `content_type`, `size_bytes`, `width`, `height` are decoded once at
--     ingest and stored alongside the row so list views and previews can
--     render dimensions and an Accept-correct response without re-sniffing
--     bytes on every read.
--   * `prompt`, `model`, `seed`, `generated_at` are an audit trail for
--     `source = 'generated'`; left NULL for the other two sources.
--
-- Existing rows (source='external_url' by default) keep working — the URL
-- text on disk is still the source of truth for them.
ALTER TABLE images
    ADD COLUMN source        VARCHAR(32)  NOT NULL DEFAULT 'external_url',
    ADD COLUMN content_type  VARCHAR(127),
    ADD COLUMN size_bytes    BIGINT,
    ADD COLUMN width         INTEGER,
    ADD COLUMN height        INTEGER,
    ADD COLUMN prompt        TEXT,
    ADD COLUMN model         VARCHAR(127),
    ADD COLUMN seed          BIGINT,
    ADD COLUMN generated_at  TIMESTAMPTZ;

ALTER TABLE images
    ADD CONSTRAINT ck_images_source
    CHECK (source IN ('external_url', 'uploaded', 'generated'));

-- Most callers will keep filtering by category; `source` is a low-cardinality
-- discriminator and the small images corpus does not need its own index yet.
-- Re-evaluate if a "show me everything I generated last week" query lands.
