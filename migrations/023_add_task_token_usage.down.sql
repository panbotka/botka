ALTER TABLE tasks
    DROP COLUMN input_tokens,
    DROP COLUMN output_tokens,
    DROP COLUMN cache_read_tokens,
    DROP COLUMN cache_creation_tokens,
    DROP COLUMN cost_usd;
