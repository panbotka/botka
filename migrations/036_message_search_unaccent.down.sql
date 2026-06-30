-- Revert the message search_vector to the accent-sensitive definition.
DROP INDEX IF EXISTS idx_messages_search;
ALTER TABLE messages DROP COLUMN IF EXISTS search_vector;
ALTER TABLE messages ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('pg_catalog.simple', content)) STORED;
CREATE INDEX idx_messages_search ON messages USING GIN (search_vector);

DROP FUNCTION IF EXISTS botka_immutable_unaccent(text);
-- Leave the unaccent extension installed; it is harmless and may be in use
-- elsewhere.
