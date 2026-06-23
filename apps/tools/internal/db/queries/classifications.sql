-- name: GetRoleDimensionBySlug :one
SELECT id, slug, name FROM role_dimensions WHERE slug = $1;

-- GetOrCreate pattern: ON CONFLICT DO UPDATE SET slug = EXCLUDED.slug — the no-op update
-- takes a row lock and always returns the existing row, avoiding a race under READ COMMITTED.
-- Only `slug` is SET (a no-op), so `created_at` and `name` on the existing row are preserved.
-- created_at is the emergent-vs-seeded discriminant across canonical_roles, specializations,
-- and skills — do not change this to SET name or created_at.

-- name: GetOrCreateCanonicalRole :one
INSERT INTO canonical_roles (slug, name) VALUES (@slug, @name)
ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
RETURNING id;

-- name: GetOrCreateSpecialization :one
INSERT INTO specializations (slug, name) VALUES (@slug, @name)
ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
RETURNING id;

-- name: GetOrCreateSkill :one
INSERT INTO skills (slug, name) VALUES (@slug, @name)
ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
RETURNING id;

-- Idempotent insert — ON CONFLICT DO NOTHING; does not update existing mappings.
-- name: InsertCanonicalRoleDimension :exec
INSERT INTO canonical_role_dimensions (canonical_role_id, dimension_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: InsertClassification :one
INSERT INTO classifications (job_posting_id, model, prompt_version, seniority, notes)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: InsertJobPostingRole :exec
INSERT INTO job_posting_roles (classification_id, role_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: InsertJobPostingSpecialization :exec
INSERT INTO job_posting_specializations (classification_id, specialization_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: InsertJobPostingSkill :exec
INSERT INTO job_posting_skills (classification_id, skill_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: GetCurrentClassificationForPosting :one
SELECT id, job_posting_id, model, prompt_version, classified_at, seniority, notes
FROM classifications
WHERE job_posting_id = $1
ORDER BY classified_at DESC
LIMIT 1;

-- SaveEnrichment calls the approved mcp.save_enrichment SECURITY DEFINER function,
-- which owns the full classifier writeback path (get-or-create taxonomy, insert one
-- classifications row, attach join rows) and returns a JSON envelope. Unlike
-- mcp.add_company (a multi-column RETURNS TABLE that sqlc's offline parser cannot
-- expand), this is a scalar jsonb return, so it is safe to route through sqlc and
-- keep codegen offline. The MCP action tool binds this against the action pool.
-- name: SaveEnrichment :one
SELECT mcp.save_enrichment($1, $2, $3)::jsonb AS result;

-- Full-table DISTINCT ON — no filter or limit. Suitable for local/debug use;
-- production callers should add a job_posting_id filter or pagination parameters.
-- name: ListCurrentClassifications :many
SELECT DISTINCT ON (job_posting_id) id, job_posting_id, model, prompt_version, classified_at, seniority, notes
FROM classifications
ORDER BY job_posting_id, classified_at DESC;
