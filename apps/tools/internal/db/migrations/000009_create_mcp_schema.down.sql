-- Drops the `mcp` schema baseline.
-- RESTRICT (not CASCADE): the baseline migration creates no objects, so RESTRICT
-- succeeds cleanly here. If later migrations that add mcp functions are reverted
-- first (the normal down-migration order), the schema is empty by the time this
-- runs. RESTRICT is the safe default: it refuses to silently drop functions that
-- a later migration created but whose own down-migration was skipped, surfacing
-- the ordering mistake instead of destroying the action boundary.
DROP SCHEMA IF EXISTS mcp RESTRICT;
