-- Removes the 'sales' role dimension.
-- No CASCADE: the RESTRICT FK on canonical_role_dimensions will block this delete
-- if any canonical role has been mapped to 'sales'. That FK violation is expected
-- and acceptable — historical references should surface rather than be silently dropped.
DELETE FROM role_dimensions WHERE slug = 'sales';
