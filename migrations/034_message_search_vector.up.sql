-- Cross-thread full-text search across chat messages.
--
-- The original schema added a generated tsvector column `tsv` using the
-- 'english' config, but Botka's content is mixed Czech/English (and includes
-- technical terms like "VAPID"). 'english' applies stemming and stopword
-- removal that hurts mixed-language search, so we replace it with a 'simple'
-- column named `search_vector` to match the convention already in use on the
-- `tasks` table (migration 030).
--
-- The column is GENERATED ALWAYS, so app code never has to maintain it.
DROP INDEX IF EXISTS idx_messages_tsv;
ALTER TABLE messages DROP COLUMN IF EXISTS tsv;

ALTER TABLE messages ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('pg_catalog.simple', content)) STORED;
CREATE INDEX idx_messages_search ON messages USING GIN (search_vector);
