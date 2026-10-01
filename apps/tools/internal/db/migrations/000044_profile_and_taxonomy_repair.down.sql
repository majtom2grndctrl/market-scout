-- Reverse 000044.
--
-- Drops the profile -- every past title, claimed skill, and pin entered since
-- -- and the repair functions with their archive. The profile has no other
-- copy; that loss is the accepted undo cost of this migration.
--
-- Retired-slug records a repair wrote stay, as ordinary records. A repair
-- migration written after this one runs its own down, and so its undo, before
-- this file runs; anything still standing was a repair whose term is gone, and
-- dropping its record would let the slug be minted again.

DROP FUNCTION public.taxonomy_undo(bigint);
DROP FUNCTION public.taxonomy_retire(text, jsonb, text);
DROP FUNCTION public.taxonomy_merge(text, jsonb, text);
DROP FUNCTION public.taxonomy_repair_begin(text, text);
DROP FUNCTION public.taxonomy_repair_unhandled_references();

ALTER TABLE retired_slugs DROP COLUMN retired_by_repair;

DROP TABLE taxonomy_repair_role_dimensions;
DROP TABLE taxonomy_repair_links;
DROP TABLE taxonomy_repair_terms;
DROP TABLE taxonomy_repairs;

DROP TABLE app.pins;
DROP TABLE app.claimed_skills;
DROP TABLE app.past_titles;
DROP SCHEMA app;
