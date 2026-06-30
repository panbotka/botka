-- Reweight the task full-text search vector so ranking prioritises the title,
-- then the spec (description), then the failure summary, and fold diacritics so
-- accent-free queries match (consistent with migration 036 for messages).
--
-- The old tsvector_update_trigger (migration 030) cannot apply setweight or
-- unaccent, so replace it with a custom trigger function.
CREATE OR REPLACE FUNCTION tasks_search_vector_refresh() RETURNS trigger AS $$
BEGIN
  NEW.search_vector :=
    setweight(to_tsvector('pg_catalog.simple', botka_immutable_unaccent(coalesce(NEW.title, ''))), 'A') ||
    setweight(to_tsvector('pg_catalog.simple', botka_immutable_unaccent(coalesce(NEW.spec, ''))), 'B') ||
    setweight(to_tsvector('pg_catalog.simple', botka_immutable_unaccent(coalesce(NEW.failure_summary, ''))), 'C');
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS tasks_search_vector_update ON tasks;
CREATE TRIGGER tasks_search_vector_update
  BEFORE INSERT OR UPDATE OF title, spec, failure_summary ON tasks
  FOR EACH ROW EXECUTE FUNCTION tasks_search_vector_refresh();

-- Backfill existing rows with the new weighted, unaccented vector.
UPDATE tasks SET search_vector =
    setweight(to_tsvector('pg_catalog.simple', botka_immutable_unaccent(coalesce(title, ''))), 'A') ||
    setweight(to_tsvector('pg_catalog.simple', botka_immutable_unaccent(coalesce(spec, ''))), 'B') ||
    setweight(to_tsvector('pg_catalog.simple', botka_immutable_unaccent(coalesce(failure_summary, ''))), 'C');
