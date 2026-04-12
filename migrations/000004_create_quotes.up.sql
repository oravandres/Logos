CREATE TABLE quotes (
    id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    title       VARCHAR(500)  NOT NULL,
    text        TEXT          NOT NULL,
    author_id   UUID          NOT NULL REFERENCES authors(id),
    image_id    UUID          REFERENCES images(id) ON DELETE SET NULL,
    category_id UUID          REFERENCES categories(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX ix_quotes_author_id ON quotes (author_id);
CREATE INDEX ix_quotes_image_id ON quotes (image_id);
CREATE INDEX ix_quotes_category_id ON quotes (category_id);
CREATE INDEX ix_quotes_title_trgm ON quotes USING gin (title gin_trgm_ops);
CREATE INDEX ix_quotes_created_at ON quotes (created_at);

CREATE OR REPLACE FUNCTION check_quote_category_type()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.category_id IS NOT NULL THEN
        PERFORM 1 FROM categories WHERE id = NEW.category_id AND type = 'quote' FOR SHARE;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'category must have type quote'
                USING ERRCODE = '23514',
                      CONSTRAINT = 'chk_quotes_category_type';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_quotes_category_type
    BEFORE INSERT OR UPDATE ON quotes
    FOR EACH ROW EXECUTE FUNCTION check_quote_category_type();

CREATE OR REPLACE FUNCTION prevent_category_type_change_if_referenced_by_quotes()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.type = 'quote' AND NEW.type IS DISTINCT FROM OLD.type THEN
        IF EXISTS (SELECT 1 FROM quotes WHERE category_id = NEW.id) THEN
            RAISE EXCEPTION 'cannot change category type while referenced by quotes'
                USING ERRCODE = '23514',
                      CONSTRAINT = 'chk_category_type_in_use_by_quotes';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_category_type_guard_quotes
    BEFORE UPDATE ON categories
    FOR EACH ROW EXECUTE FUNCTION prevent_category_type_change_if_referenced_by_quotes();
