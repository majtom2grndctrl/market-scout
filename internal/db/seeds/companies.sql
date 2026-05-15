-- Watchlist companies for dev/test runs. Re-run safely: ON CONFLICT DO NOTHING.
-- See agent-context/lib/watchlist.md for context on adding companies.

INSERT INTO companies (name, ats, board_token, industry) VALUES
    -- Greenhouse
    ('Anthropic',  'greenhouse', 'anthropic',  'AI research'),
    ('Stripe',     'greenhouse', 'stripe',      'fintech'),
    ('Figma',      'greenhouse', 'figma',       'design tools'),
    ('Scale AI',   'greenhouse', 'scaleai',     'AI data'),
    ('Glean',      'greenhouse', 'gleanwork',   'enterprise search'),

    -- Ashby
    ('Cognition',  'ashby', 'cognition',  'AI coding'),
    ('Harvey',     'ashby', 'harvey',     'legal AI'),
    ('ElevenLabs', 'ashby', 'elevenlabs', 'AI voice'),
    ('Linear',     'ashby', 'linear',     'dev tooling'),

    -- Lever
    ('Mistral',    'lever', 'mistral',    'AI models'),

    -- Greenhouse (GeekWire batch, May 2026)
    ('Temporal',   'greenhouse', 'temporaltechnologies', 'developer tooling'),
    ('Gradial',    'greenhouse', 'gradial',               'AI marketing'),

    -- Ashby (GeekWire batch, May 2026)
    ('Nooks',                  'ashby', 'nooks',      'AI sales'),
    ('Oumi',                   'ashby', 'oumi',       'AI models'),
    ('Ineffable Intelligence', 'ashby', 'ineffable',  'AI research'),

    -- Greenhouse (Built In Seattle / AI filter, May 2026)
    ('Runpod',  'greenhouse', 'runpod',  'AI infrastructure'),
    ('Phaidra', 'greenhouse', 'phaidra', 'industrial AI'),
    ('Textio',  'greenhouse', 'textio',  'AI writing'),

    -- Ashby (Built In Seattle / AI filter, May 2026)
    ('Statsig',    'ashby', 'statsig',    'developer tooling'),
    ('Superhuman', 'ashby', 'superhuman', 'AI productivity'),

    -- Greenhouse (Built In Seattle / full sweep, May 2026)
    ('CommerceIQ', 'greenhouse', 'commerceiq', 'AI retail'),
    ('super.AI',   'greenhouse', 'superai',    'AI data labeling'),

    -- Lever (Built In Seattle / full sweep, May 2026)
    ('Spice AI',    'lever', 'spiceai',    'developer tooling'),
    ('Revefi',      'lever', 'revefi',     'AI data observability'),
    ('Avante',      'lever', 'avante',     'AI HR'),
    ('Conversica',  'lever', 'conversica', 'AI sales')

ON CONFLICT (ats, board_token) DO NOTHING;
