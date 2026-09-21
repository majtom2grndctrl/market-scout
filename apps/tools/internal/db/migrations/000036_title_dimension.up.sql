-- Derive posting title as a discrete dimension from run-scoped snapshot titles
-- without changing append-only snapshots. Location text got this treatment in
-- 000020 for the same reason: the raw string is too noisy to group on, so it is
-- curated once in the read model instead of re-parsed by every consumer.
--
-- Titles overload their separators. A comma is function-from-team
-- ("Software Engineer, Trainium"), seniority-from-function ("Director,
-- Strategic Sales"), or role-from-abbreviation ("Sales Development
-- Representative, SDR"), and a blind split_part conflates all three. A hyphen
-- separates ("Senior Support Engineer - Dublin") and also joins ("Full-Stack
-- Engineer"); spacing tells them apart. The rules below are deliberate about
-- each case, and where a shape stays ambiguous the title is left whole: an
-- unsplit title is honest, a mis-split one silently corrupts the measure.
-- title_head_source records which rule produced the head so a consumer can
-- exclude the risky class, the same way workplace_type_source does in 000019.
--
-- Two questions are answered separately. Which words are the role name is a
-- question about position: only a rank at the front of a segment can be
-- peeled off it, because only there is the word certainly a modifier rather
-- than the role's own noun -- "Engineering Manager" is a role, "Manager,
-- Product Marketing" is a rank and a role. Which rank the title states is not
-- a question about position: "Engineering Manager" states manager wherever
-- the word sits. So the head comes from peeling and the rank comes from
-- reading the whole title, with the peeled text taking precedence. See the
-- rank_text comment in derived.
--
-- Local performance check (2026-09-21, 89,243 snapshots, 4,810 open postings):
-- after warm-up, EXPLAIN (ANALYZE, TIMING OFF) on open_posting_titles had a
-- 521ms median across five runs. open_posting_taxonomy carries that cost for
-- every consumer, as it already does for the market branch.

CREATE TABLE title_seniority_seeds (
    slug     text PRIMARY KEY,
    name     text NOT NULL,
    rank     integer NOT NULL,
    patterns text[] NOT NULL DEFAULT '{}'
);

-- Seniority observed in the title, which is not classifications.seniority:
-- one is what the employer wrote, the other what the classifier inferred from
-- the description, and their disagreement is signal. Eight of the nine values
-- the classifications CHECK allows appear here under the same name (its ninth,
-- unknown, is this column's NULL). The five additions -- associate, manager,
-- head, vp, chief -- are ranks employers write in titles but the classifier
-- has no value for; dropping them would silently reclassify a stated rank as
-- part of the role name.
--
-- rank orders one ladder, so a title stating several ranks resolves to the
-- highest. It mixes the IC and management tracks because the classifications
-- vocabulary it aligns with already does.
--
-- Patterns are matched at a word boundary and are case-insensitive; the view
-- supplies the anchoring, so they omit it. Every pattern was observed in the
-- 2026-09-21 open-cohort title harvest. Two absences are deliberate: bare
-- 'mid' would swallow Mid-Market, a sales segment, and bare 'graduate' would
-- swallow Graduate Program.
--
-- Three patterns spell a trailing connective -- 'head of', 'vp of', 'director
-- of'. A rank word followed by a connective is otherwise refused, because the
-- phrase it opens is a role name rather than a rank claim ("Chief of Staff").
-- Spelling the connective into the pattern is how a rank that genuinely reads
-- that way opts back in, and it is why 'chief of' is absent.
INSERT INTO title_seniority_seeds (slug, name, rank, patterns) VALUES
    ('intern',    'Intern',          10, ARRAY['intern', 'internship', 'co-?op']),
    ('junior',    'Junior',          20, ARRAY['junior', 'jr\.?', 'entry[ -]level', 'new grad(uate)?']),
    ('associate', 'Associate',       25, ARRAY['associate']),
    ('mid',       'Mid',             30, ARRAY['mid[ -]level']),
    ('senior',    'Senior',          40, ARRAY['senior', 'sr\.?', 'snr\.?']),
    ('lead',      'Lead',            45, ARRAY['lead']),
    ('staff',     'Staff',           50, ARRAY['staff\+?']),
    ('principal', 'Principal',       60, ARRAY['principal', 'distinguished']),
    ('manager',   'Manager',         70, ARRAY['manager of', 'manager', 'mgr\.?']),
    ('head',      'Head',            75, ARRAY['head of', 'head']),
    ('director',  'Director',        80, ARRAY['director of', 'director']),
    ('vp',        'Vice President',  90, ARRAY['vice president of', 'vice president', 'vp of', 'vp', 'svp', 'evp']),
    ('chief',     'Chief',          100, ARRAY['chief']),
    -- No patterns: the explicit value for a title that states no rank, so a
    -- seniority grouping cannot imply coverage it lacks. Mirrors unmapped.
    ('unstated',  'Unstated',         0, ARRAY[]::text[]);

CREATE VIEW open_posting_titles AS
-- Columns: title_clean is the whitespace-normalized title; title_head the
-- base role name; title_qualifier everything stripped off it, preserved
-- rather than discarded; title_seniority the rank the title states, NULL when
-- it states none; title_head_source which rule produced the head.
WITH rank_alts AS MATERIALIZED (
    -- One alternation over every seniority pattern, longest branch first so
    -- "head of" wins over "head". Both expressions below are built from it.
    SELECT string_agg(pattern.value, '|' ORDER BY length(pattern.value) DESC, pattern.value)
               AS alternation
    FROM title_seniority_seeds
    CROSS JOIN LATERAL unnest(title_seniority_seeds.patterns) AS pattern(value)
),
rank_prefix AS MATERIALIZED (
    -- Anchored at the front of a segment. The lookahead is what keeps "Chief
    -- of Staff" whole: peeling chief would leave a title starting with a
    -- connective, which is never a role name, so the match is refused.
    --
    -- The inner repetition takes a slash-alternated run of ranks in one bite.
    -- A slash is not a separator here (see peeled) because it alternates
    -- inside a name, but "Senior / Staff Product Engineer" alternates ranks,
    -- and peeling only the first left the second sitting in the head.
    SELECT '^(?:' || rank_alts.alternation || ')(?:\s*/\s*(?:' || rank_alts.alternation || '))*'
               || '(?:\s+(?!(?:of|and|to|for|the)\y|&)|$)' AS expression
    FROM rank_alts
),
seniority_patterns AS MATERIALIZED (
    -- Unanchored, for reading the rank out of a title at any position. One
    -- expression per seed rather than one per pattern: the read below tests
    -- each posting against 13 regexes instead of 30.
    --
    -- The lookaheads carry the same refusal the prefix expression makes, so a
    -- rank word heading a connective phrase is not read as a rank claim.
    -- Terminating on any non-alphanumeric rather than on whitespace is what
    -- lets "Engineering Manager, Cybersecurity Products" and "Backend
    -- Software Engineer (Staff/Principal)" register at all.
    SELECT
        title_seniority_seeds.slug,
        title_seniority_seeds.rank,
        '(?<![[:alnum:]])(?:'
            || string_agg(pattern.value, '|' ORDER BY length(pattern.value) DESC, pattern.value)
            || ')(?![[:alnum:]])(?!\s+(?:of|and|to|for|the)\y)(?!\s*&)' AS expression
    FROM title_seniority_seeds
    CROSS JOIN LATERAL unnest(title_seniority_seeds.patterns) AS pattern(value)
    GROUP BY title_seniority_seeds.slug, title_seniority_seeds.rank
),
current_titles AS MATERIALIZED (
    -- Content comes from the same run that established openness, never from
    -- a later failed run. See the same predicate in 000019.
    SELECT open_postings.job_posting_id, current_snapshot.title
    FROM open_postings
    LEFT JOIN LATERAL (
        SELECT posting_snapshots.title
        FROM posting_snapshots
        WHERE posting_snapshots.job_posting_id = open_postings.job_posting_id
            AND posting_snapshots.fetch_run_id = open_postings.fetch_run_id
        ORDER BY posting_snapshots.fetched_at DESC, posting_snapshots.id DESC
        LIMIT 1
    ) AS current_snapshot ON TRUE
),
cleaned AS MATERIALIZED (
    -- 370 of 4,810 open titles carry leading or trailing whitespace and 44
    -- doubled internal spaces, so an unnormalized title is its own head.
    -- translate handles the space characters \s does not, as 000019 does for
    -- location_text.
    SELECT
        current_titles.job_posting_id,
        btrim(regexp_replace(
            translate(coalesce(current_titles.title, ''), U&'\00A0\2007\2009\202F', '    '),
            '\s+', ' ', 'g')) AS title_clean
    FROM current_titles
),
stripped AS MATERIALIZED (
    -- Parenthesized and bracketed groups come out first, before any split.
    -- They are never the head -- they carry a location ("(London, United
    -- Kingdom)"), a modality ("(Remote)"), a disclaimer ("[Expression of
    -- Interest]"), or nothing structured at all ("(4-8 YOE)") -- and removing
    -- them early is also what stops their inner commas from splitting the
    -- title, as in "Legal Engineer (Law Firm, Corporate)". The character
    -- class needs no closing-bracket match, so "Associate(CDMX)" parses
    -- despite the missing space. A rank inside one still counts: the rank is
    -- read from title_clean, which keeps them.
    SELECT
        cleaned.job_posting_id,
        cleaned.title_clean,
        coalesce((
            SELECT array_agg(btrim(bracketed.captured[1]))
            FROM regexp_matches(cleaned.title_clean, '[(\[]([^)\]]*)[)\]]', 'g') AS bracketed(captured)
            WHERE btrim(bracketed.captured[1]) <> ''
        ), '{}'::text[]) AS extras,
        btrim(regexp_replace(
            regexp_replace(cleaned.title_clean, '[(\[][^)\]]*[)\]]', ' ', 'g'),
            '\s+', ' ', 'g'), ' ,-|/') AS core
    FROM cleaned
),
peeled AS MATERIALIZED (
    -- Split into segments, then peel rank words off the front of each.
    --
    -- Separators: a comma, a hyphen or an en or em dash with a space on at
    -- least one side, or a pipe. The spacing is the whole test for a hyphen.
    -- One with no space either side joins -- "Full-Stack Engineer",
    -- "end-to-end", "Mid-Market" -- while one spaced on either side is a
    -- writer separating, however sloppily: "Senior Product Manager- Voice AI"
    -- and "Customer Success Manager -EMEA" both mean the same as the spaced
    -- form. A slash is not a separator: in all 144 observed cases it
    -- alternates inside a name ("Backend/API Engineer", "Research Engineer /
    -- Scientist"), and where it alternates ranks instead the prefix
    -- expression takes the whole run.
    --
    -- head_ord is the first segment that survives peeling. That is the rule
    -- that tells the three comma senses apart. "Software Engineer, Trainium"
    -- keeps segment 1, because "software engineer" is not rank words.
    -- "Director, Strategic Sales" loses it, because "director" is nothing but
    -- rank, so the function has to be in segment 2. "Sales Development
    -- Representative, SDR" keeps segment 1 and the abbreviation lands in the
    -- qualifier with the other remainders.
    SELECT
        stripped.job_posting_id,
        segment.ord,
        btrim(segment.value) AS segment,
        peel3.value AS peeled,
        min(segment.ord) FILTER (WHERE peel3.value <> '')
            OVER (PARTITION BY stripped.job_posting_id) AS head_ord
    FROM stripped
    CROSS JOIN rank_prefix
    CROSS JOIN LATERAL unnest(string_to_array(
        regexp_replace(stripped.core,
            '\s*,\s*|\s+-\s*|\s*-\s+|\s*[–—]\s+|\s+[–—]\s*|\s*\|\s*', E'\x01', 'g'),
        E'\x01')) WITH ORDINALITY AS segment(value, ord)
    -- OFFSET 0 fences each step so the planner evaluates its regexp_replace
    -- once. Inlined, the three references the next step makes to a step's
    -- value each re-run that step's regex, and the cost compounds per level.
    CROSS JOIN LATERAL (
        SELECT btrim(regexp_replace(btrim(segment.value), rank_prefix.expression, '', 'i')) AS value
        OFFSET 0
    ) AS peel1
    CROSS JOIN LATERAL (
        SELECT btrim(regexp_replace(peel1.value, rank_prefix.expression, '', 'i')) AS value
        OFFSET 0
    ) AS peel2
    CROSS JOIN LATERAL (
        SELECT btrim(regexp_replace(peel2.value, rank_prefix.expression, '', 'i')) AS value
        OFFSET 0
    ) AS peel3
),
folded AS MATERIALIZED (
    -- consumed is the rank text peeling removed, from the head segment and
    -- from every segment that was nothing but rank, so "Software Engineer,
    -- Intern" still reads as an internship. Anything that is neither the head
    -- nor rank is a qualifier.
    SELECT
        peeled.job_posting_id,
        peeled.head_ord,
        count(*) AS segment_count,
        max(peeled.peeled) FILTER (WHERE peeled.ord = peeled.head_ord) AS head_text,
        btrim(string_agg(
            CASE
                WHEN peeled.peeled = '' THEN peeled.segment
                ELSE left(peeled.segment, length(peeled.segment) - length(peeled.peeled))
            END, ' ' ORDER BY peeled.ord)
            FILTER (WHERE peeled.peeled = '' OR peeled.ord = peeled.head_ord)) AS consumed,
        coalesce(
            array_agg(peeled.segment ORDER BY peeled.ord)
                FILTER (WHERE peeled.peeled <> '' AND peeled.ord <> peeled.head_ord),
            '{}'::text[]) AS qualifier_segments
    FROM peeled
    GROUP BY peeled.job_posting_id, peeled.head_ord
),
derived AS MATERIALIZED (
    SELECT
        stripped.job_posting_id,
        nullif(stripped.title_clean, '') AS title_clean,
        -- The text the rank is read out of. Peeled text first: a rank the
        -- writer put in front of the role name is the level they are stating,
        -- so "Senior Product Manager" is senior and not a manager posting.
        -- With nothing peeled, the whole title answers instead, which is what
        -- gives "Engineering Manager", "Product Design Intern (2027)" and
        -- "Data Center Energy Lead, EMEA" the rank they plainly state.
        --
        -- The rewrite drops a rank word that is the object of a connective,
        -- leaving the connective in place so the word before it is refused
        -- too. Both halves of "Chief of Staff" go that way, which is the
        -- point: it is a role name, not a chief and not a staff engineer.
        coalesce(nullif(folded.consumed, ''),
                 regexp_replace(stripped.title_clean,
                     '(\y(?:of|and|to|for|the)\s+)(?:' || rank_alts.alternation || ')(?![[:alnum:]])',
                     '\1x', 'gi')) AS rank_text,
        lower(coalesce(nullif(levelled.head_text, ''), nullif(stripped.core, ''),
                       nullif(stripped.title_clean, ''))) AS title_head,
        -- whole: no separator, the head is the title. segment: a separator
        -- split and the head is what came first. rank_peel: a leading
        -- rank-only segment was dropped, the class most likely to misfire on
        -- a title that leads with a team. unresolved: nothing survived
        -- peeling, so the cleaned title stands unsplit.
        CASE
            WHEN stripped.title_clean = '' THEN NULL
            WHEN folded.head_ord IS NULL OR levelled.head_text = '' THEN 'unresolved'
            WHEN folded.head_ord > 1 THEN 'rank_peel'
            WHEN folded.segment_count > 1 THEN 'segment'
            ELSE 'whole'
        END AS title_head_source,
        nullif(array_to_string(
            folded.qualifier_segments
                || stripped.extras
                || CASE WHEN levelled.level_marker IS NULL
                        THEN '{}'::text[] ELSE ARRAY[levelled.level_marker] END,
            ' | '), '') AS title_qualifier
    FROM stripped
    JOIN folded ON folded.job_posting_id = stripped.job_posting_id
    CROSS JOIN rank_alts
    -- A trailing roman numeral is a level, not part of the role name, and
    -- leaving it attached splits "Electrical Engineer" in two. Matched
    -- case-sensitively and only after a space or hyphen, so "UK/I" survives.
    -- It is preserved in the qualifier rather than mapped onto a seed: the
    -- ladders these numerals index are per-employer and not comparable.
    CROSS JOIN LATERAL (
        SELECT
            substring(folded.head_text from '[ -](I{1,3}|IV|VI?)$') AS level_marker,
            btrim(regexp_replace(coalesce(folded.head_text, ''), '[ -](I{1,3}|IV|VI?)$', ''),
                  ' ,-|/') AS head_text
    ) AS levelled
)
SELECT
    derived.job_posting_id,
    derived.title_clean,
    derived.title_head,
    nullif(btrim(regexp_replace(coalesce(derived.title_head, ''), '[^a-z0-9]+', '-', 'g'), '-'), '')
        AS title_head_slug,
    derived.title_qualifier,
    seniority.slug AS title_seniority,
    derived.title_head_source
FROM derived
LEFT JOIN LATERAL (
    SELECT seniority_patterns.slug
    FROM seniority_patterns
    WHERE derived.rank_text ~* seniority_patterns.expression
    ORDER BY seniority_patterns.rank DESC
    LIMIT 1
) AS seniority ON TRUE;

-- Title joins market as a derived grouping term. Two branches: the head, and
-- the seniority the title states -- which resolves to the explicit unstated
-- value when it states none, so a seniority grouping cannot imply coverage it
-- lacks. Replacing the view preserves its four-column interface.
CREATE OR REPLACE VIEW open_posting_taxonomy AS
SELECT
    open_postings_display.job_posting_id,
    'role'::text AS term_kind,
    canonical_roles.slug,
    canonical_roles.name
FROM open_postings_display
JOIN job_posting_roles
    ON job_posting_roles.classification_id = open_postings_display.classification_id
JOIN canonical_roles ON canonical_roles.id = job_posting_roles.role_id

UNION ALL

SELECT
    open_postings_display.job_posting_id,
    'specialization'::text AS term_kind,
    specializations.slug,
    specializations.name
FROM open_postings_display
JOIN job_posting_specializations
    ON job_posting_specializations.classification_id = open_postings_display.classification_id
JOIN specializations ON specializations.id = job_posting_specializations.specialization_id

UNION ALL

SELECT
    open_postings_display.job_posting_id,
    'skill'::text AS term_kind,
    skills.slug,
    skills.name
FROM open_postings_display
JOIN job_posting_skills
    ON job_posting_skills.classification_id = open_postings_display.classification_id
JOIN skills ON skills.id = job_posting_skills.skill_id

UNION ALL

SELECT DISTINCT
    open_postings_display.job_posting_id,
    'dimension'::text AS term_kind,
    role_dimensions.slug,
    role_dimensions.name
FROM open_postings_display
JOIN job_posting_roles
    ON job_posting_roles.classification_id = open_postings_display.classification_id
JOIN canonical_role_dimensions
    ON canonical_role_dimensions.canonical_role_id = job_posting_roles.role_id
JOIN role_dimensions ON role_dimensions.id = canonical_role_dimensions.dimension_id

UNION ALL

SELECT
    open_posting_markets.job_posting_id,
    'market'::text AS term_kind,
    open_posting_markets.slug,
    open_posting_markets.name
FROM open_posting_markets

UNION ALL

SELECT
    open_posting_titles.job_posting_id,
    'title_head'::text AS term_kind,
    open_posting_titles.title_head_slug,
    open_posting_titles.title_head
FROM open_posting_titles
WHERE open_posting_titles.title_head_slug IS NOT NULL

UNION ALL

SELECT
    open_posting_titles.job_posting_id,
    'title_seniority'::text AS term_kind,
    stated.slug,
    stated.name
FROM open_posting_titles
JOIN title_seniority_seeds AS stated
    ON stated.slug = coalesce(open_posting_titles.title_seniority, 'unstated')
WHERE open_posting_titles.title_clean IS NOT NULL;
