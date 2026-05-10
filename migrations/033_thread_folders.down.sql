DROP INDEX IF EXISTS idx_threads_folder_id;
ALTER TABLE threads DROP COLUMN IF EXISTS folder_id;
DROP TABLE IF EXISTS thread_folders;
