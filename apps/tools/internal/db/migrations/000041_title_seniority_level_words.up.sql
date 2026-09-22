-- Bring title_seniority_seeds up to the classifier's level-word list. Step 1
-- of the seniority ladder in the batch-enrich skill names the words an
-- employer uses to state a level: Intern, Co-op, Apprentice, Graduate / New
-- Grad, Junior, Mid / Mid-level, Senior, Staff, Principal, Lead, Leader,
-- Director, Head, VP. 000036's seeds predate that list, so a title stating a
-- level the classifier reads could state none in the title dimension:
-- "Customer Success Leader" had no title_seniority at all.
--
-- Checked word by word against 000036's patterns and every snapshot title
-- (2026-09-22). Already covered, so unchanged: Intern, Co-op ('co-?op'), New
-- Grad, Junior, Mid-level, Senior, Staff, Principal, Lead, Director, Head, VP.
-- Four gaps, each added to an existing rank. No rank is new, so the ranks stay
-- the eight classifications.seniority values plus 000036's five additions.
--
-- Leader -> lead. The classifier has no leader value, and the lead rung is
-- the one it names. Two guards, both on the pattern, keep it a level word:
--   - Only as the last word of its phrase. Leader is a noun, not a modifier
--     like Lead, so a word after it makes it the object of another noun:
--     "Sales Leader Enablement" is enablement for leaders, not a leader.
--     Customer Success Leader, "Leader - Tokyo", "Leader (Greater China)"
--     and "Leader, APAC" all still count.
--   - Never after Thought. "Thought Leader" names a role, not a rung. Not yet
--     observed; guarded because the phrase is common.
--   Leadership, Leaderboard and plural Leaders need no guard: the view's
--   word boundary already refuses a longer word. Team Leader counts; it is a
--   rung.
--
-- Apprentice -> intern. The classifier has no apprentice value. Apprentice
-- sits with Intern and Co-op in the Step 1 list, and like them it names a
-- time-bound training placement rather than a regular entry-level hire, which
-- is what junior means here. 000036 put Co-op on intern for the same reason.
-- Apprentice is a modifier and counts anywhere ("Apprentice Electrician").
-- Apprenticeship is a noun and takes Leader's last-word guard, so
-- "Apprenticeship Program Lead" is not peeled into an intern posting.
--
-- University Grad -> junior, beside New Grad. Observed on a closed posting.
-- Bare Graduate stays absent, as 000036 left it: no title in the corpus uses
-- it as a level, and it would swallow Graduate Program and Graduate Research.
--
-- Mid -> mid, only in the Mid-Senior band ('mid-senior', 'mid-sr'). That is
-- the one shape in the corpus where bare Mid states a level. Everywhere else
-- it names a segment, a place, a phase or a career stage -- Mid-Market,
-- Mid-Atlantic, Mid-Training, Mid-Career -- so bare 'mid' stays absent, as
-- 000036 left it.
-- The band still resolves to senior in title_seniority, because the view
-- takes the highest rank a title states. Registering mid matters where every
-- matched rank is kept, not just the highest, and it lets "Mid-Sr Avionics
-- Mechanical Engineer" peel the way "Senior Avionics Mechanical Engineer"
-- does.
--
-- Manager is untouched. It is not a Step 1 level word, and it stays a rank
-- here on purpose; see project.md.
--
-- Patterns now carry lookarounds, which 000036's did not. The view wraps each
-- pattern in its own boundary and connective guards and in a case-insensitive
-- match, so a pattern's lookaround only narrows it further. Anything else that
-- builds a regex from these patterns must match case-insensitively on
-- whitespace-normalized text, as the view does.
--
-- Open-cohort effect at 4,810 postings: 9 titles with no rank gain lead, and
-- one moves from associate to intern ("Operations Associate,
-- Apprenticeship": the rank-only segment now peels and takes precedence, so
-- Apprenticeship leaves the qualifier the way New Grad already does). No
-- head, head source or other seniority changes.

UPDATE title_seniority_seeds
SET patterns = ARRAY['intern', 'internship', 'co-?op', 'apprentice',
                     'apprenticeship(?!\s+[[:alnum:]])']
WHERE slug = 'intern';

UPDATE title_seniority_seeds
SET patterns = ARRAY['junior', 'jr\.?', 'entry[ -]level', 'new grad(uate)?', 'university grad(uate)?']
WHERE slug = 'junior';

UPDATE title_seniority_seeds
SET patterns = ARRAY['mid[ -]level', 'mid-(senior|sr\.?)']
WHERE slug = 'mid';

UPDATE title_seniority_seeds
SET patterns = ARRAY['lead', '(?<!thought\s)leader(?!\s+[[:alnum:]])']
WHERE slug = 'lead';
