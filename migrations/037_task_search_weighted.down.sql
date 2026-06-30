-- Restore the unweighted, accent-sensitive task search vector (migration 030).
DROP TRIGGER IF EXISTS tasks_search_vector_update ON tasks;
DROP FUNCTION IF EXISTS tasks_search_vector_refresh();

CREATE TRIGGER tasks_search_vector_update
  BEFORE INSERT OR UPDATE OF title, spec, failure_summary ON tasks
  FOR EACH ROW EXECUTE FUNCTION
  tsvector_update_trigger(search_vector, 'pg_catalog.simple', title, spec, failure_summary);

UPDATE tasks SET search_vector = to_tsvector(
  'pg_catalog.simple',
  coalesce(title, '') || ' ' || coalesce(spec, '') || ' ' || coalesce(failure_summary, ''));
