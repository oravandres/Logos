CREATE TABLE quote_tags (
    quote_id UUID NOT NULL REFERENCES quotes(id) ON DELETE CASCADE,
    tag_id   UUID NOT NULL REFERENCES tags(id)   ON DELETE CASCADE,
    PRIMARY KEY (quote_id, tag_id)
);

CREATE INDEX ix_quote_tags_tag_id ON quote_tags (tag_id);
