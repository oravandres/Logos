DROP TRIGGER IF EXISTS trg_category_type_guard_quotes ON categories;
DROP FUNCTION IF EXISTS prevent_category_type_change_if_referenced_by_quotes();
DROP TABLE IF EXISTS quotes;
DROP FUNCTION IF EXISTS check_quote_category_type();
