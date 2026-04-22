-- Full-text search over quotes (title + text) via a stored tsvector column.
--
-- Why a GENERATED ... STORED column rather than a trigger-maintained column?
--   * Declarative: the column can never drift from (title, text). No out-of-
--     band sync hazard; no trigger to reason about.
--   * Zero write overhead over what a trigger would cost: Postgres recomputes
--     the vector on INSERT/UPDATE to (title, text) either way.
--   * Indexable directly; no intermediate VIEW or computed-expression index.
--
-- Weighting: title is promoted to weight 'A' and body text to 'B', so
-- ts_rank_cd in ListQuotes sorts title hits above body-only hits. The 'english'
-- text search configuration is hard-coded for v1; the corpus is English-majority
-- and introducing a per-row search_config would add moving parts without a
-- concrete driver. If we ever need multilingual search, switch to a per-row
-- regconfig column and recompute the vector from it.
--
-- Operational note: adding a STORED generated column rewrites the table in
-- Postgres 16 (one-shot full-table scan + GIN build). Sub-second on the
-- current corpus (<10K rows on the cluster). The migration runs inside the
-- logos-api init container under strategy: Recreate / replicas: 1, so the
-- rewrite fits inside the already-documented ~10s rollout gap (see MiMi
-- .cursor/rules/12-image-pinning-and-gitops.mdc §Post-merge).
ALTER TABLE quotes ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(text,  '')), 'B')
    ) STORED;

CREATE INDEX ix_quotes_search_vector ON quotes USING gin (search_vector);
