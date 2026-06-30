-- Make message full-text search diacritic-insensitive so Czech queries typed
-- without diacritics (e.g. "zaluzie") match accented content ("žaluzie") and
-- vice versa. The built-in unaccent() is only STABLE, which a GENERATED column
-- and a GIN index expression cannot use, so we wrap the two-argument form
-- (which is IMMUTABLE) in a stably-named IMMUTABLE function.
CREATE EXTENSION IF NOT EXISTS unaccent;

CREATE OR REPLACE FUNCTION botka_immutable_unaccent(text)
RETURNS text
LANGUAGE sql
IMMUTABLE PARALLEL SAFE STRICT
AS $$ SELECT unaccent('unaccent', $1) $$;

-- Rebuild the generated search_vector column to fold diacritics. Dropping the
-- column also drops idx_messages_search, so recreate it afterwards. The
-- messages table is small, so the rewrite is effectively instant.
DROP INDEX IF EXISTS idx_messages_search;
ALTER TABLE messages DROP COLUMN IF EXISTS search_vector;
ALTER TABLE messages ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('pg_catalog.simple', botka_immutable_unaccent(content))) STORED;
CREATE INDEX idx_messages_search ON messages USING GIN (search_vector);
