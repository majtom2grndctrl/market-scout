-- name: ListNearDuplicatePairs :many
-- Pairs among the given terms that mcp.save_enrichment's within-payload dedup
-- (Phase C, 000043) would treat as one concept: the greater of slug and name
-- similarity at or above its c_payload_dup_at. cmd/classify drops the less
-- probable side of each pair before saving, so the save has nothing to drop.
-- terms is a JSON array of {"slug","name"}; indexes are 1-based positions in it.
SELECT a.ord::int AS left_index, b.ord::int AS right_index
FROM jsonb_array_elements(@terms::jsonb) WITH ORDINALITY AS a(term, ord)
JOIN jsonb_array_elements(@terms::jsonb) WITH ORDINALITY AS b(term, ord)
  ON b.ord > a.ord
WHERE greatest(
          public.similarity(a.term ->> 'slug', b.term ->> 'slug'),
          public.similarity(a.term ->> 'name', b.term ->> 'name'))::numeric
      >= @threshold::numeric
ORDER BY a.ord, b.ord;
