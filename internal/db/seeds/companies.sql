-- Canonical company seed file. Hand-edit for one-off additions;
-- cmd/onboard appends verified records from research sidecars.
-- Re-run safely: ON CONFLICT (ats, board_token) DO NOTHING.
-- See agent-context/lib/watchlist.md for context on adding companies
-- and the verification workflow.

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
    ('Conversica',  'lever', 'conversica', 'AI sales'),

    -- Ashby (GeekWire funding tracker, May 2026)
    ('Cascade',  'ashby', 'cascade', 'AI HR'),
    ('Humanly',  'ashby', 'humanly', 'AI recruiting'),
    ('Union.ai', 'ashby', 'union',   'AI workflows'),
    ('Read AI',  'ashby', 'read-ai', 'AI productivity'),
    ('QA Wolf',  'ashby', 'qawolf',  'developer tooling'),
    ('Depot',    'ashby', 'depot',   'developer tooling'),
    ('Casium',   'ashby', 'casium',  'AI legal'),

    -- Greenhouse (GeekWire funding tracker, May 2026)
    ('Chainguard',  'greenhouse', 'chainguard',  'supply chain security'),
    ('Panthalassa', 'greenhouse', 'panthalassa', 'AI infrastructure'),
    ('Starcloud',   'greenhouse', 'starcloud',   'AI infrastructure'),

    -- Workable (GeekWire 200, May 2026)
    ('Seeq', 'workable', 'seeq', 'industrial analytics')

ON CONFLICT (ats, board_token) DO NOTHING;

-- Greenhouse (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Airtable', 'greenhouse', 'airtable', 'collaboration software')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Greenhouse (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Truveta', 'greenhouse', 'truveta', 'Hospitals and Health Care'),
    ('Agility Robotics', 'greenhouse', 'agilityrobotics', 'Robotics Engineering'),
    ('iSpot.tv', 'greenhouse', 'ispottv', 'Advertising Services'),
    ('Stoke Space', 'greenhouse', 'stokespacetechnologies', 'Defense and Space Manufacturing'),
    ('Carbon Robotics', 'greenhouse', 'carbonrobotics', 'Automation Machinery Manufacturing'),
    ('Karat', 'greenhouse', 'karat', 'Software Development'),
    ('Proprio', 'greenhouse', 'proprio', 'Medical Equipment Manufacturing'),
    ('Customer.io', 'greenhouse', 'customerio', 'Software Development'),
    ('Place Technology', 'greenhouse', 'place', 'Real Estate'),
    ('Pulumi', 'greenhouse', 'pulumicorporation', 'Software Development'),
    ('Yoodli', 'greenhouse', 'yoodliinc', 'Software Development'),
    ('Stackline', 'greenhouse', 'stackline', 'Software Development'),
    ('Group14', 'greenhouse', 'group14', 'Manufacturing'),
    ('Snap! Raise', 'greenhouse', 'snapmobileinc', 'Software Development'),
    ('Amperity', 'greenhouse', 'amperity', 'Software Development'),
    ('Syndio', 'greenhouse', 'syndio', 'Software Development'),
    ('Hyperproof', 'greenhouse', 'hyperproof', 'Software Development'),
    ('Boulder Care', 'greenhouse', 'bouldercare', 'Hospitals and Health Care'),
    ('Echodyne', 'greenhouse', 'echodynecorp', 'Appliances, Electrical, and Electronics Manufacturing'),
    ('LevelTen Energy', 'greenhouse', 'leveltenenergy', 'Services for Renewable Energy'),
    ('Carbon Direct', 'greenhouse', 'carbondirect', 'Climate Data and Analytics'),
    ('Archera.ai', 'greenhouse', 'archera', 'Software Development'),
    ('Flexe', 'greenhouse', 'flexe', 'Transportation, Logistics, Supply Chain and Storage'),
    ('Levanta', 'greenhouse', 'levanta', 'Internet Marketplace Platforms'),
    ('TerraClear', 'greenhouse', 'terraclear', 'Automation Machinery Manufacturing'),
    ('Parse Biosciences', 'greenhouse', 'parsebiosciences', 'Biotechnology Research'),
    ('Aspect Biosystems', 'greenhouse', 'aspectbiosystems', 'Biotechnology Research'),
    ('Lumen Bioscience', 'greenhouse', 'lumenbioscience', 'Biotechnology Research'),
    ('Digs', 'greenhouse', 'digs', 'Software Development'),
    ('Upbound', 'greenhouse', 'upbound', 'Software Development'),
    ('Recurrent', 'greenhouse', 'recurrent', 'Motor Vehicle Manufacturing'),
    ('Tune Therapeutics', 'greenhouse', 'tunetherapeutics', 'Biotechnology'),
    ('Corelight', 'greenhouse', 'corelight', 'Computer and Network Security'),
    ('Attunely', 'greenhouse', 'attunely', 'Financial Services'),
    ('Legion', 'greenhouse', 'legion', 'Software Development'),
    ('MediaAlpha', 'greenhouse', 'mediaalpha', 'Marketing Services'),
    ('Xealth', 'greenhouse', 'xealth', 'Hospitals and Health Care'),
    ('Tenable', 'greenhouse', 'tenableinc', 'Computer and Network Security'),
    ('OfferUp', 'greenhouse', 'offerup', 'Internet Marketplace Platforms')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Ashby (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Brinc', 'ashby', 'brinc', 'Aviation and Aerospace Component Manufacturing'),
    ('MotherDuck', 'ashby', 'MotherDuck', 'Data Infrastructure and Analytics'),
    ('Klue', 'ashby', 'klue', 'Software Development'),
    ('Certn', 'ashby', 'certn', 'Software Development'),
    ('Vibe', 'ashby', 'vibe', 'IT Services and IT Consulting'),
    ('Eigen Labs', 'ashby', 'eigen-labs', 'Software Development'),
    ('QA Wolf', 'ashby', 'QAWolf', 'Software Development'),
    ('Common Room', 'ashby', 'commonroom', 'Software Development'),
    ('Atlas Health', 'ashby', 'atlas', 'Hospitals and Health Care')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Lever (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Outreach', 'lever', 'outreach', 'Software Development'),
    ('Omnidian', 'lever', 'omnidian', 'Services for Renewable Energy'),
    ('Outpace Bio', 'lever', 'outpacebio', 'Biotechnology Research'),
    ('Viome', 'lever', 'viome', 'Wellness and Fitness Services'),
    ('DexCare', 'lever', 'dexcarehealth', 'Software Development'),
    ('Oleria', 'lever', 'oleria-security', 'Software Development'),
    ('Mast Reforestation', 'lever', 'MastReforestation', 'Environmental Services'),
    ('Educative', 'lever', 'educative', 'E-Learning Providers'),
    ('Sanctuary AI', 'lever', 'sanctuary', 'Software Development'),
    ('SkyPoint Cloud', 'lever', 'skypointcloud', 'Software Development'),
    ('Aigen', 'lever', 'aigen', 'Renewable Energy Equipment Manufacturing'),
    ('Lumotive', 'lever', 'lumotive', 'Semiconductor Manufacturing'),
    ('Highspot', 'lever', 'highspot', 'Software Development')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Workable (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Starfish Space', 'workable', 'starfish-space-1', 'Space Research and Technology'),
    ('Vouched', 'workable', 'vouched', 'Technology, Information and Internet'),
    ('Likewise', 'workable', 'likewise', 'Entertainment Providers'),
    ('Discovery Health MD', 'workable', 'discovery-health-md', 'Maritime Transportation'),
    ('Banzai', 'workable', 'banzai', 'Marketing Services')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Workday (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Remitly', 'workday', 'remitly.wd5.myworkdayjobs.com/Remitly_Careers', 'Financial Services')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Ashby (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Helion', 'ashby', 'helion', 'Renewable Energy Power Generation'),
    ('Hiya', 'ashby', 'hiya', 'Software Development'),
    ('Qumulo', 'ashby', 'qumulo', 'Software Development'),
    ('Polly', 'ashby', 'polly', 'Software Development')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Workday (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Tinuiti', 'workday', 'tinuiti.wd12.myworkdayjobs.com/Tinuiti', 'Marketing Services')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Lever (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Ridwell', 'lever', 'Ridwell', 'Consumer Services')
ON CONFLICT (ats, board_token) DO NOTHING;
