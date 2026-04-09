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

CREATE INDEX ix_authors_name ON authors (name);
CREATE INDEX ix_authors_category_id ON authors (category_id);
