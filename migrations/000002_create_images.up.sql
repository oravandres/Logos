CREATE TABLE images (
    id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    url         VARCHAR(2048) NOT NULL,
    alt_text    VARCHAR(500),
    category_id UUID          REFERENCES categories(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX ix_images_category_id ON images (category_id);
