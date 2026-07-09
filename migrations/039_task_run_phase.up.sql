-- run_phase names the executor step a running task is currently in.
-- It is meaningful only while status = 'running'; every terminal status
-- write clears it back to NULL.
ALTER TABLE tasks
    ADD COLUMN run_phase TEXT;
