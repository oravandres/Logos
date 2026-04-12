CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE authors (
    id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255)  NOT NULL,
    bio         TEXT,
    born_date   DATE,
    died_date   DATE,
    image_id    UUID          REFERENCES images(id) ON DELETE SET NULL,
    category_id UUID          REFERENCES categories(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX ix_authors_name_trgm ON authors USING gin (name gin_trgm_ops);
CREATE INDEX ix_authors_category_id ON authors (category_id);
