-- Reverse 000037 in full.
--
-- The up migration deleted 71 taxonomy rows and moved 670 join rows, so unlike
-- 000030 this one cannot be reversed from the file alone. retired_slug_links
-- is what makes it reversible: the up migration wrote every deleted row's id,
-- slug, name and created_at there, along with every link it moved and whether
-- the survivor already held that pair. This file reads that record back and
-- drops the table afterward.
--
-- Original ids are restored, not regenerated. Nothing else references
-- skills.id or specializations.id -- only the two join tables, which this file
-- rebuilds -- and the identity sequences have already advanced past the
-- deleted ids, so reinserting them cannot collide with a row minted since. The
-- sequences are then resynchronised, because an explicit-id INSERT does not
-- advance them and the next mint would otherwise fail on a duplicate key.
--
-- Order is load-bearing in one place: the restored rows have to exist before
-- their links can point at them, and both have to be back before the
-- retired_slugs entries and the archive itself are removed.

-- ---------------------------------------------------------------------------
-- 1. Restore the names 000037 repaired. Every prior value is recorded here;
-- none of them was blank, so the 000030 CHECK constraints stay in place.
-- ---------------------------------------------------------------------------

UPDATE skills SET name = 'Analytics & Data-Driven Problem Solving'      WHERE slug = 'analytical';
UPDATE skills SET name = 'Software Architecture'                        WHERE slug = 'architecture';
UPDATE skills SET name = 'SSO, SAML, OAuth Configuration'               WHERE slug = 'authentication';
UPDATE skills SET name = 'Financial Analysis'                           WHERE slug = 'balance-sheet-reconciliation';
UPDATE skills SET name = 'Candidate Relationship Management'            WHERE slug = 'candidate-management';
UPDATE skills SET name = 'Leadership Coaching'                          WHERE slug = 'coaching';
UPDATE skills SET name = 'Technical Communication'                      WHERE slug = 'communication';
UPDATE skills SET name = 'Deal Negotiation'                             WHERE slug = 'complex-deal-negotiation';
UPDATE skills SET name = 'Sales Negotiations & Deal Closure'            WHERE slug = 'complex-sales-negotiations';
UPDATE skills SET name = 'Stakeholder Coordination'                     WHERE slug = 'complex-stakeholder-navigation';
UPDATE skills SET name = 'Corrosion Control'                            WHERE slug = 'corrosion-science';
UPDATE skills SET name = 'Problem Solving'                              WHERE slug = 'critical-thinking';
UPDATE skills SET name = 'Customer Discovery'                           WHERE slug = 'customer-engagement';
UPDATE skills SET name = 'Solutions Engineering'                        WHERE slug = 'customer-solutions-engineering';
UPDATE skills SET name = 'Statistical Analysis and Experimental Design' WHERE slug = 'data-analysis';
UPDATE skills SET name = 'Data-driven decision making'                  WHERE slug = 'data-driven-workflows';
UPDATE skills SET name = 'Data Pipeline Architecture'                   WHERE slug = 'data-orchestration';
UPDATE skills SET name = 'Design Sensibility'                           WHERE slug = 'design-thinking';
UPDATE skills SET name = 'Troubleshooting & Diagnostics'                WHERE slug = 'diagnostics';
UPDATE skills SET name = 'Customer Discovery'                           WHERE slug = 'discovery';
UPDATE skills SET name = 'Technical Writing & Documentation'            WHERE slug = 'documentation';
UPDATE skills SET name = 'Financial Diligence'                          WHERE slug = 'due-diligence';
UPDATE skills SET name = 'Email Marketing'                              WHERE slug = 'email-outreach';
UPDATE skills SET name = 'Large Language Models'                        WHERE slug = 'embeddings';
UPDATE skills SET name = 'API Integration & System Integration'         WHERE slug = 'fastapi';
UPDATE skills SET name = 'Financial Diligence'                          WHERE slug = 'financial-operations';
UPDATE skills SET name = 'FMEA/Risk Analysis'                           WHERE slug = 'hazard-analysis';
UPDATE skills SET name = 'HRIS Administration'                          WHERE slug = 'hr-systems';
UPDATE skills SET name = 'Industrial Controls & Fluid Systems'          WHERE slug = 'industrial-controls';
UPDATE skills SET name = 'Observability, Monitoring & Instrumentation'  WHERE slug = 'instrumentation';
UPDATE skills SET name = 'Relationship Management'                      WHERE slug = 'investor-relations';
UPDATE skills SET name = 'HTML/CSS'                                     WHERE slug = 'javascript';
UPDATE skills SET name = 'Product Launch Strategy'                      WHERE slug = 'launch-strategy';
UPDATE skills SET name = 'Technical Leadership'                         WHERE slug = 'leadership';
UPDATE skills SET name = 'CNC Machining'                                WHERE slug = 'manufacturing-processes';
UPDATE skills SET name = 'Leadership Coaching'                          WHERE slug = 'mentoring';
UPDATE skills SET name = 'Data-Driven Decision Making'                  WHERE slug = 'metrics-driven-decisions';
UPDATE skills SET name = 'Modeling & Simulation'                        WHERE slug = 'multiphysics-modeling';
UPDATE skills SET name = 'Contract Negotiation and Structuring'         WHERE slug = 'negotiation';
UPDATE skills SET name = 'Networking'                                   WHERE slug = 'networking-basics';
UPDATE skills SET name = 'SSO, SAML, OAuth Configuration'               WHERE slug = 'oauth';
UPDATE skills SET name = 'End-to-End Implementation'                    WHERE slug = 'onboarding';
UPDATE skills SET name = 'Performance Optimization'                     WHERE slug = 'optimization';
UPDATE skills SET name = 'Performance Optimization'                     WHERE slug = 'performance-profiling';
UPDATE skills SET name = 'Executive Presentation'                       WHERE slug = 'presentation';
UPDATE skills SET name = 'Public Speaking'                              WHERE slug = 'presentation-skills';
UPDATE skills SET name = 'Industrial Controls & Fluid Systems'          WHERE slug = 'process-control';
UPDATE skills SET name = 'Product Judgment'                             WHERE slug = 'product-knowledge';
UPDATE skills SET name = 'Product Positioning & Messaging'              WHERE slug = 'product-messaging';
UPDATE skills SET name = 'Product Judgment'                             WHERE slug = 'product-sensibility';
UPDATE skills SET name = 'Go-to-Market Strategy'                        WHERE slug = 'product-strategy';
UPDATE skills SET name = 'PyTorch or TensorFlow'                        WHERE slug = 'pytorch';
UPDATE skills SET name = 'Quality Control'                              WHERE slug = 'quality-assurance';
UPDATE skills SET name = 'Sourcing & Candidate Identification'          WHERE slug = 'recruitment';
UPDATE skills SET name = 'Enterprise Relationship Management'           WHERE slug = 'relationship-management';
UPDATE skills SET name = 'Signal Processing'                            WHERE slug = 'rotor-dynamics';
UPDATE skills SET name = 'Sales Management'                             WHERE slug = 'sales-leadership';
UPDATE skills SET name = 'SaaS Metrics (NRR, Churn, Adoption)'          WHERE slug = 'sales-metrics';
UPDATE skills SET name = 'Process Improvement'                          WHERE slug = 'sales-process-improvement';
UPDATE skills SET name = 'Code Reading & Analysis'                      WHERE slug = 'secure-code-review';
UPDATE skills SET name = 'Solution architecture and design'             WHERE slug = 'solution-architecture';
UPDATE skills SET name = 'Prospecting Tools Fluency'                    WHERE slug = 'sourcing-tools-fluency';
UPDATE skills SET name = 'Software Engineering'                         WHERE slug = 'systems-engineering';
UPDATE skills SET name = 'Data Visualization'                           WHERE slug = 'tableau';
UPDATE skills SET name = 'Talent Acquisition Strategy'                  WHERE slug = 'talent-strategy';
UPDATE skills SET name = 'Leadership Coaching'                          WHERE slug = 'team-leadership';
UPDATE skills SET name = 'Account Management'                           WHERE slug = 'technical-account-management';
UPDATE skills SET name = 'Strategic Planning'                           WHERE slug = 'technical-decision-making';
UPDATE skills SET name = 'Strategic Planning'                           WHERE slug = 'technical-planning';
UPDATE skills SET name = 'PyTorch or TensorFlow'                        WHERE slug = 'tensorflow';
UPDATE skills SET name = 'Infrastructure as Code (Terraform)'           WHERE slug = 'terraform';
UPDATE skills SET name = 'Test Engineering & Testing Infrastructure'    WHERE slug = 'testing';
UPDATE skills SET name = 'Network Troubleshooting'                      WHERE slug = 'troubleshooting';
UPDATE skills SET name = 'UX Design'                                    WHERE slug = 'ui-design';
UPDATE skills SET name = 'Technical Writing & Documentation'            WHERE slug = 'user-documentation';
UPDATE skills SET name = 'Commissioning & Qualification'                WHERE slug = 'validation';

UPDATE specializations SET name = 'AI Development Tools'                     WHERE slug = 'ai-development';
UPDATE specializations SET name = 'Learning Science'                         WHERE slug = 'behavioral-science';
UPDATE specializations SET name = 'Business Process Management'              WHERE slug = 'business-automation';
UPDATE specializations SET name = 'Data Collection Operations'               WHERE slug = 'data-operations';
UPDATE specializations SET name = 'LLM Applications'                         WHERE slug = 'data-products';
UPDATE specializations SET name = 'Enterprise Software'                      WHERE slug = 'enterprise-saas';
UPDATE specializations SET name = 'Financial Services & Insurance'           WHERE slug = 'financial-services';
UPDATE specializations SET name = 'Industrial & Manufacturing'               WHERE slug = 'industrial-automation';
UPDATE specializations SET name = 'Payment Processing'                       WHERE slug = 'payments-operations';
UPDATE specializations SET name = 'Marketing Measurement & Attribution'      WHERE slug = 'performance-marketing';
UPDATE specializations SET name = 'Identity & Customer Verification Systems' WHERE slug = 'trust-and-verification';

-- ---------------------------------------------------------------------------
-- 2. Recreate the deleted taxonomy rows at their original ids, then
-- resynchronise the identity sequences.
--
-- DISTINCT ON collapses the one-row-per-link archive back to one row per
-- retired slug.
-- ---------------------------------------------------------------------------

INSERT INTO skills (id, slug, name, created_at)
SELECT DISTINCT ON (l.retired_slug)
       l.retired_id, l.retired_slug, l.retired_name, l.retired_created_at
FROM retired_slug_links l
WHERE l.retired_by_migration = '000037_repair_taxonomy_duplicates'
  AND l.table_name = 'skills'
ORDER BY l.retired_slug, l.id;

INSERT INTO specializations (id, slug, name, created_at)
SELECT DISTINCT ON (l.retired_slug)
       l.retired_id, l.retired_slug, l.retired_name, l.retired_created_at
FROM retired_slug_links l
WHERE l.retired_by_migration = '000037_repair_taxonomy_duplicates'
  AND l.table_name = 'specializations'
ORDER BY l.retired_slug, l.id;

SELECT setval('skills_id_seq',          (SELECT max(id) FROM skills));
SELECT setval('specializations_id_seq', (SELECT max(id) FROM specializations));

-- ---------------------------------------------------------------------------
-- 3. Put the links back on the restored rows.
-- ---------------------------------------------------------------------------

INSERT INTO job_posting_skills (classification_id, skill_id)
SELECT l.classification_id, l.retired_id
FROM retired_slug_links l
WHERE l.retired_by_migration = '000037_repair_taxonomy_duplicates'
  AND l.table_name = 'skills'
  AND l.classification_id IS NOT NULL
ON CONFLICT (classification_id, skill_id) DO NOTHING;

INSERT INTO job_posting_specializations (classification_id, specialization_id)
SELECT l.classification_id, l.retired_id
FROM retired_slug_links l
WHERE l.retired_by_migration = '000037_repair_taxonomy_duplicates'
  AND l.table_name = 'specializations'
  AND l.classification_id IS NOT NULL
ON CONFLICT (classification_id, specialization_id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 4. Remove the survivor links 000037 created.
--
-- `collided = false` is the whole point of that column: a link the up
-- migration's ON CONFLICT DO NOTHING skipped because the survivor already held
-- that pair predates 000037 and must survive this down migration. Only the
-- ones 000037 actually inserted are removed.
-- ---------------------------------------------------------------------------

DELETE FROM job_posting_skills j
USING retired_slug_links l, skills s
WHERE l.retired_by_migration = '000037_repair_taxonomy_duplicates'
  AND l.table_name = 'skills'
  AND l.classification_id IS NOT NULL
  AND l.collided = false
  AND s.slug = l.survivor_slug
  AND j.classification_id = l.classification_id
  AND j.skill_id = s.id;

DELETE FROM job_posting_specializations j
USING retired_slug_links l, specializations s
WHERE l.retired_by_migration = '000037_repair_taxonomy_duplicates'
  AND l.table_name = 'specializations'
  AND l.classification_id IS NOT NULL
  AND l.collided = false
  AND s.slug = l.survivor_slug
  AND j.classification_id = l.classification_id
  AND j.specialization_id = s.id;

-- ---------------------------------------------------------------------------
-- 5. Unregister the retirements and drop the archive.
-- ---------------------------------------------------------------------------

DELETE FROM retired_slugs
WHERE retired_by_migration = '000037_repair_taxonomy_duplicates';

DROP TABLE retired_slug_links;
