-- Watchlist companies for dev/test runs. Re-run safely: ON CONFLICT DO NOTHING.
-- See agent-context/lib/watchlist.md for the full list and integration status.

INSERT INTO companies (name, ats, board_token) VALUES
    -- Greenhouse
    ('Anthropic',  'greenhouse', 'anthropic'),
    ('Stripe',     'greenhouse', 'stripe'),
    ('Figma',      'greenhouse', 'figma'),
    ('Scale AI',   'greenhouse', 'scaleai'),
    ('Glean',      'greenhouse', 'gleanwork'),

    -- Ashby
    ('Cognition',  'ashby', 'cognition'),
    ('Harvey',     'ashby', 'harvey'),
    ('ElevenLabs', 'ashby', 'elevenlabs'),
    ('Linear',     'ashby', 'linear'),

    -- Lever
    ('Mistral',    'lever', 'mistral')

ON CONFLICT (ats, board_token) DO NOTHING;
