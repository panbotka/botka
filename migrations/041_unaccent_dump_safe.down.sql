-- Restore the unqualified body from migration 036. Note this reintroduces the
-- defect that makes a pg_dump of this database unrestorable — the down
-- migration exists for symmetry, not because rolling back is advisable.
CREATE OR REPLACE FUNCTION botka_immutable_unaccent(text)
RETURNS text
LANGUAGE sql
IMMUTABLE PARALLEL SAFE STRICT
AS $$ SELECT unaccent('unaccent', $1) $$;
