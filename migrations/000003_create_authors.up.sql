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

CREATE OR REPLACE FUNCTION check_author_category_type()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.category_id IS NOT NULL THEN
        PERFORM 1 FROM categories WHERE id = NEW.category_id AND type = 'author';
        IF NOT FOUND THEN
            RAISE EXCEPTION 'category must have type author'
                USING ERRCODE = '23514',
                      CONSTRAINT = 'chk_authors_category_type';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_authors_category_type
    BEFORE INSERT OR UPDATE ON authors
    FOR EACH ROW EXECUTE FUNCTION check_author_category_type();

CREATE OR REPLACE FUNCTION prevent_category_type_change_if_referenced_by_authors()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.type = 'author' AND NEW.type IS DISTINCT FROM OLD.type THEN
        IF EXISTS (SELECT 1 FROM authors WHERE category_id = NEW.id) THEN
            RAISE EXCEPTION 'cannot change category type while referenced by authors'
                USING ERRCODE = '23514',
                      CONSTRAINT = 'chk_category_type_in_use_by_authors';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_category_type_guard_authors
    BEFORE UPDATE ON categories
    FOR EACH ROW EXECUTE FUNCTION prevent_category_type_change_if_referenced_by_authors();
