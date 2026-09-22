-- Bring title_seniority_seeds up to two words the classifier's level-word list
-- gained after 000041: Entry-level and Early-career, both of which Step 1 of
-- the seniority ladder in the batch-enrich skill resolves to junior, in any
-- spacing or hyphenation.
--
-- Checked against 000041's patterns and every snapshot title (2026-09-22).
--
-- Entry-level -> junior, already covered by 000036's 'entry[ -]level' for the
-- spaced and hyphenated forms ("Entry Level Aerospace Technician", "Software
-- Engineer (Entry-level)"). The separator becomes optional so "Entrylevel"
-- counts too, matching the classifier's any-spacing rule. Not yet observed;
-- the run-together word has no other reading, so it needs no guard.
--
-- Early-career -> junior, with the separator optional for the same reason.
-- One guard, on the pattern: never before Recruit, Talent or Program. There
-- the phrase names a hiring pipeline for early-career people, not this role's
-- rung, and all three are observed on closed postings: "Early Career
-- Recruiter", "Early Career Talent Partner (Sourcing)", "Principal Recruiter-
-- Early Career Programs". The guard matches word prefixes, so Recruiting,
-- Recruitment and Programs are refused too. Plural "Early Careers" needs no
-- guard: the view's word boundary already refuses a longer word, and the
-- plural usually names the programme rather than a rung.
--
-- Two phrases stay absent. Bare "Early" is not a level word: "Early Access
-- Deployment Engineer" names a product phase, and "Early to Mid-Career Sales
-- Opportunities" is not a rank the classifier reads. "Early Talent" is not on
-- the classifier's list either. Numbered levels ("Level 1", "II") stay absent
-- because the classifier maps none of them to a rung.
--
-- Manager is untouched. It is not a Step 1 level word, and it stays a rank
-- here on purpose; see project.md.
--
-- Open-cohort effect at 4,810 postings: 2 titles with no rank gain junior.
-- "Early Career Engineer, Nova" also peels, so its head becomes Engineer the
-- way "Senior Engineer" already does; "Test Engineer (Level 1 - Early
-- Career)" keeps its head, as the phrase sits in the bracketed qualifier. No
-- other head, head source or seniority changes.

UPDATE title_seniority_seeds
SET patterns = ARRAY['junior', 'jr\.?', 'entry[ -]?level',
                     'early[ -]?career(?!\s+(?:recruit|talent|program))',
                     'new grad(uate)?', 'university grad(uate)?']
WHERE slug = 'junior';
