-- Extends the closed role_dimensions seed set (established in 000001) with 'sales'.
-- role_dimensions is the only sanctioned extension point for dimensions; agents cannot
-- extend it at runtime. batch-enrich rejects any dimension slug not present in the
-- taxonomy loaded at invocation start (SKILL.md §4).
-- Idempotent: ON CONFLICT (slug) DO NOTHING makes repeated runs safe.
INSERT INTO role_dimensions (slug, name) VALUES
    ('sales', 'Sales')
ON CONFLICT (slug) DO NOTHING;
