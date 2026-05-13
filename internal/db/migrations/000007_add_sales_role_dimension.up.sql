-- Adds the 'sales' role dimension to the closed seed set established in 000001.
-- Idempotent: ON CONFLICT (slug) DO NOTHING makes repeated runs safe.
INSERT INTO role_dimensions (slug, name) VALUES
    ('sales', 'Sales')
ON CONFLICT (slug) DO NOTHING;
