DROP INDEX IF EXISTS ix_quotes_search_vector;
ALTER TABLE quotes DROP COLUMN IF EXISTS search_vector;
