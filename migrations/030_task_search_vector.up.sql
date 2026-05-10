-- Full-text search across tasks: title, spec, and failure_summary.
--
-- Uses the 'simple' text search config (no stemming or stopword removal) since
-- task content is mixed Czech/English and we want exact matches on technical
-- terms like "VAPID" or "tsvector".
--
-- Maintained via tsvector_update_trigger so app code never has to touch the
-- column. The trigger fires on INSERT and UPDATE, automatically rebuilding the
-- vector whenever any of the source fields change.

ALTER TABLE tasks ADD COLUMN search_vector tsvector;

CREATE INDEX idx_tasks_search ON tasks USING GIN (search_vector);

CREATE TRIGGER tasks_search_vector_update
    BEFORE INSERT OR UPDATE OF title, spec, failure_summary ON tasks
    FOR EACH ROW EXECUTE FUNCTION
    tsvector_update_trigger(search_vector, 'pg_catalog.simple', title, spec, failure_summary);

-- Backfill existing rows. The trigger only fires on future writes, so the
-- column would be NULL for everything currently in the table.
UPDATE tasks
SET search_vector = to_tsvector(
    'pg_catalog.simple',
    coalesce(title, '') || ' ' || coalesce(spec, '') || ' ' || coalesce(failure_summary, '')
);
