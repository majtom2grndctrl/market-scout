-- Canonical company seed file. Hand-edit for one-off additions;
-- cmd/onboard appends verified records from research sidecars.
-- Re-run safely: ON CONFLICT (ats, board_token) DO NOTHING.
-- See agent-context/lib/watchlist.md for context on adding companies
-- and the verification workflow.

INSERT INTO companies (name, ats, board_token, industry) VALUES
    -- Greenhouse
    ('Anthropic',  'greenhouse', 'anthropic',  'AI research & models'),
    ('Stripe',     'greenhouse', 'stripe',      'fintech'),
    ('Figma',      'greenhouse', 'figma',       'productivity & collaboration'),
    ('Scale AI',   'greenhouse', 'scaleai',     'AI infrastructure'),
    ('Glean',      'greenhouse', 'gleanwork',   'productivity & collaboration'),

    -- Ashby
    ('Cognition',  'ashby', 'cognition',  'dev tooling'),
    ('Harvey',     'ashby', 'harvey',     'legal tech'),
    ('ElevenLabs', 'ashby', 'elevenlabs', 'AI research & models'),
    ('Linear',     'ashby', 'linear',     'dev tooling'),

    -- Lever
    ('Mistral',    'lever', 'mistral',    'AI research & models'),

    -- Greenhouse (GeekWire batch, May 2026)
    ('Temporal',   'greenhouse', 'temporaltechnologies', 'dev tooling'),
    ('Gradial',    'greenhouse', 'gradial',               'sales & marketing tech'),

    -- Ashby (GeekWire batch, May 2026)
    ('Nooks',                  'ashby', 'nooks',      'sales & marketing tech'),
    ('Oumi',                   'ashby', 'oumi',       'AI research & models'),
    ('Ineffable Intelligence', 'ashby', 'ineffable',  'AI research & models'),

    -- Greenhouse (Built In Seattle / AI filter, May 2026)
    ('Runpod',  'greenhouse', 'runpod',  'AI infrastructure'),
    ('Phaidra', 'greenhouse', 'phaidra', 'robotics & hardware'),
    ('Textio',  'greenhouse', 'textio',  'HR & recruiting tech'),

    -- Ashby (Built In Seattle / AI filter, May 2026)
    ('Statsig',    'ashby', 'statsig',    'dev tooling'),
    ('Superhuman', 'ashby', 'superhuman', 'productivity & collaboration'),

    -- Greenhouse (Built In Seattle / full sweep, May 2026)
    ('CommerceIQ', 'greenhouse', 'commerceiq', 'sales & marketing tech'),
    ('super.AI',   'greenhouse', 'superai',    'AI infrastructure'),

    -- Lever (Built In Seattle / full sweep, May 2026)
    ('Spice AI',    'lever', 'spiceai',    'AI infrastructure'),
    ('Revefi',      'lever', 'revefi',     'data & analytics'),
    ('Avante',      'lever', 'avante',     'HR & recruiting tech'),
    ('Conversica',  'lever', 'conversica', 'sales & marketing tech'),

    -- Ashby (GeekWire funding tracker, May 2026)
    ('Cascade',  'ashby', 'cascade', 'HR & recruiting tech'),
    ('Humanly',  'ashby', 'humanly', 'HR & recruiting tech'),
    ('Union.ai', 'ashby', 'union',   'AI infrastructure'),
    ('Read AI',  'ashby', 'read-ai', 'productivity & collaboration'),
    ('QA Wolf',  'ashby', 'qawolf',  'dev tooling'),
    ('Depot',    'ashby', 'depot',   'dev tooling'),
    ('Casium',   'ashby', 'casium',  'legal tech'),

    -- Greenhouse (GeekWire funding tracker, May 2026)
    ('Chainguard',  'greenhouse', 'chainguard',  'security & identity'),
    ('Panthalassa', 'greenhouse', 'panthalassa', 'AI infrastructure'),
    ('Starcloud',   'greenhouse', 'starcloud',   'AI infrastructure'),

    -- Workable (GeekWire 200, May 2026)
    ('Seeq', 'workable', 'seeq', 'data & analytics')

ON CONFLICT (ats, board_token) DO NOTHING;

-- Greenhouse (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Airtable', 'greenhouse', 'airtable', 'productivity & collaboration')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Greenhouse (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Truveta', 'greenhouse', 'truveta', 'healthtech'),
    ('Agility Robotics', 'greenhouse', 'agilityrobotics', 'robotics & hardware'),
    ('iSpot.tv', 'greenhouse', 'ispottv', 'sales & marketing tech'),
    ('Stoke Space', 'greenhouse', 'stokespacetechnologies', 'aerospace & defense'),
    ('Carbon Robotics', 'greenhouse', 'carbonrobotics', 'robotics & hardware'),
    ('Karat', 'greenhouse', 'karat', 'HR & recruiting tech'),
    ('Proprio', 'greenhouse', 'proprio', 'healthtech'),
    ('Customer.io', 'greenhouse', 'customerio', 'sales & marketing tech'),
    ('Place Technology', 'greenhouse', 'place', 'proptech & construction'),
    ('Pulumi', 'greenhouse', 'pulumicorporation', 'dev tooling'),
    ('Yoodli', 'greenhouse', 'yoodliinc', 'productivity & collaboration'),
    ('Stackline', 'greenhouse', 'stackline', 'commerce & marketplaces'),
    ('Group14', 'greenhouse', 'group14', 'climate & energy'),
    ('Snap! Raise', 'greenhouse', 'snapmobileinc', 'edtech'),
    ('Amperity', 'greenhouse', 'amperity', 'data & analytics'),
    ('Syndio', 'greenhouse', 'syndio', 'HR & recruiting tech'),
    ('Hyperproof', 'greenhouse', 'hyperproof', 'security & identity'),
    ('Boulder Care', 'greenhouse', 'bouldercare', 'healthtech'),
    ('Echodyne', 'greenhouse', 'echodynecorp', 'robotics & hardware'),
    ('LevelTen Energy', 'greenhouse', 'leveltenenergy', 'climate & energy'),
    ('Carbon Direct', 'greenhouse', 'carbondirect', 'climate & energy'),
    ('Archera.ai', 'greenhouse', 'archera', 'dev tooling'),
    ('Flexe', 'greenhouse', 'flexe', 'logistics'),
    ('Levanta', 'greenhouse', 'levanta', 'commerce & marketplaces'),
    ('TerraClear', 'greenhouse', 'terraclear', 'robotics & hardware'),
    ('Parse Biosciences', 'greenhouse', 'parsebiosciences', 'biotech'),
    ('Aspect Biosystems', 'greenhouse', 'aspectbiosystems', 'biotech'),
    ('Lumen Bioscience', 'greenhouse', 'lumenbioscience', 'biotech'),
    ('Digs', 'greenhouse', 'digs', 'proptech & construction'),
    ('Upbound', 'greenhouse', 'upbound', 'dev tooling'),
    ('Recurrent', 'greenhouse', 'recurrent', 'climate & energy'),
    ('Tune Therapeutics', 'greenhouse', 'tunetherapeutics', 'biotech'),
    ('Corelight', 'greenhouse', 'corelight', 'security & identity'),
    ('Attunely', 'greenhouse', 'attunely', 'fintech'),
    ('Legion', 'greenhouse', 'legion', 'HR & recruiting tech'),
    ('MediaAlpha', 'greenhouse', 'mediaalpha', 'sales & marketing tech'),
    ('Xealth', 'greenhouse', 'xealth', 'healthtech'),
    ('Tenable', 'greenhouse', 'tenableinc', 'security & identity'),
    ('OfferUp', 'greenhouse', 'offerup', 'commerce & marketplaces')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Ashby (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Brinc', 'ashby', 'brinc', 'aerospace & defense'),
    ('MotherDuck', 'ashby', 'motherduck', 'data & analytics'),
    ('Klue', 'ashby', 'klue', 'sales & marketing tech'),
    ('Certn', 'ashby', 'certn', 'HR & recruiting tech'),
    ('Vibe', 'ashby', 'vibe', 'sales & marketing tech'),
    ('Eigen Labs', 'ashby', 'eigen-labs', 'crypto'),
    ('Common Room', 'ashby', 'commonroom', 'sales & marketing tech'),
    ('Atlas Health', 'ashby', 'atlas', 'healthtech')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Lever (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Outreach', 'lever', 'outreach', 'sales & marketing tech'),
    ('Omnidian', 'lever', 'omnidian', 'climate & energy'),
    ('Outpace Bio', 'lever', 'outpacebio', 'biotech'),
    ('Viome', 'lever', 'viome', 'healthtech'),
    ('DexCare', 'lever', 'dexcarehealth', 'healthtech'),
    ('Oleria', 'lever', 'oleria-security', 'security & identity'),
    ('Mast Reforestation', 'lever', 'MastReforestation', 'climate & energy'),
    ('Educative', 'lever', 'educative', 'edtech'),
    ('Sanctuary AI', 'lever', 'sanctuary', 'robotics & hardware'),
    ('SkyPoint Cloud', 'lever', 'skypointcloud', 'data & analytics'),
    ('Aigen', 'lever', 'aigen', 'climate & energy'),
    ('Lumotive', 'lever', 'lumotive', 'robotics & hardware'),
    ('Highspot', 'lever', 'highspot', 'sales & marketing tech')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Workable (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Starfish Space', 'workable', 'starfish-space-1', 'aerospace & defense'),
    ('Vouched', 'workable', 'vouched', 'security & identity'),
    ('Likewise', 'workable', 'likewise', 'commerce & marketplaces'),
    ('Discovery Health MD', 'workable', 'discovery-health-md', 'healthtech'),
    ('Banzai', 'workable', 'banzai', 'sales & marketing tech')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Workday (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Remitly', 'workday', 'remitly.wd5.myworkdayjobs.com/Remitly_Careers', 'fintech')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Ashby (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Helion',   'ashby', 'helion',   'climate & energy'),
    ('Hiya',     'ashby', 'hiya',     'security & identity'),
    ('Qumulo',   'ashby', 'qumulo',   'dev tooling'),
    ('Polly',    'ashby', 'polly',    'fintech'),
    ('LiveKit',  'ashby', 'livekit',  'dev tooling')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Workday (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Tinuiti', 'workday', 'tinuiti.wd12.myworkdayjobs.com/Tinuiti', 'sales & marketing tech')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Lever (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Ridwell', 'lever', 'Ridwell', 'climate & energy')
ON CONFLICT (ats, board_token) DO NOTHING;
