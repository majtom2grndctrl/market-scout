-- Removes the 'sales' role dimension.
-- If any canonical role has been mapped to 'sales', the RESTRICT FK
-- (canonical_role_dimensions.dimension_id → role_dimensions.id ON DELETE RESTRICT)
-- will block this delete. That block is intentional: historical provenance surfaces
-- rather than being silently dropped.
-- To force removal: first delete rows from canonical_role_dimensions where
-- dimension_id = (SELECT id FROM role_dimensions WHERE slug = 'sales'), then re-run.
DELETE FROM role_dimensions WHERE slug = 'sales';
