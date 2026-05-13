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
    ('Mistral',    'lever', 'mistral',    'AI models')

ON CONFLICT (ats, board_token) DO NOTHING;
