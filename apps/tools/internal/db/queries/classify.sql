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

-- name: ListCrossTableSlugs :many
-- Slugs that live in more than one taxonomy table. mcp.save_enrichment rejects
-- any payload naming one (slug_collision), even as reuse, so cmd/classify
-- offers none of them.
SELECT slug FROM (
    SELECT slug FROM canonical_roles
    UNION ALL SELECT slug FROM specializations
    UNION ALL SELECT slug FROM skills
    UNION ALL SELECT slug FROM role_dimensions
) t
GROUP BY slug
HAVING count(*) > 1
ORDER BY slug;

-- name: ListSkillLinkMass :many
-- Every live skill with its link count across all classifications. The seed
-- generator chooses the noul head by it.
SELECT s.slug, s.name, count(jps.classification_id)::bigint AS link_mass
FROM skills s
LEFT JOIN job_posting_skills jps ON jps.skill_id = s.id
GROUP BY s.id, s.slug, s.name
ORDER BY s.slug;

-- name: ListSeedCorpus :many
-- The most recently classified postings' latest descriptions, each with the
-- skill slugs its latest classification assigned. The seed generator measures
-- how often a skill name appears against how often it is the assigned skill.
SELECT lc.job_posting_id,
       snap.description_text::text AS description_text,
       coalesce(array_agg(sk.slug ORDER BY sk.slug) FILTER (WHERE sk.slug IS NOT NULL), '{}')::text[] AS skill_slugs
FROM latest_classifications lc
JOIN LATERAL (
    SELECT description_text
    FROM posting_snapshots
    WHERE job_posting_id = lc.job_posting_id
    ORDER BY fetched_at DESC
    LIMIT 1
) snap ON snap.description_text IS NOT NULL
LEFT JOIN job_posting_skills jps ON jps.classification_id = lc.classification_id
LEFT JOIN skills sk ON sk.id = jps.skill_id
GROUP BY lc.job_posting_id, lc.classified_at, snap.description_text
ORDER BY lc.classified_at DESC, lc.job_posting_id DESC
LIMIT @max_docs::int;

-- name: ListPostingsForClassify :many
-- Named postings with their latest snapshot's title and description, in the
-- shape selection returns. cmd/classify reads a fixed probe sample through it,
-- so repeated runs and arms classify exactly the same postings.
SELECT jp.id AS posting_id,
       jp.company_id,
       c.name AS company_name,
       coalesce(s.title, '')::text AS title,
       s.description_text::text AS description_text
FROM job_postings jp
JOIN companies c ON c.id = jp.company_id
JOIN LATERAL (
    SELECT title, description_text
    FROM posting_snapshots
    WHERE job_posting_id = jp.id
    ORDER BY fetched_at DESC
    LIMIT 1
) s ON s.description_text IS NOT NULL
WHERE jp.id = ANY(@ids::bigint[])
ORDER BY jp.id;
