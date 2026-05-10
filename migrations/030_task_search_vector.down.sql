DROP TRIGGER IF EXISTS tasks_search_vector_update ON tasks;
DROP INDEX IF EXISTS idx_tasks_search;
ALTER TABLE tasks DROP COLUMN IF EXISTS search_vector;
