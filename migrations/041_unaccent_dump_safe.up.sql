-- Make the unaccent wrapper survive a pg_dump/psql restore.
--
-- pg_dump writes `SELECT pg_catalog.set_config('search_path', '', false)` near
-- the top of every dump, so during a restore nothing resolves through
-- search_path. Migration 036 defined the body as `SELECT unaccent('unaccent',
-- $1)`, where both the function and the dictionary are unqualified. That
-- resolves fine in an application session but not during a restore:
--
--   ERROR:  function unaccent(unknown, text) does not exist
--   CONTEXT:  SQL function "botka_immutable_unaccent" during inlining
--
-- It fails while COPY fills messages.search_vector — a GENERATED column over
-- this function — which aborts the restore of the entire database. Every dump
-- taken before this migration is affected; restoring one needs the same
-- qualification patched in by hand, and only in the schema section, because
-- botka stores chat transcripts in which the same SQL text also appears as data.
--
-- Qualifying the function and casting the dictionary to a schema-qualified
-- regdictionary makes the body self-contained. The result is byte-identical, so
-- stored generated values and idx_messages_search stay valid — no rebuild.

-- The qualification below hardcodes `public`. Fail loudly rather than silently
-- writing a body that cannot resolve if the extension ever lives elsewhere.
DO $$
BEGIN
  IF (SELECT n.nspname
        FROM pg_extension e
        JOIN pg_namespace n ON n.oid = e.extnamespace
       WHERE e.extname = 'unaccent') IS DISTINCT FROM 'public' THEN
    RAISE EXCEPTION 'unaccent extension is not in schema public — update migration 041';
  END IF;
END $$;

CREATE OR REPLACE FUNCTION botka_immutable_unaccent(text)
RETURNS text
LANGUAGE sql
IMMUTABLE PARALLEL SAFE STRICT
AS $$ SELECT public.unaccent('public.unaccent'::regdictionary, $1) $$;
