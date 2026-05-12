-- WARNING: drops all tables and the vector extension. Not a reversible rollback —
-- this migration was destructively rewritten and cannot restore a prior schema state.
-- Recover by applying migrate up on an empty database.
DROP TABLE IF EXISTS job_posting_skills;
DROP TABLE IF EXISTS job_posting_specializations;
DROP TABLE IF EXISTS job_posting_roles;
DROP TABLE IF EXISTS classifications;
DROP TABLE IF EXISTS canonical_role_dimensions;
DROP TABLE IF EXISTS role_dimensions;
DROP TABLE IF EXISTS canonical_roles;
DROP TABLE IF EXISTS specializations;
DROP TABLE IF EXISTS skills;
DROP TABLE IF EXISTS posting_snapshots;
DROP TABLE IF EXISTS job_postings;
DROP TABLE IF EXISTS companies;
DROP EXTENSION IF EXISTS vector;
