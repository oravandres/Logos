DROP TRIGGER IF EXISTS trg_category_type_guard_authors ON categories;
DROP FUNCTION IF EXISTS prevent_category_type_change_if_referenced_by_authors();
DROP TABLE IF EXISTS authors;
DROP FUNCTION IF EXISTS check_author_category_type();
