ALTER TABLE tasks
    ADD COLUMN input_tokens BIGINT,
    ADD COLUMN output_tokens BIGINT,
    ADD COLUMN cache_read_tokens BIGINT,
    ADD COLUMN cache_creation_tokens BIGINT,
    ADD COLUMN cost_usd NUMERIC(12, 6);
