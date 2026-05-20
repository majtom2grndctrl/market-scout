-- Adds 'legal-engineer' as a canonical role for legal-AI companies (e.g. Harvey).
-- The role bridges legal domain expertise with technical implementation and is
-- distinct from Solutions Engineer (pre-sales) and Customer Success Manager (post-sales).
-- Idempotent: ON CONFLICT (slug) DO NOTHING makes repeated runs safe.
INSERT INTO canonical_roles (slug, name) VALUES
    ('legal-engineer', 'Legal Engineer')
ON CONFLICT (slug) DO NOTHING;
